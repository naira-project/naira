package openaiutil

import (
	"context"
	"encoding/json"
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
		wantModels      []Datum
		wantErr         bool
		wantUnauthorize bool
	}{
		{
			name:       "200 OK returns models",
			statusCode: http.StatusOK,
			body:       `{"data":[{"id":"gpt-4o","owned_by":"openai"},{"id":"claude-sonnet-5","owned_by":"anthropic"}]}`,
			wantModels: []Datum{
				{ID: "gpt-4o", OwnedBy: "openai"},
				{ID: "claude-sonnet-5", OwnedBy: "anthropic"},
			},
		},
		{
			name:       "empty data array returns no models",
			statusCode: http.StatusOK,
			body:       `{"data":[]}`,
			wantModels: []Datum{},
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

			var resp ModelsResponse[Datum]
			err := FetchModels(context.Background(), mockServer.Client(), mockServer.URL, testToken, &resp)

			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, tt.wantUnauthorize, errors.Is(err, ErrUnauthorized))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantModels, resp.Data)
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

	var resp ModelsResponse[Datum]
	err := FetchModels(context.Background(), mockServer.Client(), mockServer.URL+"/base/", "  ", &resp)
	require.NoError(t, err)
	assert.Equal(t, []string{"local-model"}, ModelIDs(resp.Data))
}

func TestGetModels_LlamacppResponseWithComplexExtras(t *testing.T) {
	// Real response from llamacpp, with extra fields not present in usual /v1/models response:
	// - "models" field in the root object,
	// - "meta" field in the data[] sub-object.
	// Some sub-fields were removed for brevity.
	const llamacppResponse = `{
      "models": [
        {
          "name": "test-model-name",
          "model": "test-model-name",
          "capabilities": [
            "completion"
          ],
          "details": {
            "format": "gguf"
          }
        }
      ],
      "object": "list",
      "data": [
        {
          "id": "test-model-name",
          "aliases": [
            "test-model-name"
          ],
          "object": "model",
          "created": 1790088392,
          "owned_by": "llamacpp",
          "meta": {
            "n_params": 292800,
            "size": 1171200,
            "ftype": "(guessed) all F32"
          }
        }
      ]
    }`

	type myDatum struct {
		Datum
		Aliases []string               `json:"aliases"`
		Meta    map[string]interface{} `json:"meta"`
	}
	type myModelsResponse struct {
		ModelsResponse[myDatum]
		Models []map[string]interface{} `json:"models"`
	}

	v := myModelsResponse{}
	err := json.Unmarshal([]byte(llamacppResponse), &v)
	require.NoError(t, err)

	wantResponse := myModelsResponse{
		Models: []map[string]interface{}{
			{
				"name":  "test-model-name",
				"model": "test-model-name",
				"capabilities": []any{
					"completion",
				},
				"details": map[string]any{
					"format": "gguf",
				},
			},
		},
		ModelsResponse: ModelsResponse[myDatum]{
			Data: []myDatum{
				{
					Datum: Datum{
						ID:      "test-model-name",
						Object:  "model",
						Created: 1790088392,
						OwnedBy: "llamacpp",
					},
					Aliases: []string{"test-model-name"},
					Meta: map[string]interface{}{
						"n_params": float64(292800),
						"size":     float64(1171200),
						"ftype":    "(guessed) all F32",
					},
				},
			},
		},
	}
	assert.Equal(t, wantResponse, v)
}
