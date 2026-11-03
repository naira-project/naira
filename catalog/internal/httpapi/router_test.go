package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/naira-project/naira/catalog/internal/auth/keycloak"
	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/httpapi/mocks"
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

type routerDeps struct {
	reader  catalog.Reader
	runner  PluginRunner
	plugins catalog.PluginConfigsByName
}

func newTestRouter(t *testing.T, deps routerDeps) http.Handler {
	t.Helper()

	if deps.reader == nil {
		deps.reader = &mocks.CatalogReaderMock{}
	}
	if deps.runner == nil {
		deps.runner = &mocks.PluginRunnerMock{}
	}

	router, err := NewRouter(deps.reader, deps.runner, deps.plugins, nil, keycloak.Config{Client: stubTokenDecoder{}, Issuer: testIssuer})
	require.NoError(t, err)
	return router
}

// doRequest executes an authenticated HTTP request against the router.
func doRequest(t *testing.T, router http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()

	req := withAuth(httptest.NewRequest(method, path, nil), testBearerToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// testClaims builds the claims of a single plugin for catalog fixtures.
func testClaims(props map[string]string) map[string]catalog.PluginClaim {
	return map[string]catalog.PluginClaim{
		"test-plugin": {Properties: props},
	}
}

var (
	modelNode = catalog.Node{
		ID:           catalog.NodeID{Kind: "model", Path: "mlflow/fraud-detector"},
		PluginClaims: testClaims(map[string]string{"source": "mlflow", "description": "registry model"}),
	}
	appNode = catalog.Node{
		ID:           catalog.NodeID{Kind: "application", Path: "litellm/fraud-assistant"},
		PluginClaims: testClaims(map[string]string{"namespace": "apps"}),
	}
	usesModelRelation = catalog.Relation{
		Kind:         "uses_model",
		From:         appNode.ID,
		To:           modelNode.ID,
		PluginClaims: testClaims(map[string]string{"via": "virtual-key"}),
	}
	usedByRelation = catalog.Relation{
		Kind:         "used_by",
		From:         modelNode.ID,
		To:           appNode.ID,
		PluginClaims: testClaims(nil),
	}
)

func TestRouterServesCurrentEndpoints(t *testing.T) {
	reader := &mocks.CatalogReaderMock{
		ListNodesFunc: func(context.Context) ([]catalog.Node, error) {
			return []catalog.Node{modelNode, appNode}, nil
		},
		GetNodeFunc: func(context.Context, catalog.NodeID) (catalog.Node, error) {
			return modelNode, nil
		},
		ListRelationsFunc: func(context.Context) ([]catalog.Relation, error) {
			return []catalog.Relation{usesModelRelation, usedByRelation}, nil
		},
	}
	router := newTestRouter(t, routerDeps{reader: reader})

	tests := []struct {
		name            string
		path            string
		validatePayload func(*testing.T, []byte)
	}{
		{
			name: "returns health status",
			path: "/healthz",
			validatePayload: func(t *testing.T, body []byte) {
				assert.JSONEq(t, `{"status":"ok"}`, string(body))
			},
		},
		{
			name: "lists models",
			path: "/v1/nodes?filter=kind=%22model%22",
			validatePayload: func(t *testing.T, body []byte) {
				var payload ListNodesResponse
				require.NoError(t, json.Unmarshal(body, &payload))

				assert.Equal(t, ListNodesResponse{
					Nodes: []Node{{
						Name: "nodes/model/mlflow/fraud-detector",
						Kind: "model",
						Path: "mlflow/fraud-detector",
						PluginClaims: []PluginClaim{{
							Plugin: "test-plugin",
							Props:  map[string]string{"source": "mlflow", "description": "registry model"},
						}},
					}},
					TotalSize: 1,
				}, payload)
			},
		},
		{
			name: "returns node",
			path: "/v1/nodes/model/mlflow/fraud-detector",
			validatePayload: func(t *testing.T, body []byte) {
				var payload Node
				require.NoError(t, json.Unmarshal(body, &payload))

				assert.Equal(t, Node{
					Name: "nodes/model/mlflow/fraud-detector",
					Kind: "model",
					Path: "mlflow/fraud-detector",
					PluginClaims: []PluginClaim{{
						Plugin: "test-plugin",
						Props:  map[string]string{"source": "mlflow", "description": "registry model"},
					}},
				}, payload)
			},
		},
		{
			name: "filters relations by toNode",
			path: "/v1/relations?filter=toNode=%22nodes/model/mlflow/fraud-detector%22",
			validatePayload: func(t *testing.T, body []byte) {
				var payload ListRelationsResponse
				require.NoError(t, json.Unmarshal(body, &payload))

				assert.Equal(t, ListRelationsResponse{
					Relations: []Relation{{
						Name:     "relations/uses_model/nodes%2Fapplication%2Flitellm%2Ffraud-assistant|nodes%2Fmodel%2Fmlflow%2Ffraud-detector",
						Kind:     "uses_model",
						FromNode: "nodes/application/litellm/fraud-assistant",
						ToNode:   "nodes/model/mlflow/fraud-detector",
						PluginClaims: []PluginClaim{{
							Plugin: "test-plugin",
							Props:  map[string]string{"via": "virtual-key"},
						}},
					}},
					TotalSize: 1,
				}, payload)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := doRequest(t, router, http.MethodGet, tt.path)

			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			tt.validatePayload(t, rec.Body.Bytes())
		})
	}
}

func TestListNodesSortsByName(t *testing.T) {
	reader := &mocks.CatalogReaderMock{
		ListNodesFunc: func(context.Context) ([]catalog.Node, error) {
			return []catalog.Node{modelNode, appNode}, nil // intentionally not sorted
		},
	}
	router := newTestRouter(t, routerDeps{reader: reader})

	rec := doRequest(t, router, http.MethodGet, "/v1/nodes")

	require.Equal(t, http.StatusOK, rec.Code)
	var payload ListNodesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Len(t, payload.Nodes, 2)
	assert.Equal(t, "nodes/application/litellm/fraud-assistant", payload.Nodes[0].Name)
	assert.Equal(t, "nodes/model/mlflow/fraud-detector", payload.Nodes[1].Name)
}

func TestGetNode(t *testing.T) {
	t.Run("decodes escaped path segments", func(t *testing.T) {
		node := catalog.Node{
			ID:           catalog.NodeID{Kind: "owner", Path: "@naira-project/dev"},
			PluginClaims: testClaims(nil),
		}
		reader := &mocks.CatalogReaderMock{
			GetNodeFunc: func(context.Context, catalog.NodeID) (catalog.Node, error) {
				return node, nil
			},
		}
		router := newTestRouter(t, routerDeps{reader: reader})

		rec := doRequest(t, router, http.MethodGet, "/v1/nodes/owner/%40naira-project/dev")

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		var response Node
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
		assert.Equal(t, Node{
			Name:         "nodes/owner/@naira-project/dev",
			Kind:         "owner",
			Path:         "@naira-project/dev",
			PluginClaims: []PluginClaim{{Plugin: "test-plugin", Props: map[string]string{}}},
		}, response)
	})

	t.Run("maps missing node to 404", func(t *testing.T) {
		reader := &mocks.CatalogReaderMock{
			GetNodeFunc: func(context.Context, catalog.NodeID) (catalog.Node, error) {
				return catalog.Node{}, fmt.Errorf("node: %w", catalog.ErrNodeNotFound)
			},
		}
		router := newTestRouter(t, routerDeps{reader: reader})

		rec := doRequest(t, router, http.MethodGet, "/v1/nodes/model/missing")

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestListKindsEndpoint(t *testing.T) {
	newKindsRouter := func(t *testing.T, kinds []string) (http.Handler, *mocks.CatalogReaderMock) {
		t.Helper()
		reader := &mocks.CatalogReaderMock{
			ListKindsFunc: func(context.Context) ([]string, error) { return kinds, nil },
		}
		return newTestRouter(t, routerDeps{reader: reader}), reader
	}

	t.Run("empty catalog", func(t *testing.T) {
		router, _ := newKindsRouter(t, nil)

		rec := doRequest(t, router, http.MethodGet, "/v1/kinds")

		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"kinds":[],"totalSize":0}`, rec.Body.String())
	})

	t.Run("returns kinds from the reader", func(t *testing.T) {
		router, _ := newKindsRouter(t, []string{"application", "model"})

		rec := doRequest(t, router, http.MethodGet, "/v1/kinds")

		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"kinds":["application","model"],"totalSize":2}`, rec.Body.String())
	})

	t.Run("pagination", func(t *testing.T) {
		router, _ := newKindsRouter(t, []string{"application", "model"})

		rec := doRequest(t, router, http.MethodGet, "/v1/kinds?pageSize=1")
		require.Equal(t, http.StatusOK, rec.Code)
		var first ListKindsResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &first))
		require.NotEmpty(t, first.NextPageToken)
		assert.Equal(t, ListKindsResponse{
			Kinds:         []string{"application"},
			NextPageToken: first.NextPageToken,
			TotalSize:     2,
		}, first)

		rec = doRequest(t, router, http.MethodGet, "/v1/kinds?pageSize=1&pageToken="+first.NextPageToken)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"kinds":["model"],"totalSize":2}`, rec.Body.String())
	})

	for _, path := range []string{
		"/v1/kinds?pageSize=-1",
		"/v1/kinds?pageToken=invalid",
		"/v1/kinds?filter=kind=%22uses_model%22",
	} {
		t.Run("rejects "+path, func(t *testing.T) {
			router, reader := newKindsRouter(t, nil)

			rec := doRequest(t, router, http.MethodGet, path)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Empty(t, reader.ListKindsCalls(), "invalid requests must not reach the reader")
		})
	}

	t.Run("maps reader failure to 500", func(t *testing.T) {
		reader := &mocks.CatalogReaderMock{
			ListKindsFunc: func(context.Context) ([]string, error) { return nil, errors.New("database offline") },
		}
		router := newTestRouter(t, routerDeps{reader: reader})

		rec := doRequest(t, router, http.MethodGet, "/v1/kinds")

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("requires authentication", func(t *testing.T) {
		router, _ := newKindsRouter(t, nil)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/kinds", nil))

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

func TestAuthVerify(t *testing.T) {
	router := newTestRouter(t, routerDeps{})

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
