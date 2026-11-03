package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/naira-project/naira/catalog/internal/operations"
	"github.com/naira-project/naira/catalog/internal/pluginrun"
)

var (
	opCreatedAt = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	opStartTime = opCreatedAt.Add(time.Second)
	opEndTime   = opCreatedAt.Add(2 * time.Second)
)

// --- fixtures ----------------------------------------------------------

func pendingOperation(name, plugin string) operations.Operation {
	return operations.Operation{
		Name:      name,
		Plugin:    plugin,
		State:     operations.StatePending,
		CreatedAt: opCreatedAt,
	}
}

func runningOperation(name, plugin string) operations.Operation {
	return operations.Operation{
		Name:      name,
		Plugin:    plugin,
		State:     operations.StateRunning,
		StartTime: opStartTime,
		CreatedAt: opCreatedAt,
	}
}

func succeededOperation(name, plugin string, nodes, relations int) operations.Operation {
	end := opEndTime
	op := runningOperation(name, plugin)
	op.State = operations.StateSucceeded
	op.EndTime = &end
	op.NodesUpserted = nodes
	op.RelationsUpserted = relations
	return op
}

func failedOperation(name, plugin, message string) operations.Operation {
	end := opEndTime
	op := runningOperation(name, plugin)
	op.State = operations.StateFailed
	op.EndTime = &end
	op.Error = &operations.StatusError{Message: message}
	return op
}

const (
	pendingMlflowJSON = `{
		"name": "plugin-run-1", "done": false,
		"metadata": {"plugin": "mlflow", "startTime": "0001-01-01T00:00:00Z", "createdAt": "2026-01-02T03:04:05Z"}
	}`
	runningMlflowJSON = `{
		"name": "plugin-run-1", "done": false,
		"metadata": {"plugin": "mlflow", "startTime": "2026-01-02T03:04:06Z", "createdAt": "2026-01-02T03:04:05Z"}
	}`
	succeededMlflowJSON = `{
		"name": "plugin-run-1", "done": true,
		"metadata": {"plugin": "mlflow", "startTime": "2026-01-02T03:04:06Z", "endTime": "2026-01-02T03:04:07Z", "createdAt": "2026-01-02T03:04:05Z"},
		"response": {"nodesUpserted": 3, "relationsUpserted": 2}
	}`
	failedMlflowJSON = `{
		"name": "plugin-run-1", "done": true,
		"metadata": {"plugin": "mlflow", "startTime": "2026-01-02T03:04:06Z", "endTime": "2026-01-02T03:04:07Z", "createdAt": "2026-01-02T03:04:05Z"},
		"error": {"message": "sync failed: database offline"}
	}`
)

// --- tests -------------------------------------------------------------

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

func TestRunPluginEndpoint(t *testing.T) {
	tests := []struct {
		name       string
		runnerOp   operations.Operation
		runnerErr  error
		wantStatus int
		wantJSON   string
	}{
		{
			name:       "accepts and returns the pending operation",
			runnerOp:   pendingOperation("plugin-run-1", "mlflow"),
			wantStatus: http.StatusAccepted,
			wantJSON:   pendingMlflowJSON,
		},
		{
			name:       "unknown plugin is a bad request",
			runnerErr:  pluginrun.ErrPluginNotFound,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "plugin already running is a conflict",
			runnerErr:  pluginrun.ErrPluginAlreadyRunning,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "unexpected runner failure is an internal error",
			runnerErr:  errors.New("boom"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &PluginRunnerMock{
				RunPluginAsyncFunc: func(context.Context, string) (operations.Operation, error) {
					return tt.runnerOp, tt.runnerErr
				},
			}
			router := newTestRouter(t, routerDeps{runner: runner})

			rec := doRequest(t, router, http.MethodPost, "/v1/plugins/mlflow:run")

			require.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
			if tt.wantJSON != "" {
				assert.JSONEq(t, tt.wantJSON, rec.Body.String())
			}
		})
	}
}

