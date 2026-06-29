package fluxcd

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sunstructured "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	discfake "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/naira-project/naira/catalog/pluginapi"
)

const testClusterID = "test-cluster-uid-1234"

// attrs is a helper alias reducing verbosity of use of map[string]any throughout this test file.
type attrs = map[string]any

// gvrk extends schema.GroupVersionResource with Kind to simplify fake CRDs creation.
type gvrk struct {
	schema.GroupVersionResource
	Kind string
}

func newGVRK(group, version, resource, kind string) gvrk {
	return gvrk{
		GroupVersionResource: schema.GroupVersionResource{
			Group:    group,
			Version:  version,
			Resource: resource,
		},
		Kind: kind,
	}
}

// FluxCD CRD constants used to build mock resources.
var (
	gvrkKustomization = newGVRK("kustomize.toolkit.fluxcd.io", "v1", "kustomizations", "Kustomization")
	gvrkHelmRelease   = newGVRK("helm.toolkit.fluxcd.io", "v2", "helmreleases", "HelmRelease")
	gvrkGitRepository = newGVRK("source.toolkit.fluxcd.io", "v1", "gitrepositories", "GitRepository")
)

func TestCollect(t *testing.T) {
	tests := []struct {
		name    string
		objs    []runtime.Object
		reactor func(*fake.FakeDynamicClient)
		want    pluginapi.IngestionRequest
	}{
		{
			name: "Deployment not managed by FluxCD produces no nodes or relations",
			objs: []runtime.Object{
				namespace("team-a"),
				deployment("team-a", "app"),
			},
			want: pluginapi.IngestionRequest{},
		},
		{
			name: `Kustomization with GitRepository source produces Nodes and "sourced_from" Relation`,
			objs: []runtime.Object{
				namespace("flux-system"),
				gitRepository("flux-system", "my-repo", "https://github.com/example/repo"),
				kustomization("flux-system", "my-app",
					sourceRef("GitRepository", "", "my-repo")),
			},
			want: pluginapi.IngestionRequest{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("Kustomization.fluxcd", "flux-system/my-app")},
					{ID: nodeID("git_repository", "flux-system/my-repo"),
						Properties: pluginapi.PropertyMap{"url": "https://github.com/example/repo"}},
				},
				Relations: []pluginapi.RelationClaim{
					{Kind: "sourced_from",
						From: nodeID("Kustomization.fluxcd", "flux-system/my-app"),
						To:   nodeID("git_repository", "flux-system/my-repo")},
				},
			},
		},
		{
			name: `HelmRelease with GitRepository source produces Nodes and "sourced_from" Relation`,
			objs: []runtime.Object{
				namespace("flux-system"),
				gitRepository("flux-system", "my-repo", "https://github.com/example/repo"),
				helmRelease("flux-system", "my-chart",
					helmSourceRef("GitRepository", "", "my-repo")),
			},
			want: pluginapi.IngestionRequest{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("HelmChart.fluxcd", "flux-system/my-chart")},
					{ID: nodeID("git_repository", "flux-system/my-repo"),
						Properties: pluginapi.PropertyMap{"url": "https://github.com/example/repo"}},
				},
				Relations: []pluginapi.RelationClaim{
					{Kind: "sourced_from",
						From: nodeID("HelmChart.fluxcd", "flux-system/my-chart"),
						To:   nodeID("git_repository", "flux-system/my-repo")},
				},
			},
		},
		{
			name: `Deployment with Kustomization label produces "deployed_from" and (reverse) "describes" Relations`,
			objs: []runtime.Object{
				namespace("flux-system"),
				gitRepository("flux-system", "my-repo", "https://github.com/example/repo"),
				kustomization("flux-system", "my-app",
					sourceRef("GitRepository", "", "my-repo")),
				namespace("team-a"),
				deployment("team-a", "app",
					kustLabel("flux-system", "my-app")),
			},
			want: pluginapi.IngestionRequest{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("Kustomization.fluxcd", "flux-system/my-app")},
					{ID: nodeID("deployment", "team-a/app")},
					{ID: nodeID("git_repository", "flux-system/my-repo"),
						Properties: pluginapi.PropertyMap{"url": "https://github.com/example/repo"}},
				},
				Relations: []pluginapi.RelationClaim{
					{Kind: "deployed_from",
						From: nodeID("deployment", "team-a/app"),
						To:   nodeID("git_repository", "flux-system/my-repo")},
					{Kind: "describes",
						From: nodeID("Kustomization.fluxcd", "flux-system/my-app"),
						To:   nodeID("deployment", "team-a/app")},
					{Kind: "sourced_from",
						From: nodeID("Kustomization.fluxcd", "flux-system/my-app"),
						To:   nodeID("git_repository", "flux-system/my-repo")},
				},
			},
		},
		{
			name: `Deployment with HelmRelease label produces "deployed_from" and (reverse) "describes" Relations`,
			objs: []runtime.Object{
				namespace("flux-system"),
				gitRepository("flux-system", "my-repo", "https://github.com/example/repo"),
				helmRelease("flux-system", "my-chart",
					helmSourceRef("GitRepository", "", "my-repo")),
				namespace("team-a"),
				deployment("team-a", "app",
					helmLabel("flux-system", "my-chart")),
			},
			want: pluginapi.IngestionRequest{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("HelmChart.fluxcd", "flux-system/my-chart")},
					{ID: nodeID("deployment", "team-a/app")},
					{ID: nodeID("git_repository", "flux-system/my-repo"),
						Properties: pluginapi.PropertyMap{"url": "https://github.com/example/repo"}},
				},
				Relations: []pluginapi.RelationClaim{
					{Kind: "deployed_from",
						From: nodeID("deployment", "team-a/app"),
						To:   nodeID("git_repository", "flux-system/my-repo")},
					{Kind: "describes",
						From: nodeID("HelmChart.fluxcd", "flux-system/my-chart"),
						To:   nodeID("deployment", "team-a/app")},
					{Kind: "sourced_from",
						From: nodeID("HelmChart.fluxcd", "flux-system/my-chart"),
						To:   nodeID("git_repository", "flux-system/my-repo")},
				},
			},
		},
		{
			name: "Kustomization with cross-namespace GitRepository source detected from explicit sourceRef namespace",
			objs: []runtime.Object{
				namespace("flux-system"),
				gitRepository("flux-system", "my-repo", "https://github.com/example/repo"),
				namespace("team-a"),
				kustomization("team-a", "my-app",
					sourceRef("GitRepository", "flux-system", "my-repo")),
			},
			want: pluginapi.IngestionRequest{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("Kustomization.fluxcd", "team-a/my-app")},
					{ID: nodeID("git_repository", "flux-system/my-repo"),
						Properties: pluginapi.PropertyMap{"url": "https://github.com/example/repo"}},
				},
				Relations: []pluginapi.RelationClaim{
					{Kind: "sourced_from",
						From: nodeID("Kustomization.fluxcd", "team-a/my-app"),
						To:   nodeID("git_repository", "flux-system/my-repo")},
				},
			},
		},
		{
			name: "one Kustomization can be linked to multiple Deployments",
			objs: []runtime.Object{
				namespace("flux-system"),
				gitRepository("flux-system", "my-repo", "https://github.com/example/repo"),
				kustomization("flux-system", "my-app",
					sourceRef("GitRepository", "", "my-repo")),
				namespace("team-a"),
				deployment("team-a", "depl1", kustLabel("flux-system", "my-app")),
				deployment("team-a", "depl2", kustLabel("flux-system", "my-app")),
			},
			want: pluginapi.IngestionRequest{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("Kustomization.fluxcd", "flux-system/my-app")},
					{ID: nodeID("deployment", "team-a/depl1")},
					{ID: nodeID("deployment", "team-a/depl2")},
					{ID: nodeID("git_repository", "flux-system/my-repo"),
						Properties: pluginapi.PropertyMap{"url": "https://github.com/example/repo"}},
				},
				Relations: []pluginapi.RelationClaim{
					{Kind: "deployed_from",
						From: nodeID("deployment", "team-a/depl1"),
						To:   nodeID("git_repository", "flux-system/my-repo")},
					{Kind: "deployed_from",
						From: nodeID("deployment", "team-a/depl2"),
						To:   nodeID("git_repository", "flux-system/my-repo")},
					{Kind: "describes",
						From: nodeID("Kustomization.fluxcd", "flux-system/my-app"),
						To:   nodeID("deployment", "team-a/depl1")},
					{Kind: "describes",
						From: nodeID("Kustomization.fluxcd", "flux-system/my-app"),
						To:   nodeID("deployment", "team-a/depl2")},
					{Kind: "sourced_from",
						From: nodeID("Kustomization.fluxcd", "flux-system/my-app"),
						To:   nodeID("git_repository", "flux-system/my-repo")},
				},
			},
		},
		{
			// TODO: could be improved in the future to also handle Buckets etc.
			name: `Kustomization with non-GitRepository source produces a Node, but no "sourced_from" relation`,
			objs: []runtime.Object{
				namespace("flux-system"),
				kustomization("flux-system", "my-app",
					sourceRef("Bucket", "", "my-bucket")),
			},
			want: pluginapi.IngestionRequest{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("Kustomization.fluxcd", "flux-system/my-app")},
				},
			},
		},
		{
			// TODO: could be improved in the future to also handle Buckets etc.
			name: `Deployment from Kustomization with non-GitRepository source produces Nodes, but no "sourced_from" relation`,
			objs: []runtime.Object{
				namespace("flux-system"),
				kustomization("flux-system", "my-app",
					sourceRef("Bucket", "", "my-bucket")),
				namespace("team-a"),
				deployment("team-a", "app",
					kustLabel("flux-system", "my-app")),
			},
			want: pluginapi.IngestionRequest{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("Kustomization.fluxcd", "flux-system/my-app")},
					{ID: nodeID("deployment", "team-a/app")},
				},
				Relations: []pluginapi.RelationClaim{
					{Kind: "describes",
						From: nodeID("Kustomization.fluxcd", "flux-system/my-app"),
						To:   nodeID("deployment", "team-a/app")},
				},
			},
		},
		{
			name: "in case of error listing Deployments in one namespace, other namespaces are still processed",
			objs: []runtime.Object{
				namespace("flux-system"),
				gitRepository("flux-system", "my-repo", "https://github.com/example/repo"),
				kustomization("flux-system", "my-app",
					sourceRef("GitRepository", "", "my-repo")),
				namespace("secret-ns"),
				deployment("secret-ns", "secret-app",
					kustLabel("flux-system", "my-app")),
				namespace("team-a"),
				deployment("team-a", "app",
					kustLabel("flux-system", "my-app")),
			},
			reactor: func(client *fake.FakeDynamicClient) {
				client.PrependReactor("list", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
					if action.(k8stesting.ListActionImpl).Namespace == "secret-ns" {
						return true, nil, assert.AnError
					}
					return false, nil, nil
				})
			},
			want: pluginapi.IngestionRequest{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("Kustomization.fluxcd", "flux-system/my-app")},
					{ID: nodeID("deployment", "team-a/app")},
					{ID: nodeID("git_repository", "flux-system/my-repo"),
						Properties: pluginapi.PropertyMap{"url": "https://github.com/example/repo"}},
				},
				Relations: []pluginapi.RelationClaim{
					{Kind: "deployed_from",
						From: nodeID("deployment", "team-a/app"),
						To:   nodeID("git_repository", "flux-system/my-repo")},
					{Kind: "describes",
						From: nodeID("Kustomization.fluxcd", "flux-system/my-app"),
						To:   nodeID("deployment", "team-a/app")},
					{Kind: "sourced_from",
						From: nodeID("Kustomization.fluxcd", "flux-system/my-app"),
						To:   nodeID("git_repository", "flux-system/my-repo")},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dynClient, disc := fakeClients(tt.objs...)
			if tt.reactor != nil {
				tt.reactor(dynClient)
			}
			result, err := New(Config{}).collect(context.Background(), disc, dynClient)
			require.NoError(t, err)
			assert.Equal(t, tt.want, sortedByIDs(result))
		})
	}
}

