package main

import (
	"encoding/json"
	"sort"
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

const testClusterID = "c1"

func TestCollect(t *testing.T) {
	tests := []struct {
		name string
		app  string
		want pluginapi.CollectResponse
	}{
		{
			name: "managed resources become nodes, describes and deployed_from relations",
			app: `{
				"apiVersion": "argoproj.io/v1alpha1", "kind": "Application",
				"metadata": {"name": "demo", "namespace": "argocd"},
				"spec": {
					"project": "default",
					"destination": {"server": "https://kubernetes.default.svc", "namespace": "default"},
					"source": {"repoURL": "https://github.com/acme/config.git"}
				},
				"status": {
					"sync": {"status": "Synced"},
					"health": {"status": "Healthy"},
					"resources": [
						{"group": "apps", "kind": "Deployment", "name": "web", "namespace": "default"},
						{"kind": "Service", "name": "web", "namespace": "default"},
						{"kind": "ConfigMap", "name": "cfg", "namespace": "default"},
						{"kind": "PersistentVolume", "name": "pv"},
						{"group": "argoproj.io", "kind": "Application", "name": "child", "namespace": "argocd"}
					]
				}
			}`,
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: pluginapi.NodeID{Kind: "Application.argocd", Path: "c1/argocd/child"}},
					{
						ID: pluginapi.NodeID{Kind: "Application.argocd", Path: "c1/argocd/demo"},
						Properties: pluginapi.PropertyMap{
							"project":               "default",
							"sync_status":           "Synced",
							"health_status":         "Healthy",
							"destination_namespace": "default",
						},
					},
					{ID: pluginapi.NodeID{Kind: "deployment", Path: "c1/default/web"}},
					{
						ID:         pluginapi.NodeID{Kind: "git_repository", Path: "github.com/acme/config"},
						Properties: pluginapi.PropertyMap{"url": "https://github.com/acme/config.git"},
					},
					{ID: pluginapi.NodeID{Kind: "k8s_resource", Path: "c1/core/ConfigMap/default/cfg"}},
					{ID: pluginapi.NodeID{Kind: "k8s_resource", Path: "c1/core/PersistentVolume/pv"}},
					{ID: pluginapi.NodeID{Kind: "service", Path: "c1/default/web"}},
				},
				Relations: []pluginapi.RelationClaim{
					{
						Kind: "deployed_from",
						From: pluginapi.NodeID{Kind: "deployment", Path: "c1/default/web"},
						To:   pluginapi.NodeID{Kind: "git_repository", Path: "github.com/acme/config"},
					},
					{
						Kind: "describes",
						From: pluginapi.NodeID{Kind: "Application.argocd", Path: "c1/argocd/demo"},
						To:   pluginapi.NodeID{Kind: "Application.argocd", Path: "c1/argocd/child"},
					},
					{
						Kind: "describes",
						From: pluginapi.NodeID{Kind: "Application.argocd", Path: "c1/argocd/demo"},
						To:   pluginapi.NodeID{Kind: "deployment", Path: "c1/default/web"},
					},
					{
						Kind: "describes",
						From: pluginapi.NodeID{Kind: "Application.argocd", Path: "c1/argocd/demo"},
						To:   pluginapi.NodeID{Kind: "k8s_resource", Path: "c1/core/ConfigMap/default/cfg"},
					},
					{
						Kind: "describes",
						From: pluginapi.NodeID{Kind: "Application.argocd", Path: "c1/argocd/demo"},
						To:   pluginapi.NodeID{Kind: "k8s_resource", Path: "c1/core/PersistentVolume/pv"},
					},
					{
						Kind: "describes",
						From: pluginapi.NodeID{Kind: "Application.argocd", Path: "c1/argocd/demo"},
						To:   pluginapi.NodeID{Kind: "service", Path: "c1/default/web"},
					},
					{
						Kind: "sourced_from",
						From: pluginapi.NodeID{Kind: "Application.argocd", Path: "c1/argocd/demo"},
						To:   pluginapi.NodeID{Kind: "git_repository", Path: "github.com/acme/config"},
					},
				},
			},
		},
		{
			name: "Application targeting another cluster is skipped",
			app: `{
				"apiVersion": "argoproj.io/v1alpha1", "kind": "Application",
				"metadata": {"name": "remote", "namespace": "argocd"},
				"spec": {"destination": {"server": "https://other.example.com"}},
				"status": {"resources": [{"group": "apps", "kind": "Deployment", "name": "web", "namespace": "default"}]}
			}`,
			want: pluginapi.CollectResponse{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var obj map[string]any
			require.NoError(t, json.Unmarshal([]byte(tt.app), &obj))

			gvr := schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}
			s := runtime.NewScheme()
			_ = corev1.AddToScheme(s)
			s.AddKnownTypeWithName(gvr.GroupVersion().WithKind("ApplicationList"), &unstructured.UnstructuredList{})
			kubeSystem := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system", UID: testClusterID}}
			argocd := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "argocd", UID: "argocd-uid"}}
			dyn := fake.NewSimpleDynamicClientWithCustomListKinds(s,
				map[schema.GroupVersionResource]string{gvr: "ApplicationList"},
				kubeSystem, argocd, &unstructured.Unstructured{Object: obj})
			disc := &discfake.FakeDiscovery{Fake: &dyn.Fake}
			disc.Resources = []*metav1.APIResourceList{{
				GroupVersion: gvr.GroupVersion().String(),
				APIResources: []metav1.APIResource{{Name: "applications", Kind: "Application", Verbs: []string{"list"}}},
			}}

			got, err := New(config{}).collect(t.Context(), disc, dyn)
			require.NoError(t, err)
			sort.Slice(got.Nodes, func(i, j int) bool {
				a, b := got.Nodes[i].ID, got.Nodes[j].ID
				return a.Kind+"|"+a.Path < b.Kind+"|"+b.Path
			})
			sort.Slice(got.Relations, func(i, j int) bool {
				a, b := got.Relations[i], got.Relations[j]
				return a.Kind+a.From.Path+a.To.Kind+a.To.Path < b.Kind+b.From.Path+b.To.Kind+b.To.Path
			})
			assert.Equal(t, tt.want, got)
		})
	}
}
