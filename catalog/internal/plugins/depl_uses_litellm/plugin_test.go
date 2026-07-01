package depl_uses_litellm

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go-simpler.org/env"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

const (
	testClusterID = "test-cluster-uid-1234"
	testAPIKey    = "sk-123456789_123456789_12" // should match default Config.APIKeyRegexp
)

var defaultKeyRegexp = compileDefaultAPIKeyRegexp()

// attrs reduces verbosity when building unstructured objects inline.
type attrs = map[string]any
type stringmap = map[string]string

func TestFindDeploymentsWithMatchingSecrets(t *testing.T) {
	testKeyRegexp := regexp.MustCompile(`^sk-.{3}$`)
	sampleKey := "sk-AAA"
	assert.True(t, testKeyRegexp.MatchString(sampleKey), "testKeyRegexp should match sampleKey")

	tests := []struct {
		name       string
		namespaces []string
		objs       []runtime.Object
		reactor    func(*fake.FakeDynamicClient)
		want       []deploymentWithSecret
	}{
		{
			name:       "Various Secret locations are detected",
			namespaces: []string{"team-a"},
			objs: []runtime.Object{
				namespace("team-a"),
				deployment("team-a", "app",
					withEnvFrom(0, "secret-1"),
					withSecretKeyRef(0, "SOME_ENV_VAR_NAME", "secret-2", "CCC"),
					withSecretVolume("secret-3"),
					withEnvFrom(1, "secret-4"),
					withSecretKeyRef(1, "SOME_ENV_VAR_NAME", "secret-5", "GGG"),
				),
				secret("team-a", "secret-1", stringmap{
					"AAA": "sk-AAA",
					"BBB": "sk-BBB",
				}),
				secret("team-a", "secret-2", stringmap{
					"CCC":    "sk-CCC",
					"UNUSED": "sk-BAD",
				}),
				secret("team-a", "secret-3", stringmap{
					"DDD": "sk-DDD",
					"EEE": "sk-EEE",
				}),
				secret("team-a", "secret-4", stringmap{
					"FFF": "sk-FFF",
				}),
				secret("team-a", "secret-5", stringmap{
					"GGG": "sk-GGG",
				}),
			},
			want: []deploymentWithSecret{
				{namespace: "team-a", deployment: "app", secret: "sk-AAA"},
				{namespace: "team-a", deployment: "app", secret: "sk-BAD"}, // FIXME: should not be here?
				{namespace: "team-a", deployment: "app", secret: "sk-BBB"},
				{namespace: "team-a", deployment: "app", secret: "sk-CCC"},
				{namespace: "team-a", deployment: "app", secret: "sk-DDD"},
				{namespace: "team-a", deployment: "app", secret: "sk-EEE"},
				{namespace: "team-a", deployment: "app", secret: "sk-FFF"},
				{namespace: "team-a", deployment: "app", secret: "sk-GGG"},
			},
		},
		{
			name:       "Secret with no matching value produces no result",
			namespaces: []string{"team-a"},
			objs: []runtime.Object{
				namespace("team-a"),
				secret("team-a", "secret-1", stringmap{"DB_PASS": "not-a-litellm-key"}),
				deployment("team-a", "app", withEnvFrom(0, "secret-1")),
			},
			want: nil,
		},
		{
			name:       "Deployment with no secret references produces no result",
			namespaces: []string{"team-a"},
			objs: []runtime.Object{
				namespace("team-a"),
				deployment("team-a", "app"),
				secret("team-a", "secret-1", stringmap{"KEY": sampleKey}),
			},
			want: nil,
		},
		{
			name:       "Scanning multiple namespaces",
			namespaces: []string{"ns-a", "ns-b"},
			objs: []runtime.Object{
				namespace("ns-a"),
				deployment("ns-a", "app-a", withEnvFrom(0, "secret")),
				secret("ns-a", "secret", stringmap{"KEY": "sk-AAA"}),
				namespace("ns-b"),
				deployment("ns-b", "app-b", withEnvFrom(0, "secret")),
				secret("ns-b", "secret", stringmap{"KEY": "sk-BBB"}),
			},
			want: []deploymentWithSecret{
				{namespace: "ns-a", deployment: "app-a", secret: "sk-AAA"},
				{namespace: "ns-b", deployment: "app-b", secret: "sk-BBB"},
			},
		},
		{
			name:       "Same API key found in two secrets of one deployment produces one finding (deduplicated)",
			namespaces: []string{"team-a"},
			objs: []runtime.Object{
				namespace("team-a"),
				secret("team-a", "secret-1", stringmap{"KEY": "sk-AAA"}),
				secret("team-a", "secret-2", stringmap{"KEY": "sk-AAA"}),
				deployment("team-a", "app", withEnvFrom(0, "secret-1"), withEnvFrom(0, "secret-2")),
			},
			want: []deploymentWithSecret{
				{namespace: "team-a", deployment: "app", secret: "sk-AAA"},
			},
		},
		{
			name:       "Error listing deployments in one namespace: other namespaces still processed",
			namespaces: []string{"restricted", "team-b"},
			objs: []runtime.Object{
				namespace("restricted"),
				secret("restricted", "secret", stringmap{"KEY": "sk-AAA"}),
				deployment("restricted", "app", withEnvFrom(0, "secret")),
				namespace("team-b"),
				secret("team-b", "secret", stringmap{"KEY": "sk-BBB"}),
				deployment("team-b", "app", withEnvFrom(0, "secret")),
			},
			reactor: func(c *fake.FakeDynamicClient) {
				c.PrependReactor("list", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
					if action.(k8stesting.ListActionImpl).Namespace == "restricted" {
						return true, nil, assert.AnError
					}
					return false, nil, nil
				})
			},
			want: []deploymentWithSecret{
				{namespace: "team-b", deployment: "app", secret: "sk-BBB"},
			},
		},
		{
			name:       "On error reading a secret, that Deployment skipped, others in namespace still processed",
			namespaces: []string{"team-a"},
			objs: []runtime.Object{
				namespace("team-a"),
				secret("team-a", "bad-secret", stringmap{"KEY": "sk-AAA"}),
				secret("team-a", "good-secret", stringmap{"KEY": "sk-BBB"}),
				deployment("team-a", "app-bad", withEnvFrom(0, "bad-secret")),
				deployment("team-a", "app-good", withEnvFrom(0, "good-secret")),
			},
			reactor: func(c *fake.FakeDynamicClient) {
				c.PrependReactor("get", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
					if action.(k8stesting.GetActionImpl).Name == "bad-secret" {
						return true, nil, assert.AnError
					}
					return false, nil, nil
				})
			},
			want: []deploymentWithSecret{
				{namespace: "team-a", deployment: "app-good", secret: "sk-BBB"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fakeDynamicClient(tt.objs...)
			if tt.reactor != nil {
				tt.reactor(client)
			}
			result, err := findDeploymentsWithMatchingSecrets(context.Background(), client, tt.namespaces, testKeyRegexp)
			require.NoError(t, err)
			assert.Equal(t, tt.want, sortedDeplsWithSecrets(result))
		})
	}
}

