// Package fluxcd collects FluxCD Kustomization & HelmRelease objects,
// the Deployments they manage, and the GitRepositories they source from.
package fluxcd

import (
	"context"
	"fmt"
	"log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"

	"github.com/naira-project/naira/catalog/internal/kubeutil"
	"github.com/naira-project/naira/catalog/pluginapi"
)

const pluginName = "fluxcd"

var gvrDeployments = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

const (
	labelKustName = "kustomize.toolkit.fluxcd.io/name"
	labelKustNs   = "kustomize.toolkit.fluxcd.io/namespace"
	labelHelmName = "helm.toolkit.fluxcd.io/name"
	labelHelmNs   = "helm.toolkit.fluxcd.io/namespace"
)

type Config struct {
	Enabled    bool   `env:"ENABLED" default:"true"`
	Kubeconfig string `env:"KUBECONFIG"`
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

	namespaces, clusterID, err := kubeutil.NamespacesAndClusterID(ctx, dyn)
	if err != nil {
		return pluginapi.IngestionRequest{}, fmt.Errorf("listing namespaces: %w", err)
	}

	kusts, err := listGroupKind(ctx, disc, dyn, "kustomize.toolkit.fluxcd.io", "Kustomization", namespaces)
	if err != nil {
		return pluginapi.IngestionRequest{}, fmt.Errorf("listing Kustomizations: %w", err)
	}
	helms, err := listGroupKind(ctx, disc, dyn, "helm.toolkit.fluxcd.io", "HelmRelease", namespaces)
	if err != nil {
		return pluginapi.IngestionRequest{}, fmt.Errorf("listing HelmReleases: %w", err)
	}
	repos, err := listGroupKind(ctx, disc, dyn, "source.toolkit.fluxcd.io", "GitRepository", namespaces)
	if err != nil {
		return pluginapi.IngestionRequest{}, fmt.Errorf("listing GitRepositories: %w", err)
	}
	var depls []unstructured.Unstructured
	for _, ns := range namespaces {
		nsDeplList, err := dyn.Resource(gvrDeployments).Namespace(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Printf("%s: WARN: listing Deployments in namespace %q: %v", pluginName, ns, err)
		} else {
			depls = append(depls, nsDeplList.Items...)
		}
	}

	var nodes []pluginapi.NodeClaim
	var relations []pluginapi.RelationClaim

	// Phase 1: GitRepository nodes.
	repoByPath := map[string]pluginapi.NodeID{} // "ns/name" → NodeID
	for _, r := range repos {
		shortPath := r.GetNamespace() + "/" + r.GetName()
		id := pluginapi.NodeID{
			Kind: pluginapi.NodeKindGitRepository,
			Path: clusterID + "/" + shortPath,
		}
		url, _, _ := unstructured.NestedString(r.Object, "spec", "url")
		nodes = append(nodes, pluginapi.NodeClaim{
			ID: id,
			Properties: pluginapi.PropertyMap{
				"url": url,
			},
		})
		repoByPath[shortPath] = id
	}

	type nodeAndRepoIDs struct {
		node pluginapi.NodeID
		repo pluginapi.NodeID
	}
	// Phase 2: Kustomization nodes + "sourced_from" relations.
	kustByPath := map[string]nodeAndRepoIDs{}
	for _, k := range kusts {
		shortPath := k.GetNamespace() + "/" + k.GetName()
		ids := nodeAndRepoIDs{
			node: pluginapi.NodeID{
				Kind: pluginapi.NodeKindFluxKustomization,
				Path: clusterID + "/" + shortPath,
			},
		}
		nodes = append(nodes, pluginapi.NodeClaim{
			ID: ids.node,
		})

		if repoID, ok := repoFromKustomization(k, repoByPath); ok {
			ids.repo = repoID
			relations = append(relations, pluginapi.RelationClaim{
				Kind: pluginapi.RelationKindSourcedFrom,
				From: ids.node,
				To:   ids.repo,
			})
		}
		kustByPath[shortPath] = ids
	}

	// Phase 3: HelmRelease nodes + sourced_from relations.
	helmByPath := map[string]nodeAndRepoIDs{}
	for _, h := range helms {
		shortPath := h.GetNamespace() + "/" + h.GetName()
		ids := nodeAndRepoIDs{
			node: pluginapi.NodeID{
				Kind: pluginapi.NodeKindFluxHelmChart,
				Path: clusterID + "/" + shortPath,
			},
		}
		nodes = append(nodes, pluginapi.NodeClaim{
			ID: ids.node,
		})

		if gitID, ok := repoFromHelm(h, repoByPath); ok {
			ids.repo = gitID
			relations = append(relations, pluginapi.RelationClaim{
				Kind: pluginapi.RelationKindSourcedFrom,
				From: ids.node,
				To:   gitID,
			})
		}
		helmByPath[shortPath] = ids
	}

	// Phase 4: flux-managed Deployment Nodes + "describes" + "deployed_from" Relations.
	for _, dep := range depls {
		var (
			labels   = dep.GetLabels()
			kustName = labels[labelKustName]
			helmName = labels[labelHelmName]
		)
		if kustName == "" && helmName == "" {
			continue // not flux-managed
		}

		ns := dep.GetNamespace()
		depID := pluginapi.NodeID{
			Kind: pluginapi.NodeKindDeployment,
			Path: clusterID + "/" + ns + "/" + dep.GetName(),
		}
		nodes = append(nodes, pluginapi.NodeClaim{
			ID: depID,
		})

		if kustName != "" {
			path := nsOrFallback(labels[labelKustNs], ns) + "/" + kustName
			if ids, ok := kustByPath[path]; ok {
				relations = append(relations, pluginapi.RelationClaim{
					Kind: pluginapi.RelationKindDescribes,
					From: ids.node,
					To:   depID,
				}, pluginapi.RelationClaim{
					Kind: pluginapi.RelationKindDeployedFrom,
					From: depID,
					To:   ids.repo,
				})
			}
		}

		if helmName != "" {
			path := nsOrFallback(labels[labelHelmNs], ns) + "/" + helmName
			if ids, ok := helmByPath[path]; ok {
				relations = append(relations, pluginapi.RelationClaim{
					Kind: pluginapi.RelationKindDescribes,
					From: ids.node,
					To:   depID,
				}, pluginapi.RelationClaim{
					Kind: pluginapi.RelationKindDeployedFrom,
					From: depID,
					To:   ids.repo,
				})
			}
		}
	}

	return pluginapi.IngestionRequest{Nodes: nodes, Relations: relations}, nil
}

