// Plugin fluxcd collects FluxCD Kustomization & HelmRelease objects,
// the Deployments they manage, and the GitRepositories they source from.
//
// TODO: add support for Bucket and other FluxCD sources kinds.
// TODO: add support for any other resources managed by FluxCD (Services, Ingresses, thirdparty CRDs, ...)
//
// GitRepository nodes currently support GitHub URLs only. Other Git hosts are
// skipped until host-specific canonicalization and enrichment are implemented.
package main

import (
	"context"
	"fmt"
	"log"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"

	"github.com/naira-project/naira/plugins/internal/kubeutil"
	"github.com/naira-project/naira/plugins/internal/repositorydiscovery"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/naira-project/naira/plugins/pkg/pluginmain"
)

const pluginName = "fluxcd"

var gvrDeployments = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

const (
	labelKustName = "kustomize.toolkit.fluxcd.io/name"
	labelKustNs   = "kustomize.toolkit.fluxcd.io/namespace"
	labelHelmName = "helm.toolkit.fluxcd.io/name"
	labelHelmNs   = "helm.toolkit.fluxcd.io/namespace"
)

type config struct {
	Kubeconfig string `env:"KUBECONFIG"`
}

type Plugin struct {
	config config
}

func New(config config) *Plugin {
	return &Plugin{config: config}
}

func main() {
	app := pluginmain.New[config]()

	app.Serve(New(app.PluginConfig))
}

func (p *Plugin) Collect(ctx context.Context) (pluginapi.CollectResponse, error) {
	disc, dyn, err := p.connect()
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("connecting to cluster: %w", err)
	}
	return p.collect(ctx, disc, dyn)
}