func TestFetchModels(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantModels []string
		wantErr    bool
	}{
		{
			name:       "200 OK returns model IDs",
			statusCode: http.StatusOK,
			body:       `{"data":[{"id":"gpt-4o"},{"id":"claude-3-5-sonnet"}]}`,
			wantModels: []string{"gpt-4o", "claude-3-5-sonnet"},
		},
		{
			name:       "empty data array returns no models",
			statusCode: http.StatusOK,
			body:       `{"data":[]}`,
			wantModels: nil,
		},
		{
			name:       "401 Unauthorized returns no models without error",
			statusCode: http.StatusUnauthorized,
			body:       `whatever`,
			wantModels: nil,
		},
		{
			name:       "403 Forbidden returns no models without error",
			statusCode: http.StatusForbidden,
			body:       `whatever`,
			wantModels: nil,
		},
		{
			name:       "non-200 status returns error",
			statusCode: http.StatusInternalServerError,
			body:       `{"data":[]}`,
			wantErr:    true,
		},
		{
			name:       "invalid JSON body returns error",
			statusCode: http.StatusOK,
			body:       `not-json`,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/v1/models", r.URL.Path)
				assert.Equal(t, "Bearer "+testAPIKey, r.Header.Get("Authorization"))
				w.WriteHeader(tt.statusCode)
				fmt.Fprint(w, tt.body)
			}))
			defer mockServer.Close()

			host := strings.TrimPrefix(mockServer.URL, "https://")
			models, err := fetchModels(mockServer.Client(), host, testAPIKey)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.ElementsMatch(t, tt.wantModels, models)
			}
		})
	}
}

