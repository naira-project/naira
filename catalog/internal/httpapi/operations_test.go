package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/naira-project/naira/catalog/internal/auth/keycloak"
	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/operations"
	"github.com/naira-project/naira/catalog/internal/pluginrun"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunPluginFlowJSONContract(t *testing.T) {
	// Names are server-generated (plugin-run-<uuid>)
	opNamePattern := regexp.MustCompile(`^plugin-run-[0-9a-f-]{36}$`)

	tests := []struct {
		name              string
		pluginImpl        pluginrun.Plugin
		waitForCompletion bool
		wantInitialJSON   string
		wantTerminalJSON  string
	}{
		{
			name:       "pending immediately after trigger",
			pluginImpl: stubPlugin{},
			wantInitialJSON: `{
				"name": "{{NAME}}",
				"done": false,
				"metadata": {
					"plugin": "mlflow", "state": "PENDING",
					"startTime": "{{START}}", "createdAt": "{{CREATED}}"
				}
			}`,
		},
		{
			name:              "succeeded after completion",
			pluginImpl:        stubPlugin{},
			waitForCompletion: true,
			wantTerminalJSON: `{
				"name": "{{NAME}}",
				"done": true,
				"metadata": {
					"plugin": "mlflow", "state": "SUCCEEDED",
					"startTime": "{{START}}", "endTime": "{{END}}", "createdAt": "{{CREATED}}"
				},
				"response": {"nodesUpserted": 0, "relationsUpserted": 0}
			}`,
		},
		{
			name:              "failed after completion",
			pluginImpl:        stubPlugin{err: errors.New("sync failed: database offline")},
			waitForCompletion: true,
			wantTerminalJSON: `{
				"name": "{{NAME}}",
				"done": true,
				"metadata": {
					"plugin": "mlflow", "state": "FAILED",
					"startTime": "{{START}}", "endTime": "{{END}}", "createdAt": "{{CREATED}}"
				},
				"error": {"message": "collecting response from plugin \"mlflow\": sync failed: database offline"}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opStore := operations.NewMemoryStore()
			router := newTestRouter(t, catalog.NewMemoryStore(), opStore, map[string]pluginrun.Plugin{
				"mlflow": tt.pluginImpl,
			})

			rec := postAuthorized(t, router, "/v1/plugins/mlflow:run")
			require.Equal(t, http.StatusAccepted, rec.Code)

			var opRes OperationResource
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &opRes))
			assert.Regexp(t, opNamePattern, opRes.Name)

			if tt.wantInitialJSON != "" {
				assertOperationJSON(t, tt.wantInitialJSON, rec.Body.Bytes())
			}
			if !tt.waitForCompletion {
				return
			}

			completed := waitForOperation(t, opStore, opRes.Name)

			getRec := getAuthorized(t, router, "/v1/operations/"+url.PathEscape(completed.Name))
			require.Equal(t, http.StatusOK, getRec.Code)
			assertOperationJSON(t, tt.wantTerminalJSON, getRec.Body.Bytes())
		})
	}
}

func TestRunPluginAsyncEndpointUnknownPlugin(t *testing.T) {
	router := newTestRouter(t, catalog.NewMemoryStore(), operations.NewMemoryStore(), nil)

	rec := postAuthorized(t, router, "/v1/plugins/missing:run")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRunPluginAsyncEndpointConflict(t *testing.T) {
	opStore := operations.NewMemoryStore()
	block := make(chan struct{})
	store := catalog.NewMemoryStore()
	catalogService := catalog.NewService(store)
	runner := pluginrun.NewRunner(context.Background(), store, opStore, map[string]pluginrun.Plugin{"mlflow": blockingStubPlugin{block: block}}, 5*time.Minute, log.New(io.Discard, "", 0))
	router, err := NewRouter(catalogService, runner, catalog.PluginConfigsByName{"mlflow": {}}, log.New(io.Discard, "", 0), keycloak.Config{Client: stubTokenDecoder{}, Issuer: testIssuer})
	require.NoError(t, err)

	rec1 := postAuthorized(t, router, "/v1/plugins/mlflow:run")
	assert.Equal(t, http.StatusAccepted, rec1.Code)

	var firstOp OperationResource
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &firstOp))
	waitForRunning(t, opStore, firstOp.Name)

	rec2 := postAuthorized(t, router, "/v1/plugins/mlflow:run")
	assert.Equal(t, http.StatusConflict, rec2.Code)

	close(block)
	runner.Wait()
}

func TestGetOperationsEndpoint(t *testing.T) {
	opStore := operations.NewMemoryStore()
	router := newTestRouter(t, catalog.NewMemoryStore(), opStore, map[string]pluginrun.Plugin{"seed": stubPlugin{}})

	opName := runSeedAndWait(t, router, opStore)

	rec := getAuthorized(t, router, "/v1/operations")
	assert.Equal(t, http.StatusOK, rec.Code)

	var listResp ListOperationsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	require.Len(t, listResp.Operations, 1)

	op := listResp.Operations[0]
	assertSucceededSeedOperation(t, op, opName)
	assert.NotNil(t, op.Response)
}

// --- helpers -----------------------------------------------------------

// postAuthorized sends an authenticated POST request with no body.
func postAuthorized(t *testing.T, router http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := withAuth(httptest.NewRequest(http.MethodPost, path, nil), testBearerToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// getAuthorized sends an authenticated GET request.
func getAuthorized(t *testing.T, router http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := withAuth(httptest.NewRequest(http.MethodGet, path, nil), testBearerToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// runSeedAndWait triggers the "seed" plugin via POST /v1/plugins:run and
// waits for its operation to reach a terminal state, returning the
// operation's name.
func runSeedAndWait(t *testing.T, router http.Handler, opStore operations.Store) string {
	t.Helper()

	rec := postAuthorized(t, router, "/v1/plugins:run")
	require.Equal(t, http.StatusAccepted, rec.Code)

	var runResp RunPluginsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &runResp))
	require.Len(t, runResp.Operations, 1)

	opName := runResp.Operations[0].Name
	waitForOperation(t, opStore, opName)
	return opName
}

// assertSucceededSeedOperation asserts the common fields of a completed
// "seed" plugin operation resource.
func assertSucceededSeedOperation(t *testing.T, op OperationResource, wantName string) {
	t.Helper()
	assert.Equal(t, wantName, op.Name)
	assert.Equal(t, "seed", op.Metadata.Plugin)
	assert.Equal(t, "SUCCEEDED", op.Metadata.State)
	assert.True(t, op.Done)
}

// waitForOperation polls the operation store until the operation reaches a
// terminal state (SUCCEEDED or FAILED) or the timeout elapses.
func waitForOperation(t *testing.T, opStore operations.Store, name string) operations.Operation {
	t.Helper()
	return waitForOperationState(t, opStore, name, func(op operations.Operation) bool {
		return op.State == operations.StateSucceeded || op.State == operations.StateFailed
	}, "a terminal state")
}

// waitForRunning polls the operation store until the operation reaches the
// RUNNING state or the timeout elapses.
func waitForRunning(t *testing.T, opStore operations.Store, name string) operations.Operation {
	t.Helper()
	return waitForOperationState(t, opStore, name, func(op operations.Operation) bool {
		return op.State == operations.StateRunning
	}, string(operations.StateRunning))
}

// waitForOperationState polls the operation store until want(op) is true or
// the timeout elapses, failing the test with a description of what it was
// waiting for otherwise.
func waitForOperationState(t *testing.T, opStore operations.Store, name string, want func(operations.Operation) bool, wantDescription string) operations.Operation {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		op, err := opStore.Get(name)
		if err == nil && want(op) {
			return op
		}
		time.Sleep(10 * time.Millisecond)
	}

	op, err := opStore.Get(name)
	require.NoError(t, err, "operation %q not found", name)
	t.Fatalf("operation %q state = %s, want %s", name, op.State, wantDescription)
	return operations.Operation{}
}

// assertOperationJSON compares actual JSON against wantTemplate.
//
// Dynamic placeholders ({{NAME}}, {{START}}, {{END}}, {{CREATED}}) represent
// unpredictable values such as server-generated IDs or timestamps. These placeholders
// are populated directly from the actual JSON payload.
//
// This validates the overall schema and predictable values without asserting on dynamic
// runtime identifiers or wall-clock times.
func assertOperationJSON(t *testing.T, wantTemplate string, actual []byte) {
	t.Helper()

	var got struct {
		Name     string `json:"name"`
		Metadata struct {
			StartTime string `json:"startTime"`
			EndTime   string `json:"endTime"`
			CreatedAt string `json:"createdAt"`
		} `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(actual, &got))

	want := strings.NewReplacer(
		"{{NAME}}", got.Name,
		"{{START}}", got.Metadata.StartTime,
		"{{END}}", got.Metadata.EndTime,
		"{{CREATED}}", got.Metadata.CreatedAt,
	).Replace(wantTemplate)

	assert.JSONEq(t, want, string(actual))
}
