package depl_calls_svc

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sort"
	"testing"

	"github.com/naira-project/naira/catalog/pluginapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

const testClusterID = "test-cluster-uid-1234"

func TestCollect(t *testing.T) {
	tests := []struct {
		name    string
		objs    []runtime.Object
		reactor func(*fake.FakeDynamicClient)
		want    pluginapi.CollectResponse
	}{
		{
			name: `"simple happy path": Deployment referencing same-namespace Service by name produces a "calls" Relation`,
			objs: []runtime.Object{
				namespace("team-a"),
				service("team-a", "payments"),
				deployment("team-a", "checkout",
					env("PAYMENTS_URL", "http://payments/api")),
			},
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("deployment", "team-a/checkout")},
					{ID: nodeID("service", "team-a/payments")},
				},
				Relations: []pluginapi.RelationClaim{
					{
						From:       nodeID("deployment", "team-a/checkout"),
						Kind:       "calls",
						To:         nodeID("service", "team-a/payments"),
						Properties: pluginapi.PropertyMap{"env": "PAYMENTS_URL"},
					},
				},
			},
		},
		{
			name: `Deployment referencing cross-namespace Service via "$svc.$namespace" form produces "calls" Relation`,
			objs: []runtime.Object{
				namespace("infra"),
				service("infra", "redis"),
				namespace("team-a"),
				deployment("team-a", "worker",
					env("CACHE_URL", "redis.infra:6379")),
			},
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("deployment", "team-a/worker")},
					{ID: nodeID("service", "infra/redis")},
				},
				Relations: []pluginapi.RelationClaim{
					{
						From:       nodeID("deployment", "team-a/worker"),
						Kind:       "calls",
						To:         nodeID("service", "infra/redis"),
						Properties: pluginapi.PropertyMap{"env": "CACHE_URL"},
					},
				},
			},
		},
		{
			name: "Deployment with no Env match produces no Relation claims",
			objs: []runtime.Object{
				namespace("team-a"),
				service("team-a", "payments"),
				deployment("team-a", "checkout",
					env("SOME_URL", "http://external.example.com/api")),
			},
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("deployment", "team-a/checkout")},
					{ID: nodeID("service", "team-a/payments")},
				},
			},
		},
		{
			name: `Deployment can "call" multiple Services, in same and different namespaces`,
			objs: []runtime.Object{
				namespace("team-a"),
				service("team-a", "svc1"),
				namespace("team-b"),
				service("team-b", "svc2"),
				service("team-b", "svc3"),
				deployment("team-b", "depl",
					env("SVC1_URL", "http://svc1.team-a/api"),
					env("SVC2_URL", "http://svc2.team-b/api"),
					env("SVC3_URL", "http://svc3/api")),
			},
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("deployment", "team-b/depl")},
					{ID: nodeID("service", "team-a/svc1")},
					{ID: nodeID("service", "team-b/svc2")},
					{ID: nodeID("service", "team-b/svc3")},
				},
				Relations: []pluginapi.RelationClaim{
					{
						From:       nodeID("deployment", "team-b/depl"),
						Kind:       "calls",
						To:         nodeID("service", "team-a/svc1"),
						Properties: pluginapi.PropertyMap{"env": "SVC1_URL"},
					},
					{
						From:       nodeID("deployment", "team-b/depl"),
						Kind:       "calls",
						To:         nodeID("service", "team-b/svc2"),
						Properties: pluginapi.PropertyMap{"env": "SVC2_URL"},
					},
					{
						From:       nodeID("deployment", "team-b/depl"),
						Kind:       "calls",
						To:         nodeID("service", "team-b/svc3"),
						Properties: pluginapi.PropertyMap{"env": "SVC3_URL"},
					},
				},
			},
		},
		{
			name: `Service in different namespace cannot be referenced without namespace suffix`,
			objs: []runtime.Object{
				namespace("team-a"),
				service("team-a", "svc"),
				namespace("team-b"),
				deployment("team-b", "depl",
					env("SVC_URL", "http://svc/api")),
			},
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("deployment", "team-b/depl")},
					{ID: nodeID("service", "team-a/svc")},
				},
			},
		},
		{
			name: `same Service can be referenced by multiple Deployments, producing multiple "calls" Relations`,
			objs: []runtime.Object{
				namespace("team-a"),
				service("team-a", "svc"),
				deployment("team-a", "depl1",
					env("SVC_URL", "http://svc/api")),
				namespace("team-b"),
				deployment("team-b", "depl2",
					env("SVC_URL", "http://svc.team-a/api")),
			},
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("deployment", "team-a/depl1")},
					{ID: nodeID("deployment", "team-b/depl2")},
					{ID: nodeID("service", "team-a/svc")},
				},
				Relations: []pluginapi.RelationClaim{
					{
						From:       nodeID("deployment", "team-a/depl1"),
						Kind:       "calls",
						To:         nodeID("service", "team-a/svc"),
						Properties: pluginapi.PropertyMap{"env": "SVC_URL"},
					},
					{
						From:       nodeID("deployment", "team-b/depl2"),
						Kind:       "calls",
						To:         nodeID("service", "team-a/svc"),
						Properties: pluginapi.PropertyMap{"env": "SVC_URL"},
					},
				},
			},
		},
		{
			name: "in case of error when listing Services in one namespace, other namespaces are still processed",
			objs: []runtime.Object{
				namespace("secret-namespace"),
				service("secret-namespace", "secret-service"),
				namespace("team-b"),
				service("team-b", "payments"),
				deployment("team-b", "checkout",
					env("PAYMENTS_URL", "http://payments/api"),
					env("SECRET_SERVICE_URL", "http://secret-service.secret-namespace/api")),
			},
			reactor: func(client *fake.FakeDynamicClient) {
				client.PrependReactor("list", "services", func(action k8stesting.Action) (bool, runtime.Object, error) {
					if action.(k8stesting.ListActionImpl).Namespace == "secret-namespace" {
						return true, nil, assert.AnError
					}
					return false, nil, nil
				})
			},
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("deployment", "team-b/checkout")},
					{ID: nodeID("service", "team-b/payments")},
				},
				Relations: []pluginapi.RelationClaim{
					{
						From:       nodeID("deployment", "team-b/checkout"),
						Kind:       "calls",
						To:         nodeID("service", "team-b/payments"),
						Properties: pluginapi.PropertyMap{"env": "PAYMENTS_URL"},
					},
				},
			},
		},
		{
			name: "in case of error when listing Deployments in one namespace, other namespaces are still processed",
			objs: []runtime.Object{
				namespace("secret-namespace"),
				deployment("secret-namespace", "secret-deployment"),
				namespace("team-b"),
				service("team-b", "payments"),
				deployment("team-b", "checkout",
					env("PAYMENTS_URL", "http://payments/api")),
			},
			reactor: func(client *fake.FakeDynamicClient) {
				client.PrependReactor("list", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
					if action.(k8stesting.ListActionImpl).Namespace == "secret-namespace" {
						return true, nil, assert.AnError
					}
					return false, nil, nil
				})
			},
			want: pluginapi.CollectResponse{
				Nodes: []pluginapi.NodeClaim{
					{ID: nodeID("deployment", "team-b/checkout")},
					{ID: nodeID("service", "team-b/payments")},
				},
				Relations: []pluginapi.RelationClaim{
					{
						From:       nodeID("deployment", "team-b/checkout"),
						Kind:       "calls",
						To:         nodeID("service", "team-b/payments"),
						Properties: pluginapi.PropertyMap{"env": "PAYMENTS_URL"},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fakeDynamicClient(tt.objs...)
			if tt.reactor != nil {
				tt.reactor(client)
			}
			result, err := New(Config{}).collect(context.Background(), client)
			require.NoError(t, err)
			assert.Equal(t, tt.want, sortedByIDs(result))
		})
	}
}

