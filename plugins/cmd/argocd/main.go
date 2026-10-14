// Plugin argocd collects ArgoCD Application objects and all the Kubernetes
// resources they manage, as reported in each Application's status.
//
// Managed Deployments and Services are emitted as deployment and service Nodes.
// Child Applications (app-of-apps) are emitted as Application Nodes. Any other
// managed resource is emitted as a k8s_resource Node, with a path of
// "<cluster>/<group>/<Kind>/<namespace>/<name>" ("core" is used as the group of
// the core API, and the namespace segment is omitted for cluster-scoped resources).
//
// An Application "describes" each resource it manages. If its source is a GitHub
// repository, the Application is "sourced_from" a git_repository Node, and
// managed Deployments are "deployed_from" it.
//
// # Known Limitations
//
// Only Applications deploying to the cluster they live in are supported;
// Applications targeting other clusters are skipped.
//
// TODO:
// - add support for ApplicationSets and AppProjects.
// - git repository Nodes could support other Git hosts than GitHub.
//
//go:generate bash -c "goreadme -use-stdlib-markdown -title 'argocd plugin' | sed 's/ {#hdr-[^}]*}//g' > README.md"
package main

import (
	"context"
	"fmt"
	"log"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"

	"github.com/naira-project/naira/plugins/internal/kubeutil"
	"github.com/naira-project/naira/plugins/internal/repositoryidentity"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/naira-project/naira/plugins/pkg/pluginmain"
)

const (
	argoGroup       = "argoproj.io"
	argoApplication = "Application"
)

// ArgoCD's names for the cluster the Application lives in.
const (
	inClusterServer = "https://kubernetes.default.svc"
	inClusterName   = "in-cluster"
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
	namespaces, clusterID, err := kubeutil.NamespacesAndClusterIDFromDynamic(ctx, dyn)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("listing namespaces: %w", err)
	}
	_, resourceAPIs, err := disc.ServerGroupsAndResources()
	if err != nil {
		log.Printf("argocd: WARN: listing resource APIs: %v", err)
		// NOTE: ServerGroupsAndResources godoc says the returned lists might be
		// non-nil with partial results even in the case of non-nil error.
		// Thus, we continue trying to scan what we can.
	}

	apps, err := kubeutil.ListGroupKind(ctx, resourceAPIs, dyn, argoGroup, argoApplication, namespaces)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("listing Applications: %w", err)
	}

	var (
		nodes     []pluginapi.NodeClaim
		relations []pluginapi.RelationClaim
		seenNodes = map[pluginapi.NodeID]bool{}
	)
	addNode := func(claim pluginapi.NodeClaim) {
		if seenNodes[claim.ID] {
			return
		}
		seenNodes[claim.ID] = true
		nodes = append(nodes, claim)
	}

	// Application nodes are added first, so that child Applications
	// listed as managed resources are claimed with their properties.
	var managed []unstructured.Unstructured
	for _, app := range apps {
		if !isInCluster(app) {
			log.Printf("argocd: skipping Application %s/%s: destination is not the local cluster", app.GetNamespace(), app.GetName())
			continue
		}
		managed = append(managed, app)
		project, _, _ := unstructured.NestedString(app.Object, "spec", "project")
		syncStatus, _, _ := unstructured.NestedString(app.Object, "status", "sync", "status")
		health, _, _ := unstructured.NestedString(app.Object, "status", "health", "status")
		destNs, _, _ := unstructured.NestedString(app.Object, "spec", "destination", "namespace")
		addNode(pluginapi.NodeClaim{
			ID: applicationID(clusterID, app.GetNamespace(), app.GetName()),
			Properties: pluginapi.PropertyMap{
				"project":               project,
				"sync_status":           syncStatus,
				"health_status":         health,
				"destination_namespace": destNs,
			},
		})
	}

	for _, app := range managed {
		appID := applicationID(clusterID, app.GetNamespace(), app.GetName())

		var repoIDs []pluginapi.NodeID
		for _, url := range sourceURLs(app) {
			repoPath := repositoryidentity.GitHubRepositoryNodePathFromURL(url)
			if repoPath == "" {
				continue
			}
			repoID := pluginapi.NodeID{
				Kind: pluginapi.NodeKindGitRepository,
				Path: repoPath,
			}
			addNode(pluginapi.NodeClaim{
				ID: repoID,
				Properties: pluginapi.PropertyMap{
					"url": url,
				},
			})
			relations = append(relations, pluginapi.RelationClaim{
				Kind: pluginapi.RelationKindSourcedFrom,
				From: appID,
				To:   repoID,
			})
			repoIDs = append(repoIDs, repoID)
		}

		resources, _, _ := unstructured.NestedSlice(app.Object, "status", "resources")
		for _, r := range resources {
			res, ok := r.(map[string]any)
			if !ok {
				continue
			}
			group, _ := res["group"].(string)
			kind, _ := res["kind"].(string)
			ns, _ := res["namespace"].(string)
			name, _ := res["name"].(string)
			if kind == "" || name == "" {
				continue
			}

			resID := resourceID(clusterID, group, kind, ns, name)
			if resID == appID {
				continue
			}
			addNode(pluginapi.NodeClaim{ID: resID})
			relations = append(relations, pluginapi.RelationClaim{
				Kind: pluginapi.RelationKindDescribes,
				From: appID,
				To:   resID,
			})
			if resID.Kind == pluginapi.NodeKindDeployment {
				for _, repoID := range repoIDs {
					relations = append(relations, pluginapi.RelationClaim{
						Kind: pluginapi.RelationKindDeployedFrom,
						From: resID,
						To:   repoID,
					})
				}
			}
		}
	}

	return pluginapi.CollectResponse{Nodes: nodes, Relations: relations}, nil
}

