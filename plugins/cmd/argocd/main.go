// Plugin argocd collects ArgoCD Application objects and all the Kubernetes
// resources they manage, as reported in each Application's status.
//
// The data is read from the REST API of argocd-server ("/api/v1/applications"),
// not from the Kubernetes API.
//
// Managed Deployments and Services are emitted as deployment and service Nodes.
// Child Applications (app-of-apps) are emitted as Application Nodes. Any other
// managed resource is emitted as a k8s_resource Node, with a path of
// "<cluster>/<group>/<Kind>/<namespace>/<name>" ("core" is used as the group of
// the core API, and the namespace segment is omitted for cluster-scoped resources).
//
// Application Nodes have a path of "<PATH_PREFIX>/<namespace>/<name>".
// All other Nodes use CLUSTER_ID as the "<cluster>" path segment.
//
// # Environment Variables
//
//   - ARGOCD_BASE_URL (optional, default "http://argocd-server.argocd.svc.cluster.local") -
//     base URL of argocd-server.
//   - ARGOCD_TOKEN (optional) - API token, sent as a Bearer token. Needs only
//     read access ("applications, get"), e.g. a token of a local account
//     with such a role, see "argocd account generate-token".
//   - ARGOCD_INSECURE_SKIP_VERIFY (optional, default false) - skip verification
//     of the TLS certificate of argocd-server, e.g. when it uses the self-signed
//     one it generates by default.
//   - ARGOCD_HTTP_TIMEOUT (optional, default 10s) - timeout of the HTTP requests.
//   - PATH_PREFIX (optional, default "argocd") - prefix of the paths of the
//     Application Nodes. Set it to a different value for each ArgoCD instance.
//   - CLUSTER_ID - ID of the Kubernetes cluster that the Applications deploy to,
//     which is the UID of its "kube-system" namespace, as used by other plugins.
//     ArgoCD API doesn't expose it, so it must be provided by the user.
//
// # Known Limitations
//
// Only Applications deploying to the cluster ArgoCD runs in are supported;
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
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/naira-project/naira/plugins/internal/repositoryidentity"
	"github.com/naira-project/naira/plugins/pkg/httpjson"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/naira-project/naira/plugins/pkg/pluginmain"
)

const (
	argoGroup       = "argoproj.io"
	argoApplication = "Application"
)

type config struct {
	BaseURL            string        `env:"ARGOCD_BASE_URL" default:"http://argocd-server.argocd.svc.cluster.local"`
	Token              string        `env:"ARGOCD_TOKEN"`
	InsecureSkipVerify bool          `env:"ARGOCD_INSECURE_SKIP_VERIFY"`
	HTTPTimeout        time.Duration `env:"ARGOCD_HTTP_TIMEOUT" default:"10s"`
	PathPrefix         string        `env:"PATH_PREFIX" default:"argocd"`
	ClusterID          string        `env:"CLUSTER_ID"`
}

type Plugin struct {
	config     config
	httpClient *http.Client
}

func New(config config) *Plugin {
	return &Plugin{
		config: config,
		httpClient: &http.Client{
			Timeout: config.HTTPTimeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: config.InsecureSkipVerify},
			},
		},
	}
}

func main() {
	app := pluginmain.New[config]()
	app.Serve(New(app.PluginConfig))
}

func (p *Plugin) Collect(ctx context.Context) (pluginapi.CollectResponse, error) {
	if p.config.ClusterID == "" {
		return pluginapi.CollectResponse{}, errors.New("CLUSTER_ID is not set")
	}
	apps, err := p.listApplications(ctx)
	if err != nil {
		return pluginapi.CollectResponse{}, fmt.Errorf("listing Applications: %w", err)
	}
	return p.collect(apps), nil
}

func (p *Plugin) listApplications(ctx context.Context) ([]v1alpha1.Application, error) {
	var list v1alpha1.ApplicationList
	err := httpjson.Get(ctx, p.httpClient,
		strings.TrimRight(p.config.BaseURL, "/")+"/api/v1/applications",
		&list,
		httpjson.WithAuthorizationBearer(p.config.Token))
	if err != nil {
		return nil, fmt.Errorf("calling argocd-server API: %w", err)
	}
	return list.Items, nil
}

