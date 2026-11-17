package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/httpapi/mocks"
)

func TestListPluginsEndpoint(t *testing.T) {
	router, err := NewRouter(&mocks.CatalogReaderMock{}, &mocks.PluginRunnerMock{}, catalog.PluginConfigsByName{
		"zeta":   {Schedule: "0 0 * * *"},
		"alpha":  {Schedule: "*/5 * * * *"},
		"manual": {},
	}, discardLogger, testKeycloak)
	require.NoError(t, err)

	tests := []struct {
		name            string
		path            string
		want            []PluginResource
		expectNextToken bool
	}{
		{
			name: "lists all plugins in deterministic order",
			path: "/v1/plugins",
			want: []PluginResource{
				{Name: "alpha", Schedule: "*/5 * * * *"},
				{Name: "manual"},
				{Name: "zeta", Schedule: "0 0 * * *"},
			},
		},
		{
			name:            "returns first page",
			path:            "/v1/plugins?pageSize=1",
			want:            []PluginResource{{Name: "alpha", Schedule: "*/5 * * * *"}},
			expectNextToken: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doRequest(t, router, http.MethodGet, tt.path)

			require.Equal(t, http.StatusOK, rec.Code)
			var payload ListPluginsResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
			assert.Equal(t, int32(3), payload.TotalSize)
			assert.Equal(t, tt.want, payload.Plugins)
			if tt.expectNextToken {
				assert.NotEmpty(t, payload.NextPageToken)
			} else {
				assert.Empty(t, payload.NextPageToken)
			}
		})
	}
}

func TestGetPluginEndpoint(t *testing.T) {
	router, err := NewRouter(&mocks.CatalogReaderMock{}, &mocks.PluginRunnerMock{}, catalog.PluginConfigsByName{
		"mlflow": {Schedule: "*/5 * * * *"},
	}, discardLogger, testKeycloak)
	require.NoError(t, err)

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		want           PluginResource
	}{
		{
			name:           "returns configured plugin",
			path:           "/v1/plugins/mlflow",
			expectedStatus: http.StatusOK,
			want:           PluginResource{Name: "mlflow", Schedule: "*/5 * * * *"},
		},
		{
			name:           "returns not found for unknown plugin",
			path:           "/v1/plugins/missing",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doRequest(t, router, http.MethodGet, tt.path)

			require.Equal(t, tt.expectedStatus, rec.Code)
			if tt.want.Name == "" {
				return
			}

			var payload PluginResource
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
			assert.Equal(t, tt.want, payload)
		})
	}
}
