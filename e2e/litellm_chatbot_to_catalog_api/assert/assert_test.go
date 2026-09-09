//go:build e2e

// Package e2e asserts against a live, seeded environment created by
// e2e/scripts/create-environment.sh --scenario litellm_chatbot_to_catalog_api.
// Excluded from the default `go test ./...` build via the e2e build tag —
// run explicitly with `go test -tags e2e ./e2e/litellm_chatbot_to_catalog_api/assert/...`.
//
// This scenario deploys exactly the litellm and depl_uses_litellm plugins
// against a fresh, empty catalog (see components.env), so these assertions
// can be exact (a specific count, a specific edge) rather than mere
// existence checks: nothing else is running that could add unrelated nodes
// or relations to the graph.
package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	keycloakClientID     = "naira-portal"
	keycloakClientSecret = "naira-e2e-test-secret"
	keycloakUsername     = "testuser"
	keycloakPassword     = "testpass"
	keycloakRealm        = "naira"
)

type node struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type listNodesResponse struct {
	Nodes     []node `json:"nodes"`
	TotalSize int32  `json:"totalSize"`
}

type relation struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	FromNode string `json:"fromNode"`
	ToNode   string `json:"toNode"`
}

type listRelationsResponse struct {
	Relations []relation `json:"relations"`
	TotalSize int32      `json:"totalSize"`
}

func httpClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

var (
	authTokenOnce  sync.Once
	authTokenValue string
)

// authToken fetches an OAuth2 password-grant token from the realm/client/user
// keycloak.yaml seeds — the catalog API requires a Bearer token on every
// /v1/* route. Fetched once and reused across tests. Only KEYCLOAK_URL is
// overridden (by create-environment.sh/e2e.yml, via port-forward) — the
// realm/client/user are fixed by keycloak.yaml's realm import.
func authToken(t *testing.T) string {
	t.Helper()
	authTokenOnce.Do(func() {
		keycloakURL := os.Getenv("KEYCLOAK_URL")
		form := url.Values{
			"grant_type":    {"password"},
			"client_id":     {keycloakClientID},
			"client_secret": {keycloakClientSecret},
			"username":      {keycloakUsername},
			"password":      {keycloakPassword},
		}
		tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", keycloakURL, keycloakRealm)

		resp, err := httpClient().Post(tokenURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)
		var body struct {
			AccessToken string `json:"access_token"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		authTokenValue = body.AccessToken
	})
	return authTokenValue
}

func getJSON(t *testing.T, path string, out any) {
	t.Helper()
	fullURL := os.Getenv("CATALOG_URL") + path
	req, err := http.NewRequest(http.MethodGet, fullURL, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+authToken(t))

	resp, err := httpClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(out))
}

// The litellm plugin discovers every model litellm.yaml's static config
// lists (see components/litellm.yaml) — exactly "openai" and "mistral",
// under the litellm1 PATH_PREFIX both plugins in this scenario share.
func TestLitellmModelsNodes(t *testing.T) {
	filter := url.QueryEscape(`kind="model"`)
	var response listNodesResponse
	getJSON(t, fmt.Sprintf("/v1/nodes?filter=%s", filter), &response)

	wantPaths := []string{"litellm1/openai", "litellm1/mistral"}
	var paths []string
	for _, n := range response.Nodes {
		paths = append(paths, n.Path)
	}
	assert.ElementsMatchf(t, wantPaths, paths, "full response: %#v", response)
}

// depl_uses_litellm discovers chatbot1's Deployment (its Secret's key
// authenticates against litellm1 — see components/chatbot1.yaml) and emits
// exactly one uses_model edge to the "openai" model it's configured to use.
func TestChatbotUsesLitellmModel(t *testing.T) {
	filter := url.QueryEscape(`kind="uses_model"`)
	var got listRelationsResponse
	getJSON(t, fmt.Sprintf("/v1/relations?filter=%s", filter), &got)

	require.Len(t, got.Relations, 1)
	rel := got.Relations[0]
	assert.Truef(t, strings.HasSuffix(rel.FromNode, "/chatbot1"),
		"fromNode = %q, want it to end in %q", rel.FromNode, "/chatbot1")
	assert.Equal(t, "nodes/model/litellm1/openai", rel.ToNode)
}