func (p *Plugin) collect(apps []v1alpha1.Application) pluginapi.CollectResponse {
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
	appNodeID := func(app v1alpha1.Application) pluginapi.NodeID {
		return applicationID(p.config.PathPrefix, app.Namespace, app.Name)
	}

	var inClusterApps []v1alpha1.Application
	for _, app := range apps {
		if !isInCluster(app) {
			log.Printf("argocd: skipping Application %s/%s: destination is not the local cluster", app.Namespace, app.Name)
			continue
		}
		inClusterApps = append(inClusterApps, app)

		props := pluginapi.PropertyMap{
			"project":               app.Spec.Project,
			"sync_status":           string(app.Status.Sync.Status),
			"health_status":         string(app.Status.Health.Status),
			"destination_namespace": app.Spec.Destination.Namespace,
			"source_type":           sourceType(app),
			"operation_present":     strconv.FormatBool(app.Operation != nil),
		}
		setIfNotEmpty(props, "last_deployed_at", lastDeployedAt(app))
		if op := app.Status.OperationState; op != nil {
			setIfNotEmpty(props, "operationState.phase", string(op.Phase))
			setIfNotEmpty(props, "operationState.startedAt", formatTime(&op.StartedAt))
			setIfNotEmpty(props, "operationState.finishedAt", formatTime(op.FinishedAt))
		}

		addNode(pluginapi.NodeClaim{
			ID:         appNodeID(app),
			Properties: props,
		})
	}

	for _, app := range inClusterApps {
		appID := appNodeID(app)

		// Multiple sources from a single Application can point to the same
		// repository, at different paths. A relation is identified by its kind
		// and ends, so we need to create only one per repository, listing all
		// the paths used.
		pathsByRepo := map[pluginapi.NodeID][]string{}
		for _, src := range sources(app) {
			repoPath := repositoryidentity.GitHubRepositoryNodePathFromURL(src.RepoURL)
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
						"url": src.RepoURL,
					},
				})
				pathsByRepo[repoID] = nil
			}
			if src.Path != "" && !slices.Contains(pathsByRepo[repoID], src.Path) {
				pathsByRepo[repoID] = append(pathsByRepo[repoID], src.Path)
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
		for _, res := range app.Status.Resources {
			if res.Kind == "" || res.Name == "" {
				log.Printf("argocd: WARN: skipping resource with missing kind (%q) or name (%q) in Application %s/%s", res.Kind, res.Name, app.Namespace, app.Name)
				continue
			}

			resID := resourceNodeID(p.config.PathPrefix, p.config.ClusterID, res)
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

	return pluginapi.CollectResponse{Nodes: nodes, Relations: relations}
}

func applicationID(pathPrefix, ns, name string) pluginapi.NodeID {
	return pluginapi.NodeID{
		Kind: pluginapi.NodeKindArgoApplication,
		Path: pathPrefix + "/" + ns + "/" + name,
	}
}

// resourceNodeID returns the NodeID of a resource listed in an Application's
// status.resources. For cluster-scoped resources, res.Namespace can be empty.
func resourceNodeID(pathPrefix, clusterID string, res v1alpha1.ResourceStatus) pluginapi.NodeID {
	// TODO(akavel-reply): should we just use a single naming scheme for all
	// k8s resources, without special-casing Deployments and Services? Should we
	// use custom NodeID.Kind for every resource kind, e.g.
	// `NodeID.Kind=Deployment.apps.k8s`? Would need to be also updated in
	// other plugins.
	switch {
	case res.Group == "apps" && res.Kind == "Deployment":
		return pluginapi.NodeID{
			Kind: pluginapi.NodeKindDeployment,
			Path: clusterID + "/" + res.Namespace + "/" + res.Name,
		}
	case res.Group == "" && res.Kind == "Service":
		return pluginapi.NodeID{
			Kind: pluginapi.NodeKindService,
			Path: clusterID + "/" + res.Namespace + "/" + res.Name,
		}
	case res.Group == argoGroup && res.Kind == argoApplication:
		return applicationID(pathPrefix, res.Namespace, res.Name)
	}
	group := res.Group
	if group == "" {
		group = "core"
	}
	path := clusterID + "/" + group + "/" + res.Kind
	if res.Namespace != "" {
		path += "/" + res.Namespace
	}
	return pluginapi.NodeID{
		Kind: pluginapi.NodeKindK8sResource,
		Path: path + "/" + res.Name,
	}
}

// isInCluster returns true if the Application deploys to the cluster ArgoCD runs in.
func isInCluster(app v1alpha1.Application) bool {
	// ArgoCD's names for the cluster it runs in.
	const (
		inClusterServer = "https://kubernetes.default.svc"
		inClusterName   = "in-cluster"
	)
	dest := app.Spec.Destination
	return dest.Server == inClusterServer || dest.Name == inClusterName
}

// sources returns the sources of the Application, with non-empty repoURL.
func sources(app v1alpha1.Application) []v1alpha1.ApplicationSource {
	specs := app.Spec.Sources

	// According to the ArgoCD documentation as of Oct 2026, "When you
	// specify the sources field, Argo CD will ignore the source (singular) field."
	// (https://argo-cd.readthedocs.io/en/stable/user-guide/multiple_sources/)
	if len(specs) == 0 && app.Spec.Source != nil {
		specs = []v1alpha1.ApplicationSource{*app.Spec.Source}
	}

	return slices.DeleteFunc(slices.Clone(specs), func(s v1alpha1.ApplicationSource) bool {
		return s.RepoURL == ""
	})
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
func sourceType(app v1alpha1.Application) string {
	if len(app.Status.SourceTypes) == 0 {
		return string(app.Status.SourceType)
	}
	types := make([]string, len(app.Status.SourceTypes))
	for i, t := range app.Status.SourceTypes {
		types[i] = string(t)
	}
	return strings.Join(types, ",")
}

// lastDeployedAt returns deployedAt of the newest entry of the Application's
// status.history, or empty string if there is none.
func lastDeployedAt(app v1alpha1.Application) string {
	history := app.Status.History
	if len(history) == 0 {
		return ""
	}
	return formatTime(&history[len(history)-1].DeployedAt)
}

// formatTime formats t as RFC 3339, or returns empty string for nil or zero t.
func formatTime(t *metav1.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// setIfNotEmpty sets props[key] unless value is empty.
func setIfNotEmpty(props pluginapi.PropertyMap, key, value string) {
	if value != "" {
		props[key] = value
	}
}
