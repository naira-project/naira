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

	"github.com/naira-project/naira/catalog/internal/httpapi/mocks"
	"github.com/naira-project/naira/catalog/internal/operations"
	"github.com/naira-project/naira/catalog/internal/pluginrun"
)

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

func TestOperationFromCatalogOperationPendingOmitsStartTime(t *testing.T) {
	resource := operationFromCatalogOperation(operations.Operation{
		Name:      "plugin-run-1",
		Plugin:    "mlflow",
		State:     operations.StatePending,
		CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	})

	body, err := json.Marshal(resource)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"name": "plugin-run-1",
		"done": false,
		"metadata": {
			"plugin": "mlflow",
			"createdAt": "2026-01-02T03:04:05Z"
		}
	}`, string(body))
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
			name: "accepts and returns the pending operation",
			runnerOp: operations.Operation{
				Name:      "plugin-run-1",
				Plugin:    "mlflow",
				State:     operations.StatePending,
				CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			},
			wantStatus: http.StatusAccepted,
			wantJSON: `{
				"name": "plugin-run-1",
				"done": false,
				"metadata": {
					"plugin": "mlflow",
					"createdAt": "2026-01-02T03:04:05Z"
				}
			}`,
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
			runner := &mocks.PluginRunnerMock{
				RunPluginAsyncFunc: func(context.Context, string) (operations.Operation, error) {
					return tt.runnerOp, tt.runnerErr
				},
			}
			router, err := NewRouter(&mocks.CatalogReaderMock{}, runner, nil, discardLogger, testKeycloak)
			require.NoError(t, err)

			rec := doRequest(t, router, http.MethodPost, "/v1/plugins/mlflow:run")

			require.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
			if tt.wantJSON != "" {
				assert.JSONEq(t, tt.wantJSON, rec.Body.String())
			}
		})
	}
}

func TestRunAllPluginsEndpoint(t *testing.T) {
	tests := []struct {
		name string
		ops  []operations.Operation
		want string
	}{
		{
			name: "no plugins",
			ops:  []operations.Operation{},
			want: `{"operations": []}`,
		},
		{
			name: "one plugin",
			ops: []operations.Operation{{
				Name:      "plugin-run-1",
				Plugin:    "mlflow",
				State:     operations.StatePending,
				CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			}},
			want: `{
				"operations": [
					{
						"name": "plugin-run-1",
						"done": false,
						"metadata": {
							"plugin": "mlflow",
							"createdAt": "2026-01-02T03:04:05Z"
						}
					}
				]
			}`,
		},
		{
			name: "multiple plugins in different states",
			ops: []operations.Operation{
				{
					Name:      "plugin-run-1",
					Plugin:    "mlflow",
					State:     operations.StatePending,
					CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
				},
				{
					Name:      "plugin-run-2",
					Plugin:    "litellm",
					State:     operations.StateRunning,
					CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
					StartTime: time.Date(2026, 1, 2, 3, 4, 6, 0, time.UTC),
				},
				{
					Name:      "plugin-run-3",
					Plugin:    "openmetadata",
					State:     operations.StateFailed,
					CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
					StartTime: time.Date(2026, 1, 2, 3, 4, 6, 0, time.UTC),
					EndTime:   new(time.Date(2026, 1, 2, 3, 4, 7, 0, time.UTC)),
					Error:     &operations.StatusError{Message: "sync failed"},
				},
			},
			want: `{
				"operations": [
					{
						"name": "plugin-run-1",
						"done": false,
						"metadata": {
							"plugin": "mlflow",
							"createdAt": "2026-01-02T03:04:05Z"
						}
					},
					{
						"name": "plugin-run-2",
						"done": false,
						"metadata": {
							"plugin": "litellm",
							"startTime": "2026-01-02T03:04:06Z",
							"createdAt": "2026-01-02T03:04:05Z"
						}
					},
					{
						"name": "plugin-run-3",
						"done": true,
						"metadata": {
							"plugin": "openmetadata",
							"startTime": "2026-01-02T03:04:06Z",
							"endTime": "2026-01-02T03:04:07Z",
							"createdAt": "2026-01-02T03:04:05Z"
						},
						"error": {
							"message": "sync failed"
						}
					}
				]
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &mocks.PluginRunnerMock{
				RunAllPluginsAsyncFunc: func(context.Context) []operations.Operation {
					return tt.ops
				},
			}
			router, err := NewRouter(&mocks.CatalogReaderMock{}, runner, nil, discardLogger, testKeycloak)
			require.NoError(t, err)

			rec := doRequest(t, router, http.MethodPost, "/v1/plugins:run")

			require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
			assert.JSONEq(t, tt.want, rec.Body.String())
		})
	}
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
			name: "running operation",
			runnerOp: operations.Operation{
				Name:      "plugin-run-1",
				Plugin:    "mlflow",
				State:     operations.StateRunning,
				CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
				StartTime: time.Date(2026, 1, 2, 3, 4, 6, 0, time.UTC),
			},
			wantStatus: http.StatusOK,
			wantJSON: `{
				"name": "plugin-run-1",
				"done": false,
				"metadata": {
					"plugin": "mlflow",
					"startTime": "2026-01-02T03:04:06Z",
					"createdAt": "2026-01-02T03:04:05Z"
				}
			}`,
		},
		{
			name: "succeeded operation carries response",
			runnerOp: operations.Operation{
				Name:              "plugin-run-1",
				Plugin:            "mlflow",
				State:             operations.StateSucceeded,
				CreatedAt:         time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
				StartTime:         time.Date(2026, 1, 2, 3, 4, 6, 0, time.UTC),
				EndTime:           new(time.Date(2026, 1, 2, 3, 4, 7, 0, time.UTC)),
				NodesUpserted:     3,
				RelationsUpserted: 2,
			},
			wantStatus: http.StatusOK,
			wantJSON: `{
				"name": "plugin-run-1",
				"done": true,
				"metadata": {
					"plugin": "mlflow",
					"startTime": "2026-01-02T03:04:06Z",
					"endTime": "2026-01-02T03:04:07Z",
					"createdAt": "2026-01-02T03:04:05Z"
				},
				"response": {
					"nodesUpserted": 3,
					"relationsUpserted": 2
				}
			}`,
		},
		{
			name: "failed operation carries error",
			runnerOp: operations.Operation{
				Name:      "plugin-run-1",
				Plugin:    "mlflow",
				State:     operations.StateFailed,
				CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
				StartTime: time.Date(2026, 1, 2, 3, 4, 6, 0, time.UTC),
				EndTime:   new(time.Date(2026, 1, 2, 3, 4, 7, 0, time.UTC)),
				Error:     &operations.StatusError{Message: "sync failed: database offline"},
			},
			wantStatus: http.StatusOK,
			wantJSON: `{
				"name": "plugin-run-1",
				"done": true,
				"metadata": {
					"plugin": "mlflow",
					"startTime": "2026-01-02T03:04:06Z",
					"endTime": "2026-01-02T03:04:07Z",
					"createdAt": "2026-01-02T03:04:05Z"
				},
				"error": {
					"message": "sync failed: database offline"
				}
			}`,
		},
		{
			name:       "unknown operation is not found",
			runnerErr:  operations.ErrNotFound,
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &mocks.PluginRunnerMock{
				GetOperationFunc: func(context.Context, string) (operations.Operation, error) {
					return tt.runnerOp, tt.runnerErr
				},
			}
			router, err := NewRouter(&mocks.CatalogReaderMock{}, runner, nil, discardLogger, testKeycloak)
			require.NoError(t, err)

			rec := doRequest(t, router, http.MethodGet, "/v1/operations/plugin-run-1")

			require.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
			if tt.wantJSON != "" {
				assert.JSONEq(t, tt.wantJSON, rec.Body.String())
			}
		})
	}
}