// fakeClients returns a fake dynamic client pre-loaded with objs, and a fake
// discovery client that knows about the FluxCD CRD resource types.
func fakeClients(objs ...runtime.Object) (*fake.FakeDynamicClient, *discfake.FakeDiscovery) {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)

	// Register UnstructuredList for each FluxCD GVR so the fake dynamic client
	// can handle List calls without panicking.
	// Also, build mock resources API surface for the CRDs.
	gvrToListKind := map[schema.GroupVersionResource]string{}
	resources := []*metav1.APIResourceList{}
	for _, g := range []gvrk{gvrkKustomization, gvrkHelmRelease, gvrkGitRepository} {
		listKind := g.Kind + "List"
		s.AddKnownTypeWithName(g.GroupVersion().WithKind(listKind), &k8sunstructured.UnstructuredList{})
		gvrToListKind[g.GroupVersionResource] = listKind

		resources = append(resources, &metav1.APIResourceList{
			GroupVersion: g.GroupVersion().String(),
			APIResources: []metav1.APIResource{
				{Name: g.Resource, Kind: g.Kind, Verbs: []string{"list", "get"}},
			},
		})
	}

	ns := namespace("kube-system")
	ns.ObjectMeta.UID = testClusterID
	objs = append(objs, ns)

	dynClient := fake.NewSimpleDynamicClientWithCustomListKinds(s, gvrToListKind, objs...)
	disc := &discfake.FakeDiscovery{Fake: &dynClient.Fake}
	disc.Resources = resources
	return dynClient, disc
}