func applicationID(clusterID, ns, name string) pluginapi.NodeID {
	return pluginapi.NodeID{
		Kind: pluginapi.NodeKindArgoApplication,
		Path: clusterID + "/" + ns + "/" + name,
	}
}

// resourceID returns the NodeID of a resource listed in an Application's
// status.resources. ns is empty for cluster-scoped resources.
func resourceID(clusterID, group, kind, ns, name string) pluginapi.NodeID {
	switch {
	case group == "apps" && kind == "Deployment":
		return pluginapi.NodeID{
			Kind: pluginapi.NodeKindDeployment,
			Path: clusterID + "/" + ns + "/" + name,
		}
	case group == "" && kind == "Service":
		return pluginapi.NodeID{
			Kind: pluginapi.NodeKindService,
			Path: clusterID + "/" + ns + "/" + name,
		}
	case group == argoGroup && kind == argoApplication:
		return applicationID(clusterID, ns, name)
	}
	if group == "" {
		group = "core"
	}
	path := clusterID + "/" + group + "/" + kind
	if ns != "" {
		path += "/" + ns
	}
	return pluginapi.NodeID{
		Kind: pluginapi.NodeKindK8sResource,
		Path: path + "/" + name,
	}
}

// isInCluster returns true if the Application deploys to the cluster it lives in.
func isInCluster(app unstructured.Unstructured) bool {
	server, _, _ := unstructured.NestedString(app.Object, "spec", "destination", "server")
	name, _, _ := unstructured.NestedString(app.Object, "spec", "destination", "name")
	return server == inClusterServer || name == inClusterName
}

// sourceURLs returns repoURLs of all sources of the Application.
func sourceURLs(app unstructured.Unstructured) []string {
	var urls []string
	if url, _, _ := unstructured.NestedString(app.Object, "spec", "source", "repoURL"); url != "" {
		urls = append(urls, url)
	}
	sources, _, _ := unstructured.NestedSlice(app.Object, "spec", "sources")
	for _, s := range sources {
		if m, ok := s.(map[string]any); ok {
			if url, _ := m["repoURL"].(string); url != "" {
				urls = append(urls, url)
			}
		}
	}
	return urls
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
