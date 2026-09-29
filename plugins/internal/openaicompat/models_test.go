package openaicompat

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchModels(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantModels []Datum
		wantErr    bool
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
			name:       "401 Unauthorized is reported as error",
			statusCode: http.StatusUnauthorized,
			body:       `whatever`,
			wantErr:    true,
		},
		{
			name:       "403 Forbidden is reported as error",
			statusCode: http.StatusForbidden,
			body:       `whatever`,
			wantErr:    true,
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

			var resp DataResponse[Datum]
			err := GetModels(context.Background(), mockServer.Client(), mockServer.URL, testToken, &resp)

			if tt.wantErr {
				require.Error(t, err)
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

	var resp DataResponse[Datum]
	err := GetModels(context.Background(), mockServer.Client(), mockServer.URL+"/base/", "  ", &resp)
	require.NoError(t, err)
	assert.Equal(t, []string{"local-model"}, modelIDs(resp.Data))
}

// ModelIDs reduces a datum list to its IDs, preserving order.
func modelIDs(models []Datum) []string {
	ids := make([]string, 0, len(models))
	for _, m := range models {
		ids = append(ids, m.ID)
	}
	return ids
}

// ExampleGetModels demonstrates decoding a llama.cpp /v1/models response,
// which includes extra fields not present in the usual OpenAI response:
//   - a "models" field in the root object,
//   - a "meta" field in each data[] sub-object.
//
// Some sub-fields were removed for brevity.
func ExampleGetModels() {
	const llamacppResponse = `{
      "models": [
        {
          "name": "test-model-name",
          "capabilities": ["completion"]
        }
      ],
      "object": "list",
      "data": [
        {
          "id": "test-model-name",
          "aliases": ["test-model-name"],
          "object": "model",
          "created": 1790088392,
          "owned_by": "llamacpp",
          "meta": {
            "n_params": 292800,
            "ftype": "(guessed) all F32"
          }
        }
      ]
    }`

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, llamacppResponse)
	}))
	defer mockServer.Close()

	type myDatum struct {
		Datum
		Aliases []string       `json:"aliases"`
		Meta    map[string]any `json:"meta"`
	}
	type myModelsResponse struct {
		DataResponse[myDatum]
		Models []map[string]any `json:"models"`
	}

	var resp myModelsResponse
	if err := GetModels(context.Background(), mockServer.Client(), mockServer.URL, "", &resp); err != nil {
		fmt.Println("error:", err)
		return
	}

	datum := resp.Data[0]
	fmt.Println("data[].id:", datum.ID)
	fmt.Println("data[].owned_by:", datum.OwnedBy)
	fmt.Println("data[].aliases:", datum.Aliases)
	fmt.Println("data[].meta.n_params:", datum.Meta["n_params"])
	fmt.Println("data[].meta.ftype:", datum.Meta["ftype"])
	fmt.Println("models[].name:", resp.Models[0]["name"])
	fmt.Println("models[].capabilities:", resp.Models[0]["capabilities"])

	// Output:
	// data[].id: test-model-name
	// data[].owned_by: llamacpp
	// data[].aliases: [test-model-name]
	// data[].meta.n_params: 292800
	// data[].meta.ftype: (guessed) all F32
	// models[].name: test-model-name
	// models[].capabilities: [completion]
}
