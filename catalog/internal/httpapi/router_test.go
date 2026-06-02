package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/pluginapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubPlugin struct {
	name    string
	request catalog.IngestionRequest
	err     error
}

func (p stubPlugin) Name() string {
	return p.name
}

func (p stubPlugin) Collect(context.Context) (catalog.IngestionRequest, error) {
	return p.request, p.err
}

func upsertGraph(t *testing.T, store *catalog.MemoryStore, nodes []catalog.NodeClaim, relations []catalog.RelationClaim) {
	t.Helper()

	_, _, err := store.UpsertGraph(nodes, relations)
	require.NoError(t, err)
}

func TestRouterServesCurrentEndpoints(t *testing.T) {
	store := catalog.NewMemoryStore()
	upsertGraph(t, store,
		[]catalog.NodeClaim{
			{
				ID: catalog.NodeID{Kind: "model", Path: "mlflow/fraud-detector"},
				Properties: pluginapi.PropertyMap{
					"source":      "mlflow",
					"description": "registry model",
				},
			},
			{
				ID: catalog.NodeID{Kind: "application", Path: "litellm/fraud-assistant"},
				Properties: pluginapi.PropertyMap{
					"namespace": "apps",
				},
			},
		},
		[]catalog.RelationClaim{{
			Kind: "uses_model",
			From: catalog.NodeID{Kind: "application", Path: "litellm/fraud-assistant"},
			To:   catalog.NodeID{Kind: "model", Path: "mlflow/fraud-detector"},
		}},
	)

	router := NewRouter(catalog.NewService(store, log.New(io.Discard, "", 0), stubPlugin{name: "seed"}), log.New(io.Discard, "", 0))

	tests := []struct {
		name               string
		method             string
		path               string
		expectedStatusCode int
		validatePayload    func(*testing.T, []byte)
	}{
		{
			name:               "returns health status",
			method:             http.MethodGet,
			path:               "/healthz",
			expectedStatusCode: http.StatusOK,
			validatePayload: func(t *testing.T, body []byte) {
				var payload map[string]string
				require.NoError(t, json.Unmarshal(body, &payload))
				assert.Equal(t, "ok", payload["status"])
			},
		},
		{
			name:               "lists models",
			method:             http.MethodGet,
			path:               "/v1/nodes?filter=kind=%22model%22",
			expectedStatusCode: http.StatusOK,
			validatePayload: func(t *testing.T, body []byte) {
				var payload ListNodesResponse
				require.NoError(t, json.Unmarshal(body, &payload))
				require.Len(t, payload.Nodes, 1)
				assert.Equal(t, "nodes/model/mlflow/fraud-detector", payload.Nodes[0].Name)
			},
		},
		{
			name:               "returns node",
			method:             http.MethodGet,
			path:               "/v1/nodes/model/mlflow/fraud-detector",
			expectedStatusCode: http.StatusOK,
			validatePayload: func(t *testing.T, body []byte) {
				var payload Node
				require.NoError(t, json.Unmarshal(body, &payload))
				assert.Equal(t, "model", payload.Kind)
				assert.Equal(t, "mlflow/fraud-detector", payload.Path)
			},
		},
		{
			name:               "lists relations",
			method:             http.MethodGet,
			path:               "/v1/relations?filter=toNode=%22nodes/model/mlflow/fraud-detector%22",
			expectedStatusCode: http.StatusOK,
			validatePayload: func(t *testing.T, body []byte) {
				var payload ListRelationsResponse
				require.NoError(t, json.Unmarshal(body, &payload))
				require.Len(t, payload.Relations, 1)
				assert.Equal(t, "relations/uses_model/nodes%2Fapplication%2Flitellm%2Ffraud-assistant|nodes%2Fmodel%2Fmlflow%2Ffraud-detector", payload.Relations[0].Name)
				assert.Equal(t, "uses_model", payload.Relations[0].Kind)
			},
		},
		{
			name:               "run all plugins",
			method:             http.MethodPost,
			path:               "/v1/plugins:run",
			expectedStatusCode: http.StatusAccepted,
			validatePayload: func(t *testing.T, body []byte) {
				var payload RunPluginsResponse
				require.NoError(t, json.Unmarshal(body, &payload))
				require.Len(t, payload.Results, 1)
				assert.Equal(t, "seed", payload.Results[0].Plugin)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectedStatusCode, rec.Code)
			if tt.validatePayload != nil {
				tt.validatePayload(t, rec.Body.Bytes())
			}
		})
	}
}

func TestRunAllPluginsReturnsPluginErrorsInResults(t *testing.T) {
	router := NewRouter(catalog.NewService(
		catalog.NewMemoryStore(),
		log.New(io.Discard, "", 0),
		stubPlugin{name: "seed", err: errors.New("seed failed")},
	), log.New(io.Discard, "", 0))

	req := httptest.NewRequest(http.MethodPost, "/v1/plugins:run", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusAccepted, rec.Code)

	var payload RunPluginsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Len(t, payload.Results, 1)
	assert.Equal(t, "seed", payload.Results[0].Plugin)
	assert.Equal(t, "collecting response from plugin \"seed\": seed failed", payload.Results[0].Error)
}
