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

func catalogURL(t *testing.T) string {
	t.Helper()
	if u := os.Getenv("CATALOG_URL"); u != "" {
		return u
	}
	return "http://localhost:8090"
}

func httpClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

var (
	authTokenOnce  sync.Once
	authTokenValue string
)

// authToken fetches an OAuth2 password-grant token from the realm/client/user
// keycloak.yaml seeds — the catalog API requires a Bearer token on every
// /v1/* route. Fetched once and reused across tests. Only KEYCLOAK_URL is
// ever overridden (by create-environment.sh/e2e.yml, via port-forward) — the
// realm/client/user are fixed by keycloak.yaml's realm import, so they're
// literal constants rather than configurable overrides nothing ever sets.
func authToken(t *testing.T) string {
	t.Helper()
	authTokenOnce.Do(func() {
		keycloakURL := envOr("KEYCLOAK_URL", "http://localhost:8080")
		form := url.Values{
			"grant_type":    {"password"},
			"client_id":     {keycloakClientID},
			"client_secret": {keycloakClientSecret},
			"username":      {keycloakUsername},
			"password":      {keycloakPassword},
		}
		tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", keycloakURL, keycloakRealm)

		resp, err := httpClient().Post(tokenURL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		if err != nil {
			t.Fatalf("POST %s: %v", tokenURL, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST %s: status %d", tokenURL, resp.StatusCode)
		}
		var body struct {
			AccessToken string `json:"access_token"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("POST %s: decoding response: %v", tokenURL, err)
		}
		authTokenValue = body.AccessToken
	})
	return authTokenValue
}

func getJSON(t *testing.T, path string, out any) {
	t.Helper()
	fullURL := catalogURL(t) + path
	req, err := http.NewRequest(http.MethodGet, fullURL, nil)
	if err != nil {
		t.Fatalf("GET %s: building request: %v", fullURL, err)
	}
	req.Header.Set("Authorization", "Bearer "+authToken(t))

	resp, err := httpClient().Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", fullURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", fullURL, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("GET %s: decoding response: %v", fullURL, err)
	}
}

func TestHealthz(t *testing.T) {
	resp, err := httpClient().Get(catalogURL(t) + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /healthz: status %d, want 200", resp.StatusCode)
	}
}

// The litellm plugin discovers every model litellm.yaml's static config
// lists (see components/litellm.yaml) — exactly "openai" and "mistral",
// under the litellm1 PATH_PREFIX both plugins in this scenario share.
func TestLitellmModelsSeeded(t *testing.T) {
	filter := url.QueryEscape(`kind="model"`)
	var got listNodesResponse
	getJSON(t, fmt.Sprintf("/v1/nodes?filter=%s", filter), &got)

	want := map[string]bool{"litellm1/openai": true, "litellm1/mistral": true}
	if len(got.Nodes) != len(want) {
		t.Fatalf("model nodes = %d, want exactly %d (got %+v)", len(got.Nodes), len(want), got.Nodes)
	}
	for _, n := range got.Nodes {
		if !want[n.Path] {
			t.Errorf("unexpected model node path %q", n.Path)
			continue
		}
		delete(want, n.Path)
	}
	for path := range want {
		t.Errorf("missing model node path %q", path)
	}
}

// depl_uses_litellm discovers chatbot1's Deployment (its Secret's key
// authenticates against litellm1 — see components/chatbot1.yaml) and emits
// exactly one uses_model edge to the "openai" model it's configured to use.
func TestChatbotUsesLitellmModel(t *testing.T) {
	filter := url.QueryEscape(`kind="uses_model"`)
	var got listRelationsResponse
	getJSON(t, fmt.Sprintf("/v1/relations?filter=%s", filter), &got)

	if len(got.Relations) != 1 {
		t.Fatalf("uses_model relations = %d, want exactly 1 (got %+v)", len(got.Relations), got.Relations)
	}
	rel := got.Relations[0]
	if !strings.HasSuffix(rel.FromNode, "/chatbot1") {
		t.Errorf("fromNode = %q, want it to end in %q", rel.FromNode, "/chatbot1")
	}
	if rel.ToNode != "nodes/model/litellm1/openai" {
		t.Errorf("toNode = %q, want %q", rel.ToNode, "nodes/model/litellm1/openai")
	}
}