func (p *Plugin) collect(ctx context.Context, disc discovery.DiscoveryInterface, dyn dynamic.Interface) (pluginapi.CollectResponse, error) {
	namespaces, clusterID, err := kubeutil.NamespacesAndClusterIDDynamic(ctx, dyn)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("listing namespaces: %w", err)
	}
	_, resourceAPIs, err := disc.ServerGroupsAndResources()
	if err != nil {
		log.Printf("%s: WARN: listing resource APIs: %v", pluginName, err)
		// NOTE: ServerGroupsAndResources godoc says:
		// "The returned group and resource lists might be non-nil with partial results even in the
		// case of non-nil error."
		// Thus, we don't return an error, but continue trying to scan what we can.
	}

	kusts, err := listGroupKind(ctx, resourceAPIs, dyn, "kustomize.toolkit.fluxcd.io", "Kustomization", namespaces)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("listing Kustomizations: %w", err)
	}
	helms, err := listGroupKind(ctx, resourceAPIs, dyn, "helm.toolkit.fluxcd.io", "HelmRelease", namespaces)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("listing HelmReleases: %w", err)
	}
	repos, err := listGroupKind(ctx, resourceAPIs, dyn, "source.toolkit.fluxcd.io", "GitRepository", namespaces)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("listing GitRepositories: %w", err)
	}
	var depls []unstructured.Unstructured
	for _, ns := range namespaces {
		// TODO[PERF]: try scanning `.Namespace(metav1.NamespaceAll)` first, only fall back to per-namespace if that fails due to RBAC.
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
		url, _, _ := unstructured.NestedString(r.Object, "spec", "url")
		repoPath := repositorydiscovery.CanonicalPath(url)
		if repoPath == "" {
			log.Printf("%s: WARN: skipping GitRepository %s with unsupported URL %q", pluginName, shortPath, url)
			continue
		}

		id := pluginapi.NodeID{
			Kind: pluginapi.NodeKindGitRepository,
			Path: repoPath,
		}
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
	zeroNodeID := pluginapi.NodeID{}

	// Phase 2: Kustomization nodes + "sourced_from" relations.
	kustByPath := map[string]nodeAndRepoIDs{}
	for _, k := range kusts {
		shortPath := k.GetNamespace() + "/" + k.GetName()
		ids := nodeAndRepoIDs{
			node: pluginapi.NodeID{
				Kind: pluginapi.NodeKindFluxKustomization,
				Path: clusterID + "/" + shortPath,
			},
			repo: repoFromKustomization(k, repoByPath),
		}
		nodes = append(nodes, pluginapi.NodeClaim{
			ID: ids.node,
		})
		if ids.repo != zeroNodeID {
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
			repo: repoFromHelm(h, repoByPath),
		}
		nodes = append(nodes, pluginapi.NodeClaim{
			ID: ids.node,
		})
		if ids.repo != zeroNodeID {
			relations = append(relations, pluginapi.RelationClaim{
				Kind: pluginapi.RelationKindSourcedFrom,
				From: ids.node,
				To:   ids.repo,
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
			ids := kustByPath[path]
			if ids.node != zeroNodeID {
				relations = append(relations, pluginapi.RelationClaim{
					Kind: pluginapi.RelationKindDescribes,
					From: ids.node,
					To:   depID,
				})
			}
			if ids.repo != zeroNodeID {
				relations = append(relations, pluginapi.RelationClaim{
					Kind: pluginapi.RelationKindDeployedFrom,
					From: depID,
					To:   ids.repo,
				})
			}
		}

		if helmName != "" {
			path := nsOrFallback(labels[labelHelmNs], ns) + "/" + helmName
			ids := helmByPath[path]
			if ids.node != zeroNodeID {
				relations = append(relations, pluginapi.RelationClaim{
					Kind: pluginapi.RelationKindDescribes,
					From: ids.node,
					To:   depID,
				})
			}
			if ids.repo != zeroNodeID {
				relations = append(relations, pluginapi.RelationClaim{
					Kind: pluginapi.RelationKindDeployedFrom,
					From: depID,
					To:   ids.repo,
				})
			}
		}
	}

	return pluginapi.CollectResponse{Nodes: nodes, Relations: relations}, nil
}

func repoFromKustomization(kust unstructured.Unstructured, repos map[string]pluginapi.NodeID) pluginapi.NodeID {
	srcKind, _, _ := unstructured.NestedString(kust.Object, "spec", "sourceRef", "kind")
	if srcKind != "GitRepository" {
		return pluginapi.NodeID{}
	}
	srcName, _, _ := unstructured.NestedString(kust.Object, "spec", "sourceRef", "name")
	srcNs, _, _ := unstructured.NestedString(kust.Object, "spec", "sourceRef", "namespace")
	return repos[nsOrFallback(srcNs, kust.GetNamespace())+"/"+srcName]
}

func repoFromHelm(helm unstructured.Unstructured, repos map[string]pluginapi.NodeID) pluginapi.NodeID {
	srcKind, _, _ := unstructured.NestedString(helm.Object, "spec", "chart", "spec", "sourceRef", "kind")
	if srcKind != "GitRepository" {
		return pluginapi.NodeID{}
	}
	srcName, _, _ := unstructured.NestedString(helm.Object, "spec", "chart", "spec", "sourceRef", "name")
	srcNs, _, _ := unstructured.NestedString(helm.Object, "spec", "chart", "spec", "sourceRef", "namespace")
	return repos[nsOrFallback(srcNs, helm.GetNamespace())+"/"+srcName]
}

// listGroupKind returns all resources of the given API group + kind across all
// provided namespaces.
// Returns an empty slice (not an error) when the CRD is not installed in the cluster.
func listGroupKind(ctx context.Context, apiLists []*metav1.APIResourceList, dyn dynamic.Interface, group, kind string, namespaces []string) ([]unstructured.Unstructured, error) {
	for _, apiList := range apiLists {
		gv, err := schema.ParseGroupVersion(apiList.GroupVersion)
		if err != nil {
			log.Printf("%s: WARN: skipping API group %q due to parsing error: %v", pluginName, apiList.GroupVersion, err)
			continue
		}
		if gv.Group != group {
			continue
		}
		for _, res := range apiList.APIResources {
			if res.Kind != kind {
				continue
			}
			if !slices.Contains(res.Verbs, "list") {
				continue
			}
			gvr := schema.GroupVersionResource{Group: gv.Group, Version: gv.Version, Resource: res.Name}
			var items []unstructured.Unstructured
			for _, ns := range namespaces {
				// TODO[PERF]: try scanning `.Namespace(metav1.NamespaceAll)` first, only fall back to per-namespace if that fails due to RBAC.
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

func (p *Plugin) connect() (discovery.DiscoveryInterface, dynamic.Interface, error) {
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
