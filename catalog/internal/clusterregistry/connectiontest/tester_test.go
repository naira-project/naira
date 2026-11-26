package connectiontest

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/naira-project/naira/catalog/internal/clusterregistry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestServiceRejectsCredentialApprovedOnlyForAnotherWorkspace(t *testing.T) {
	client := fake.NewClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "naira",
			Name:      "other-workspace-access",
		},
		Data: map[string][]byte{"kubeconfig": []byte("private-kubeconfig")},
	})
	credentials, err := NewSecretStore(client.CoreV1(), map[string][]string{
		"other-workspace": {"naira/other-workspace-access#kubeconfig"},
	})
	require.NoError(t, err)
	store := clusterregistry.NewMemoryStore()
	registry := clusterregistry.NewService(store)
	tester := NewKubernetesTester(credentials)
	service := NewService(registry, store, tester.Test)
	caller := clusterregistry.Caller{
		Issuer:  "https://issuer.example.com",
		Subject: "alice",
	}
	cluster, err := registry.Create(t.Context(), caller, clusterregistry.CreateParams{
		Workspace:     clusterregistry.Workspace{ID: clusterregistry.TestWorkspaceID},
		Name:          "cluster",
		CredentialRef: "kubernetes-secret:naira/other-workspace-access#kubeconfig",
		Kubernetes:    clusterregistry.KubernetesConnection{APIEndpoint: "https://cluster.example.com"},
	})
	require.NoError(t, err)
	result, err := service.TestConnection(t.Context(), caller, cluster.Workspace, cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, clusterregistry.ValidationFailed, result.Status)
	assert.Equal(t, []clusterregistry.AccessCheck{{
		Name:    "credential",
		Code:    "CREDENTIAL_UNAVAILABLE",
		Message: "Cannot read this credential. Check the approved Secret reference and Secret permissions.",
	}}, result.Checks)
	assert.Empty(t, client.Actions(), "another workspace's credentials must not be read")
	saved, err := store.Get(t.Context(), cluster.Workspace, cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, &result, saved.LastValidation)
}

func TestServiceWithKubernetesTester(t *testing.T) {
	client := fake.NewClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "naira", Name: "approved"},
		Data:       map[string][]byte{"kubeconfig": []byte("private-invalid-kubeconfig")},
	})
	workspace := clusterregistry.Workspace{ID: "workspace-a"}
	credentials, err := NewSecretStore(client.CoreV1(), map[string][]string{workspace.ID: {"naira/approved#kubeconfig"}})
	require.NoError(t, err)
	tester := NewKubernetesTester(credentials)
	result := tester.Test(t.Context(), clusterregistry.Cluster{
		Workspace:     workspace,
		CredentialRef: "kubernetes-secret:naira/approved#kubeconfig",
	})
	require.Len(t, result.Checks, 2)
	assert.True(t, result.Checks[0].Passed)
	assert.Equal(t, "INVALID_CONFIG", result.Checks[1].Code)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "private-invalid-kubeconfig")
	result = tester.Test(t.Context(), clusterregistry.Cluster{
		Workspace:     workspace,
		CredentialRef: "kubernetes-secret:naira/other#kubeconfig",
	})
	require.Len(t, result.Checks, 1)
	assert.Equal(t, "CREDENTIAL_UNAVAILABLE", result.Checks[0].Code)
	client.ClearActions()
	result = tester.Test(t.Context(), clusterregistry.Cluster{
		Workspace:     clusterregistry.Workspace{ID: "workspace-b"},
		CredentialRef: "kubernetes-secret:naira/approved#kubeconfig",
	})
	require.Len(t, result.Checks, 1)
	assert.Equal(t, "CREDENTIAL_UNAVAILABLE", result.Checks[0].Code)
	assert.Empty(t, client.Actions())
}

