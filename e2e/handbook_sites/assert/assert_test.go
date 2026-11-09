//go:build e2e

// Package e2e asserts against a live, seeded environment created by
// e2e/scripts/create-environment.sh --scenario handbook_sites. Excluded from the
// default `go test ./...` build via the e2e build tag — run explicitly with
// `go test -tags e2e ./e2e/handbook_sites/assert/...`.
//
// The scenario installs two handbooks, `starter` and `starter-two`, behind the
// chart's handbook gateway. The checks run at HTTP level: the test connects to the
// gateway through a port-forward and sets the Host header of the handbook it wants,
// as a browser would reach it by name. Deep links, anchors and search need a
// browser and stay manual.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
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

	// Keep in sync with e2e/components/handbook-sites-values.yaml.
	handbookDomain   = "handbook.localhost"
	handbookPort     = "3003"
	portalOrigin = "http://localhost:3000"

	starterTitle = "Naira Handbook Starter"
)

var handbookIDs = []string{"starter", "starter-two"}

// ctx is t.Context() without its cancellation: that context is canceled before
// cleanup functions run, and the requests below are also made from one.
func ctx(t *testing.T) context.Context {
	return context.WithoutCancel(t.Context())
}

func httpClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second}
}

var (
	authTokenOnce  sync.Once
	authTokenValue string
)

// authToken fetches an OAuth2 password-grant token from the realm keycloak.yaml
// imports, the same as the litellm_chatbot_to_catalog_api scenario does. Its
// issuer is the in-cluster Keycloak address, which is also what the gateway is
// told to expect.
func authToken(t *testing.T) string {
	t.Helper()
	authTokenOnce.Do(func() {
		form := url.Values{
			"grant_type":    {"password"},
			"client_id":     {keycloakClientID},
			"client_secret": {keycloakClientSecret},
			"username":      {keycloakUsername},
			"password":      {keycloakPassword},
		}
		tokenURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", os.Getenv("KEYCLOAK_URL"), keycloakRealm)

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
	// Once has run already when an earlier test failed to get the token.
	require.NotEmpty(t, authTokenValue, "no Keycloak token; the first test to ask for one shows why")
	return authTokenValue
}

// gatewayRequest sends a request to the gateway for the handbook with the given id
// and returns the status, the headers and the body. headers are added as given.
func gatewayRequest(t *testing.T, method, id, path string, headers map[string]string) (int, http.Header, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx(t), method, os.Getenv("HANDBOOK_GATEWAY_URL")+path, nil)
	require.NoError(t, err)
	req.Host = fmt.Sprintf("%s.%s:%s", id, handbookDomain, handbookPort)
	for name, value := range headers {
		req.Header.Set(name, value)
	}

	resp, err := httpClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, resp.Header, string(body)
}

// signedIn returns the Cookie header the gateway's session answer asks the browser to keep.
func signedIn(t *testing.T, id string) map[string]string {
	t.Helper()
	status, header, _ := gatewayRequest(t, http.MethodPost, id, "/_naira/session", map[string]string{"x-naira-token": authToken(t)})
	require.Equal(t, http.StatusNoContent, status)
	cookie := strings.SplitN(header.Get("Set-Cookie"), ";", 2)[0]
	require.True(t, strings.HasPrefix(cookie, "naira_handbook="), "Set-Cookie = %q", header.Get("Set-Cookie"))
	return map[string]string{"Cookie": cookie}
}

// Without a session the gateway sends the browser to the handbook's own sign-in page
// and shows none of the handbook.
func TestWithoutSessionTheHandbookAsksToSignIn(t *testing.T) {
	for _, id := range handbookIDs {
		t.Run(id, func(t *testing.T) {
			status, header, body := gatewayRequest(t, http.MethodGet, id, "/guide/getting-started", nil)

			assert.Equal(t, http.StatusUnauthorized, status)
			assert.Contains(t, body, "/_naira/bootstrap.html?return=")
			assert.NotContains(t, body, starterTitle)
			assert.Equal(t, "no-store", header.Get("Cache-Control"))
		})
	}
}

// The sign-in page and its scripts are open, or a signed-out frame could not sign in.
// The handbook's metadata is not.
func TestOnlyTheSignInPageIsOpen(t *testing.T) {
	status, _, body := gatewayRequest(t, http.MethodGet, "starter", "/_naira/bootstrap.html", nil)
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, body, "/_naira/assets/bootstrap.js")

	status, _, _ = gatewayRequest(t, http.MethodGet, "starter", "/naira-handbook.json", nil)
	assert.Equal(t, http.StatusUnauthorized, status)
}

// A real Keycloak token opens a session, and the cookie it sets opens the handbook.
func TestSessionOpensTheHandbook(t *testing.T) {
	for _, id := range handbookIDs {
		t.Run(id, func(t *testing.T) {
			status, header, _ := gatewayRequest(t, http.MethodPost, id, "/_naira/session", map[string]string{"x-naira-token": authToken(t)})
			assert.Equal(t, http.StatusNoContent, status)
			// No Domain attribute: the browser keeps the cookie for this handbook's address only.
			assert.Equal(t, "naira_handbook="+authToken(t)+"; HttpOnly; SameSite=Lax; Path=/", header.Get("Set-Cookie"))

			cookie := signedIn(t, id)
			status, header, body := gatewayRequest(t, http.MethodGet, id, "/", cookie)
			assert.Equal(t, http.StatusOK, status)
			assert.Contains(t, body, starterTitle)
			assert.Equal(t, "frame-ancestors "+portalOrigin, header.Get("Content-Security-Policy"))
			assert.Empty(t, header.Get("Server"))

			status, _, body = gatewayRequest(t, http.MethodGet, id, "/naira-handbook.json", cookie)
			require.Equal(t, http.StatusOK, status)
			var metadata struct {
				Contract string `json:"contract"`
			}
			require.NoError(t, json.Unmarshal([]byte(body), &metadata))
			assert.Equal(t, "naira.handbook/v1alpha1", metadata.Contract)
		})
	}
}