func sortedByIDs(r pluginapi.IngestionRequest) pluginapi.IngestionRequest {
	lessID := func(a, b pluginapi.NodeID) bool {
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Path < b.Path
	}
	sort.Slice(r.Nodes, func(i, j int) bool {
		return lessID(r.Nodes[i].ID, r.Nodes[j].ID)
	})
	sort.Slice(r.Relations, func(i, j int) bool {
		a, b := r.Relations[i], r.Relations[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.From != b.From {
			return lessID(a.From, b.From)
		}
		return lessID(a.To, b.To)
	})
	return r
}

func nodeID(kind, path string) pluginapi.NodeID {
	return pluginapi.NodeID{Kind: kind, Path: testClusterID + "/" + path}
}

func namespace(name string) *corev1.Namespace {
	uid := types.UID(fmt.Sprintf("random-ns-uid-%s-%d", name, rand.Uint64()))
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: uid},
	}
}

func deployment(ns, name string, labels ...map[string]string) *appsv1.Deployment {
	merged := map[string]string{}
	for _, l := range labels {
		for k, v := range l {
			merged[k] = v
		}
	}
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
	}
	if len(merged) > 0 {
		d.Labels = merged
	}
	return d
}

func kustLabel(ns, name string) map[string]string {
	return map[string]string{
		labelKustName: name,
		labelKustNs:   ns,
	}
}

