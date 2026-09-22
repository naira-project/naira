// Integration test for the openai_api_models plugin.
// See the godoc of TestOpenAIAPIModels_Integration for more details.
//
// For an overview on integration tests philosophy in the project,
// see: docs/integration-tests.md.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/naira-project/naira/plugins/pkg/oidctest"
)

const (
	integrationTestTimeout = 5 * time.Minute
	readinessTimeout       = 30 * time.Second
	dialAttemptTimeout     = time.Second
	pollInterval           = 200 * time.Millisecond

	pluginConnectionTimeout = 20 * time.Second
	pluginRunTimeout        = 30 * time.Second

	litellmImage     = "ghcr.io/berriai/litellm:v1.97.0"
	litellmPort      = "4000/tcp"
	litellmMasterKey = "sk-test-master-key"

	// litellmConfig declares two fake models. Their litellm_params point at
	// an unreachable api_base: the test never asks litellm to complete a
	// request, only to report the model_list via GET /v1/models, so the
	// upstream "provider" is never actually contacted.
	litellmConfig = `
model_list:
  - model_name: fake-model-1
    litellm_params:
      model: openai/fake-model-1
      api_key: fake-key
      api_base: http://127.0.0.1:9/fake
  - model_name: fake-model-2
    litellm_params:
      model: openai/fake-model-2
      api_key: fake-key
      api_base: http://127.0.0.1:9/fake
`
)

// TestOpenAIAPIModels_Integration tests a real binary of the
// openai_api_models plugin against its "neighbor" components:
//
//   - a real binary of the catalog,
//   - a real LiteLLM server (chosen as a widely used, real implementation of
//     an OpenAI API-compatible /v1/models endpoint).
//
// The plugin & catalog binaries are built as part of the test (should be
// mostly cached on repeated runs).
//
// Test input: LiteLLM is started with a minimal config declaring two fake
// models and a predefined master API key.
//
// Test output assertion: the catalog API should show 2 Model nodes, one per
// LiteLLM model, with an "owned_by" property.
func TestOpenAIAPIModels_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(t.Context(), integrationTestTimeout)
	defer cancel()

	// Start a real LiteLLM server, seeded with 2 fake models.
	litellmBaseURL := startLiteLLM(t, ctx)

	// Start a mock OIDC (i.e. Keycloak-like) server.
	oidc := oidctest.New(t, "test-realm")

	// Build and start the plugin.
	pluginPort := findFreePort(t)
	buildAndStart(t, ctx, "github.com/naira-project/naira/plugins/cmd/openai_api_models", []string{
		"PORT=" + fmt.Sprint(pluginPort),
		"PATH_PREFIX=litellm1",
		"OPENAI_API_MODELS_BASE_URL=" + litellmBaseURL,
		"OPENAI_API_MODELS_API_KEY=" + litellmMasterKey,
	})
	pluginAddr := fmt.Sprintf("127.0.0.1:%d", pluginPort)
	require.Eventually(t, func() bool {
		return checkTCPReady(ctx, pluginAddr)
	}, readinessTimeout, pollInterval, "plugin at %s didn't start accepting connections", pluginAddr)

	// Build and start catalog.
	catalogPort := findFreePort(t)
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(fmt.Sprintf(`
plugins:
  openai_api_models:
    address: "%s"
`, pluginAddr)), 0o600), "writing config.yaml")
	buildAndStart(t, ctx, "github.com/naira-project/naira/catalog/cmd/catalog", []string{
		"PORT=" + fmt.Sprint(catalogPort),
		"PLUGIN_CONFIG_FILE=" + configPath,
		"PLUGIN_CONNECTION_TIMEOUT=" + pluginConnectionTimeout.String(),
		"PLUGIN_TIMEOUT=" + pluginRunTimeout.String(),
		"KEYCLOAK_BASE_URL=" + oidc.BaseURL,
		"KEYCLOAK_REALM=test-realm",
		"KEYCLOAK_ISSUER=" + oidc.Issuer,
	})
	catalogBaseURL := fmt.Sprintf("http://127.0.0.1:%d", catalogPort)
	require.Eventually(t, func() bool {
		return checkHTTPReady(ctx, catalogBaseURL+"/healthz")
	}, readinessTimeout, pollInterval, "catalog didn't become ready")

	// Generate an access token for authenticating to catalog's HTTP API.
	token := oidc.SignAccessToken(t, "test-user")

	// Trigger a run of the plugin through the catalog, and wait for the
	// operation to succeed.
	operationID := requestPluginRun(t, ctx, catalogBaseURL, token)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		op, status := doJSON[apiOperation](c, ctx, http.MethodGet, catalogBaseURL+"/v1/operations/"+operationID, token)
		assert.Equal(c, http.StatusOK, status, "GET /v1/operations/%s", operationID)
		assert.Equal(c, "SUCCEEDED", op.State, operationErrorMessage(op))
	}, readinessTimeout, pollInterval, "operation %q didn't succeed", operationID)

	//
	// Verify nodes in the catalog API.
	//

	type node struct {
		Kind string `json:"kind"`
		Path string `json:"path"`
	}
	nodes, status := doJSON[struct {
		Nodes []node `json:"nodes"`
	}](t, ctx, http.MethodGet, catalogBaseURL+"/v1/nodes", token)
	assert.Equal(t, http.StatusOK, status, "GET /v1/nodes")
	assert.ElementsMatch(t, nodes.Nodes, []node{
		{Kind: "model", Path: "litellm1/fake-model-1"},
		{Kind: "model", Path: "litellm1/fake-model-2"},
	})
}

