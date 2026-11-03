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

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//go:generate go run ./testdata/gen
//go:generate go run ./testdata/gen -in testdata/app-of-apps/input.yaml -out testdata/app-of-apps
//go:generate go run ./testdata/gen -in testdata/cluster-scoped/input.yaml -out testdata/cluster-scoped
//go:generate go run ./testdata/gen -in testdata/oci/input.yaml -out testdata/oci

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
						"project":                     "default",
						"sync.status":                 "Synced",
						"health.status":               "Healthy",
						"destination.namespace":       "helm-guestbook",
						"source_type":                 "Helm",
						"last_deployed_at":            "2000-01-01T00:00:00Z",
						"operation_present":           "false",
						"operation_state.finished_at": "2000-01-01T00:00:00Z",
						"operation_state.phase":       "Succeeded",
						"operation_state.started_at":  "2000-01-01T00:00:00Z"}},
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
						"project":                     "default",
						"sync.status":                 "Synced",
						"health.status":               "Healthy",
						"destination.namespace":       "kustomize-guestbook",
						"source_type":                 "Kustomize",
						"last_deployed_at":            "2000-01-01T00:00:00Z",
						"operation_present":           "false",
						"operation_state.finished_at": "2000-01-01T00:00:00Z",
						"operation_state.phase":       "Succeeded",
						"operation_state.started_at":  "2000-01-01T00:00:00Z"}},
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
						"project":                     "default",
						"sync.status":                 "Synced",
						"health.status":               "Healthy",
						"destination.namespace":       "multi-source",
						"source_type":                 "Helm,",
						"last_deployed_at":            "2000-01-01T00:00:00Z",
						"operation_present":           "false",
						"operation_state.finished_at": "2000-01-01T00:00:00Z",
						"operation_state.phase":       "Succeeded",
						"operation_state.started_at":  "2000-01-01T00:00:00Z"}},
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
						"project":                     "default",
						"sync.status":                 "Synced",
						"health.status":               "Healthy",
						"destination.namespace":       "argocd",
						"source_type":                 "Helm",
						"last_deployed_at":            "2000-01-01T00:00:00Z",
						"operation_present":           "false",
						"operation_state.finished_at": "2000-01-01T00:00:00Z",
						"operation_state.phase":       "Succeeded",
						"operation_state.started_at":  "2000-01-01T00:00:00Z"}},
					{ID: nodeID("Application.argocd", "test-prefix/argocd/example.guestbook"), Properties: props{
						"project":                     "default",
						"sync.status":                 "Synced",
						"health.status":               "Healthy",
						"destination.namespace":       "guestbook",
						"source_type":                 "Directory",
						"last_deployed_at":            "2000-01-01T00:00:00Z",
						"operation_present":           "false",
						"operation_state.finished_at": "2000-01-01T00:00:00Z",
						"operation_state.phase":       "Succeeded",
						"operation_state.started_at":  "2000-01-01T00:00:00Z"}},
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
						"project":                     "default",
						"sync.status":                 "Synced",
						"health.status":               "Healthy",
						"destination.namespace":       "multi-path",
						"source_type":                 "Directory,Kustomize",
						"last_deployed_at":            "2000-01-01T00:00:00Z",
						"operation_present":           "false",
						"operation_state.finished_at": "2000-01-01T00:00:00Z",
						"operation_state.phase":       "Succeeded",
						"operation_state.started_at":  "2000-01-01T00:00:00Z"}},
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
			name:     "resource other than Deployment and Service becomes a k8s_resource Node",
			testdata: []string{"k8s-resource"},
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("Application.argocd", "test-prefix/argocd/k8s-resource"), Properties: props{
						"project":                     "default",
						"sync.status":                 "Synced",
						"health.status":               "Healthy",
						"destination.namespace":       "k8s-resource-test",
						"source_type":                 "Directory",
						"last_deployed_at":            "2000-01-01T00:00:00Z",
						"operation_present":           "false",
						"operation_state.finished_at": "2000-01-01T00:00:00Z",
						"operation_state.phase":       "Succeeded",
						"operation_state.started_at":  "2000-01-01T00:00:00Z"}},
					{ID: nodeID("git_repository", "github.com/argoproj/argocd-example-apps"), Properties: props{
						"url": "https://github.com/argoproj/argocd-example-apps"}},
					{ID: nodeID("k8s_resource", "mock-cluster/core/ConfigMap/k8s-resource-test/appset-demo")},
				},
				Relations: []pluginapi.RelationClaim{
					{Kind: "deployed_from",
						From: nodeID("k8s_resource", "mock-cluster/core/ConfigMap/k8s-resource-test/appset-demo"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["lightweight/app-a"]`}},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/k8s-resource"),
						To:   nodeID("k8s_resource", "mock-cluster/core/ConfigMap/k8s-resource-test/appset-demo")},
					{Kind: "sourced_from",
						From: nodeID("Application.argocd", "test-prefix/argocd/k8s-resource"),
						To:   nodeID("git_repository", "github.com/argoproj/argocd-example-apps"),
						Properties: props{
							"paths": `["lightweight/app-a"]`}},
				},
			},
		},
		{
			name:     "Cluster-scoped resource + non-core group + helm chart source",
			testdata: []string{"cluster-scoped/cluster-rbac"},
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("Application.argocd", "test-prefix/argocd/cluster-rbac"), Properties: props{
						"project":                     "default",
						"sync.status":                 "Synced",
						"health.status":               "Healthy",
						"destination.namespace":       "cluster-rbac",
						"source_type":                 "Helm",
						"last_deployed_at":            "2000-01-01T00:00:00Z",
						"operation_present":           "false",
						"operation_state.finished_at": "2000-01-01T00:00:00Z",
						"operation_state.phase":       "Succeeded",
						"operation_state.started_at":  "2000-01-01T00:00:00Z"}},
					{ID: nodeID("deployment", "mock-cluster/cluster-rbac/cluster-rbac-kube-state-metrics")},
					{ID: nodeID("k8s_resource", "mock-cluster/core/ServiceAccount/cluster-rbac/cluster-rbac-kube-state-metrics")},
					{ID: nodeID("k8s_resource", "mock-cluster/rbac.authorization.k8s.io/ClusterRole/cluster-rbac-kube-state-metrics")},
					{ID: nodeID("k8s_resource", "mock-cluster/rbac.authorization.k8s.io/ClusterRoleBinding/cluster-rbac-kube-state-metrics")},
					{ID: nodeID("service", "mock-cluster/cluster-rbac/cluster-rbac-kube-state-metrics")},
				},
				Relations: []pluginapi.RelationClaim{
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/cluster-rbac"),
						To:   nodeID("deployment", "mock-cluster/cluster-rbac/cluster-rbac-kube-state-metrics")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/cluster-rbac"),
						To:   nodeID("k8s_resource", "mock-cluster/core/ServiceAccount/cluster-rbac/cluster-rbac-kube-state-metrics")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/cluster-rbac"),
						To:   nodeID("k8s_resource", "mock-cluster/rbac.authorization.k8s.io/ClusterRole/cluster-rbac-kube-state-metrics")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/cluster-rbac"),
						To:   nodeID("k8s_resource", "mock-cluster/rbac.authorization.k8s.io/ClusterRoleBinding/cluster-rbac-kube-state-metrics")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/cluster-rbac"),
						To:   nodeID("service", "mock-cluster/cluster-rbac/cluster-rbac-kube-state-metrics")},
				},
			},
		},
		{
			name:     "namespaced resource of a non-core group has the group and namespace segments",
			testdata: []string{"namespaced-rbac"},
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("Application.argocd", "test-prefix/argocd/namespaced-rbac"), Properties: props{
						"project":                     "default",
						"sync.status":                 "Synced",
						"health.status":               "Healthy",
						"destination.namespace":       "namespaced-rbac",
						"source_type":                 "Helm",
						"last_deployed_at":            "2000-01-01T00:00:00Z",
						"operation_present":           "false",
						"operation_state.finished_at": "2000-01-01T00:00:00Z",
						"operation_state.phase":       "Succeeded",
						"operation_state.started_at":  "2000-01-01T00:00:00Z"}},
					{ID: nodeID("deployment", "mock-cluster/namespaced-rbac/namespaced-rbac-kube-state-metrics")},
					{ID: nodeID("k8s_resource", "mock-cluster/core/ServiceAccount/namespaced-rbac/namespaced-rbac-kube-state-metrics")},
					{ID: nodeID("k8s_resource", "mock-cluster/rbac.authorization.k8s.io/Role/namespaced-rbac/namespaced-rbac-kube-state-metrics")},
					{ID: nodeID("k8s_resource", "mock-cluster/rbac.authorization.k8s.io/RoleBinding/namespaced-rbac/namespaced-rbac-kube-state-metrics")},
					{ID: nodeID("service", "mock-cluster/namespaced-rbac/namespaced-rbac-kube-state-metrics")},
				},
				Relations: []pluginapi.RelationClaim{
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/namespaced-rbac"),
						To:   nodeID("deployment", "mock-cluster/namespaced-rbac/namespaced-rbac-kube-state-metrics")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/namespaced-rbac"),
						To:   nodeID("k8s_resource", "mock-cluster/core/ServiceAccount/namespaced-rbac/namespaced-rbac-kube-state-metrics")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/namespaced-rbac"),
						To:   nodeID("k8s_resource", "mock-cluster/rbac.authorization.k8s.io/Role/namespaced-rbac/namespaced-rbac-kube-state-metrics")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/namespaced-rbac"),
						To:   nodeID("k8s_resource", "mock-cluster/rbac.authorization.k8s.io/RoleBinding/namespaced-rbac/namespaced-rbac-kube-state-metrics")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/namespaced-rbac"),
						To:   nodeID("service", "mock-cluster/namespaced-rbac/namespaced-rbac-kube-state-metrics")},
				},
			},
		},
		{
			name:     "Helm chart from an OCI registry (oci:// URL)",
			testdata: []string{"oci/oci-chart"},
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("Application.argocd", "test-prefix/argocd/oci-chart"), Properties: props{
						"project":                     "default",
						"sync.status":                 "Synced",
						"health.status":               "Healthy",
						"destination.namespace":       "oci-chart",
						"source_type":                 "Helm",
						"last_deployed_at":            "2000-01-01T00:00:00Z",
						"operation_present":           "false",
						"operation_state.finished_at": "2000-01-01T00:00:00Z",
						"operation_state.phase":       "Succeeded",
						"operation_state.started_at":  "2000-01-01T00:00:00Z"}},
					{ID: nodeID("deployment", "mock-cluster/oci-chart/oci-chart-kube-state-metrics")},
					{ID: nodeID("k8s_resource", "mock-cluster/core/ServiceAccount/oci-chart/oci-chart-kube-state-metrics")},
					{ID: nodeID("k8s_resource", "mock-cluster/rbac.authorization.k8s.io/ClusterRole/oci-chart-kube-state-metrics")},
					{ID: nodeID("k8s_resource", "mock-cluster/rbac.authorization.k8s.io/ClusterRoleBinding/oci-chart-kube-state-metrics")},
					{ID: nodeID("service", "mock-cluster/oci-chart/oci-chart-kube-state-metrics")},
				},
				Relations: []pluginapi.RelationClaim{
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/oci-chart"),
						To:   nodeID("deployment", "mock-cluster/oci-chart/oci-chart-kube-state-metrics")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/oci-chart"),
						To:   nodeID("k8s_resource", "mock-cluster/core/ServiceAccount/oci-chart/oci-chart-kube-state-metrics")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/oci-chart"),
						To:   nodeID("k8s_resource", "mock-cluster/rbac.authorization.k8s.io/ClusterRole/oci-chart-kube-state-metrics")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/oci-chart"),
						To:   nodeID("k8s_resource", "mock-cluster/rbac.authorization.k8s.io/ClusterRoleBinding/oci-chart-kube-state-metrics")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "test-prefix/argocd/oci-chart"),
						To:   nodeID("service", "mock-cluster/oci-chart/oci-chart-kube-state-metrics")},
				},
			},
		},
		{
			name:     "sync in progress: operation_state is copied, and last_deployed_at is absent without history",
			testdata: []string{"mid-sync"},
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("Application.argocd", "test-prefix/argocd/mid-sync"), Properties: props{
						"project":                     "default",
						"sync.status":                 "OutOfSync",
						"health.status":               "Missing",
						"destination.namespace":       "mid-sync",
						"source_type":                 "Kustomize",
						"operation_present":           "true",
						"operation_state.finished_at": "2000-01-01T00:00:00Z",
						"operation_state.phase":       "Running",
						"operation_state.started_at":  "2000-01-01T00:00:00Z"}},
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

func TestCollectMergesRelationsAcrossApplications(t *testing.T) {
	app := func(name, repoPath string) v1alpha1.Application {
		return v1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "argocd",
				Name:      name,
			},
			Spec: v1alpha1.ApplicationSpec{
				Destination: v1alpha1.ApplicationDestination{
					Name: "in-cluster",
				},
				Source: &v1alpha1.ApplicationSource{
					RepoURL: "https://github.com/shared-org/shared-repo",
					Path:    repoPath,
				},
			},
			Status: v1alpha1.ApplicationStatus{
				Resources: []v1alpha1.ResourceStatus{
					{
						Group:     "apps",
						Kind:      "Deployment",
						Namespace: "shared-ns",
						Name:      "shared-deployment",
					},
				},
			},
		}
	}

	apps := []v1alpha1.Application{
		app("app1", "path/z"),
		app("app2", "path/yyy"),
		app("app3", "path/z"),
	}

	got := New(config{PathPrefix: "prefix", ClusterID: "cluster"}).collect(apps)

	// We're only interested in the "deployed_from" relations.
	var testedRelations []pluginapi.RelationClaim
	for _, r := range got.Relations {
		if r.Kind == "deployed_from" {
			testedRelations = append(testedRelations, r)
		}
	}
	assert.Equal(t, []pluginapi.RelationClaim{
		{Kind: "deployed_from",
			From: nodeID("deployment", "cluster/shared-ns/shared-deployment"),
			To:   nodeID("git_repository", "github.com/shared-org/shared-repo"),
			Properties: props{
				"paths": `["path/yyy","path/z"]`}},
	}, testedRelations)
}
