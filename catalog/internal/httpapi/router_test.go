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

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/naira-project/naira/catalog/internal/auth/keycloak"
	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testBearerToken = "test-token"

type stubTokenDecoder struct{}

func (stubTokenDecoder) DecodeAccessToken(_ context.Context, accessToken, _ string) (*jwt.Token, *jwt.MapClaims, error) {
	if accessToken != testBearerToken {
		return nil, nil, errors.New("invalid token")
	}

	claims := jwt.MapClaims{
		"sub":                "test-user",
		"preferred_username": "test-user",
	}
	return nil, &claims, nil
}

func withAuth(req *http.Request, bearerToken string) *http.Request {
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	return req
}

type stubPlugin struct {
	response catalog.CollectResponse
	err      error
}

func (p stubPlugin) Collect(context.Context) (catalog.CollectResponse, error) {
	return p.response, p.err
}

func applyPluginSnapshot(t *testing.T, store *catalog.MemoryStore, nodes []catalog.NodeClaim, relations []catalog.RelationClaim) {
	t.Helper()

	_, _, err := store.ApplyPluginSnapshot("test-plugin", uuid.MustParse("00000000-0000-0000-0000-000000000001"), nodes, relations)
	require.NoError(t, err)
}

func TestRouterServesCurrentEndpoints(t *testing.T) {
	store := catalog.NewMemoryStore()
	applyPluginSnapshot(t, store,
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
		[]catalog.RelationClaim{
			{
				Kind:       "uses_model",
				From:       catalog.NodeID{Kind: "application", Path: "litellm/fraud-assistant"},
				To:         catalog.NodeID{Kind: "model", Path: "mlflow/fraud-detector"},
				Properties: pluginapi.PropertyMap{"via": "virtual-key"},
			},
			{
				Kind: "used_by",
				From: catalog.NodeID{Kind: "model", Path: "mlflow/fraud-detector"},
				To:   catalog.NodeID{Kind: "application", Path: "litellm/fraud-assistant"},
			}},
	)

	router := NewRouter(catalog.NewService(
		store,
		map[string]catalog.Plugin{"seed": stubPlugin{}},
		log.New(io.Discard, "", 0),
	), log.New(io.Discard, "", 0), keycloak.Config{Client: stubTokenDecoder{}})

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
				assert.Equal(t, map[string]string{"status": "ok"}, payload)
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

				expected := ListNodesResponse{
					Nodes: []Node{
						{
							Name: "nodes/model/mlflow/fraud-detector",
							Kind: "model",
							Path: "mlflow/fraud-detector",
							PluginClaims: []PluginClaim{
								{
									Plugin: "test-plugin",
									Props: map[string]string{
										"source":      "mlflow",
										"description": "registry model",
									},
								},
							},
						},
					},
					TotalSize: 1,
				}
				assert.Equal(t, expected, payload)
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

				expected := Node{
					Kind: "model",
					Path: "mlflow/fraud-detector",
					Name: "nodes/model/mlflow/fraud-detector",
					PluginClaims: []PluginClaim{
						{
							Plugin: "test-plugin",
							Props: map[string]string{
								"source":      "mlflow",
								"description": "registry model",
							},
						},
					},
				}
				assert.Equal(t, expected, payload)
			},
		},
		{
			name:               "filters relations by toNode",
			method:             http.MethodGet,
			path:               "/v1/relations?filter=toNode=%22nodes/model/mlflow/fraud-detector%22",
			expectedStatusCode: http.StatusOK,
			validatePayload: func(t *testing.T, body []byte) {
				var payload ListRelationsResponse
				require.NoError(t, json.Unmarshal(body, &payload))

				expected := ListRelationsResponse{
					Relations: []Relation{
						{
							Name:     "relations/uses_model/nodes%2Fapplication%2Flitellm%2Ffraud-assistant|nodes%2Fmodel%2Fmlflow%2Ffraud-detector",
							Kind:     "uses_model",
							FromNode: "nodes/application/litellm/fraud-assistant",
							ToNode:   "nodes/model/mlflow/fraud-detector",
							PluginClaims: []PluginClaim{
								{
									Plugin: "test-plugin",
									Props:  map[string]string{"via": "virtual-key"},
								},
							},
						},
					},
					TotalSize: 1,
				}
				assert.Equal(t, expected, payload)
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

				expected := RunPluginsResponse{
					Results: []RunPluginResult{
						{Plugin: "seed", Error: ""},
					},
				}
				assert.Equal(t, expected, payload)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := withAuth(httptest.NewRequest(tt.method, tt.path, nil), testBearerToken)
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
		map[string]catalog.Plugin{"seed": stubPlugin{err: errors.New("seed failed")}},
		log.New(io.Discard, "", 0),
	), log.New(io.Discard, "", 0), keycloak.Config{Client: stubTokenDecoder{}})

	req := withAuth(httptest.NewRequest(http.MethodPost, "/v1/plugins:run", nil), testBearerToken)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusAccepted, rec.Code)

	var payload RunPluginsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))

	expected := RunPluginsResponse{
		Results: []RunPluginResult{
			{
				Plugin: "seed",
				Error:  `collecting response from plugin "seed": seed failed`,
			},
		},
	}
	assert.Equal(t, expected, payload)
}

