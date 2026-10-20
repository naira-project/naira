// Plugin argocd collects ArgoCD Application objects and all the Kubernetes
// resources they manage, as reported in each Application's status.
//
// Managed Deployments and Services are emitted as deployment and service Nodes.
// Child Applications (app-of-apps) are emitted as Application Nodes. Any other
// managed resource is emitted as a k8s_resource Node, with a path of
// "<cluster>/<group>/<Kind>/<namespace>/<name>" ("core" is used as the group of
// the core API, and the namespace segment is omitted for cluster-scoped resources).
//
// # Environment Variables
//
//   - KUBECONFIG (optional) - path to the kubeconfig file; if unset, the in-cluster
//     configuration is used.
//
// # Known Limitations
//
// Only Applications deploying to the cluster they live in are supported;
// Applications targeting other clusters are skipped.
//
// Currently, we only recognize Application sources ("repoURL") that match
// GitHub repository URLs. Unfortunately, ArgoCD uses ad-hoc heuristics to
// detect the source type based on the URL, and they're not trivial to reproduce.
//
// TODO:
//   - add support for ApplicationSets and AppProjects.
//   - git repository Nodes could support other Git hosts than GitHub.
//   - allow configuring custom regexps to recognize Git sources ("repoURL")
//     other than GitHub, and Helm chart sources, to emit Nodes for them.
//   - recognize oci:// "repoURL" sources and emit Nodes for them.
//
//go:generate bash -c "goreadme -use-stdlib-markdown -title 'argocd plugin' | sed 's/ {#hdr-[^}]*}//g' > README.md"
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"sort"
	"strconv"
	"strings"

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

	var inClusterApps []unstructured.Unstructured
	for _, app := range apps {
		if !isInCluster(app) {
			log.Printf("argocd: skipping Application %s/%s: destination is not the local cluster", app.GetNamespace(), app.GetName())
			continue
		}
		inClusterApps = append(inClusterApps, app)

		str := nestedStringOrEmpty // helper func
		props := pluginapi.PropertyMap{
			"project":               str(app, "spec", "project"),
			"sync_status":           str(app, "status", "sync", "status"),
			"health_status":         str(app, "status", "health", "status"),
			"destination_namespace": str(app, "spec", "destination", "namespace"),
			"source_type":           sourceType(app),
		}
		_, hasOperation, _ := unstructured.NestedFieldNoCopy(app.Object, "operation")
		props["operation_present"] = strconv.FormatBool(hasOperation)
		setIfNotEmpty(props, "last_deployed_at", lastDeployedAt(app))
		for _, field := range []string{"phase", "startedAt", "finishedAt"} {
			setIfNotEmpty(props,
				"operationState."+field,
				nestedStringOrEmpty(app, "status", "operationState", field))
		}

		addNode(pluginapi.NodeClaim{
			ID:         applicationID(clusterID, app.GetNamespace(), app.GetName()),
			Properties: props,
		})
	}

	for _, app := range inClusterApps {
		appID := applicationID(clusterID, app.GetNamespace(), app.GetName())

		// Multiple sources from a single Application can point to the same
		// repository, at different paths. A relation is identified by its kind
		// and ends, so we need to create only one per repository, listing all
		// the paths used.
		pathsByRepo := map[pluginapi.NodeID][]string{}
		for _, src := range sources(app) {
			repoPath := repositoryidentity.GitHubRepositoryNodePathFromURL(src.repoURL)
			if repoPath == "" {
				// As described in the README of the plugin, we currently only
				// recognize GitHub repository URLs, as for other URLs it's
				// difficult to know if they're a Git repo, Helm chart, OCI
				// image, tarball, etc.
				continue
			}
			repoID := pluginapi.NodeID{
				Kind: pluginapi.NodeKindGitRepository,
				Path: repoPath,
			}
			if _, found := pathsByRepo[repoID]; !found {
				addNode(pluginapi.NodeClaim{
					ID: repoID,
					Properties: pluginapi.PropertyMap{
						"url": src.repoURL,
					},
				})
				pathsByRepo[repoID] = nil
			}
			if src.path != "" && !slices.Contains(pathsByRepo[repoID], src.path) {
				pathsByRepo[repoID] = append(pathsByRepo[repoID], src.path)
			}
		}
		for repoID, paths := range pathsByRepo {
			sort.Strings(paths)
			relations = append(relations, pluginapi.RelationClaim{
				Kind:       pluginapi.RelationKindSourcedFrom,
				From:       appID,
				To:         repoID,
				Properties: pathsProperties(paths),
			})
		}

		// Find all resources managed by the Application, as listed in its
		// status.resources.
		resources, _, _ := unstructured.NestedSlice(app.Object, "status", "resources")
		for _, r := range resources {
			res, ok := r.(map[string]any)
			if !ok {
				continue
			}
			var (
				group, _ = res["group"].(string)
				kind, _  = res["kind"].(string)
				ns, _    = res["namespace"].(string)
				name, _  = res["name"].(string)
			)
			if kind == "" || name == "" {
				log.Printf("argocd: WARN: skipping resource with missing kind (%q) or name (%q) in Application %s/%s", kind, name, app.GetNamespace(), app.GetName())
				continue
			}

			resID := resourceNodeID(clusterID, group, kind, ns, name)
			if resID == appID {
				continue
			}
			addNode(pluginapi.NodeClaim{ID: resID})
			relations = append(relations, pluginapi.RelationClaim{
				Kind: pluginapi.RelationKindDescribes,
				From: appID,
				To:   resID,
			})
			for repoID, paths := range pathsByRepo {
				relations = append(relations, pluginapi.RelationClaim{
					Kind:       pluginapi.RelationKindDeployedFrom,
					From:       resID,
					To:         repoID,
					Properties: pathsProperties(paths),
				})
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

// resourceNodeID returns the NodeID of a resource listed in an Application's
// status.resources. For cluster-scoped resources, ns can be empty.
func resourceNodeID(clusterID, group, kind, ns, name string) pluginapi.NodeID {
	// TODO(akavel-reply): should we just use a single naming scheme for all
	// k8s resources, without special-casing Deployments and Services? Should we
	// use custom NodeID.Kind for every resource kind, e.g.
	// `NodeID.Kind=Deployment.apps.k8s`? Would need to be also updated in
	// other plugins.
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
	var (
		server = nestedStringOrEmpty(app, "spec", "destination", "server")
		name   = nestedStringOrEmpty(app, "spec", "destination", "name")
	)
	// ArgoCD's names for the cluster the Application lives in.
	const (
		inClusterServer = "https://kubernetes.default.svc"
		inClusterName   = "in-cluster"
	)
	return server == inClusterServer || name == inClusterName
}

// source is the part of an Application source we use. A source with a Helm
// chart or a "ref" has no path.
type source struct {
	repoURL string
	path    string
}

// sources returns the sources of the Application, with non-empty repoURL.
func sources(app unstructured.Unstructured) []source {
	specs, _, _ := unstructured.NestedSlice(app.Object, "spec", "sources")

	// According to the ArgoCD documentation as of Oct 2026, "When you
	// specify the sources field, Argo CD will ignore the source (singular) field."
	// (https://argo-cd.readthedocs.io/en/stable/user-guide/multiple_sources/)
	if len(specs) == 0 {
		single, _, _ := unstructured.NestedMap(app.Object, "spec", "source")
		if single == nil {
			return nil
		}
		specs = []any{single}
	}

	var sources []source
	for _, spec := range specs {
		m, _ := spec.(map[string]any)
		url, _ := m["repoURL"].(string)
		if url == "" {
			continue
		}
		path, _ := m["path"].(string)
		sources = append(sources, source{repoURL: url, path: path})
	}
	return sources
}

// pathsProperties returns the "paths" property: a JSON list of paths within a
// repository, or no properties if there are none.
func pathsProperties(paths []string) pluginapi.PropertyMap {
	if len(paths) == 0 {
		return nil
	}
	encoded, err := json.Marshal(paths)
	if err != nil {
		panic(fmt.Sprintf("marshaling []string to JSON: %v", err)) // cannot happen
	}
	return pluginapi.PropertyMap{
		"paths": string(encoded),
	}
}

// sourceType returns the tool(s) ArgoCD detected for the Application's source(s).
func sourceType(app unstructured.Unstructured) string {
	types, _, _ := unstructured.NestedStringSlice(app.Object, "status", "sourceTypes")
	if len(types) == 0 {
		return nestedStringOrEmpty(app, "status", "sourceType")
	}
	return strings.Join(types, ",")
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

// lastDeployedAt returns deployedAt of the newest entry of the Application's
// status.history, or empty string if there is none.
func lastDeployedAt(app unstructured.Unstructured) string {
	history, _, _ := unstructured.NestedSlice(app.Object, "status", "history")
	if len(history) == 0 {
		return ""
	}
	last, _ := history[len(history)-1].(map[string]any)
	deployedAt, _ := last["deployedAt"].(string)
	return deployedAt
}

// setIfNotEmpty sets props[key] unless value is empty.
func setIfNotEmpty(props pluginapi.PropertyMap, key, value string) {
	if value != "" {
		props[key] = value
	}
}

func nestedStringOrEmpty(u unstructured.Unstructured, fields ...string) string {
	s, _, _ := unstructured.NestedString(u.Object, fields...)
	return s
}