func helmLabel(ns, name string) map[string]string {
	return map[string]string{
		labelHelmName: name,
		labelHelmNs:   ns,
	}
}

func gitRepository(ns, name, url string) runtime.Object {
	return newUnstructured(gvrkGitRepository, ns, name, attrs{"url": url})
}

func kustomization(ns, name string, spec attrs) runtime.Object {
	return newUnstructured(gvrkKustomization, ns, name, spec)
}

func helmRelease(ns, name string, spec attrs) runtime.Object {
	return newUnstructured(gvrkHelmRelease, ns, name, spec)
}

func newUnstructured(g gvrk, ns, name string, spec attrs) runtime.Object {
	return &k8sunstructured.Unstructured{Object: attrs{
		"apiVersion": g.GroupVersion().String(),
		"kind":       g.Kind,
		"metadata": attrs{
			"name":      name,
			"namespace": ns,
		},
		"spec": spec,
	}}
}

func helmSourceRef(kind, ns, name string) attrs {
	return attrs{"chart": attrs{"spec": sourceRef(kind, ns, name)}}
}

func sourceRef(kind, ns, name string) attrs {
	ref := attrs{"kind": kind, "name": name}
	if ns != "" {
		ref["namespace"] = ns
	}
	return attrs{"sourceRef": ref}
}
