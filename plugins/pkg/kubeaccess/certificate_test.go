package kubeaccess

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func clientCA(t *testing.T) (*x509.Certificate, ed25519.PrivateKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test-client-ca",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	require.NoError(t, err)
	certificate, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return certificate, privateKey
}

func clientIdentity(t *testing.T, ca *x509.Certificate, caKey ed25519.PrivateKey, mutate func(*x509.Certificate)) ([]byte, []byte) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "sensitive-test-client",
			Organization: []string{"sensitive-test-group"},
		},
		NotBefore:   time.Now().Add(-time.Minute),
		NotAfter:    time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	if mutate != nil {
		mutate(template)
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, publicKey, caKey)
	require.NoError(t, err)
	key, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key})
}

func TestClientCertificateAccess(t *testing.T) {
	ca, caKey := clientCA(t)
	for _, tt := range []struct {
		name       string
		mutate     func(*x509.Certificate)
		untrusted  bool
		services   int
		connection bool
		noClient   bool
		noServerCA bool
	}{
		{name: "reader", connection: true},
		{name: "services forbidden", connection: true, services: http.StatusForbidden},
		{name: "expired", mutate: func(cert *x509.Certificate) {
			cert.NotBefore = time.Now().Add(-2 * time.Hour)
			cert.NotAfter = time.Now().Add(-time.Hour)
		}},
		{name: "not yet valid", mutate: func(cert *x509.Certificate) {
			cert.NotBefore = time.Now().Add(time.Hour)
			cert.NotAfter = time.Now().Add(2 * time.Hour)
		}},
		{name: "wrong certificate usage", mutate: func(cert *x509.Certificate) {
			cert.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		}},
		{name: "untrusted client CA", untrusted: true},
		{name: "untrusted server CA", noServerCA: true},
		{name: "token cannot replace required client certificate", noClient: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			issuer, issuerKey := ca, caKey
			if tt.untrusted {
				issuer, issuerKey = clientCA(t)
			}
			certificate, key := clientIdentity(t, issuer, issuerKey, tt.mutate)
			block, _ := pem.Decode(certificate)
			var requests atomic.Int32
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				assert.Empty(t, r.Header.Get("Authorization"), "certificate authentication must not send a bearer token")
				assert.Equal(t, http.MethodGet, r.Method)
				if !assert.NotEmpty(t, r.TLS.VerifiedChains) {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				assert.Equal(t, block.Bytes, r.TLS.PeerCertificates[0].Raw)
				w.Header().Set("Content-Type", "application/json")
				if tt.services != 0 && r.URL.Path == "/api/v1/services" {
					w.WriteHeader(tt.services)
					_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Forbidden","code":403,"message":"sensitive-upstream-error"}`))
					return
				}
				responses := map[string]string{
					"/version":                       `{"gitVersion":"v1.36.1"}`,
					"/api/v1/namespaces/kube-system": `{"kind":"Namespace","apiVersion":"v1","metadata":{"name":"kube-system","uid":"installation-certificate"}}`,
					"/api/v1/namespaces":             `{"kind":"NamespaceList","apiVersion":"v1","items":[]}`,
					"/apis/apps/v1/deployments":      `{"kind":"DeploymentList","apiVersion":"apps/v1","items":[]}`,
					"/api/v1/services":               `{"kind":"ServiceList","apiVersion":"v1","items":[]}`,
				}
				body, ok := responses[r.URL.Path]
				if !assert.True(t, ok, "unexpected request: %s", r.URL.Path) {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				_, _ = w.Write([]byte(body))
			}))
			pool := x509.NewCertPool()
			pool.AddCert(ca)
			server.TLS = &tls.Config{
				ClientAuth: tls.RequireAndVerifyClientCert,
				ClientCAs:  pool,
				MinVersion: tls.VersionTLS12,
			}
			server.Config.ErrorLog = log.New(io.Discard, "", 0)
			server.StartTLS()
			defer server.Close()
			data := kubeconfig(t, server, func(cluster map[string]any, user map[string]any) {
				if tt.noServerCA {
					delete(cluster, "certificate-authority-data")
				}
				if tt.noClient {
					return
				}
				delete(user, "token")
				user["client-certificate-data"] = base64.StdEncoding.EncodeToString(certificate)
				user["client-key-data"] = base64.StdEncoding.EncodeToString(key)
			})
			result := CheckAccess(t.Context(), data, server.URL, "test")
			if !tt.connection {
				require.Len(t, result.Checks, 2)
				assert.True(t, result.Checks[0].Passed)
				assert.Equal(t, "connection", result.Checks[1].Name)
				assert.False(t, result.Checks[1].Passed)
				assert.Empty(t, result.KubeSystemUID)
				assert.Zero(t, requests.Load(), "rejected certificates must not reach the HTTP handler")
			} else {
				services := Check{Name: "services", Passed: true, Code: "OK", Message: "Can list Services cluster-wide."}
				if tt.services == http.StatusForbidden {
					services = Check{Name: "services", Code: "FORBIDDEN", Message: "The supplied credential does not have the required read permission."}
				}
				assert.Equal(t, Result{
					KubeSystemUID: "installation-certificate",
					Checks: []Check{
						{Name: "configuration", Passed: true, Code: "OK", Message: "Selected kubeconfig and endpoint accepted."},
						{Name: "connection", Passed: true, Code: "OK", Message: "Reached the Kubernetes API with verified TLS; resource checks verify access separately."},
						{Name: "fingerprint", Passed: true, Code: "OK", Message: "Read the kube-system namespace UID."},
						{Name: "namespaces", Passed: true, Code: "OK", Message: "Can list namespaces cluster-wide."},
						{Name: "deployments", Passed: true, Code: "OK", Message: "Can list Deployments cluster-wide."},
						services,
					},
				}, result)
				assert.EqualValues(t, 5, requests.Load())
			}
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "sensitive")
			assert.NotContains(t, string(encoded), string(certificate))
			assert.NotContains(t, string(encoded), string(key))
		})
	}
}