func TestListOperationsEndpoint(t *testing.T) {
	t.Run("lists names of all operations", func(t *testing.T) {
		runner := &mocks.PluginRunnerMock{
			ListOperationsFunc: func(context.Context, operations.Filter) ([]operations.Operation, error) {
				return []operations.Operation{
					{
						Name:      "plugin-run-1",
						Plugin:    "seed",
						State:     operations.StateSucceeded,
						CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
						StartTime: time.Date(2026, 1, 2, 3, 4, 6, 0, time.UTC),
						EndTime:   new(time.Date(2026, 1, 2, 3, 4, 7, 0, time.UTC)),
					},
					{
						Name:      "plugin-run-2",
						Plugin:    "mlflow",
						State:     operations.StateRunning,
						CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
						StartTime: time.Date(2026, 1, 2, 3, 4, 6, 0, time.UTC),
					},
				}, nil
			},
		}
		router, err := NewRouter(&mocks.CatalogReaderMock{}, runner, nil, discardLogger, testKeycloak)
		require.NoError(t, err)

		rec := doRequest(t, router, http.MethodGet, "/v1/operations")

		require.Equal(t, http.StatusOK, rec.Code)
		var payload ListOperationsResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
		assert.Equal(t, []string{"plugin-run-1", "plugin-run-2"}, operationNames(payload))
	})

	t.Run("filters plugin lists names", func(t *testing.T) {
		runner := &mocks.PluginRunnerMock{
			ListOperationsFunc: func(context.Context, operations.Filter) ([]operations.Operation, error) {
				return []operations.Operation{
					{Name: "plugin-run-1", Plugin: "seed", State: operations.StateRunning},
					{Name: "plugin-run-2", Plugin: "mlflow", State: operations.StateRunning},
				}, nil
			},
		}
		router, err := NewRouter(&mocks.CatalogReaderMock{}, runner, nil, discardLogger, testKeycloak)
		require.NoError(t, err)

		rec := doRequest(t, router, http.MethodGet, "/v1/operations?filter=plugin=%22seed%22")

		require.Equal(t, http.StatusOK, rec.Code)
		var payload ListOperationsResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
		assert.Equal(t, []string{"plugin-run-1"}, operationNames(payload))
	})

	t.Run("paginates", func(t *testing.T) {
		runner := &mocks.PluginRunnerMock{
			ListOperationsFunc: func(context.Context, operations.Filter) ([]operations.Operation, error) {
				return []operations.Operation{
					{Name: "plugin-run-1", Plugin: "seed", State: operations.StateRunning},
					{Name: "plugin-run-2", Plugin: "mlflow", State: operations.StateRunning},
				}, nil
			},
		}
		router, err := NewRouter(&mocks.CatalogReaderMock{}, runner, nil, discardLogger, testKeycloak)
		require.NoError(t, err)

		rec := doRequest(t, router, http.MethodGet, "/v1/operations?pageSize=1")

		require.Equal(t, http.StatusOK, rec.Code)
		var payload ListOperationsResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
		assert.Len(t, payload.Operations, 1)
		assert.NotEmpty(t, payload.NextPageToken)
		assert.Equal(t, int32(2), payload.TotalSize)
	})

	t.Run("rejects unsupported filter field", func(t *testing.T) {
		runner := &mocks.PluginRunnerMock{
			ListOperationsFunc: func(context.Context, operations.Filter) ([]operations.Operation, error) {
				return nil, nil
			},
		}
		router, err := NewRouter(&mocks.CatalogReaderMock{}, runner, nil, discardLogger, testKeycloak)
		require.NoError(t, err)

		rec := doRequest(t, router, http.MethodGet, "/v1/operations?filter=state=%22running%22")

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Empty(t, runner.ListOperationsCalls())
	})
}

func operationNames(payload ListOperationsResponse) []string {
	names := make([]string, 0, len(payload.Operations))
	for _, op := range payload.Operations {
		names = append(names, op.Name)
	}
	return names
}
