// Package fluxcddeploys collects FluxCD Kustomization & HelmRelease objects,
// the Deployments they manage, and the GitRepositories they source from.
package fluxcddeploys

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"

	"github.com/naira-project/naira/catalog/internal/kubeconn"
	"github.com/naira-project/naira/catalog/pluginapi"
)

const pluginName = "fluxcd_deploys"

var gvrDeployments = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

const (
	labelKustName = "kustomize.toolkit.fluxcd.io/name"
	labelKustNs   = "kustomize.toolkit.fluxcd.io/namespace"
	labelHelmName = "helm.toolkit.fluxcd.io/name"
	labelHelmNs   = "helm.toolkit.fluxcd.io/namespace"
)

type Config struct {
	Enabled    bool
	Kubeconfig string // explicit path; empty = auto-detect
	Namespace  string // empty = all namespaces
}

type Plugin struct {
	config Config
}

func New(config Config) *Plugin {
	return &Plugin{config: config}
}

func (*Plugin) Name() string { return pluginName }

func (p *Plugin) Collect(ctx context.Context) (pluginapi.IngestionRequest, error) {
	disc, dyn, err := p.connect()
	if err != nil {
		return pluginapi.IngestionRequest{}, fmt.Errorf("connecting to cluster: %w", err)
	}

	ns := p.config.Namespace

	kusts, err := listGroupKind(ctx, disc, dyn, "kustomize.toolkit.fluxcd.io", "Kustomization", ns)
	if err != nil {
		return pluginapi.IngestionRequest{}, fmt.Errorf("listing Kustomizations: %w", err)
	}
	helms, err := listGroupKind(ctx, disc, dyn, "helm.toolkit.fluxcd.io", "HelmRelease", ns)
	if err != nil {
		return pluginapi.IngestionRequest{}, fmt.Errorf("listing HelmReleases: %w", err)
	}
	gitRepoItems, err := listGroupKind(ctx, disc, dyn, "source.toolkit.fluxcd.io", "GitRepository", ns)
	if err != nil {
		return pluginapi.IngestionRequest{}, fmt.Errorf("listing GitRepositories: %w", err)
	}
	depList, err := dyn.Resource(gvrDeployments).Namespace(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return pluginapi.IngestionRequest{}, fmt.Errorf("listing Deployments: %w", err)
	}

	var nodes []pluginapi.NodeClaim
	var relations []pluginapi.RelationClaim

	// Phase 1: GitRepository nodes.
	gitRepoByNsName := map[string]pluginapi.NodeID{} // "ns/name" → NodeID
	for _, gr := range gitRepoItems {
		id := pluginapi.NodeID{
			Kind: pluginapi.NodeKindGitRepository,
			Path: gr.GetNamespace() + "/" + gr.GetName(),
		}
		url, _, _ := unstructured.NestedString(gr.Object, "spec", "url")
		nodes = append(nodes, pluginapi.NodeClaim{
			ID: id,
			Properties: pluginapi.PropertyMap{
				"namespace": gr.GetNamespace(),
				"name":      gr.GetName(),
				"url":       url,
			},
		})
		gitRepoByNsName[gr.GetNamespace()+"/"+gr.GetName()] = id
	}

	// Phase 2: Kustomization nodes + "sourced_from" relations.
	kustByNsName := map[string]pluginapi.NodeID{} // "ns/name" → NodeID
	kustGitRepo := map[string]pluginapi.NodeID{}  // "ns/name" → git repo NodeID
	for _, kust := range kusts {
		id := pluginapi.NodeID{
			Kind: pluginapi.NodeKindFluxKustomization,
			Path: kust.GetNamespace() + "/" + kust.GetName(),
		}
		nodes = append(nodes, pluginapi.NodeClaim{
			ID: id,
			Properties: pluginapi.PropertyMap{
				"namespace": kust.GetNamespace(),
				"name":      kust.GetName(),
			},
		})
		key := kust.GetNamespace() + "/" + kust.GetName()
		kustByNsName[key] = id

		if gitID, ok := kustomizationGitRepo(kust, gitRepoByNsName); ok {
			relations = append(relations, pluginapi.RelationClaim{
				Kind: pluginapi.RelationKindSourcedFrom,
				From: id,
				To:   gitID,
			})
			kustGitRepo[key] = gitID
		}
	}

	// Phase 3: HelmRelease nodes + sourced_from relations.
	helmByNsName := map[string]pluginapi.NodeID{} // "ns/name" → NodeID
	helmGitRepo := map[string]pluginapi.NodeID{}  // "ns/name" → git repo NodeID
	for _, hr := range helms {
		id := pluginapi.NodeID{
			Kind: pluginapi.NodeKindFluxHelmChart,
			Path: hr.GetNamespace() + "/" + hr.GetName(),
		}
		nodes = append(nodes, pluginapi.NodeClaim{
			ID: id,
			Properties: pluginapi.PropertyMap{
				"namespace": hr.GetNamespace(),
				"name":      hr.GetName(),
			},
		})
		key := hr.GetNamespace() + "/" + hr.GetName()
		helmByNsName[key] = id

		if gitID, ok := helmReleaseGitRepo(hr, gitRepoByNsName); ok {
			relations = append(relations, pluginapi.RelationClaim{
				Kind: pluginapi.RelationKindSourcedFrom,
				From: id,
				To:   gitID,
			})
			helmGitRepo[key] = gitID
		}
	}

	// Phase 4: flux-managed Deployment nodes + describes + deployed_from relations.
	for _, dep := range depList.Items {
		labels := dep.GetLabels()
		kustName := labels[labelKustName]
		helmName := labels[labelHelmName]
		if kustName == "" && helmName == "" {
			continue // not flux-managed
		}

		depID := pluginapi.NodeID{
			Kind: pluginapi.NodeKindDeployment,
			Path: dep.GetNamespace() + "/" + dep.GetName(),
		}
		nodes = append(nodes, pluginapi.NodeClaim{
			ID: depID,
			Properties: pluginapi.PropertyMap{
				"namespace": dep.GetNamespace(),
				"name":      dep.GetName(),
			},
		})

		if kustName != "" {
			key := nsOrFallback(labels[labelKustNs], dep.GetNamespace()) + "/" + kustName
			if kustID, ok := kustByNsName[key]; ok {
				relations = append(relations, pluginapi.RelationClaim{
					Kind: pluginapi.RelationKindDescribes,
					From: kustID,
					To:   depID,
				})
			}
			if gitID, ok := kustGitRepo[key]; ok {
				relations = append(relations, pluginapi.RelationClaim{
					Kind: pluginapi.RelationKindDeployedFrom,
					From: depID,
					To:   gitID,
				})
			}
		}

		if helmName != "" {
			key := nsOrFallback(labels[labelHelmNs], dep.GetNamespace()) + "/" + helmName
			if helmID, ok := helmByNsName[key]; ok {
				relations = append(relations, pluginapi.RelationClaim{
					Kind: pluginapi.RelationKindDescribes,
					From: helmID,
					To:   depID,
				})
			}
			if gitID, ok := helmGitRepo[key]; ok {
				relations = append(relations, pluginapi.RelationClaim{
					Kind: pluginapi.RelationKindDeployedFrom,
					From: depID,
					To:   gitID,
				})
			}
		}
	}

	return pluginapi.IngestionRequest{Nodes: nodes, Relations: relations}, nil
}

