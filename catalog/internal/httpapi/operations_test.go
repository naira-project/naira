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
	"testing"
	"time"

	"github.com/naira-project/naira/catalog/internal/auth/keycloak"
	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/operations"
	"github.com/naira-project/naira/catalog/internal/pluginrun"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperationFromCatalogOperation(t *testing.T) {
	start := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	created := start.Add(-time.Minute)
	end := start.Add(time.Minute)

	tests := []struct {
		name string
		op   operations.Operation
		want OperationResource
	}{
		{
			name: "pending",
			op: operations.Operation{
				Name: "operations/pending", Plugin: "seed", State: operations.StatePending,
				StartTime: start, CreatedAt: created,
			},
			want: OperationResource{
				Name: "operations/pending", Done: false,
				Metadata: OperationMetadataResource{Plugin: "seed", State: "PENDING", StartTime: start, CreatedAt: created},
			},
		},
		{
			name: "succeeded",
			op: operations.Operation{
				Name: "operations/succeeded", Plugin: "seed", State: operations.StateSucceeded,
				StartTime: start, EndTime: &end, CreatedAt: created, NodesUpserted: 3, RelationsUpserted: 2,
			},
			want: OperationResource{
				Name: "operations/succeeded", Done: true,
				Metadata: OperationMetadataResource{Plugin: "seed", State: "SUCCEEDED", StartTime: start, EndTime: &end, CreatedAt: created},
				Response: &RunPluginResult{NodesUpserted: 3, RelationsUpserted: 2},
			},
		},
		{
			name: "failed",
			op: operations.Operation{
				Name: "operations/failed", Plugin: "seed", State: operations.StateFailed,
				StartTime: start, EndTime: &end, CreatedAt: created,
				Error: &operations.StatusError{Message: "seed failed"},
			},
			want: OperationResource{
				Name: "operations/failed", Done: true,
				Metadata: OperationMetadataResource{Plugin: "seed", State: "FAILED", StartTime: start, EndTime: &end, CreatedAt: created},
				Error:    &StatusErrorResource{Message: "seed failed"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, operationFromCatalogOperation(tt.op))
		})
	}
}

func TestRunAllPluginsReturnsPluginErrorsInResults(t *testing.T) {
	opStore := operations.NewMemoryStore()
	router := newTestRouter(t, catalog.NewMemoryStore(), opStore, map[string]pluginrun.Plugin{"seed": stubPlugin{err: errors.New("seed failed")}})

	rec := postAuthorized(t, router, "/v1/plugins:run")
	assert.Equal(t, http.StatusAccepted, rec.Code)

	var payload RunPluginsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Len(t, payload.Operations, 1)

	op := payload.Operations[0]
	assert.Equal(t, "seed", op.Metadata.Plugin)
	assert.False(t, op.Done)

	completed := waitForOperation(t, opStore, op.Name)
	assert.Equal(t, operations.StateFailed, completed.State)
	require.NotNil(t, completed.Error)
	assert.Contains(t, completed.Error.Message, "seed failed")
}

func TestRunPluginAsyncEndpoint(t *testing.T) {
	opStore := operations.NewMemoryStore()
	router := newTestRouter(t, catalog.NewMemoryStore(), opStore, map[string]pluginrun.Plugin{"mlflow": stubPlugin{}})

	rec := postAuthorized(t, router, "/v1/mlflow:run")
	assert.Equal(t, http.StatusAccepted, rec.Code)

	var op OperationResource
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &op))
	assert.Equal(t, "mlflow", op.Metadata.Plugin)
	assert.Equal(t, "PENDING", op.Metadata.State)
	assert.False(t, op.Done)

	completed := waitForOperation(t, opStore, op.Name)
	assert.Equal(t, operations.StateSucceeded, completed.State)
}

func TestRunPluginAsyncEndpointUnknownPlugin(t *testing.T) {
	router := newTestRouter(t, catalog.NewMemoryStore(), operations.NewMemoryStore(), nil)

	rec := postAuthorized(t, router, "/v1/missing:run")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRunPluginAsyncEndpointConflict(t *testing.T) {
	opStore := operations.NewMemoryStore()
	block := make(chan struct{})
	store := catalog.NewMemoryStore()
	catalogService := catalog.NewService(store)
	runner := pluginrun.NewRunner(context.Background(), store, opStore, map[string]pluginrun.Plugin{"mlflow": blockingStubPlugin{block: block}}, 5*time.Minute, log.New(io.Discard, "", 0))
	router, err := NewRouter(catalogService, runner, log.New(io.Discard, "", 0), keycloak.Config{Client: stubTokenDecoder{}, Issuer: testIssuer})
	require.NoError(t, err)

	rec1 := postAuthorized(t, router, "/v1/mlflow:run")
	assert.Equal(t, http.StatusAccepted, rec1.Code)

	var firstOp OperationResource
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &firstOp))
	waitForRunning(t, opStore, firstOp.Name)

	rec2 := postAuthorized(t, router, "/v1/mlflow:run")
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

func TestGetOperationByIDEndpoint(t *testing.T) {
	opStore := operations.NewMemoryStore()
	router := newTestRouter(t, catalog.NewMemoryStore(), opStore, map[string]pluginrun.Plugin{"seed": stubPlugin{}})

	opName := runSeedAndWait(t, router, opStore)

	rec := getAuthorized(t, router, "/v1/operations/"+url.PathEscape(opName))
	assert.Equal(t, http.StatusOK, rec.Code)

	var op OperationResource
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &op))
	assertSucceededSeedOperation(t, op, opName)
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