func TestConnectionThroughSecretStore(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "private-reader",
		},
		NotBefore:   time.Now().Add(-time.Minute),
		NotAfter:    time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	require.NoError(t, err)
	certificate, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	key, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key})
	for _, authentication := range []string{"token", "certificate"} {
		for _, profile := range []string{"reader", "no-services", "no-access"} {
			t.Run(authentication+"/"+profile, func(t *testing.T) {
				server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if authentication == "certificate" {
						assert.Empty(t, r.Header.Get("Authorization"))
						if !assert.NotEmpty(t, r.TLS.VerifiedChains) {
							w.WriteHeader(http.StatusUnauthorized)
							return
						}
						assert.Equal(t, der, r.TLS.PeerCertificates[0].Raw)
					} else if r.Header.Get("Authorization") != "Bearer private-reader-token" {
						t.Error("target request must use the token from the Secret kubeconfig")
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					if profile == "no-access" && r.URL.Path != "/version" || profile == "no-services" && r.URL.Path == "/api/v1/services" {
						w.WriteHeader(http.StatusForbidden)
						_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Forbidden","code":403,"message":"private-upstream-error"}`))
						return
					}
					switch r.URL.Path {
					case "/version":
						_, _ = w.Write([]byte(`{"gitVersion":"v1.36.1"}`))
					case "/api/v1/namespaces/kube-system":
						_, _ = w.Write([]byte(`{"kind":"Namespace","apiVersion":"v1","metadata":{"name":"kube-system","uid":"installation-a"}}`))
					case "/api/v1/namespaces":
						_, _ = w.Write([]byte(`{"kind":"NamespaceList","apiVersion":"v1","items":[]}`))
					case "/apis/apps/v1/deployments":
						_, _ = w.Write([]byte(`{"kind":"DeploymentList","apiVersion":"apps/v1","items":[]}`))
					case "/api/v1/services":
						_, _ = w.Write([]byte(`{"kind":"ServiceList","apiVersion":"v1","items":[]}`))
					default:
						t.Errorf("unexpected target request: %s", r.URL.Path)
						w.WriteHeader(http.StatusNotFound)
					}
				}))
				user := map[string]any{"token": "private-reader-token"}
				if authentication == "certificate" {
					pool := x509.NewCertPool()
					pool.AddCert(certificate)
					server.TLS = &tls.Config{
						ClientAuth: tls.RequireAndVerifyClientCert,
						ClientCAs:  pool,
						MinVersion: tls.VersionTLS12,
					}
					user = map[string]any{
						"client-certificate-data": base64.StdEncoding.EncodeToString(certificatePEM),
						"client-key-data":         base64.StdEncoding.EncodeToString(keyPEM),
					}
				}
				server.StartTLS()
				defer server.Close()
				kubeconfig, err := json.Marshal(map[string]any{
					"apiVersion": "v1", "kind": "Config", "current-context": "reader",
					"clusters": []any{map[string]any{"name": "target", "cluster": map[string]any{
						"server": server.URL, "certificate-authority-data": base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{
							Type:  "CERTIFICATE",
							Bytes: server.Certificate().Raw,
						})),
					}}},
					"users":    []any{map[string]any{"name": "reader", "user": user}},
					"contexts": []any{map[string]any{"name": "reader", "context": map[string]any{"cluster": "target", "user": "reader"}}},
				})
				require.NoError(t, err)
				client := fake.NewClientset(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "naira", Name: "reader"},
					Data:       map[string][]byte{"kubeconfig": kubeconfig},
				})
				credentials, err := NewSecretStore(client.CoreV1(), map[string][]string{
					clusterregistry.TestWorkspaceID: {"naira/reader#kubeconfig"},
				})
				require.NoError(t, err)
				tester := NewKubernetesTester(credentials)
				store := clusterregistry.NewMemoryStore()
				registry := clusterregistry.NewService(store)
				caller := clusterregistry.Caller{Issuer: "https://issuer.example.com", Subject: "alice"}
				cluster, err := registry.Create(t.Context(), caller, clusterregistry.CreateParams{
					Name: profile, Workspace: clusterregistry.Workspace{ID: clusterregistry.TestWorkspaceID},
					CredentialRef: "kubernetes-secret:naira/reader#kubeconfig",
					Kubernetes:    clusterregistry.KubernetesConnection{APIEndpoint: server.URL, Context: "reader"},
				})
				require.NoError(t, err)
				service := NewService(registry, store, tester.Test)
				result, err := service.TestConnection(t.Context(), caller, cluster.Workspace, cluster.ID)
				require.NoError(t, err)
				if profile == "reader" {
					assert.Equal(t, clusterregistry.ValidationReady, result.Status)
				} else {
					assert.Equal(t, clusterregistry.ValidationFailed, result.Status)
					assert.Contains(t, result.Checks, clusterregistry.AccessCheck{
						Name:    "services",
						Code:    "FORBIDDEN",
						Message: "The supplied credential does not have the required read permission.",
					})
				}
				stored, err := store.Get(t.Context(), cluster.Workspace, cluster.ID)
				require.NoError(t, err)
				assert.Equal(t, result, *stored.LastValidation)
				encoded, err := json.Marshal(result)
				require.NoError(t, err)
				assert.NotContains(t, string(encoded), "private-reader-token")
				assert.NotContains(t, string(encoded), "private-upstream-error")
				assert.NotContains(t, string(encoded), "private-reader")
				assert.NotContains(t, string(encoded), string(keyPEM))
			})
		}
	}
}