func kustomizationGitRepo(kust unstructured.Unstructured, gitRepos map[string]pluginapi.NodeID) (pluginapi.NodeID, bool) {
	srcKind, _, _ := unstructured.NestedString(kust.Object, "spec", "sourceRef", "kind")
	if srcKind != "GitRepository" {
		return pluginapi.NodeID{}, false
	}
	srcName, _, _ := unstructured.NestedString(kust.Object, "spec", "sourceRef", "name")
	srcNs, _, _ := unstructured.NestedString(kust.Object, "spec", "sourceRef", "namespace")
	id, ok := gitRepos[nsOrFallback(srcNs, kust.GetNamespace())+"/"+srcName]
	return id, ok
}

func helmReleaseGitRepo(hr unstructured.Unstructured, gitRepos map[string]pluginapi.NodeID) (pluginapi.NodeID, bool) {
	srcKind, _, _ := unstructured.NestedString(hr.Object, "spec", "chart", "spec", "sourceRef", "kind")
	if srcKind != "GitRepository" {
		return pluginapi.NodeID{}, false
	}
	srcName, _, _ := unstructured.NestedString(hr.Object, "spec", "chart", "spec", "sourceRef", "name")
	srcNs, _, _ := unstructured.NestedString(hr.Object, "spec", "chart", "spec", "sourceRef", "namespace")
	id, ok := gitRepos[nsOrFallback(srcNs, hr.GetNamespace())+"/"+srcName]
	return id, ok
}

// listGroupKind returns all resources of the given API group + kind using discovery.
// Returns an empty slice (not an error) when the CRD is not installed in the cluster.
func listGroupKind(ctx context.Context, disc *discovery.DiscoveryClient, dyn dynamic.Interface, group, kind, namespace string) ([]unstructured.Unstructured, error) {
	_, apiLists, _ := disc.ServerGroupsAndResources()
	for _, apiList := range apiLists {
		gv, err := schema.ParseGroupVersion(apiList.GroupVersion)
		if err != nil || gv.Group != group {
			continue
		}
		for _, res := range apiList.APIResources {
			if res.Kind != kind {
				continue
			}
			var hasListVerb bool
			for _, v := range res.Verbs {
				if v == "list" {
					hasListVerb = true
					break
				}
			}
			if !hasListVerb {
				continue
			}
			gvr := schema.GroupVersionResource{Group: gv.Group, Version: gv.Version, Resource: res.Name}
			list, err := dyn.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				return nil, fmt.Errorf("list %s/%s: %w", group, kind, err)
			}
			return list.Items, nil
		}
	}
	return nil, nil // CRD not installed → no data
}

func nsOrFallback(ns, fallback string) string {
	if ns != "" {
		return ns
	}
	return fallback
}

func (p *Plugin) connect() (*discovery.DiscoveryClient, dynamic.Interface, error) {
	cfg, err := kubeconn.RestConfig(p.config.Kubeconfig)
	if err != nil {
		return nil, nil, fmt.Errorf("loading k8s config: %w", err)
	}
	disc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("creating k8s discovery client: %w", err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("creating k8s dynamic client: %w", err)
	}
	return disc, dyn, nil
}
