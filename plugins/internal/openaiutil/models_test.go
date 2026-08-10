package openaiutil

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchModels(t *testing.T) {
	tests := []struct {
		name            string
		statusCode      int
		body            string
		wantModels      []Model
		wantErr         bool
		wantUnauthorize bool
	}{
		{
			name:       "200 OK returns models",
			statusCode: http.StatusOK,
			body:       `{"data":[{"id":"gpt-4o","owned_by":"openai"},{"id":"claude-sonnet-5","owned_by":"anthropic"}]}`,
			wantModels: []Model{
				{ID: "gpt-4o", OwnedBy: "openai"},
				{ID: "claude-sonnet-5", OwnedBy: "anthropic"},
			},
		},
		{
			name:       "empty data array returns no models",
			statusCode: http.StatusOK,
			body:       `{"data":[]}`,
			wantModels: []Model{},
		},
		{
			name:            "401 Unauthorized is reported as ErrUnauthorized",
			statusCode:      http.StatusUnauthorized,
			body:            `whatever`,
			wantErr:         true,
			wantUnauthorize: true,
		},
		{
			name:            "403 Forbidden is reported as ErrUnauthorized",
			statusCode:      http.StatusForbidden,
			body:            `whatever`,
			wantErr:         true,
			wantUnauthorize: true,
		},
		{
			name:       "non-2xx status returns error",
			statusCode: http.StatusInternalServerError,
			body:       `{"data":[]}`,
			wantErr:    true,
		},
		{
			name:       "invalid JSON body returns error",
			statusCode: http.StatusOK,
			body:       `not-json`,
			wantErr:    true,
		},
	}

	const testToken = "test-api-key"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/v1/models", r.URL.Path)
				assert.Equal(t, "Bearer "+testToken, r.Header.Get("Authorization"))
				w.WriteHeader(tt.statusCode)
				fmt.Fprint(w, tt.body)
			}))
			defer mockServer.Close()

			models, err := FetchModels(context.Background(), mockServer.Client(), mockServer.URL, testToken)

			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, tt.wantUnauthorize, errors.Is(err, ErrUnauthorized))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantModels, models)
		})
	}
}

func TestFetchModelsTrimsTrailingSlashAndOmitsEmptyToken(t *testing.T) {
	mockServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/base/v1/models", r.URL.Path)
		assert.Empty(t, r.Header.Get("Authorization"))
		fmt.Fprint(w, `{"data":[{"id":"local-model"}]}`)
	}))
	defer mockServer.Close()

	models, err := FetchModels(context.Background(), mockServer.Client(), mockServer.URL+"/base/", "  ")
	require.NoError(t, err)
	assert.Equal(t, []string{"local-model"}, ModelIDs(models))
}