func sortedByIDs(r pluginapi.CollectResponse) pluginapi.CollectResponse {
	lessID := func(a, b pluginapi.NodeID) bool {
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Path < b.Path
	}
	sort.Slice(r.Nodes, func(i, j int) bool {
		a, b := r.Nodes[i], r.Nodes[j]
		return lessID(a.ID, b.ID)
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

func fakeDynamicClient(objs ...runtime.Object) *fake.FakeDynamicClient {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)

	// The `kube-system` namespace should be always present in k8s cluster; the
	// plugin fetches its UID to use as the cluster ID, so we need to ensure it
	// exists in the fake cluster.
	ns := namespace("kube-system")
	ns.ObjectMeta.UID = testClusterID
	objs = append(objs, ns)

	return fake.NewSimpleDynamicClient(s, objs...)
}

func namespace(name string) *corev1.Namespace {
	uid := types.UID(fmt.Sprintf("random-ns-uid-%s-%d", name, rand.Uint64()))
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name, UID: uid},
	}
}

func service(ns, name string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
	}
}

func deployment(ns, name string, envVars ...corev1.EnvVar) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Env: envVars},
					},
				},
			},
		},
	}
}

func env(name, value string) corev1.EnvVar {
	return corev1.EnvVar{Name: name, Value: value}
}

func nodeID(kind, path string) pluginapi.NodeID {
	return pluginapi.NodeID{Kind: kind, Path: testClusterID + "/" + path}
}
