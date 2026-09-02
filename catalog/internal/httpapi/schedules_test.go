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

func newScheduleTestRouter(t *testing.T, scheduler *scheduling.Scheduler, pluginNames ...string) http.Handler {
	t.Helper()

	plugins := make(map[string]pluginrun.Plugin, len(pluginNames))
	for _, name := range pluginNames {
		plugins[name] = stubPlugin{}
	}
	catalogService := catalog.NewService(catalog.NewMemoryStore())
	runner := pluginrun.NewRunner(context.Background(), catalog.NewMemoryStore(), operations.NewMemoryStore(), plugins, 5*time.Minute, log.New(io.Discard, "", 0))
	definitions := make(map[string]catalog.PluginDefinition, len(pluginNames))
	for _, name := range pluginNames {
		definitions[name] = catalog.PluginDefinition{}
	}
	for _, schedule := range schedulerSchedules(scheduler) {
		definition := definitions[schedule.Plugin]
		definition.Schedule = schedule.Expression
		definitions[schedule.Plugin] = definition
	}
	router, err := NewRouter(catalogService, runner, definitions, log.New(io.Discard, "", 0), keycloak.Config{Client: stubTokenDecoder{}, Issuer: testIssuer})
	require.NoError(t, err)
	return router
}

func newStartedScheduleTestScheduler(t *testing.T, schedules ...scheduling.Schedule) *scheduling.Scheduler {
	t.Helper()

	scheduleMap := make(map[string]string, len(schedules))
	for _, schedule := range schedules {
		if schedule.Enabled && schedule.Expression != "" {
			scheduleMap[schedule.Plugin] = schedule.Expression
		}
	}

	scheduler, err := scheduling.NewConfiguredScheduler(scheduleMap, nil, log.New(io.Discard, "", 0))
	require.NoError(t, err)
	t.Cleanup(func() { scheduler.Stop(context.Background()) })
	return scheduler
}

func TestListPluginsIncludesSchedules(t *testing.T) {
	tests := []struct {
		name              string
		path              string
		expectedSchedules []string
		expectedTotal     int32
		expectNextToken   bool
	}{
		{
			name:              "lists all schedules in plugin order",
			path:              "/v1/plugins",
			expectedSchedules: []string{"alpha", "zeta"},
			expectedTotal:     2,
		},
		{
			name:              "returns first page",
			path:              "/v1/plugins?pageSize=1",
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
			router := newScheduleTestRouter(t, scheduler, "zeta", "alpha")
			req := withAuth(httptest.NewRequest(http.MethodGet, tt.path, nil), testBearerToken)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			var payload ListPluginsResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
			assert.Equal(t, tt.expectedTotal, payload.TotalSize)
			assert.Equal(t, tt.expectedSchedules, pluginNames(payload.Plugins))
			if tt.expectNextToken {
				assert.NotEmpty(t, payload.NextPageToken)
			} else {
				assert.Empty(t, payload.NextPageToken)
			}
		})
	}
}

func TestGetPluginWithScheduleEndpoint(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		expectedStatus int
		expected       *scheduling.Schedule
	}{
		{
			name:           "returns configured schedule",
			path:           "/v1/plugins/mlflow",
			expectedStatus: http.StatusOK,
			expected:       &scheduling.Schedule{Plugin: "mlflow", Expression: "*/5 * * * *", Enabled: true},
		},
		{
			name:           "returns not found for unknown plugin",
			path:           "/v1/plugins/missing",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheduler := newStartedScheduleTestScheduler(t, scheduling.Schedule{Plugin: "mlflow", Expression: "*/5 * * * *", Enabled: true})
			router := newScheduleTestRouter(t, scheduler, "mlflow")
			req := withAuth(httptest.NewRequest(http.MethodGet, tt.path, nil), testBearerToken)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
			if tt.expected == nil {
				return
			}

			var actual PluginResource
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &actual))
			assert.Equal(t, PluginResource{Name: "mlflow", Schedule: "*/5 * * * *"}, actual)
		})
	}
}

func schedulerSchedules(scheduler *scheduling.Scheduler) []scheduling.Schedule {
	schedules, _ := scheduler.ListSchedules()
	return schedules
}

func pluginNames(plugins []PluginResource) []string {
	names := make([]string, len(plugins))
	for i, p := range plugins {
		names[i] = p.Name
	}
	return names
}
