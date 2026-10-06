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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/naira-project/naira/catalog/internal/auth/keycloak"
	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/catalog/catalogtest"
	"github.com/naira-project/naira/catalog/internal/pluginrun"
	"github.com/naira-project/naira/catalog/internal/pluginrun/pluginruntest"
)

const testBearerToken = "test-token"
const testIssuer = "http://localhost:8080/realms/naira"

var testSnapshotID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

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

func newTestRouter(t *testing.T, store *catalogtest.MockStore, snapshotStore *pluginruntest.MockSnapshotStore, plugins map[string]pluginrun.Plugin) (http.Handler, *pluginrun.Runner) {
	t.Helper()

	if store == nil {
		store = &catalogtest.MockStore{}
	}
	if snapshotStore == nil {
		snapshotStore = pluginruntest.NewMockSnapshotStore()
	}

	catalogService := catalog.NewService(store)
	runner := pluginrun.NewRunner(context.Background(), snapshotStore, snapshotStore, plugins, 5*time.Minute, log.New(io.Discard, "", 0))
	configs := make(catalog.PluginConfigsByName, len(plugins))
	for name := range plugins {
		configs[name] = catalog.PluginConfig{}
	}
	router, err := NewRouter(catalogService, runner, configs, log.New(io.Discard, "", 0), keycloak.Config{Client: stubTokenDecoder{}, Issuer: testIssuer})
	require.NoError(t, err)
	return router, runner
}

func TestRouterServesCurrentEndpoints(t *testing.T) {
	nodes := []catalog.Node{
		{
			ID: catalog.NodeID{Kind: "model", Path: "mlflow/fraud-detector"},
			PluginClaims: map[string]catalog.PluginClaim{
				"test-plugin": {
					SnapshotID: testSnapshotID,
					Properties: map[string]string{
						"source":      "mlflow",
						"description": "registry model",
					},
				},
			},
		},
		{
			ID: catalog.NodeID{Kind: "application", Path: "litellm/fraud-assistant"},
			PluginClaims: map[string]catalog.PluginClaim{
				"test-plugin": {
					SnapshotID: testSnapshotID,
					Properties: map[string]string{"namespace": "apps"},
				},
			},
		},
	}
	relations := []catalog.Relation{
		{
			Kind: "uses_model",
			From: catalog.NodeID{Kind: "application", Path: "litellm/fraud-assistant"},
			To:   catalog.NodeID{Kind: "model", Path: "mlflow/fraud-detector"},
			PluginClaims: map[string]catalog.PluginClaim{
				"test-plugin": {SnapshotID: testSnapshotID, Properties: map[string]string{"via": "virtual-key"}},
			},
		},
		{
			Kind: "used_by",
			From: catalog.NodeID{Kind: "model", Path: "mlflow/fraud-detector"},
			To:   catalog.NodeID{Kind: "application", Path: "litellm/fraud-assistant"},
			PluginClaims: map[string]catalog.PluginClaim{
				"test-plugin": {SnapshotID: testSnapshotID},
			},
		},
	}

	store := &catalogtest.MockStore{
		ListNodesFunc:     func(context.Context) ([]catalog.Node, error) { return nodes, nil },
		ListRelationsFunc: func(context.Context) ([]catalog.Relation, error) { return relations, nil },
		GetNodeFunc: func(_ context.Context, id catalog.NodeID) (catalog.Node, error) {
			for _, n := range nodes {
				if n.ID == id {
					return n, nil
				}
			}
			return catalog.Node{}, catalog.ErrNodeNotFound
		},
	}

	router, _ := newTestRouter(t, store, nil, map[string]pluginrun.Plugin{"seed": stubPlugin{}})

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
					Nodes: []Node{{
						Name: "nodes/model/mlflow/fraud-detector",
						Kind: "model",
						Path: "mlflow/fraud-detector",
						PluginClaims: []PluginClaim{{
							Plugin: "test-plugin",
							Props: map[string]string{
								"source":      "mlflow",
								"description": "registry model",
							},
						}},
					}},
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
					PluginClaims: []PluginClaim{{
						Plugin: "test-plugin",
						Props: map[string]string{
							"source":      "mlflow",
							"description": "registry model",
						},
					}},
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

func TestGetNodeDecodesEscapedPathSegments(t *testing.T) {
	node := catalog.Node{
		ID: catalog.NodeID{Kind: "owner", Path: "@naira-project/dev"},
		PluginClaims: map[string]catalog.PluginClaim{
			"test-plugin": {SnapshotID: testSnapshotID},
		},
	}

	store := &catalogtest.MockStore{
		GetNodeFunc: func(_ context.Context, id catalog.NodeID) (catalog.Node, error) {
			if id == node.ID {
				return node, nil
			}
			return catalog.Node{}, catalog.ErrNodeNotFound
		},
	}

	router, _ := newTestRouter(t, store, nil, nil)
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
