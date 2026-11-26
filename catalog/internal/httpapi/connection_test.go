package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/naira-project/naira/catalog/internal/auth/keycloak"
	"github.com/naira-project/naira/catalog/internal/clusterregistry"
	"github.com/naira-project/naira/catalog/internal/clusterregistry/connectiontest"
	"github.com/naira-project/naira/catalog/internal/clusters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManualConnectionEndpoint(t *testing.T) {
	store := clusterregistry.NewMemoryStore()
	caller := clusterregistry.Caller{Issuer: testIssuer, Subject: "alice"}
	report := clusterregistry.ValidationResult{KubeSystemUID: "installation-a", Checks: []clusterregistry.AccessCheck{{Name: "services", Passed: true, Code: "OK", Message: "Services are readable."}}}
	called := 0
	service := clusterregistry.NewService(store)
	connectionTestService := connectiontest.NewService(service, store, func(context.Context, clusterregistry.Cluster) clusterregistry.ValidationResult {
		called++
		return report
	})
	cluster, err := service.Create(t.Context(), caller, clusterregistry.CreateParams{
		Name: "target", Workspace: clusterregistry.Workspace{ID: clusterregistry.TestWorkspaceID}, CredentialRef: "private-reference",
		Kubernetes: clusterregistry.KubernetesConnection{APIEndpoint: "https://cluster.example.com"},
	})
	require.NoError(t, err)
	router, err := NewRouter(nil, nil, nil, nil, keycloak.Config{Client: workspaceTokenDecoder{}, Issuer: testIssuer}, &clusters.Registry{
		Registrations:   service,
		ConnectionTests: connectionTestService,
	})
	require.NoError(t, err)
	path := "/v1/workspaces/test-workspace/registrations/" + cluster.ID.String() + ":test"
	request := func(token, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	assert.Equal(t, http.StatusUnauthorized, request("", path).Code)
	assert.Equal(t, http.StatusNotFound, request("alice", strings.Replace(path, "test-workspace", "other", 1)).Code)
	assert.Equal(t, http.StatusNotFound, request("alice", strings.Replace(path, cluster.ID.String(), uuid.NewString(), 1)).Code)
	assert.Equal(t, 0, called)
	legacyPath := strings.TrimSuffix(path, ":test") + "/test"
	assert.Equal(t, http.StatusNotFound, request("alice", legacyPath).Code)
	assert.Equal(t, 0, called, "the removed subresource route must not invoke validation")
	rec := request("alice", path)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var result clusterregistry.ValidationResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	assert.Equal(t, clusterregistry.ValidationReady, result.Status)
	assert.NotContains(t, rec.Body.String(), "private-reference")
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	get := httptest.NewRequest(http.MethodGet, strings.TrimSuffix(path, ":test"), nil)
	get.Header.Set("Authorization", "Bearer alice")
	saved := httptest.NewRecorder()
	router.ServeHTTP(saved, get)
	require.Equal(t, http.StatusOK, saved.Code)
	assert.Contains(t, saved.Body.String(), `"lastValidation"`)
	assert.Contains(t, saved.Body.String(), `"READY"`)
	report.Checks[0].Passed = false
	report.Checks[0].Code = "FORBIDDEN"
	rec = request("bob", path)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	assert.Equal(t, clusterregistry.ValidationFailed, result.Status)
	assert.Equal(t, "FORBIDDEN", result.Checks[0].Code)
}

func TestManualConnectionEndpointWithoutValidator(t *testing.T) {
	store := clusterregistry.NewMemoryStore()
	registry := clusterregistry.NewService(store)
	cluster, err := registry.Create(t.Context(), clusterregistry.Caller{
		Issuer:  testIssuer,
		Subject: "alice",
	}, clusterregistry.CreateParams{
		Name:          "target",
		Workspace:     clusterregistry.Workspace{ID: clusterregistry.TestWorkspaceID},
		CredentialRef: "private-reference",
		Kubernetes:    clusterregistry.KubernetesConnection{APIEndpoint: "https://cluster.example.com"},
	})
	require.NoError(t, err)
	router, err := NewRouter(nil, nil, nil, nil, keycloak.Config{
		Client: workspaceTokenDecoder{},
		Issuer: testIssuer,
	}, &clusters.Registry{
		Registrations:   registry,
		ConnectionTests: connectiontest.NewService(registry, store, nil),
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/v1/workspaces/test-workspace/registrations/"+cluster.ID.String()+":test", nil)
	req.Header.Set("Authorization", "Bearer alice")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.JSONEq(t, `{"error":"Service Unavailable"}`, rec.Body.String())
	stored, err := store.Get(t.Context(), cluster.Workspace, cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, cluster, stored)
}
