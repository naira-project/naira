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

	litellmImage = "ghcr.io/berriai/litellm:v1.97.0"

	litellmMasterKey = "sk-test-master-key"
	llamaCppAPIKey   = "test-llamacpp-apikey"
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
func TestOpenAIAPIModels_Integration_LiteLLM(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(t.Context(), integrationTestTimeout)
	defer cancel()

	// Start a real LiteLLM server, seeded with 2 fake models. Their
	// litellm_params point at an unreachable api_base: the test never asks
	// litellm to complete a request, only to report the model_list via GET
	// /v1/models, so the upstream "provider" is never actually contacted.
	const litellmPort = "4000/tcp"
	litellmConfigPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(litellmConfigPath, []byte(`
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
`), 0o600), "writing litellm config.yaml")
	litellmCtr, err := testcontainers.Run(ctx, litellmImage,
		testcontainers.WithExposedPorts(litellmPort),
		testcontainers.WithEnv(map[string]string{
			"LITELLM_MASTER_KEY": litellmMasterKey,
		}),
		testcontainers.WithFiles(testcontainers.ContainerFile{
			HostFilePath:      litellmConfigPath,
			ContainerFilePath: "/app/config.yaml",
			FileMode:          0o444,
		}),
		testcontainers.WithCmd("--config", "/app/config.yaml", "--port", "4000", "--host", "0.0.0.0"),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/health/readiness").WithPort(litellmPort)),
	)
	require.NoError(t, err, "starting litellm container")
	t.Cleanup(func() { _ = litellmCtr.Terminate(context.Background()) })
	litellmBaseURL, err := litellmCtr.PortEndpoint(ctx, litellmPort, "http")
	require.NoError(t, err, "getting litellm endpoint")

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

// TestOpenAIAPIModels_Integration_LlamaCpp tests a real binary of the
// openai_api_models plugin against its "neighbor" components:
//
//   - a real binary of the catalog,
//   - a real llama.cpp server (chosen as another widely used, real
//     implementation of an OpenAI API-compatible /v1/models endpoint, distinct
//     from LiteLLM in how it reports models: through --alias, and with a
//     fixed "llamacpp" owned_by).
//
// The plugin & catalog binaries are built as part of the test (should be
// mostly cached on repeated runs).
//
// Test input: llama.cpp is started with the tiny "stories260K" placeholder
// model (llama.cpp's own CI test model) and a predefined API key.
//
// Test output assertion: the catalog API should show 1 Model node, with an
// "owned_by" property of "llamacpp".
func TestOpenAIAPIModels_Integration_LlamaCpp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(t.Context(), integrationTestTimeout)
	defer cancel()

	// Start a real llama.cpp server, seeded with a tiny dummy model. The
	// model is baked into the image via llamaCppDockerfile rather than
	// mounted from the host: Docker then caches the download as an image
	// layer (keyed on the Dockerfile content), so repeated test runs don't
	// re-download it, without leaving anything in a host-side cache
	// directory.
	dockerfileDir := t.TempDir()
	const llamaCppPort = "8080/tcp"
	require.NoError(t, os.WriteFile(filepath.Join(dockerfileDir, "Dockerfile"), []byte(`
FROM ghcr.io/ggml-org/llama.cpp:server-b11096

# Download a tiny ~1.2MB model (used in llama.cpp's own CI).
RUN mkdir -p /models && \
  curl -fsSL -o /models/test-model.gguf \
    https://huggingface.co/ggml-org/models/resolve/499bc8821c6b12b4e53c5bffcb21ec206f212d81/tinyllamas/stories260K.gguf
`), 0o600), "writing llama.cpp Dockerfile")
	llamaCppCtr, err := testcontainers.Run(ctx, "",
		testcontainers.WithDockerfile(testcontainers.FromDockerfile{
			Context: dockerfileDir,
		}),
		testcontainers.WithExposedPorts(llamaCppPort),
		testcontainers.WithCmd(
			"-m", "/models/test-model.gguf",
			"--host", "0.0.0.0",
			"--port", "8080",
			"--alias", "test-model-name",
			"--api-key", llamaCppAPIKey,
			"--no-webui",
			"-c", "512",
		),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/health").WithPort(llamaCppPort)),
	)
	require.NoError(t, err, "starting llama.cpp container")
	t.Cleanup(func() { _ = llamaCppCtr.Terminate(context.Background()) })
	llamaCppBaseURL, err := llamaCppCtr.PortEndpoint(ctx, llamaCppPort, "http")
	require.NoError(t, err, "getting llama.cpp endpoint")

	// Start a mock OIDC (i.e. Keycloak-like) server.
	oidc := oidctest.New(t, "test-realm")

	// Build and start the plugin.
	pluginPort := findFreePort(t)
	buildAndStart(t, ctx, "github.com/naira-project/naira/plugins/cmd/openai_api_models", []string{
		"PORT=" + fmt.Sprint(pluginPort),
		"PATH_PREFIX=llamacpp1",
		"OPENAI_API_MODELS_BASE_URL=" + llamaCppBaseURL,
		"OPENAI_API_MODELS_API_KEY=" + llamaCppAPIKey,
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
		Kind         string        `json:"kind"`
		Path         string        `json:"path"`
		PluginClaims []pluginClaim `json:"pluginClaims"`
	}
	nodes, status := doJSON[struct {
		Nodes []node `json:"nodes"`
	}](t, ctx, http.MethodGet, catalogBaseURL+"/v1/nodes", token)
	assert.Equal(t, http.StatusOK, status, "GET /v1/nodes")
	require.Len(t, nodes.Nodes, 1)
	assert.Equal(t, "model", nodes.Nodes[0].Kind)
	assert.Equal(t, "llamacpp1/test-model-name", nodes.Nodes[0].Path)
	require.Len(t, nodes.Nodes[0].PluginClaims, 1)
	assert.Equal(t, "llamacpp", nodes.Nodes[0].PluginClaims[0].Props["owned_by"])
}

type pluginClaim struct {
	Plugin string            `json:"plugin"`
	Props  map[string]string `json:"props"`
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
