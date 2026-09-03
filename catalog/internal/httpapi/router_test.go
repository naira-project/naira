package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
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

// waitForOperation polls the operation store until the operation reaches a
// terminal state (SUCCEEDED or FAILED) or the timeout elapses.
func waitForOperation(t *testing.T, opStore operations.Store, name string) operations.Operation {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		op, err := opStore.Get(name)
		if err == nil && (op.State == operations.StateSucceeded || op.State == operations.StateFailed) {
			return op
		}
		time.Sleep(10 * time.Millisecond)
	}

	op, err := opStore.Get(name)
	if err != nil {
		t.Fatalf("operation %q: %v", name, err)
	}
	t.Fatalf("operation %q state = %s, want terminal state", name, op.State)
	return operations.Operation{}
}

// waitForOperationState polls the operation store until the operation reaches
// the given state or the timeout elapses.
func waitForOperationState(t *testing.T, opStore operations.Store, name string, state operations.State) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		op, err := opStore.Get(name)
		if err == nil && op.State == state {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	op, err := opStore.Get(name)
	if err != nil {
		t.Fatalf("operation %q: %v", name, err)
	}
	t.Fatalf("operation %q state = %s, want %s", name, op.State, state)
}

// newTestRouter wires a fresh catalog.Service and pluginrun.Runner sharing a
// single graph store, mirroring how main.go wires the real router.
func newTestRouter(t *testing.T, store *catalog.MemoryStore, opStore operations.Store, plugins map[string]pluginrun.Plugin) http.Handler {
	t.Helper()

	catalogService := catalog.NewService(store)
	runner := pluginrun.NewRunner(context.Background(), store, opStore, plugins, 5*time.Minute, log.New(io.Discard, "", 0))
	definitions := make(map[string]catalog.PluginDefinition, len(plugins))
	for name := range plugins {
		definitions[name] = catalog.PluginDefinition{}
	}
	router, err := NewRouter(catalogService, runner, definitions, log.New(io.Discard, "", 0), keycloak.Config{Client: stubTokenDecoder{}, Issuer: testIssuer})
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

func TestRunAllPluginsReturnsPluginErrorsInResults(t *testing.T) {
	opStore := operations.NewMemoryStore()
	router := newTestRouter(t, catalog.NewMemoryStore(), opStore, map[string]pluginrun.Plugin{"seed": stubPlugin{err: errors.New("seed failed")}})

	req := withAuth(httptest.NewRequest(http.MethodPost, "/v1/plugins:run", nil), testBearerToken)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusAccepted, rec.Code)

	var payload RunPluginsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Len(t, payload.Operations, 1)

	op := payload.Operations[0]
	assert.Equal(t, "seed", op.Plugin)

	// Wait for the async run to complete.
	completed := waitForOperation(t, opStore, op.Name)
	assert.Equal(t, operations.StateFailed, completed.State)
	require.NotNil(t, completed.Error)
	assert.Contains(t, completed.Error.Message, "seed failed")
}

func TestRunPluginAsyncEndpoint(t *testing.T) {
	opStore := operations.NewMemoryStore()
	router := newTestRouter(t, catalog.NewMemoryStore(), opStore, map[string]pluginrun.Plugin{"mlflow": stubPlugin{}})

	req := withAuth(httptest.NewRequest(http.MethodPost, "/v1/plugins/mlflow:run", nil), testBearerToken)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusAccepted, rec.Code)

	var op OperationResource
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &op))
	assert.Equal(t, "mlflow", op.Plugin)
	assert.Equal(t, "PENDING", op.State)

	// Wait for completion.
	completed := waitForOperation(t, opStore, op.Name)
	assert.Equal(t, operations.StateSucceeded, completed.State)
}