// A token is not a way in anywhere but the session path.
func TestTokenIsAcceptedOnlyAsSessionCookie(t *testing.T) {
	token := authToken(t)
	for name, headers := range map[string]map[string]string{
		"Authorization":  {"Authorization": "Bearer " + token},
		"x-naira-token":  {"x-naira-token": token},
		"a broken token": {"Cookie": "naira_handbook=" + token[:len(token)-4] + "AAAA"},
	} {
		t.Run(name, func(t *testing.T) {
			status, _, _ := gatewayRequest(t, http.MethodGet, "starter", "/", headers)
			assert.Equal(t, http.StatusUnauthorized, status)
		})
	}
}

// A host that is no handbook's address gets nothing, with or without a session.
func TestUnknownHostIsNotFound(t *testing.T) {
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, os.Getenv("HANDBOOK_GATEWAY_URL")+"/", nil)
	require.NoError(t, err)
	req.Host = "nope." + handbookDomain + ":" + handbookPort
	req.Header.Set("Cookie", "naira_handbook="+authToken(t))

	resp, err := httpClient().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func scaleHandbook(t *testing.T, id string, replicas int) {
	t.Helper()
	out, err := exec.CommandContext(ctx(t), "kubectl", "-n", os.Getenv("E2E_NAMESPACE"),
		"scale", "deploy/handbook-"+id, fmt.Sprintf("--replicas=%d", replicas)).CombinedOutput()
	require.NoErrorf(t, err, "kubectl scale: %s", out)
}

func handbookStatus(t *testing.T, id string, cookie map[string]string) int {
	t.Helper()
	status, _, _ := gatewayRequest(t, http.MethodGet, id, "/", cookie)
	return status
}

// A handbook that is down shows its own degraded page on its own address and takes
// no other handbook with it.
func TestStoppedHandbookShowsDegradedPageAndOthersKeepWorking(t *testing.T) {
	cookies := map[string]map[string]string{
		"starter":     signedIn(t, "starter"),
		"starter-two": signedIn(t, "starter-two"),
	}
	require.Equal(t, http.StatusOK, handbookStatus(t, "starter-two", cookies["starter-two"]))

	scaleHandbook(t, "starter-two", 0)
	t.Cleanup(func() {
		scaleHandbook(t, "starter-two", 1)
		assert.Eventually(t, func() bool { return handbookStatus(t, "starter-two", cookies["starter-two"]) == http.StatusOK },
			2*time.Minute, time.Second, "starter-two did not come back")
	})

	require.Eventually(t, func() bool {
		return handbookStatus(t, "starter-two", cookies["starter-two"]) == http.StatusServiceUnavailable
	},
		2*time.Minute, time.Second, "starter-two did not show the degraded page")

	status, header, body := gatewayRequest(t, http.MethodGet, "starter-two", "/", cookies["starter-two"])
	assert.Equal(t, http.StatusServiceUnavailable, status)
	assert.Contains(t, body, "<h1>Handbook Starter Two is unavailable</h1>")
	assert.NotContains(t, body, starterTitle)
	assert.Equal(t, "no-store", header.Get("Cache-Control"))

	assert.Equal(t, http.StatusOK, handbookStatus(t, "starter", cookies["starter"]))
}

type portalConfig struct {
	Providers []struct {
		Nodes []struct {
			PathSegment string `json:"pathSegment"`
			Label       string `json:"label"`
			Order       int    `json:"order"`
			ViewURL     string `json:"viewUrl"`
			VirtualTree bool   `json:"virtualTree"`
			Category    *struct {
				ID string `json:"id"`
			} `json:"category"`
		} `json:"nodes"`
	} `json:"providers"`
}

// The portal lists the installed handbooks in the Docs group after its own pages,
// the primary handbook first, each opening at its own address.
func TestPortalListsTheHandbooksInOrder(t *testing.T) {
	resp, err := httpClient().Get(os.Getenv("PORTAL_URL") + "/rest/config")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var config portalConfig
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&config))
	require.Len(t, config.Providers, 1)

	type handbookNode struct {
		PathSegment string
		Label       string
		ViewURL     string
		VirtualTree bool
	}
	var docs []handbookNode
	lastPortalPageOrder, firstHandbookOrder := -1, -1
	for _, n := range config.Providers[0].Nodes {
		if n.Category == nil || n.Category.ID != "docs" {
			lastPortalPageOrder = max(lastPortalPageOrder, n.Order)
			continue
		}
		if firstHandbookOrder < 0 {
			firstHandbookOrder = n.Order
		}
		docs = append(docs, handbookNode{PathSegment: n.PathSegment, Label: n.Label, ViewURL: n.ViewURL, VirtualTree: n.VirtualTree})
	}

	// No trailing slash: Luigi appends "/:virtualSegment_1" to the url of a virtual tree.
	assert.Equal(t, []handbookNode{
		{PathSegment: "starter", Label: "Handbook Starter", ViewURL: "http://starter.handbook.localhost:3003", VirtualTree: true},
		{PathSegment: "starter-two", Label: "Handbook Starter Two", ViewURL: "http://starter-two.handbook.localhost:3003", VirtualTree: true},
	}, docs)
	assert.Greater(t, firstHandbookOrder, lastPortalPageOrder)
}
