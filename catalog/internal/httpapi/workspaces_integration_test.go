package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Nerzal/gocloak/v13"
	"github.com/naira-project/naira/catalog/internal/auth/keycloak"
	"github.com/naira-project/naira/catalog/internal/sources"
	"github.com/naira-project/naira/catalog/internal/workspaces"
	"github.com/naira-project/naira/plugins/pkg/oidctest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceAPIWithSignedTokens(t *testing.T) {
	oidc := oidctest.New(t, "workspaces")
	workspaceService := workspaces.NewService(workspaces.NewMemoryStore())
	router, err := NewRouter(nil, nil, nil, nil, keycloak.Config{
		Client: gocloak.NewClient(oidc.BaseURL),
		Realm:  "workspaces",
		Issuer: oidc.Issuer,
	}, workspaceService, sources.NewService(sources.NewMemoryStore(), workspaceService))
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
	status, body := call(alice, http.MethodPost, "/v1/workspaces", `{"name":"Production"}`)
	require.Equal(t, http.StatusCreated, status, body)
	var workspace workspaceResource
	require.NoError(t, json.Unmarshal([]byte(body), &workspace))
	path := "/v1/workspaces/" + workspace.ID.String() + "/registrations"
	status, body = call(alice, http.MethodPost, path, `{"name":"Cluster A","kind":"kubernetes","credentialRef":"private-reference","kubernetes":{"apiEndpoint":"https://cluster.example.com"}}`)
	require.Equal(t, http.StatusCreated, status, body)
	assert.NotContains(t, body, "private-reference")
	status, body = call(bob, http.MethodGet, path, "")
	require.Equal(t, http.StatusForbidden, status)
	assert.JSONEq(t, `{"error":"Forbidden"}`, body)
	status, body = call(bob, http.MethodGet, "/v1/workspaces", "")
	require.Equal(t, http.StatusOK, status)
	assert.JSONEq(t, `{"workspaces":[]}`, body)

	untrustedIssuer := oidctest.New(t, "workspaces")
	status, _ = call(untrustedIssuer.SignAccessToken(t, "alice"), http.MethodGet, path, "")
	assert.Equal(t, http.StatusUnauthorized, status)
}