func repoFromKustomization(kust unstructured.Unstructured, repos map[string]pluginapi.NodeID) (pluginapi.NodeID, bool) {
	srcKind, _, _ := unstructured.NestedString(kust.Object, "spec", "sourceRef", "kind")
	if srcKind != "GitRepository" {
		return pluginapi.NodeID{}, false
	}
	srcName, _, _ := unstructured.NestedString(kust.Object, "spec", "sourceRef", "name")
	srcNs, _, _ := unstructured.NestedString(kust.Object, "spec", "sourceRef", "namespace")
	id, ok := repos[nsOrFallback(srcNs, kust.GetNamespace())+"/"+srcName]
	return id, ok
}

func repoFromHelm(helm unstructured.Unstructured, gitRepos map[string]pluginapi.NodeID) (pluginapi.NodeID, bool) {
	srcKind, _, _ := unstructured.NestedString(helm.Object, "spec", "chart", "spec", "sourceRef", "kind")
	if srcKind != "GitRepository" {
		return pluginapi.NodeID{}, false
	}
	srcName, _, _ := unstructured.NestedString(helm.Object, "spec", "chart", "spec", "sourceRef", "name")
	srcNs, _, _ := unstructured.NestedString(helm.Object, "spec", "chart", "spec", "sourceRef", "namespace")
	id, ok := gitRepos[nsOrFallback(srcNs, helm.GetNamespace())+"/"+srcName]
	return id, ok
}

// listGroupKind returns all resources of the given API group + kind across all
// provided namespaces, using discovery to resolve the GVR.
// Returns an empty slice (not an error) when the CRD is not installed in the cluster.
func listGroupKind(ctx context.Context, disc *discovery.DiscoveryClient, dyn dynamic.Interface, group, kind string, namespaces []string) ([]unstructured.Unstructured, error) {
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
			var items []unstructured.Unstructured
			for _, ns := range namespaces {
				list, err := dyn.Resource(gvr).Namespace(ns).List(ctx, metav1.ListOptions{})
				if err != nil {
					log.Printf("%s: WARN: listing %s/%s in namespace %q: %v", pluginName, group, kind, ns, err)
					continue
				}
				items = append(items, list.Items...)
			}
			return items, nil
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
	cfg, err := kubeutil.RestConfig(p.config.Kubeconfig)
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
