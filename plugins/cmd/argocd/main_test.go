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
		apps []string
		want pluginapi.CollectResponse
	}{
		{
			name: "managed resources become nodes, describes and deployed_from relations",
			apps: []string{`{
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
			}`},
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("Application.argocd", "c1/argocd/child")},
					{ID: nodeID("Application.argocd", "c1/argocd/demo"), Properties: props{
						"project":               "default",
						"sync_status":           "Synced",
						"health_status":         "Healthy",
						"destination_namespace": "default"}},
					{ID: nodeID("deployment", "c1/default/web")},
					{ID: nodeID("git_repository", "github.com/acme/config"), Properties: props{
						"url": "https://github.com/acme/config.git"}},
					{ID: nodeID("k8s_resource", "c1/core/ConfigMap/default/cfg")},
					{ID: nodeID("k8s_resource", "c1/core/PersistentVolume/pv")},
					{ID: nodeID("service", "c1/default/web")},
				},
				Relations: []pluginapi.RelationClaim{
					{Kind: "deployed_from",
						From: nodeID("deployment", "c1/default/web"),
						To:   nodeID("git_repository", "github.com/acme/config")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "c1/argocd/demo"),
						To:   nodeID("Application.argocd", "c1/argocd/child")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "c1/argocd/demo"),
						To:   nodeID("deployment", "c1/default/web")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "c1/argocd/demo"),
						To:   nodeID("k8s_resource", "c1/core/ConfigMap/default/cfg")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "c1/argocd/demo"),
						To:   nodeID("k8s_resource", "c1/core/PersistentVolume/pv")},
					{Kind: "describes",
						From: nodeID("Application.argocd", "c1/argocd/demo"),
						To:   nodeID("service", "c1/default/web")},
					{Kind: "sourced_from",
						From: nodeID("Application.argocd", "c1/argocd/demo"),
						To:   nodeID("git_repository", "github.com/acme/config")},
				},
			},
		},
		{
			name: "Application targeting another cluster is skipped",
			apps: []string{`{
				"apiVersion": "argoproj.io/v1alpha1", "kind": "Application",
				"metadata": {"name": "remote", "namespace": "argocd"},
				"spec": {"destination": {"server": "https://other.example.com"}},
				"status": {"resources": [{"group": "apps", "kind": "Deployment", "name": "web", "namespace": "default"}]}
			}`},
			want: pluginapi.CollectResponse{},
		},
		{
			name: "child Applications keep their properties regardless of processing order",
			apps: []string{
				`{
					"apiVersion": "argoproj.io/v1alpha1", "kind": "Application",
					"metadata": {"name": "a-parent", "namespace": "argocd"},
					"spec": {"destination": {"server": "https://kubernetes.default.svc"}},
					"status": {"resources": [
						{"group": "argoproj.io", "kind": "Application", "name": "z-child", "namespace": "argocd"},
						{"group": "argoproj.io", "kind": "Application", "name": "remote", "namespace": "argocd"}
					]}
				}`,
				`{
					"apiVersion": "argoproj.io/v1alpha1", "kind": "Application",
					"metadata": {"name": "z-child", "namespace": "argocd"},
					"spec": {"project": "p", "destination": {"server": "https://kubernetes.default.svc"}},
					"status": {"sync": {"status": "Synced"}, "health": {"status": "Healthy"}}
				}`,
				`{
					"apiVersion": "argoproj.io/v1alpha1", "kind": "Application",
					"metadata": {"name": "remote", "namespace": "argocd"},
					"spec": {"project": "p", "destination": {"server": "https://other.example.com"}},
					"status": {"sync": {"status": "Synced"}}
				}`,
			},
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("Application.argocd", "c1/argocd/a-parent"), Properties: props{
						"project":               "",
						"sync_status":           "",
						"health_status":         "",
						"destination_namespace": ""}},
					{ID: nodeID("Application.argocd", "c1/argocd/remote")},
					{ID: nodeID("Application.argocd", "c1/argocd/z-child"), Properties: props{
						"project":               "p",
						"sync_status":           "Synced",
						"health_status":         "Healthy",
						"destination_namespace": ""}},
				},
				Relations: []pluginapi.RelationClaim{
					{Kind: "describes",
						From: nodeID("Application.argocd", "c1/argocd/a-parent"),
						To:   nodeID("Application.argocd", "c1/argocd/remote"),
					},
					{Kind: "describes",
						From: nodeID("Application.argocd", "c1/argocd/a-parent"),
						To:   nodeID("Application.argocd", "c1/argocd/z-child"),
					},
				},
			},
		},
	}
	kubeSystem := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system", UID: testClusterID}}
	argocd := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "argocd", UID: "argocd-uid"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := []runtime.Object{kubeSystem, argocd}
			for _, app := range tt.apps {
				var obj map[string]any
				require.NoError(t, json.Unmarshal([]byte(app), &obj))
				objs = append(objs, &unstructured.Unstructured{Object: obj})
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

func nodeID(kind, path string) pluginapi.NodeID {
	return pluginapi.NodeID{Kind: kind, Path: path}
}

type props = pluginapi.PropertyMap
