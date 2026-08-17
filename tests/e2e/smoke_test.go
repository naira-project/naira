//go:build e2e

// Package e2e holds smoke tests that run against a live, seeded environment
// created by deploy/e2e/scripts/create-environment.sh. Excluded from the
// default `go test ./...` build via the e2e build tag — run explicitly with
// `go test -tags e2e ./tests/e2e/...`.
//
// The `full` deployment profile means every dependency is empty except for
// what the seed Job put there, so these assertions can be exact (a specific
// count, a specific node) rather than mere existence checks. See the RFC's
// "Isolation Scope" for why that distinction matters.
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

type node struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type listNodesResponse struct {
	Nodes     []node `json:"nodes"`
	TotalSize int32  `json:"totalSize"`
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

// authToken fetches an OAuth2 password-grant token from the same
// realm/client/user the seed Job uses (see deploy/e2e/seed/trigger_catalog_sync.py
// and deploy/e2e/keycloak.yaml) — the catalog API requires a Bearer token on
// every /v1/* route. Fetched once and reused across tests.
func authToken(t *testing.T) string {
	t.Helper()
	authTokenOnce.Do(func() {
		keycloakURL := envOr("KEYCLOAK_URL", "http://localhost:8080")
		form := url.Values{
			"grant_type":    {"password"},
			"client_id":     {envOr("KEYCLOAK_CLIENT_ID", "naira-portal")},
			"client_secret": {envOr("KEYCLOAK_CLIENT_SECRET", "naira-e2e-test-secret")},
			"username":      {envOr("KEYCLOAK_USERNAME", "testuser")},
			"password":      {envOr("KEYCLOAK_PASSWORD", "testpass")},
		}
		tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", keycloakURL, envOr("KEYCLOAK_REALM", "naira"))

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

// The MLflow seed script (register_dummy_model.py) registers exactly one
// model named "demo-model" under the "mlflow" plugin path prefix, into an
// MLflow instance that — under the `full` profile — belongs to this PR alone.
func TestMLflowModelSeeded(t *testing.T) {
	filter := url.QueryEscape(`path="mlflow/demo-model"`)
	var got listNodesResponse
	getJSON(t, fmt.Sprintf("/v1/nodes?filter=%s", filter), &got)

	if len(got.Nodes) != 1 {
		t.Fatalf("nodes matching mlflow/demo-model = %d, want exactly 1 (got %+v)", len(got.Nodes), got.Nodes)
	}
	if got.Nodes[0].Kind != "model" {
		t.Errorf("node kind = %q, want %q", got.Nodes[0].Kind, "model")
	}
}

// seed_sample_tables.py creates exactly 5 tables. Under `full` this OpenMetadata
// instance is exclusively this PR's, so the count is exact — not "at least 5".
func TestOpenMetadataTablesSeeded(t *testing.T) {
	filter := url.QueryEscape(`kind="dataset"`)
	var got listNodesResponse
	getJSON(t, fmt.Sprintf("/v1/nodes?filter=%s", filter), &got)

	const wantTables = 5
	if len(got.Nodes) != wantTables {
		t.Fatalf("dataset nodes = %d, want exactly %d (got %+v)", len(got.Nodes), wantTables, got.Nodes)
	}
}
