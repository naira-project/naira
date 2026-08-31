package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/naira-project/naira/catalog/internal/auth/keycloak"
	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/operations"
	"github.com/naira-project/naira/catalog/internal/pluginrun"
	"github.com/naira-project/naira/catalog/internal/scheduling"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newScheduleTestRouter(t *testing.T, scheduler *scheduling.Scheduler) http.Handler {
	t.Helper()

	catalogService := catalog.NewService(catalog.NewMemoryStore())
	runner := pluginrun.NewRunner(context.Background(), catalog.NewMemoryStore(), operations.NewMemoryStore(), nil, 5*time.Minute, log.New(io.Discard, "", 0))
	router, err := NewRouter(catalogService, runner, log.New(io.Discard, "", 0), keycloak.Config{Client: stubTokenDecoder{}, Issuer: testIssuer}, scheduler)
	require.NoError(t, err)
	return router
}

func newStartedScheduleTestScheduler(t *testing.T, schedules ...scheduling.Schedule) *scheduling.Scheduler {
	t.Helper()

	plugins := make(map[string]string, len(schedules))
	defaults := make(map[string]string, len(schedules))
	for _, schedule := range schedules {
		plugins[schedule.Plugin] = ""
		if schedule.Enabled {
			defaults[schedule.Plugin] = schedule.Expression
		}
	}

	scheduler, err := scheduling.NewConfiguredScheduler(plugins, defaults, nil, log.New(io.Discard, "", 0))
	require.NoError(t, err)
	t.Cleanup(func() { scheduler.Stop() })
	return scheduler
}

func TestListSchedulesEndpoint(t *testing.T) {
	tests := []struct {
		name              string
		path              string
		expectedSchedules []string
		expectedTotal     int32
		expectNextToken   bool
	}{
		{
			name:              "lists all schedules in plugin order",
			path:              "/v1/schedules",
			expectedSchedules: []string{"alpha", "zeta"},
			expectedTotal:     2,
		},
		{
			name:              "returns first page",
			path:              "/v1/schedules?pageSize=1",
			expectedSchedules: []string{"alpha"},
			expectedTotal:     2,
			expectNextToken:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheduler := newStartedScheduleTestScheduler(t,
				scheduling.Schedule{Plugin: "zeta", Expression: "0 0 * * *", Enabled: true},
				scheduling.Schedule{Plugin: "alpha", Expression: "*/5 * * * *", Enabled: true},
			)
			router := newScheduleTestRouter(t, scheduler)
			req := withAuth(httptest.NewRequest(http.MethodGet, tt.path, nil), testBearerToken)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			var payload ListSchedulesResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
			assert.Equal(t, tt.expectedTotal, payload.TotalSize)
			assert.Equal(t, tt.expectedSchedules, schedulePlugins(payload.Schedules))
			if tt.expectNextToken {
				assert.NotEmpty(t, payload.NextPageToken)
			} else {
				assert.Empty(t, payload.NextPageToken)
			}
		})
	}
}

func TestGetScheduleEndpoint(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		expectedStatus int
		expected       *scheduling.Schedule
	}{
		{
			name:           "returns configured schedule",
			path:           "/v1/mlflow/schedule",
			expectedStatus: http.StatusOK,
			expected:       &scheduling.Schedule{Plugin: "mlflow", Expression: "*/5 * * * *", Enabled: true},
		},
		{
			name:           "returns not found for unknown plugin",
			path:           "/v1/missing/schedule",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheduler := newStartedScheduleTestScheduler(t, scheduling.Schedule{Plugin: "mlflow", Expression: "*/5 * * * *", Enabled: true})
			router := newScheduleTestRouter(t, scheduler)
			req := withAuth(httptest.NewRequest(http.MethodGet, tt.path, nil), testBearerToken)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
			if tt.expected == nil {
				return
			}

			var actual scheduling.Schedule
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &actual))
			assert.Equal(t, *tt.expected, scheduling.Schedule{
				Plugin: actual.Plugin, Expression: actual.Expression, Enabled: actual.Enabled,
			})
			assert.False(t, actual.UpdatedAt.IsZero())
		})
	}
}

func schedulePlugins(schedules []scheduling.Schedule) []string {
	plugins := make([]string, 0, len(schedules))
	for _, schedule := range schedules {
		plugins = append(plugins, schedule.Plugin)
	}
	return plugins
}