func TestRunPluginAsyncEndpointUnknownPlugin(t *testing.T) {
	router := newTestRouter(t, catalog.NewMemoryStore(), operations.NewMemoryStore(), nil)

	req := withAuth(httptest.NewRequest(http.MethodPost, "/v1/plugins/missing:run", nil), testBearerToken)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRunPluginAsyncEndpointConflict(t *testing.T) {
	opStore := operations.NewMemoryStore()
	block := make(chan struct{})
	store := catalog.NewMemoryStore()
	catalogService := catalog.NewService(store)
	runner := pluginrun.NewRunner(context.Background(), store, opStore, map[string]pluginrun.Plugin{"mlflow": blockingStubPlugin{block: block}}, 5*time.Minute, log.New(io.Discard, "", 0))
	router, err := NewRouter(catalogService, runner, map[string]catalog.PluginDefinition{"mlflow": {}}, log.New(io.Discard, "", 0), keycloak.Config{Client: stubTokenDecoder{}, Issuer: testIssuer})
	require.NoError(t, err)

	// First request — starts the plugin run.
	req1 := withAuth(httptest.NewRequest(http.MethodPost, "/v1/plugins/mlflow:run", nil), testBearerToken)
	rec1 := httptest.NewRecorder()
	router.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusAccepted, rec1.Code)

	// Wait until the operation is RUNNING.
	var firstOp OperationResource
	require.NoError(t, json.Unmarshal(rec1.Body.Bytes(), &firstOp))
	waitForOperationState(t, opStore, firstOp.Name, operations.StateRunning)

	// Second request — should be rejected with 409.
	req2 := withAuth(httptest.NewRequest(http.MethodPost, "/v1/plugins/mlflow:run", nil), testBearerToken)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusConflict, rec2.Code)

	close(block)
	runner.Wait()
}

func TestGetOperationsEndpoint(t *testing.T) {
	opStore := operations.NewMemoryStore()
	router := newTestRouter(t, catalog.NewMemoryStore(), opStore, map[string]pluginrun.Plugin{"seed": stubPlugin{}})

	// Trigger a run so we have an operation to list.
	req := withAuth(httptest.NewRequest(http.MethodPost, "/v1/plugins:run", nil), testBearerToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)

	var runResp RunPluginsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &runResp))
	require.Len(t, runResp.Operations, 1)
	opName := runResp.Operations[0].Name

	// Wait for completion.
	waitForOperation(t, opStore, opName)

	// GET /v1/operations should list the operation.
	getReq := withAuth(httptest.NewRequest(http.MethodGet, "/v1/operations", nil), testBearerToken)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	assert.Equal(t, http.StatusOK, getRec.Code)

	var listResp ListOperationsResponse
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &listResp))
	require.Len(t, listResp.Operations, 1)
	assert.Equal(t, opName, listResp.Operations[0].Name)
	assert.Equal(t, "seed", listResp.Operations[0].Plugin)
	assert.Equal(t, "SUCCEEDED", listResp.Operations[0].State)
}

func TestGetOperationByIDEndpoint(t *testing.T) {
	opStore := operations.NewMemoryStore()
	router := newTestRouter(t, catalog.NewMemoryStore(), opStore, map[string]pluginrun.Plugin{"seed": stubPlugin{}})

	// Trigger a run.
	req := withAuth(httptest.NewRequest(http.MethodPost, "/v1/plugins:run", nil), testBearerToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)

	var runResp RunPluginsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &runResp))
	require.Len(t, runResp.Operations, 1)
	opName := runResp.Operations[0].Name

	// Wait for completion.
	waitForOperation(t, opStore, opName)

	// GET /v1/operations/{name} should return the operation.
	getReq := withAuth(httptest.NewRequest(http.MethodGet, "/v1/operations/"+url.PathEscape(opName), nil), testBearerToken)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	assert.Equal(t, http.StatusOK, getRec.Code)

	var op OperationResource
	require.NoError(t, json.Unmarshal(getRec.Body.Bytes(), &op))
	assert.Equal(t, opName, op.Name)
	assert.Equal(t, "seed", op.Plugin)
}
