package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Nerzal/gocloak/v13"
	"github.com/naira-project/naira/catalog/internal/auth/keycloak"
	"github.com/naira-project/naira/plugins/pkg/oidctest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceAPIWithSignedTokens(t *testing.T) {
	oidc := oidctest.New(t, "workspaces")
	router, err := NewRouter(nil, nil, nil, nil, keycloak.Config{
		Client: gocloak.NewClient(oidc.BaseURL),
		Realm:  "workspaces",
		Issuer: oidc.Issuer,
	}, newTestClusterRegistry(t))
	require.NoError(t, err)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	alice := oidc.SignAccessToken(t, "alice")
	bob := oidc.SignAccessToken(t, "bob")
	call := func(token, method, path, body string) (int, string) {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), method, server.URL+path, strings.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token)
		response, err := server.Client().Do(req)
		require.NoError(t, err)
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		return response.StatusCode, string(data)
	}
	status, body := call(alice, http.MethodGet, "/v1/workspaces", "")
	require.Equal(t, http.StatusOK, status, body)
	assert.JSONEq(t, `{"workspaces":[{"id":"test-workspace","name":"test-workspace"}]}`, body)
	path := "/v1/workspaces/test-workspace/registrations"
	status, body = call(alice, http.MethodPost, path, `{"name":"Cluster A","credentialRef":"private-reference","kubernetes":{"apiEndpoint":"https://cluster.example.com"}}`)
	require.Equal(t, http.StatusCreated, status, body)
	assert.NotContains(t, body, "private-reference")
	status, body = call(bob, http.MethodGet, path, "")
	require.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, "Cluster A")
	assert.NotContains(t, body, "private-reference")
	status, body = call(bob, http.MethodGet, "/v1/workspaces", "")
	require.Equal(t, http.StatusOK, status)
	assert.JSONEq(t, `{"workspaces":[{"id":"test-workspace","name":"test-workspace"}]}`, body)
	status, _ = call(alice, http.MethodPost, "/v1/workspaces", `{"name":"Production"}`)
	assert.Equal(t, http.StatusMethodNotAllowed, status)
	status, body = call(bob, http.MethodGet, "/v1/workspaces/other/registrations", "")
	assert.Equal(t, http.StatusNotFound, status)
	assert.JSONEq(t, `{"error":"Not Found"}`, body)

	untrustedIssuer := oidctest.New(t, "workspaces")
	status, _ = call(untrustedIssuer.SignAccessToken(t, "alice"), http.MethodGet, path, "")
	assert.Equal(t, http.StatusUnauthorized, status)
}