// startLiteLLM starts a real LiteLLM server container seeded with 2 fake
// models and a predefined master API key, and returns its base URL.
func startLiteLLM(t *testing.T, ctx context.Context) string {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(litellmConfig), 0o600), "writing litellm config.yaml")

	ctr, err := testcontainers.Run(ctx, litellmImage,
		testcontainers.WithExposedPorts(litellmPort),
		testcontainers.WithEnv(map[string]string{
			"LITELLM_MASTER_KEY": litellmMasterKey,
		}),
		testcontainers.WithFiles(testcontainers.ContainerFile{
			HostFilePath:      configPath,
			ContainerFilePath: "/app/config.yaml",
			FileMode:          0o444,
		}),
		testcontainers.WithCmd("--config", "/app/config.yaml", "--port", "4000", "--host", "0.0.0.0"),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/health/readiness").WithPort(litellmPort)),
	)
	require.NoError(t, err, "starting litellm container")
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })

	endpoint, err := ctr.PortEndpoint(ctx, litellmPort, "http")
	require.NoError(t, err, "getting litellm endpoint")

	return endpoint
}

// buildAndStart builds pkg, trying to use the same command line used in our
// Dockerfiles, then starts the resulting binary as a background process with
// extraEnv appended to the current environment. The process is killed if ctx
// is done or on test cleanup.
func buildAndStart(t *testing.T, ctx context.Context, pkg string, extraEnv []string) {
	t.Helper()

	binary := filepath.Join(t.TempDir(), filepath.Base(pkg))

	build := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", binary, pkg)
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	output, err := build.CombinedOutput()
	require.NoError(t, err, "building %s:\n%s", pkg, output)

	run := exec.CommandContext(ctx, binary)
	run.Env = append(os.Environ(), extraEnv...)
	run.Stdout = os.Stdout
	run.Stderr = os.Stderr
	require.NoError(t, run.Start(), "starting %s", binary)
	t.Cleanup(func() {
		_ = run.Process.Kill()
		_ = run.Wait()
	})
}

func findFreePort(t *testing.T) int {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// checkTCPReady reports whether addr accepts a connection within
// dialAttemptTimeout.
func checkTCPReady(ctx context.Context, addr string) bool {
	ctx, cancel := context.WithTimeout(ctx, dialAttemptTimeout)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// checkHTTPReady reports whether a GET to url succeeds with a 200 within
// dialAttemptTimeout.
func checkHTTPReady(ctx context.Context, url string) bool {
	ctx, cancel := context.WithTimeout(ctx, dialAttemptTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

type apiOperation struct {
	Name  string `json:"name"`
	State string `json:"state"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func requestPluginRun(t *testing.T, ctx context.Context, catalogBaseURL, token string) string {
	t.Helper()

	op, status := doJSON[apiOperation](t, ctx, http.MethodPost, catalogBaseURL+"/v1/plugins/openai_api_models:run", token)
	require.Equal(t, http.StatusAccepted, status, "POST /v1/plugins/openai_api_models:run")
	require.NotEmpty(t, op.Name)
	return op.Name
}

func operationErrorMessage(op apiOperation) string {
	if op.Error == nil {
		return ""
	}
	return op.Error.Message
}

func doJSON[T any](t require.TestingT, ctx context.Context, method, url, token string) (T, int) {
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}

	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	var v T
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "reading response body: %s", string(body))
	require.NoError(t, json.Unmarshal(body, &v), "unmarshaling response body: %s", string(body))
	return v, resp.StatusCode
}
