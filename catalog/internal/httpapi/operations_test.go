package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/naira-project/naira/catalog/internal/catalog/catalogtest"
	"github.com/naira-project/naira/catalog/internal/operations"
	"github.com/naira-project/naira/catalog/internal/pluginrun"
	"github.com/naira-project/naira/catalog/internal/pluginrun/pluginruntest"
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
					"plugin": "mlflow",
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
					"plugin": "mlflow",
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
					"plugin": "mlflow",
					"startTime": "{{START}}", "endTime": "{{END}}", "createdAt": "{{CREATED}}"
				},
				"error": {"message": "collecting response from plugin \"mlflow\": sync failed: database offline"}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshotStore := pluginruntest.NewMockSnapshotStore()
			router, _ := newTestRouter(t, &catalogtest.MockStore{}, snapshotStore, map[string]pluginrun.Plugin{
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

			completed := waitForTerminalState(t, snapshotStore, opRes.Name)

			getRec := doRequest(t, router, http.MethodGet, "/v1/operations/"+url.PathEscape(completed.Name))
			require.Equal(t, http.StatusOK, getRec.Code)
			assertOperationJSON(t, tt.wantTerminalJSON, getRec.Body.Bytes())
		})
	}
}

func TestOperationFromCatalogOperationFailedWithoutError(t *testing.T) {
	resource := operationFromCatalogOperation(operations.Operation{
		Name:  "plugin-run-missing-error",
		State: operations.StateFailed,
	})

	assert.True(t, resource.Done)
	assert.Nil(t, resource.Response)
	require.NotNil(t, resource.Error)
	assert.Equal(t, "operation failed without an error", resource.Error.Message)
}

func TestRunPluginAsyncEndpointUnknownPlugin(t *testing.T) {
	router, _ := newTestRouter(t, nil, nil, nil)

	rec := doRequest(t, router, http.MethodPost, "/v1/plugins/missing:run")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRunPluginAsyncEndpointConflict(t *testing.T) {
	snapshotStore := pluginruntest.NewMockSnapshotStore()
	block := make(chan struct{})
	router, runner := newTestRouter(t, &catalogtest.MockStore{}, snapshotStore, map[string]pluginrun.Plugin{
		"mlflow": blockingStubPlugin{block: block},
	})

	rec1 := doRequest(t, router, http.MethodPost, "/v1/plugins/mlflow:run")
	assert.Equal(t, http.StatusAccepted, rec1.Code)

	var firstOp OperationResource
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &firstOp))

	waitForState(t, snapshotStore, firstOp.Name, func(op operations.Operation) bool {
		return op.State == operations.StateRunning
	})

	rec2 := doRequest(t, router, http.MethodPost, "/v1/plugins/mlflow:run")
	assert.Equal(t, http.StatusConflict, rec2.Code)

	close(block)
	runner.Wait()
}

func TestGetOperationsEndpoint(t *testing.T) {
	snapshotStore := pluginruntest.NewMockSnapshotStore()
	router, _ := newTestRouter(t, &catalogtest.MockStore{}, snapshotStore, map[string]pluginrun.Plugin{"seed": stubPlugin{}})

	// Trigger "seed" plugin flow
	runRec := doRequest(t, router, http.MethodPost, "/v1/plugins:run")
	require.Equal(t, http.StatusAccepted, runRec.Code)

	var runResp RunPluginsResponse
	require.NoError(t, json.Unmarshal(runRec.Body.Bytes(), &runResp))
	require.Len(t, runResp.Operations, 1)

	opName := runResp.Operations[0].Name
	waitForTerminalState(t, snapshotStore, opName)

	// Fetch operations list
	rec := doRequest(t, router, http.MethodGet, "/v1/operations")
	assert.Equal(t, http.StatusOK, rec.Code)

	var listResp struct {
		Operations []json.RawMessage `json:"operations"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	require.Len(t, listResp.Operations, 1)

	wantJSON := `{
		"name": "{{NAME}}",
		"done": true,
		"metadata": {
			"plugin": "seed",
			"startTime": "{{START}}", "endTime": "{{END}}", "createdAt": "{{CREATED}}"
		},
		"response": {"nodesUpserted": 0, "relationsUpserted": 0}
	}`
	assertOperationJSON(t, wantJSON, listResp.Operations[0])
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
func waitForTerminalState(t *testing.T, store *pluginruntest.MockSnapshotStore, name string) operations.Operation {
	t.Helper()
	return waitForState(t, store, name, func(op operations.Operation) bool {
		return op.State == operations.StateSucceeded || op.State == operations.StateFailed
	})
}

// waitForState polls until condition is met or times out.
func waitForState(t *testing.T, store *pluginruntest.MockSnapshotStore, name string, condition func(operations.Operation) bool) operations.Operation {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		op, err := store.Get(name)
		if err == nil && condition(op) {
			return op
		}
		time.Sleep(10 * time.Millisecond)
	}

	op, err := store.Get(name)
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
