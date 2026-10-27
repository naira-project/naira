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
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/naira-project/naira/catalog/internal/auth/keycloak"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/operations"
	"github.com/naira-project/naira/catalog/internal/pluginrun"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
)

const testBearerToken = "test-token"
const testIssuer = "http://localhost:8080/realms/naira"

type stubTokenDecoder struct{}

func (stubTokenDecoder) DecodeAccessToken(_ context.Context, accessToken, _ string) (*jwt.Token, *jwt.MapClaims, error) {
	if accessToken != testBearerToken {
		return nil, nil, errors.New("invalid token")
	}

	claims := jwt.MapClaims{
		"sub":                "test-user",
		"preferred_username": "test-user",
		"iss":                testIssuer,
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

// blockingStubPlugin blocks Collect until the block channel is closed.
type blockingStubPlugin struct {
	block    chan struct{}
	response catalog.CollectResponse
	err      error
}

func (p blockingStubPlugin) Collect(ctx context.Context) (catalog.CollectResponse, error) {
	select {
	case <-p.block:
	case <-ctx.Done():
		return catalog.CollectResponse{}, ctx.Err()
	}
	return p.response, p.err
}

func applyPluginSnapshot(t *testing.T, store *catalog.MemoryStore, nodes []catalog.NodeClaim, relations []catalog.RelationClaim) {
	t.Helper()

	_, _, err := store.ApplyPluginSnapshot("test-plugin", uuid.MustParse("00000000-0000-0000-0000-000000000001"), nodes, relations)
	require.NoError(t, err)
}

// newTestRouter wires a fresh catalog.Service and pluginrun.Runner sharing a
// single graph store, mirroring how main.go wires the real router.
func newTestRouter(t *testing.T, store *catalog.MemoryStore, opStore operations.Store, plugins map[string]pluginrun.Plugin) http.Handler {
	t.Helper()

	catalogService := catalog.NewService(store)
	runner := pluginrun.NewRunner(context.Background(), store, opStore, plugins, 5*time.Minute, log.New(io.Discard, "", 0))
	configs := make(catalog.PluginConfigsByName, len(plugins))
	for name := range plugins {
		configs[name] = catalog.PluginConfig{}
	}
	router, err := NewRouter(catalogService, runner, configs, log.New(io.Discard, "", 0), keycloak.Config{Client: stubTokenDecoder{}, Issuer: testIssuer})
	require.NoError(t, err)
	return router
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

	router := newTestRouter(t, store, operations.NewMemoryStore(), map[string]pluginrun.Plugin{"seed": stubPlugin{}})

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

func TestListKindsEndpoint(t *testing.T) {
	store := catalog.NewMemoryStore()
	router := newTestRouter(t, store, operations.NewMemoryStore(), nil)

	t.Run("empty catalog", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, withAuth(httptest.NewRequest(http.MethodGet, "/v1/kinds", nil), testBearerToken))
		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"kinds":[],"totalSize":0}`, rec.Body.String())
	})

	app := catalog.NodeID{
		Kind: "application",
		Path: "app",
	}
	otherApp := catalog.NodeID{
		Kind: "application",
		Path: "other-app",
	}
	model := catalog.NodeID{
		Kind: "model",
		Path: "model",
	}
	applyPluginSnapshot(t, store,
		[]catalog.NodeClaim{
			{ID: app},
			{ID: otherApp},
			{ID: model},
		},
		nil,
	)

	t.Run("distinct sorted kinds", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, withAuth(httptest.NewRequest(http.MethodGet, "/v1/kinds", nil), testBearerToken))
		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"kinds":["application","model"],"totalSize":2}`, rec.Body.String())
	})

	t.Run("pagination", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, withAuth(httptest.NewRequest(http.MethodGet, "/v1/kinds?pageSize=1", nil), testBearerToken))
		require.Equal(t, http.StatusOK, rec.Code)
		var first ListKindsResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &first))
		require.NotEmpty(t, first.NextPageToken)
		assert.Equal(t, ListKindsResponse{
			Kinds:         []string{"application"},
			NextPageToken: first.NextPageToken,
			TotalSize:     2,
		}, first)

		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, withAuth(httptest.NewRequest(http.MethodGet, "/v1/kinds?pageSize=1&pageToken="+first.NextPageToken, nil), testBearerToken))
		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"kinds":["model"],"totalSize":2}`, rec.Body.String())
	})

	for _, path := range []string{
		"/v1/kinds?pageSize=-1",
		"/v1/kinds?pageToken=invalid",
		"/v1/kinds?filter=kind=%22uses_model%22",
	} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, withAuth(httptest.NewRequest(http.MethodGet, path, nil), testBearerToken))
			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}

	t.Run("requires authentication", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/kinds", nil))
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

func TestGetNodeDecodesEscapedPathSegments(t *testing.T) {
	store := catalog.NewMemoryStore()
	applyPluginSnapshot(t, store, []catalog.NodeClaim{{
		ID: catalog.NodeID{Kind: "owner", Path: "@naira-project/dev"},
	}}, nil)

	router := newTestRouter(t, store, operations.NewMemoryStore(), nil)
	req := withAuth(httptest.NewRequest(http.MethodGet, "/v1/nodes/owner/%40naira-project/dev", nil), testBearerToken)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var response Node
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	assert.Equal(t, Node{
		Name: "nodes/owner/@naira-project/dev",
		Kind: "owner",
		Path: "@naira-project/dev",
		PluginClaims: []PluginClaim{{
			Plugin: "test-plugin",
			Props:  map[string]string{},
		}},
	}, response)
}

func TestAuthVerify(t *testing.T) {
	router := newTestRouter(t, catalog.NewMemoryStore(), operations.NewMemoryStore(), nil)

	tests := []struct {
		name               string
		authorization      string
		expectedStatusCode int
	}{
		{name: "accepts valid token", authorization: "Bearer test-token", expectedStatusCode: http.StatusNoContent},
		{name: "rejects missing header", authorization: "", expectedStatusCode: http.StatusUnauthorized},
		{name: "rejects invalid token", authorization: "Bearer wrong-token", expectedStatusCode: http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/auth/verify", nil)
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectedStatusCode, rec.Code)
		})
	}
}