func TestRejectInvalidClientCertificate(t *testing.T) {
	ca, caKey := clientCA(t)
	certificate, key := clientIdentity(t, ca, caKey, nil)
	_, otherKey := clientIdentity(t, ca, caKey, nil)
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("invalid configuration contacted the server")
	}))
	defer server.Close()
	for _, tt := range []struct {
		name        string
		certificate []byte
		key         []byte
		mutate      func(map[string]any, map[string]any)
		code        string
	}{
		{name: "missing key", certificate: certificate},
		{name: "missing certificate", key: key},
		{name: "mismatched key", certificate: certificate, key: otherKey},
		{name: "malformed certificate", certificate: []byte("sensitive-invalid-certificate"), key: key},
		{name: "malformed key", certificate: certificate, key: []byte("sensitive-invalid-key")},
		{name: "token with valid certificate", certificate: certificate, key: key, code: "UNSAFE_CONFIG", mutate: func(_ map[string]any, user map[string]any) {
			user["token"] = "sensitive-token"
		}},
		{name: "basic authentication with valid certificate", certificate: certificate, key: key, code: "UNSAFE_CONFIG", mutate: func(_ map[string]any, user map[string]any) {
			user["username"] = "sensitive-user"
			user["password"] = "sensitive-password"
		}},
		{name: "external certificate with embedded pair", certificate: certificate, key: key, code: "UNSAFE_CONFIG", mutate: func(_ map[string]any, user map[string]any) {
			user["client-certificate"] = "/sensitive-certificate-file"
		}},
		{name: "external key with embedded pair", certificate: certificate, key: key, code: "UNSAFE_CONFIG", mutate: func(_ map[string]any, user map[string]any) {
			user["client-key"] = "/sensitive-key-file"
		}},
		{name: "external CA with embedded pair", certificate: certificate, key: key, code: "UNSAFE_CONFIG", mutate: func(cluster map[string]any, _ map[string]any) {
			cluster["certificate-authority"] = "/sensitive-ca-file"
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data := kubeconfig(t, server, func(cluster map[string]any, user map[string]any) {
				delete(user, "token")
				user["client-certificate-data"] = base64.StdEncoding.EncodeToString(tt.certificate)
				user["client-key-data"] = base64.StdEncoding.EncodeToString(tt.key)
				if tt.mutate != nil {
					tt.mutate(cluster, user)
				}
			})
			result := CheckAccess(t.Context(), data, server.URL, "")
			require.Len(t, result.Checks, 1)
			assert.Equal(t, "configuration", result.Checks[0].Name)
			code := tt.code
			if code == "" {
				code = "INVALID_CONFIG"
			}
			assert.Equal(t, code, result.Checks[0].Code)
			assert.False(t, result.Checks[0].Passed)
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "sensitive")
		})
	}
}
