package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	discfake "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/dynamic/fake"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
)

//go:generate go run ./testdata/gen

const mockClusterID = "mock-cluster"

// TestCollect runs the plugin on real Application objects scraped from ArgoCD
// by the ./testdata/gen command, see testdata/.
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
					{ID: nodeID("Application.argocd", "mock-cluster/argocd/helm-guestbook"), Properties: props{
						"project":               "default",
						"sync_status":           "Synced",
						"health_status":         "Progressing",
						"destination_namespace": "helm-guestbook",
						"source_type":           "Helm",
						"last_synced_at":        "2000-01-01T00:00:00Z"}},
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
					{Kind: "describes",
						From: nodeID("Application.argocd", "mock-cluster/argocd/helm-guestbook"),
						To:   nodeID("deployment", "mock-cluster/helm-guestbook/helm-guestbook")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "mock-cluster/argocd/helm-guestbook"),
						To:   nodeID("service", "mock-cluster/helm-guestbook/helm-guestbook")},
					{Kind: "sourced_from",
						From: nodeID("Application.argocd", "mock-cluster/argocd/helm-guestbook"),
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
					{ID: nodeID("Application.argocd", "mock-cluster/argocd/kustomize-guestbook"), Properties: props{
						"project":               "default",
						"sync_status":           "Synced",
						"health_status":         "Progressing",
						"destination_namespace": "kustomize-guestbook",
						"source_type":           "Kustomize",
						"last_synced_at":        "2000-01-01T00:00:00Z"}},
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
					{Kind: "describes",
						From: nodeID("Application.argocd", "mock-cluster/argocd/kustomize-guestbook"),
						To:   nodeID("deployment", "mock-cluster/kustomize-guestbook/kustomize-guestbook-ui")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "mock-cluster/argocd/kustomize-guestbook"),
						To:   nodeID("service", "mock-cluster/kustomize-guestbook/kustomize-guestbook-ui")},
					{Kind: "sourced_from",
						From: nodeID("Application.argocd", "mock-cluster/argocd/kustomize-guestbook"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["kustomize-guestbook"]`}},
				},
			},
		},
		{
			name:     "multiple sources from the same repository are emitted once",
			testdata: []string{"multi-source"},
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("Application.argocd", "mock-cluster/argocd/multi-source"), Properties: props{
						"project":               "default",
						"sync_status":           "Synced",
						"health_status":         "Progressing",
						"destination_namespace": "multi-source",
						"source_type":           "Helm,",
						"last_synced_at":        "2000-01-01T00:00:00Z"}},
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
					{Kind: "describes",
						From: nodeID("Application.argocd", "mock-cluster/argocd/multi-source"),
						To:   nodeID("deployment", "mock-cluster/multi-source/multi-source-helm-guestbook")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "mock-cluster/argocd/multi-source"),
						To:   nodeID("service", "mock-cluster/multi-source/multi-source-helm-guestbook")},
					{Kind: "sourced_from",
						From: nodeID("Application.argocd", "mock-cluster/argocd/multi-source"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["helm-guestbook"]`}},
				},
			},
		},
		{
			name:     "app-of-apps: child Application keeps its properties",
			testdata: []string{"apps-parent", "example.guestbook"},
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("Application.argocd", "mock-cluster/argocd/apps-parent"), Properties: props{
						"project":               "default",
						"sync_status":           "Synced",
						"health_status":         "Healthy",
						"destination_namespace": "argocd",
						"source_type":           "Helm",
						"last_synced_at":        "2000-01-01T00:00:00Z"}},
					{ID: nodeID("Application.argocd", "mock-cluster/argocd/example.guestbook"), Properties: props{
						"project":               "default",
						"sync_status":           "Synced",
						"health_status":         "Progressing",
						"destination_namespace": "guestbook",
						"source_type":           "Directory",
						"last_synced_at":        "2000-01-01T00:00:00Z"}},
					{ID: nodeID("deployment", "mock-cluster/guestbook/guestbook-ui")},
					{ID: nodeID("git_repository", "github.com/argoproj/argocd-example-apps"), Properties: props{
						"url": "https://github.com/argoproj/argocd-example-apps"}},
					{ID: nodeID("service", "mock-cluster/guestbook/guestbook-ui")},
				},
				Relations: []pluginapi.RelationClaim{
					{Kind: "deployed_from",
						From: nodeID("deployment", "mock-cluster/guestbook/guestbook-ui"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["guestbook"]`}},
					{Kind: "describes",
						From: nodeID("Application.argocd", "mock-cluster/argocd/apps-parent"),
						To:   nodeID("Application.argocd", "mock-cluster/argocd/example.guestbook")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "mock-cluster/argocd/example.guestbook"),
						To:   nodeID("deployment", "mock-cluster/guestbook/guestbook-ui")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "mock-cluster/argocd/example.guestbook"),
						To:   nodeID("service", "mock-cluster/guestbook/guestbook-ui")},
					{Kind: "sourced_from",
						From: nodeID("Application.argocd", "mock-cluster/argocd/apps-parent"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["apps"]`}},
					{Kind: "sourced_from",
						From: nodeID("Application.argocd", "mock-cluster/argocd/example.guestbook"),
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
					{ID: nodeID("Application.argocd", "mock-cluster/argocd/multi-path"), Properties: props{
						"project":               "default",
						"sync_status":           "Synced",
						"health_status":         "Progressing",
						"destination_namespace": "multi-path",
						"source_type":           "Directory,Kustomize",
						"last_synced_at":        "2000-01-01T00:00:00Z"}},
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
					{Kind: "describes",
						From: nodeID("Application.argocd", "mock-cluster/argocd/multi-path"),
						To:   nodeID("deployment", "mock-cluster/multi-path/guestbook-ui")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "mock-cluster/argocd/multi-path"),
						To:   nodeID("deployment", "mock-cluster/multi-path/kustomize-guestbook-ui")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "mock-cluster/argocd/multi-path"),
						To:   nodeID("service", "mock-cluster/multi-path/guestbook-ui")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "mock-cluster/argocd/multi-path"),
						To:   nodeID("service", "mock-cluster/multi-path/kustomize-guestbook-ui")},
					{Kind: "sourced_from",
						From: nodeID("Application.argocd", "mock-cluster/argocd/multi-path"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["guestbook","kustomize-guestbook"]`}},
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
			objs := []runtime.Object{
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system", UID: mockClusterID}},
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "argocd", UID: "argocd-uid"}},
			}
			for _, name := range tt.testdata {
				data, err := os.ReadFile(filepath.Join("testdata", name+".json"))
				require.NoError(t, err)
				var u unstructured.Unstructured
				require.NoError(t, json.Unmarshal(data, &u.Object))
				objs = append(objs, &u)
			}

			gvr := schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}
			s := runtime.NewScheme()
			_ = corev1.AddToScheme(s)
			s.AddKnownTypeWithName(gvr.GroupVersion().WithKind("ApplicationList"), &unstructured.UnstructuredList{})
			dyn := fake.NewSimpleDynamicClientWithCustomListKinds(s,
				map[schema.GroupVersionResource]string{
					gvr: "ApplicationList",
				},
				objs...)
			disc := &discfake.FakeDiscovery{Fake: &dyn.Fake}
			disc.Resources = []*metav1.APIResourceList{{
				GroupVersion: gvr.GroupVersion().String(),
				APIResources: []metav1.APIResource{{
					Name:  "applications",
					Kind:  "Application",
					Verbs: []string{"list"},
				}},
			}}

			got, err := New(config{}).collect(t.Context(), disc, dyn)
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
