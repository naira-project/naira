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

			rec := doRequest(t, router, http.MethodPost, "/v1/plugins/mlflow:run")
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

			completed := waitForTerminalState(t, opStore, opRes.Name)

			getRec := doRequest(t, router, http.MethodGet, "/v1/operations/"+url.PathEscape(completed.Name))
			require.Equal(t, http.StatusOK, getRec.Code)
			assertOperationJSON(t, tt.wantTerminalJSON, getRec.Body.Bytes())
		})
	}
}

func TestRunPluginAsyncEndpointUnknownPlugin(t *testing.T) {
	router := newTestRouter(t, catalog.NewMemoryStore(), operations.NewMemoryStore(), nil)

	rec := doRequest(t, router, http.MethodPost, "/v1/plugins/missing:run")
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

	rec1 := doRequest(t, router, http.MethodPost, "/v1/plugins/mlflow:run")
	assert.Equal(t, http.StatusAccepted, rec1.Code)

	var firstOp OperationResource
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &firstOp))

	waitForState(t, opStore, firstOp.Name, func(op operations.Operation) bool {
		return op.State == operations.StateRunning
	})

	rec2 := doRequest(t, router, http.MethodPost, "/v1/plugins/mlflow:run")
	assert.Equal(t, http.StatusConflict, rec2.Code)

	close(block)
	runner.Wait()
}

func TestGetOperationsEndpoint(t *testing.T) {
	opStore := operations.NewMemoryStore()
	router := newTestRouter(t, catalog.NewMemoryStore(), opStore, map[string]pluginrun.Plugin{"seed": stubPlugin{}})

	// Trigger "seed" plugin flow
	runRec := doRequest(t, router, http.MethodPost, "/v1/plugins:run")
	require.Equal(t, http.StatusAccepted, runRec.Code)

	var runResp RunPluginsResponse
	require.NoError(t, json.Unmarshal(runRec.Body.Bytes(), &runResp))
	require.Len(t, runResp.Operations, 1)

	opName := runResp.Operations[0].Name
	waitForTerminalState(t, opStore, opName)

	// Fetch operations list
	rec := doRequest(t, router, http.MethodGet, "/v1/operations")
	assert.Equal(t, http.StatusOK, rec.Code)

	var listResp ListOperationsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	require.Len(t, listResp.Operations, 1)

	op := listResp.Operations[0]
	assert.Equal(t, opName, op.Name)
	assert.Equal(t, "seed", op.Metadata.Plugin)
	assert.Equal(t, "SUCCEEDED", op.Metadata.State)
	assert.True(t, op.Done)
	assert.NotNil(t, op.Response)
}

// --- helpers -----------------------------------------------------------

// doRequest executes an authenticated HTTP request against the provided router.
func doRequest(t *testing.T, router http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := withAuth(httptest.NewRequest(method, path, nil), testBearerToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// waitForTerminalState polls until the operation reaches SUCCEEDED or FAILED.
func waitForTerminalState(t *testing.T, opStore operations.Store, name string) operations.Operation {
	t.Helper()
	return waitForState(t, opStore, name, func(op operations.Operation) bool {
		return op.State == operations.StateSucceeded || op.State == operations.StateFailed
	})
}

// waitForState polls until condition is met or times out.
func waitForState(t *testing.T, opStore operations.Store, name string, condition func(operations.Operation) bool) operations.Operation {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		op, err := opStore.Get(name)
		if err == nil && condition(op) {
			return op
		}
		time.Sleep(10 * time.Millisecond)
	}

	op, err := opStore.Get(name)
	require.NoError(t, err, "operation %q not found", name)
	t.Fatalf("timed out waiting for operation %q; final state = %s", name, op.State)
	return operations.Operation{}
}

// assertOperationJSON compares actual JSON against wantTemplate replacing dynamic placeholders.
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
