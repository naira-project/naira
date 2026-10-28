package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:generate go run ./testdata/gen
//go:generate go run ./testdata/gen -in testdata/app-of-apps/input.yaml -out testdata/app-of-apps

const mockClusterID = "mock-cluster"

// TestCollect runs the plugin on real Application objects scraped from the
// argocd-server API by the ./testdata/gen command, see testdata/.
func TestCollect(t *testing.T) {
	tests := []struct {
		name     string
		testdata []string // names of testdata/*.json files to load
		want     pluginapi.CollectResponse
	}{
		{
			name:     "Helm chart from a Git path",
			testdata: []string{"helm-guestbook"},
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("Application.argocd", "test-prefix/argocd/helm-guestbook"), Properties: props{
						"project":                   "default",
						"sync_status":               "Synced",
						"health_status":             "Progressing",
						"destination_namespace":     "helm-guestbook",
						"source_type":               "Helm",
						"last_deployed_at":          "2000-01-01T00:00:00Z",
						"operation_present":         "false",
						"operationState.finishedAt": "2000-01-01T00:00:00Z",
						"operationState.phase":      "Succeeded",
						"operationState.startedAt":  "2000-01-01T00:00:00Z"}},
					{ID: nodeID("deployment", "mock-cluster/helm-guestbook/helm-guestbook")},
					{ID: nodeID("git_repository", "github.com/argoproj/argocd-example-apps"), Properties: props{
						"url": "https://github.com/argoproj/argocd-example-apps"}},
					{ID: nodeID("service", "mock-cluster/helm-guestbook/helm-guestbook")},
				},
				Relations: []pluginapi.RelationClaim{
					{Kind: "deployed_from",
						From: nodeID("deployment", "mock-cluster/helm-guestbook/helm-guestbook"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["helm-guestbook"]`}},
					{Kind: "deployed_from",
						From: nodeID("service", "mock-cluster/helm-guestbook/helm-guestbook"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["helm-guestbook"]`}},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/helm-guestbook"),
						To:   nodeID("deployment", "mock-cluster/helm-guestbook/helm-guestbook")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/helm-guestbook"),
						To:   nodeID("service", "mock-cluster/helm-guestbook/helm-guestbook")},
					{Kind: "sourced_from",
						From: nodeID("Application.argocd", "test-prefix/argocd/helm-guestbook"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["helm-guestbook"]`}},
				},
			},
		},
		{
			name:     "Kustomize from a Git path",
			testdata: []string{"kustomize-guestbook"},
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("Application.argocd", "test-prefix/argocd/kustomize-guestbook"), Properties: props{
						"project":                   "default",
						"sync_status":               "Synced",
						"health_status":             "Progressing",
						"destination_namespace":     "kustomize-guestbook",
						"source_type":               "Kustomize",
						"last_deployed_at":          "2000-01-01T00:00:00Z",
						"operation_present":         "false",
						"operationState.finishedAt": "2000-01-01T00:00:00Z",
						"operationState.phase":      "Succeeded",
						"operationState.startedAt":  "2000-01-01T00:00:00Z"}},
					{ID: nodeID("deployment", "mock-cluster/kustomize-guestbook/kustomize-guestbook-ui")},
					{ID: nodeID("git_repository", "github.com/argoproj/argocd-example-apps"), Properties: props{
						"url": "https://github.com/argoproj/argocd-example-apps"}},
					{ID: nodeID("service", "mock-cluster/kustomize-guestbook/kustomize-guestbook-ui")},
				},
				Relations: []pluginapi.RelationClaim{
					{Kind: "deployed_from",
						From: nodeID("deployment", "mock-cluster/kustomize-guestbook/kustomize-guestbook-ui"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["kustomize-guestbook"]`}},
					{Kind: "deployed_from",
						From: nodeID("service", "mock-cluster/kustomize-guestbook/kustomize-guestbook-ui"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["kustomize-guestbook"]`}},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/kustomize-guestbook"),
						To:   nodeID("deployment", "mock-cluster/kustomize-guestbook/kustomize-guestbook-ui")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/kustomize-guestbook"),
						To:   nodeID("service", "mock-cluster/kustomize-guestbook/kustomize-guestbook-ui")},
					{Kind: "sourced_from",
						From: nodeID("Application.argocd", "test-prefix/argocd/kustomize-guestbook"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["kustomize-guestbook"]`}},
				},
			},
		},
		{
			name:     "multiple spec.sources with the same repoURL are emitted once",
			testdata: []string{"multi-source"},
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("Application.argocd", "test-prefix/argocd/multi-source"), Properties: props{
						"project":                   "default",
						"sync_status":               "Synced",
						"health_status":             "Progressing",
						"destination_namespace":     "multi-source",
						"source_type":               "Helm,",
						"last_deployed_at":          "2000-01-01T00:00:00Z",
						"operation_present":         "false",
						"operationState.finishedAt": "2000-01-01T00:00:00Z",
						"operationState.phase":      "Succeeded",
						"operationState.startedAt":  "2000-01-01T00:00:00Z"}},
					{ID: nodeID("deployment", "mock-cluster/multi-source/multi-source-helm-guestbook")},
					{ID: nodeID("git_repository", "github.com/argoproj/argocd-example-apps"), Properties: props{
						"url": "https://github.com/argoproj/argocd-example-apps"}},
					{ID: nodeID("service", "mock-cluster/multi-source/multi-source-helm-guestbook")},
				},
				Relations: []pluginapi.RelationClaim{
					{Kind: "deployed_from",
						From: nodeID("deployment", "mock-cluster/multi-source/multi-source-helm-guestbook"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["helm-guestbook"]`}},
					{Kind: "deployed_from",
						From: nodeID("service", "mock-cluster/multi-source/multi-source-helm-guestbook"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["helm-guestbook"]`}},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/multi-source"),
						To:   nodeID("deployment", "mock-cluster/multi-source/multi-source-helm-guestbook")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/multi-source"),
						To:   nodeID("service", "mock-cluster/multi-source/multi-source-helm-guestbook")},
					{Kind: "sourced_from",
						From: nodeID("Application.argocd", "test-prefix/argocd/multi-source"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["helm-guestbook"]`}},
				},
			},
		},
		{
			name: "app-of-apps: child Application keeps its properties",
			testdata: []string{
				"app-of-apps/apps-parent",
				"app-of-apps/example.guestbook"},
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("Application.argocd", "test-prefix/argocd/apps-parent"), Properties: props{
						"project":                   "default",
						"sync_status":               "Synced",
						"health_status":             "Healthy",
						"destination_namespace":     "argocd",
						"source_type":               "Helm",
						"last_deployed_at":          "2000-01-01T00:00:00Z",
						"operation_present":         "false",
						"operationState.finishedAt": "2000-01-01T00:00:00Z",
						"operationState.phase":      "Succeeded",
						"operationState.startedAt":  "2000-01-01T00:00:00Z"}},
					{ID: nodeID("Application.argocd", "test-prefix/argocd/example.guestbook"), Properties: props{
						"project":                   "default",
						"sync_status":               "Synced",
						"health_status":             "Progressing",
						"destination_namespace":     "guestbook",
						"source_type":               "Directory",
						"last_deployed_at":          "2000-01-01T00:00:00Z",
						"operation_present":         "false",
						"operationState.finishedAt": "2000-01-01T00:00:00Z",
						"operationState.phase":      "Succeeded",
						"operationState.startedAt":  "2000-01-01T00:00:00Z"}},
					{ID: nodeID("deployment", "mock-cluster/guestbook/guestbook-ui")},
					{ID: nodeID("git_repository", "github.com/argoproj/argocd-example-apps"), Properties: props{
						"url": "https://github.com/argoproj/argocd-example-apps"}},
					{ID: nodeID("service", "mock-cluster/guestbook/guestbook-ui")},
				},
				Relations: []pluginapi.RelationClaim{
					{Kind: "deployed_from",
						From: nodeID("Application.argocd", "test-prefix/argocd/example.guestbook"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["apps"]`}},
					{Kind: "deployed_from",
						From: nodeID("deployment", "mock-cluster/guestbook/guestbook-ui"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["guestbook"]`}},
					{Kind: "deployed_from",
						From: nodeID("service", "mock-cluster/guestbook/guestbook-ui"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["guestbook"]`}},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/apps-parent"),
						To:   nodeID("Application.argocd", "test-prefix/argocd/example.guestbook")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/example.guestbook"),
						To:   nodeID("deployment", "mock-cluster/guestbook/guestbook-ui")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/example.guestbook"),
						To:   nodeID("service", "mock-cluster/guestbook/guestbook-ui")},
					{Kind: "sourced_from",
						From: nodeID("Application.argocd", "test-prefix/argocd/apps-parent"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["apps"]`}},
					{Kind: "sourced_from",
						From: nodeID("Application.argocd", "test-prefix/argocd/example.guestbook"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["guestbook"]`}},
				},
			},
		},
		{
			name:     "multiple sources from the same repository at different paths list all the paths",
			testdata: []string{"multi-path"},
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("Application.argocd", "test-prefix/argocd/multi-path"), Properties: props{
						"project":                   "default",
						"sync_status":               "Synced",
						"health_status":             "Progressing",
						"destination_namespace":     "multi-path",
						"source_type":               "Directory,Kustomize",
						"last_deployed_at":          "2000-01-01T00:00:00Z",
						"operation_present":         "false",
						"operationState.finishedAt": "2000-01-01T00:00:00Z",
						"operationState.phase":      "Succeeded",
						"operationState.startedAt":  "2000-01-01T00:00:00Z"}},
					{ID: nodeID("deployment", "mock-cluster/multi-path/guestbook-ui")},
					{ID: nodeID("deployment", "mock-cluster/multi-path/kustomize-guestbook-ui")},
					{ID: nodeID("git_repository", "github.com/argoproj/argocd-example-apps"), Properties: props{
						"url": "https://github.com/argoproj/argocd-example-apps"}},
					{ID: nodeID("service", "mock-cluster/multi-path/guestbook-ui")},
					{ID: nodeID("service", "mock-cluster/multi-path/kustomize-guestbook-ui")},
				},
				Relations: []pluginapi.RelationClaim{
					{Kind: "deployed_from",
						From: nodeID("deployment", "mock-cluster/multi-path/guestbook-ui"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["guestbook","kustomize-guestbook"]`}},
					{Kind: "deployed_from",
						From: nodeID("deployment", "mock-cluster/multi-path/kustomize-guestbook-ui"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["guestbook","kustomize-guestbook"]`}},
					{Kind: "deployed_from",
						From: nodeID("service", "mock-cluster/multi-path/guestbook-ui"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["guestbook","kustomize-guestbook"]`}},
					{Kind: "deployed_from",
						From: nodeID("service", "mock-cluster/multi-path/kustomize-guestbook-ui"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["guestbook","kustomize-guestbook"]`}},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/multi-path"),
						To:   nodeID("deployment", "mock-cluster/multi-path/guestbook-ui")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/multi-path"),
						To:   nodeID("deployment", "mock-cluster/multi-path/kustomize-guestbook-ui")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/multi-path"),
						To:   nodeID("service", "mock-cluster/multi-path/guestbook-ui")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/multi-path"),
						To:   nodeID("service", "mock-cluster/multi-path/kustomize-guestbook-ui")},
					{Kind: "sourced_from",
						From: nodeID("Application.argocd", "test-prefix/argocd/multi-path"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["guestbook","kustomize-guestbook"]`}},
				},
			},
		},
		{
			name:     "sync in progress: operationState is copied, and last_deployed_at is absent without history",
			testdata: []string{"mid-sync"},
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("Application.argocd", "test-prefix/argocd/mid-sync"), Properties: props{
						"project":                   "default",
						"sync_status":               "OutOfSync",
						"health_status":             "Missing",
						"destination_namespace":     "mid-sync",
						"source_type":               "Kustomize",
						"operation_present":         "true",
						"operationState.finishedAt": "2000-01-01T00:00:00Z",
						"operationState.phase":      "Running",
						"operationState.startedAt":  "2000-01-01T00:00:00Z"}},
					{ID: nodeID("deployment", "mock-cluster/mid-sync/pre-post-sync-kustomize-guestbook-ui")},
					{ID: nodeID("git_repository", "github.com/argoproj/argocd-example-apps"), Properties: props{
						"url": "https://github.com/argoproj/argocd-example-apps"}},
					{ID: nodeID("service", "mock-cluster/mid-sync/pre-post-sync-kustomize-guestbook-ui")},
				},
				Relations: []pluginapi.RelationClaim{
					{Kind: "deployed_from",
						From: nodeID("deployment", "mock-cluster/mid-sync/pre-post-sync-kustomize-guestbook-ui"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["pre-post-sync"]`}},
					{Kind: "deployed_from",
						From: nodeID("service", "mock-cluster/mid-sync/pre-post-sync-kustomize-guestbook-ui"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["pre-post-sync"]`}},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/mid-sync"),
						To:   nodeID("deployment", "mock-cluster/mid-sync/pre-post-sync-kustomize-guestbook-ui")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/mid-sync"),
						To:   nodeID("service", "mock-cluster/mid-sync/pre-post-sync-kustomize-guestbook-ui")},
					{Kind: "sourced_from",
						From: nodeID("Application.argocd", "test-prefix/argocd/mid-sync"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["pre-post-sync"]`}},
				},
			},
		},
		{
			name:     "Application targeting another cluster is skipped",
			testdata: []string{"remote-cluster"},
			want:     pluginapi.CollectResponse{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var items []string
			for _, name := range tt.testdata {
				data, err := os.ReadFile(filepath.Join("testdata", name+".json"))
				require.NoError(t, err)
				items = append(items, string(data))
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/v1/applications", r.URL.Path)
				assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
				fmt.Fprintf(w, `{"items":[%s]}`, strings.Join(items, ","))
			}))
			defer srv.Close()

			got, err := New(config{
				BaseURL:    srv.URL + "/",
				Token:      "test-token",
				PathPrefix: "test-prefix",
				ClusterID:  mockClusterID,
			}).Collect(t.Context())
			require.NoError(t, err)
			sort.Slice(got.Nodes, func(i, j int) bool {
				a, b := got.Nodes[i].ID, got.Nodes[j].ID
				return join(a.Kind, a.Path) < join(b.Kind, b.Path)
			})
			sort.Slice(got.Relations, func(i, j int) bool {
				a, b := got.Relations[i], got.Relations[j]
				return join(a.Kind, a.From.Kind, a.From.Path, a.To.Kind, a.To.Path) <
					join(b.Kind, b.From.Kind, b.From.Path, b.To.Kind, b.To.Path)
			})
			assert.Equal(t, tt.want, got)
		})
	}
}

func join(parts ...string) string {
	return strings.Join(parts, "\x00")
}

func nodeID(kind, path string) pluginapi.NodeID {
	return pluginapi.NodeID{Kind: kind, Path: path}
}

type props = pluginapi.PropertyMap
