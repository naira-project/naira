package kubeaccess

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pemCertificate(server *httptest.Server) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
}

func kubeconfig(t *testing.T, server *httptest.Server, mutate func(map[string]any, map[string]any)) []byte {
	t.Helper()
	cluster := map[string]any{
		"server":                     server.URL,
		"certificate-authority-data": base64.StdEncoding.EncodeToString(pemCertificate(server)),
	}
	user := map[string]any{"token": "sensitive-test-token"}
	if mutate != nil {
		mutate(cluster, user)
	}
	data, err := json.Marshal(map[string]any{
		"apiVersion": "v1", "kind": "Config", "current-context": "test",
		"clusters": []any{map[string]any{"name": "target", "cluster": cluster}},
		"users":    []any{map[string]any{"name": "reader", "user": user}},
		"contexts": []any{map[string]any{"name": "test", "context": map[string]any{"cluster": "target", "user": "reader"}}},
	})
	require.NoError(t, err)
	return data
}

func TestCheckKubernetesAccess(t *testing.T) {
	for _, tt := range []struct {
		name       string
		deniedPath string
		status     int
	}{
		{name: "success"},
		{name: "services forbidden", deniedPath: "/api/v1/services", status: http.StatusForbidden},
		{name: "fingerprint forbidden", deniedPath: "/api/v1/namespaces/kube-system", status: http.StatusForbidden},
		{name: "invalid credential", deniedPath: "/version", status: http.StatusUnauthorized},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "Bearer sensitive-test-token", r.Header.Get("Authorization"))
				assert.Equal(t, http.MethodGet, r.Method)
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == tt.deniedPath {
					w.WriteHeader(tt.status)
					_ = json.NewEncoder(w).Encode(map[string]any{"kind": "Status", "apiVersion": "v1", "status": "Failure", "reason": http.StatusText(tt.status), "code": tt.status, "message": "sensitive-upstream-message"})
					return
				}
				switch r.URL.Path {
				case "/version":
					_, _ = w.Write([]byte(`{"gitVersion":"v1.36.1"}`))
				case "/api/v1/namespaces/kube-system":
					_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"kube-system","uid":"installation-a"}}`))
				case "/api/v1/namespaces":
					_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"NamespaceList","items":[]}`))
				case "/apis/apps/v1/deployments":
					_, _ = w.Write([]byte(`{"apiVersion":"apps/v1","kind":"DeploymentList","items":[]}`))
				case "/api/v1/services":
					_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"ServiceList","items":[]}`))
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			result := CheckAccess(t.Context(), kubeconfig(t, server, nil), server.URL, "")
			if tt.status == http.StatusUnauthorized {
				require.Len(t, result.Checks, 2)
				assert.Equal(t, "UNAUTHORIZED", result.Checks[1].Code)
			} else {
				require.Len(t, result.Checks, 6)
				for _, check := range result.Checks {
					denied := (check.Name == "services" && tt.deniedPath == "/api/v1/services") || (check.Name == "fingerprint" && tt.deniedPath == "/api/v1/namespaces/kube-system")
					assert.Equal(t, !denied, check.Passed, check.Name)
				}
			}
			encoded, err := json.Marshal(result)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "sensitive")
		})
	}
}

func TestRejectUnsafeKubeconfig(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("unsafe configuration contacted the server") }))
	defer server.Close()
	for _, tt := range []struct {
		name   string
		mutate func(map[string]any, map[string]any)
		code   string
	}{
		{name: "exec", mutate: func(_ map[string]any, u map[string]any) {
			u["exec"] = map[string]any{"command": "must-not-execute", "apiVersion": "client.authentication.k8s.io/v1"}
		}},
		{name: "external CA", mutate: func(c map[string]any, _ map[string]any) { c["certificate-authority"] = "/sensitive-file" }},
		{name: "token file", mutate: func(_ map[string]any, u map[string]any) { u["tokenFile"] = "/sensitive-file" }},
		{name: "certificate file", mutate: func(_ map[string]any, u map[string]any) { u["client-certificate"] = "/sensitive-file" }},
		{name: "key file", mutate: func(_ map[string]any, u map[string]any) { u["client-key"] = "/sensitive-file" }},
		{name: "auth provider", mutate: func(_ map[string]any, u map[string]any) {
			u["auth-provider"] = map[string]any{"name": "must-not-load"}
		}},
		{name: "insecure TLS", mutate: func(c map[string]any, _ map[string]any) { c["insecure-skip-tls-verify"] = true }},
		{name: "proxy", mutate: func(c map[string]any, _ map[string]any) { c["proxy-url"] = "http://proxy.example" }},
		{name: "impersonation", mutate: func(_ map[string]any, u map[string]any) { u["as"] = "admin" }},
		{name: "missing credential", code: "MISSING_CREDENTIAL", mutate: func(_ map[string]any, u map[string]any) { delete(u, "token") }},
		{name: "blank token", code: "MISSING_CREDENTIAL", mutate: func(_ map[string]any, u map[string]any) { u["token"] = "  " }},
		{name: "basic authentication with token", mutate: func(_ map[string]any, u map[string]any) {
			u["username"] = "user"
			u["password"] = "password"
		}},
		{name: "client certificate with token", mutate: func(_ map[string]any, u map[string]any) {
			u["client-certificate-data"] = base64.StdEncoding.EncodeToString([]byte("certificate"))
			u["client-key-data"] = base64.StdEncoding.EncodeToString([]byte("key"))
		}},
		{name: "malformed client certificate and key", code: "INVALID_CONFIG", mutate: func(_ map[string]any, u map[string]any) {
			delete(u, "token")
			u["client-certificate-data"] = base64.StdEncoding.EncodeToString([]byte("certificate"))
			u["client-key-data"] = base64.StdEncoding.EncodeToString([]byte("key"))
		}},
		{name: "certificate without key", code: "INVALID_CONFIG", mutate: func(_ map[string]any, u map[string]any) {
			delete(u, "token")
			u["client-certificate-data"] = base64.StdEncoding.EncodeToString([]byte("certificate"))
		}},
		{name: "key without certificate", code: "INVALID_CONFIG", mutate: func(_ map[string]any, u map[string]any) {
			delete(u, "token")
			u["client-key-data"] = base64.StdEncoding.EncodeToString([]byte("key"))
		}},
		{name: "token with certificate only", mutate: func(_ map[string]any, u map[string]any) {
			u["client-certificate-data"] = base64.StdEncoding.EncodeToString([]byte("certificate"))
		}},
		{name: "token with key only", mutate: func(_ map[string]any, u map[string]any) {
			u["client-key-data"] = base64.StdEncoding.EncodeToString([]byte("key"))
		}},
		{name: "blank token with certificate", mutate: func(_ map[string]any, u map[string]any) {
			u["token"] = "  "
			u["client-certificate-data"] = base64.StdEncoding.EncodeToString([]byte("certificate"))
			u["client-key-data"] = base64.StdEncoding.EncodeToString([]byte("key"))
		}},
		{name: "basic authentication without token", mutate: func(_ map[string]any, u map[string]any) {
			delete(u, "token")
			u["username"] = "user"
			u["password"] = "password"
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := CheckAccess(t.Context(), kubeconfig(t, server, tt.mutate), server.URL, "test")
			require.Len(t, result.Checks, 1)
			code := tt.code
			if code == "" {
				code = "UNSAFE_CONFIG"
			}
			assert.Equal(t, "configuration", result.Checks[0].Name)
			assert.Equal(t, code, result.Checks[0].Code)
			assert.False(t, result.Checks[0].Passed)
		})
	}
	for _, contextName := range []string{"missing", "test"} {
		endpoint := server.URL
		if contextName == "test" {
			endpoint = "https://different.example"
		}
		result := CheckAccess(t.Context(), kubeconfig(t, server, nil), endpoint, contextName)
		require.Len(t, result.Checks, 1)
		assert.False(t, result.Checks[0].Passed)
	}
	result := CheckAccess(t.Context(), []byte("not: [valid"), server.URL, "")
	require.Len(t, result.Checks, 1)
	assert.Equal(t, "INVALID_CONFIG", result.Checks[0].Code)
}

func TestTLSAndCancellation(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) }))
	defer server.Close()
	data := kubeconfig(t, server, func(c map[string]any, _ map[string]any) { delete(c, "certificate-authority-data") })
	result := CheckAccess(t.Context(), data, server.URL, "")
	require.Len(t, result.Checks, 2)
	assert.Equal(t, "TLS_ERROR", result.Checks[1].Code)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result = CheckAccess(ctx, kubeconfig(t, server, nil), server.URL, "")
	require.Len(t, result.Checks, 2)
	assert.Equal(t, "CANCELED", result.Checks[1].Code)
}