func TestRunAllPluginsEndpoint(t *testing.T) {
	runner := &PluginRunnerMock{
		RunAllPluginsAsyncFunc: func(context.Context) []operations.Operation {
			return []operations.Operation{pendingOperation("plugin-run-1", "mlflow")}
		},
	}
	router := newTestRouter(t, routerDeps{runner: runner})

	rec := doRequest(t, router, http.MethodPost, "/v1/plugins:run")

	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.JSONEq(t, `{"operations": [`+pendingMlflowJSON+`]}`, rec.Body.String())
}

func TestGetOperationEndpoint(t *testing.T) {
	tests := []struct {
		name       string
		runnerOp   operations.Operation
		runnerErr  error
		wantStatus int
		wantJSON   string
	}{
		{
			name:       "running operation",
			runnerOp:   runningOperation("plugin-run-1", "mlflow"),
			wantStatus: http.StatusOK,
			wantJSON:   runningMlflowJSON,
		},
		{
			name:       "succeeded operation carries the response",
			runnerOp:   succeededOperation("plugin-run-1", "mlflow", 3, 2),
			wantStatus: http.StatusOK,
			wantJSON:   succeededMlflowJSON,
		},
		{
			name:       "failed operation carries the error",
			runnerOp:   failedOperation("plugin-run-1", "mlflow", "sync failed: database offline"),
			wantStatus: http.StatusOK,
			wantJSON:   failedMlflowJSON,
		},
		{
			name:       "unknown operation is not found",
			runnerErr:  operations.ErrNotFound,
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &PluginRunnerMock{
				GetOperationFunc: func(context.Context, string) (operations.Operation, error) {
					return tt.runnerOp, tt.runnerErr
				},
			}
			router := newTestRouter(t, routerDeps{runner: runner})

			rec := doRequest(t, router, http.MethodGet, "/v1/operations/plugin-run-1")

			require.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
			if tt.wantJSON != "" {
				assert.JSONEq(t, tt.wantJSON, rec.Body.String())
			}
		})
	}
}

func TestListOperationsEndpoint(t *testing.T) {
	newOperationsRouter := func(t *testing.T) (http.Handler, *PluginRunnerMock) {
		t.Helper()
		runner := &PluginRunnerMock{
			ListOperationsFunc: func(context.Context, operations.Filter) ([]operations.Operation, error) {
				return []operations.Operation{
					succeededOperation("plugin-run-1", "seed", 0, 0),
					runningOperation("plugin-run-2", "mlflow"),
				}, nil
			},
		}
		return newTestRouter(t, routerDeps{runner: runner}), runner
	}

	operationNames := func(t *testing.T, body []byte) []string {
		t.Helper()
		var payload ListOperationsResponse
		require.NoError(t, json.Unmarshal(body, &payload))
		names := make([]string, 0, len(payload.Operations))
		for _, op := range payload.Operations {
			names = append(names, op.Name)
		}
		return names
	}

	t.Run("lists all operations", func(t *testing.T) {
		router, _ := newOperationsRouter(t)

		rec := doRequest(t, router, http.MethodGet, "/v1/operations")

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, []string{"plugin-run-1", "plugin-run-2"}, operationNames(t, rec.Body.Bytes()))
	})

	t.Run("filters by plugin", func(t *testing.T) {
		router, _ := newOperationsRouter(t)

		rec := doRequest(t, router, http.MethodGet, "/v1/operations?filter=plugin=%22seed%22")

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, []string{"plugin-run-1"}, operationNames(t, rec.Body.Bytes()))
	})

	t.Run("paginates", func(t *testing.T) {
		router, _ := newOperationsRouter(t)

		rec := doRequest(t, router, http.MethodGet, "/v1/operations?pageSize=1")

		require.Equal(t, http.StatusOK, rec.Code)
		var payload ListOperationsResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
		assert.Len(t, payload.Operations, 1)
		assert.NotEmpty(t, payload.NextPageToken)
		assert.Equal(t, int32(2), payload.TotalSize)
	})

	t.Run("rejects unsupported filter field", func(t *testing.T) {
		router, runner := newOperationsRouter(t)

		rec := doRequest(t, router, http.MethodGet, "/v1/operations?filter=state=%22running%22")

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Empty(t, runner.ListOperationsCalls())
	})
}