func TestReferencedSecretsNames(t *testing.T) {
	const sharedValue = "shared"
	tests := []struct {
		name string
		obj  attrs
		want map[string]struct{}
	}{
		{
			name: "envFrom.secretRef",
			obj: attrs{"spec": attrs{"template": attrs{"spec": attrs{
				"containers": []any{
					attrs{"envFrom": []any{attrs{"secretRef": attrs{"name": "secret-a"}}}},
				},
			}}}},
			want: map[string]struct{}{"secret-a": {}},
		},
		{
			name: "env.valueFrom.secretKeyRef",
			obj: attrs{"spec": attrs{"template": attrs{"spec": attrs{
				"containers": []any{
					attrs{"env": []any{attrs{"valueFrom": attrs{"secretKeyRef": attrs{"name": "secret-b"}}}}},
				},
			}}}},
			want: map[string]struct{}{"secret-b": {}},
		},
		{
			name: "volumes.secret.secretName",
			obj: attrs{"spec": attrs{"template": attrs{"spec": attrs{
				"volumes": []any{
					attrs{"secret": attrs{"secretName": "secret-c"}},
				},
			}}}},
			want: map[string]struct{}{"secret-c": {}},
		},
		{
			name: "initContainers...",
			obj: attrs{"spec": attrs{"template": attrs{"spec": attrs{
				"initContainers": []any{
					attrs{"envFrom": []any{attrs{"secretRef": attrs{"name": "init-secret"}}}},
				},
			}}}},
			want: map[string]struct{}{"init-secret": {}},
		},
		{
			name: "multiple references to same secret are deduplicated",
			obj: attrs{"spec": attrs{"template": attrs{"spec": attrs{
				"containers": []any{
					attrs{
						"envFrom": []any{
							attrs{"secretRef": attrs{"name": sharedValue}},
							attrs{"secretRef": attrs{"name": "unique-1"}},
						},
						"env": []any{
							attrs{"valueFrom": attrs{"secretKeyRef": attrs{"name": sharedValue}}},
							attrs{"valueFrom": attrs{"secretKeyRef": attrs{"name": "unique-2"}}},
						},
					},
				},
				"volumes": []any{
					attrs{"secret": attrs{"secretName": sharedValue}},
					attrs{"secret": attrs{"secretName": "unique-3"}},
				},
			}}}},
			want: map[string]struct{}{
				sharedValue: {},
				"unique-1":  {},
				"unique-2":  {},
				"unique-3":  {},
			},
		},
		{
			name: "no secret references returns empty map",
			obj:  attrs{"spec": attrs{"template": attrs{"spec": attrs{}}}},
			want: map[string]struct{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, referencedSecretsNames(tt.obj))
		})
	}
}

func fakeDynamicClient(objs ...runtime.Object) *fake.FakeDynamicClient {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)

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

func secret(ns, name string, data map[string]string) *corev1.Secret {
	encoded := make(map[string][]byte, len(data))
	for k, v := range data {
		encoded[k] = []byte(v)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Data:       encoded,
	}
}

type deploymentOpt func(*appsv1.Deployment)

func deployment(ns, name string, opts ...deploymentOpt) *appsv1.Deployment {
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main"},
						{Name: "sidecar"},
					},
				},
			},
		},
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

func withEnvFrom(containerIdx int, secretName string) deploymentOpt {
	return func(d *appsv1.Deployment) {
		e := &d.Spec.Template.Spec.Containers[containerIdx].EnvFrom
		*e = append(*e, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
			},
		})
	}
}

func withSecretKeyRef(containerIdx int, envName, secretName, key string) deploymentOpt {
	return func(d *appsv1.Deployment) {
		e := &d.Spec.Template.Spec.Containers[containerIdx].Env
		*e = append(*e, corev1.EnvVar{
			Name: envName,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
					Key:                  key,
				},
			},
		})
	}
}

func withSecretVolume(secretName string) deploymentOpt {
	return func(d *appsv1.Deployment) {
		v := &d.Spec.Template.Spec.Volumes
		*v = append(*v, corev1.Volume{
			Name: secretName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: secretName},
			},
		})
	}
}

func sortedDeplsWithSecrets(depls []deploymentWithSecret) []deploymentWithSecret {
	sort.Slice(depls, func(i, j int) bool {
		a, b := depls[i], depls[j]
		if a.namespace != b.namespace {
			return a.namespace < b.namespace
		}
		if a.deployment != b.deployment {
			return a.deployment < b.deployment
		}
		return a.secret < b.secret
	})
	return depls
}

func compileDefaultAPIKeyRegexp() *regexp.Regexp {
	var cfg Config
	err := env.Load(&cfg, &env.Options{Source: env.Map{}})
	if err != nil {
		panic(fmt.Sprintf("failed to load default config: %v", err))
	}
	re, err := regexp.Compile(cfg.APIKeyRegexp)
	if err != nil {
		panic(fmt.Sprintf("failed to compile default APIKeyRegexp %q: %v", cfg.APIKeyRegexp, err))
	}
	if !re.MatchString(testAPIKey) {
		panic(fmt.Sprintf("testAPIKey %q does not match default APIKeyRegexp %q", testAPIKey, cfg.APIKeyRegexp))
	}
	return re
}
