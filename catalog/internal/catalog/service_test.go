package catalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	snapshotV1 = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	snapshotV2 = uuid.MustParse("00000000-0000-0000-0000-000000000002")
	snapshotV3 = uuid.MustParse("00000000-0000-0000-0000-000000000003")
	snapshotV4 = uuid.MustParse("00000000-0000-0000-0000-000000000004")
)

type stubPlugin struct {
	response CollectResponse
	err      error
}

func (p stubPlugin) Collect(context.Context) (CollectResponse, error) {
	return p.response, p.err
}

// blockingStubPlugin blocks Collect until the block channel is closed,
// allowing tests to deterministically hold a plugin run in the RUNNING state.
type blockingStubPlugin struct {
	block    chan struct{}
	response CollectResponse
	err      error
}

func (p blockingStubPlugin) Collect(ctx context.Context) (CollectResponse, error) {
	select {
	case <-p.block:
	case <-ctx.Done():
		return CollectResponse{}, ctx.Err()
	}
	return p.response, p.err
}

// waitForState polls the operation store until op reaches the given state or
// the timeout elapses.
func waitForState(t *testing.T, store OperationStore, name string, state OperationState) Operation {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		op, err := store.Get(name)
		if err == nil && op.State == state {
			return op
		}
		time.Sleep(10 * time.Millisecond)
	}

	op, err := store.Get(name)
	if err != nil {
		t.Fatalf("operation %q: %v", name, err)
	}
	t.Fatalf("operation %q state = %s, want %s", name, op.State, state)
	return Operation{}
}

func applyPluginSnapshot(t *testing.T, store *MemoryStore, nodes []NodeClaim, relations []RelationClaim) {
	t.Helper()

	_, _, err := store.ApplyPluginSnapshot("test-plugin", snapshotV1, nodes, relations)
	require.NoError(t, err)
}

func TestListNodesProjectsStoredNode(t *testing.T) {
	store := NewMemoryStore()
	applyPluginSnapshot(t, store, []NodeClaim{{
		ID: NodeID{Kind: "model", Path: "mlflow/fraud-detector"},
		Properties: PropertyMap{
			"source":      "mlflow",
			"description": "registry model",
			"owner":       "risk-platform",
		},
	}}, nil)

	response := NewService(t.Context(), store, nil, 5*time.Minute, nil).ListNodes(t.Context())

	assert.Equal(t, []Node{
		{
			ID: NodeID{Kind: "model", Path: "mlflow/fraud-detector"},
			PluginClaims: map[string]PluginClaim{
				"test-plugin": {
					SnapshotID: snapshotV1,
					Properties: map[string]string{
						"source":      "mlflow",
						"description": "registry model",
						"owner":       "risk-platform",
					},
				},
			},
		},
	}, response)
}

func TestGetNodeReturnsStoredNode(t *testing.T) {
	store := NewMemoryStore()
	applyPluginSnapshot(t, store,
		[]NodeClaim{
			{
				ID:         NodeID{Kind: "model", Path: "mlflow/fraud-detector"},
				Properties: PropertyMap{"source": "mlflow"},
			},
			{
				ID: NodeID{Kind: "application", Path: "litellm/alpha-assistant"},
				Properties: PropertyMap{
					"namespace":           "apps",
					"team":                "risk",
					"litellm_virtual_key": "vk-alpha",
				},
			},
			{
				ID: NodeID{Kind: "application", Path: "litellm/beta-assistant"},
				Properties: PropertyMap{
					"namespace":           "apps",
					"team":                "risk",
					"litellm_virtual_key": "vk-beta",
				},
			},
		},
		[]RelationClaim{
			{
				Kind: "uses_model",
				From: NodeID{Kind: "application", Path: "litellm/beta-assistant"},
				To:   NodeID{Kind: "model", Path: "mlflow/fraud-detector"},
			},
			{
				Kind: "uses_model",
				From: NodeID{Kind: "application", Path: "litellm/alpha-assistant"},
				To:   NodeID{Kind: "model", Path: "mlflow/fraud-detector"},
			},
		},
	)

	response, err := NewService(t.Context(), store, nil, 5*time.Minute, nil).GetNode(t.Context(), NodeID{Kind: "model", Path: "mlflow/fraud-detector"})
	require.NoError(t, err)
	assert.Equal(t, "model", response.ID.Kind)
	assert.Equal(t, "mlflow/fraud-detector", response.ID.Path)

	_, err = NewService(t.Context(), store, nil, 5*time.Minute, nil).GetNode(t.Context(), NodeID{Kind: "model", Path: "mlflow/missing"})
	assert.True(t, errors.Is(err, ErrNodeNotFound))
}

func TestListRelationsReturnsStoredRelations(t *testing.T) {
	store := NewMemoryStore()
	applyPluginSnapshot(t, store,
		[]NodeClaim{
			{
				ID:         NodeID{Kind: "application", Path: "litellm/fraud-assistant"},
				Properties: PropertyMap{"source": "litellm"},
			},
			{
				ID:         NodeID{Kind: "model", Path: "mlflow/fraud-detector"},
				Properties: PropertyMap{"source": "mlflow"},
			},
			{
				ID:         NodeID{Kind: "dataset", Path: "mlflow/transactions-v1"},
				Properties: PropertyMap{"source": "mlflow"},
			},
			{
				ID:         NodeID{Kind: "dataset", Path: "openmetadata/orphan-table"},
				Properties: PropertyMap{"source": "openmetadata"},
			},
		},
		[]RelationClaim{
			{
				Kind:       "uses_model",
				From:       NodeID{Kind: "application", Path: "litellm/fraud-assistant"},
				To:         NodeID{Kind: "model", Path: "mlflow/fraud-detector"},
				Properties: PropertyMap{"via": "virtual-key"},
			},
			{
				Kind: "trained_on",
				From: NodeID{Kind: "model", Path: "mlflow/fraud-detector"},
				To:   NodeID{Kind: "dataset", Path: "mlflow/transactions-v1"},
			},
		},
	)

	response := NewService(t.Context(), store, nil, 5*time.Minute, nil).ListRelations(t.Context())

	assert.Equal(t, []Relation{
		{
			Kind: "trained_on",
			From: NodeID{Kind: "model", Path: "mlflow/fraud-detector"},
			To:   NodeID{Kind: "dataset", Path: "mlflow/transactions-v1"},
			PluginClaims: map[string]PluginClaim{
				"test-plugin": {
					SnapshotID: snapshotV1,
					Properties: nil,
				},
			},
		},
		{
			Kind: "uses_model",
			From: NodeID{Kind: "application", Path: "litellm/fraud-assistant"},
			To:   NodeID{Kind: "model", Path: "mlflow/fraud-detector"},
			PluginClaims: map[string]PluginClaim{
				"test-plugin": {
					SnapshotID: snapshotV1,
					Properties: map[string]string{"via": "virtual-key"},
				},
			},
		},
	}, response)
}

func TestRunAllPluginsUpsertsCollectedGraph(t *testing.T) {
	store := NewMemoryStore()
	service := NewService(t.Context(), store, map[string]Plugin{
		"mlflow": stubPlugin{
			response: CollectResponse{
				Nodes: []NodeClaim{{
					ID:         NodeID{Kind: "model", Path: "mlflow/demo-model"},
					Properties: PropertyMap{"source": "mlflow"},
				}},
			},
		},
	}, 5*time.Minute, nil)

	response := service.RunAllPlugins(t.Context())
	require.Len(t, response.Results, 1)
	assert.Equal(t, "mlflow", response.Results[0].Plugin)
	assert.Empty(t, response.Results[0].Error)

	nodes := service.ListNodes(t.Context())
	require.Len(t, nodes, 1)
	assert.Equal(t, "mlflow/demo-model", nodes[0].ID.Path)
}

func TestApplyPluginSnapshotPrunesPreviousPluginSnapshot(t *testing.T) {
	store := NewMemoryStore()

	_, _, err := store.ApplyPluginSnapshot(
		"mlflow",
		snapshotV1,
		[]NodeClaim{
			{
				ID:         NodeID{Kind: "application", Path: "mlflow/old-app"},
				Properties: PropertyMap{"source": "mlflow"},
			},
			{
				ID:         NodeID{Kind: "model", Path: "mlflow/shared-model"},
				Properties: PropertyMap{"source": "mlflow"},
			},
		},
		[]RelationClaim{{
			Kind: "uses_model",
			From: NodeID{Kind: "application", Path: "mlflow/old-app"},
			To:   NodeID{Kind: "model", Path: "mlflow/shared-model"},
		}},
	)
	require.NoError(t, err)

	_, _, err = store.ApplyPluginSnapshot(
		"litellm",
		snapshotV2,
		[]NodeClaim{{
			ID:         NodeID{Kind: "application", Path: "litellm/current-app"},
			Properties: PropertyMap{"source": "litellm"},
		}},
		nil,
	)
	require.NoError(t, err)

	_, _, err = store.ApplyPluginSnapshot(
		"mlflow",
		snapshotV3,
		[]NodeClaim{{
			ID:         NodeID{Kind: "model", Path: "mlflow/new-model"},
			Properties: PropertyMap{"source": "mlflow"},
		}},
		nil,
	)
	require.NoError(t, err)

	nodes := store.ListNodes()
	require.Len(t, nodes, 2)
	assert.Equal(t, NodeID{Kind: "application", Path: "litellm/current-app"}, nodes[0].ID)
	assert.Equal(t, NodeID{Kind: "model", Path: "mlflow/new-model"}, nodes[1].ID)
	assert.Empty(t, store.ListRelations())
}

func TestMultiplePluginsContributingToSameNode(t *testing.T) {
	store := NewMemoryStore()

	// 1. Ingest from plugin mlflow
	_, _, err := store.ApplyPluginSnapshot(
		"mlflow",
		snapshotV1,
		[]NodeClaim{{
			ID:         NodeID{Kind: "model", Path: "shared-model"},
			Properties: PropertyMap{"release": "2.34", "token_price": "$10"},
		}},
		nil,
	)
	require.NoError(t, err)

	// 2. Ingest from plugin litellm
	_, _, err = store.ApplyPluginSnapshot(
		"litellm",
		snapshotV2,
		[]NodeClaim{{
			ID:         NodeID{Kind: "model", Path: "shared-model"},
			Properties: PropertyMap{"token_price": "$5"},
		}},
		nil,
	)
	require.NoError(t, err)

	// 3. Verify node has both claims
	assert.Equal(t, []Node{
		{
			ID: NodeID{Kind: "model", Path: "shared-model"},
			PluginClaims: map[string]PluginClaim{
				"mlflow": {
					SnapshotID: snapshotV1,
					Properties: map[string]string{"release": "2.34", "token_price": "$10"},
				},
				"litellm": {
					SnapshotID: snapshotV2,
					Properties: map[string]string{"token_price": "$5"},
				},
			},
		},
	}, store.ListNodes())

	// 4. Update mlflow with a snapshot that doesn't contain the shared-model
	_, _, err = store.ApplyPluginSnapshot(
		"mlflow",
		snapshotV3,
		[]NodeClaim{}, // empty
		nil,
	)
	require.NoError(t, err)

	// 5. Verify the node still exists because litellm still claims it
	assert.Equal(t, []Node{
		{
			ID: NodeID{Kind: "model", Path: "shared-model"},
			PluginClaims: map[string]PluginClaim{
				"litellm": { // unchanged
					SnapshotID: snapshotV2,
					Properties: map[string]string{"token_price": "$5"},
				},
				// mlflow claim pruned
			},
		},
	}, store.ListNodes())

	// 6. Update litellm with a snapshot that doesn't contain the shared-model
	_, _, err = store.ApplyPluginSnapshot(
		"litellm",
		snapshotV4,
		[]NodeClaim{}, // empty
		nil,
	)
	require.NoError(t, err)

	// 7. Verify the node is deleted now
	assert.Empty(t, store.ListNodes())
}

func TestMultiplePluginsContributingToSameRelation(t *testing.T) {
	store := NewMemoryStore()

	appNode := NodeID{Kind: "application", Path: "shared-app"}
	modelNode := NodeID{Kind: "model", Path: "shared-model"}

	// 1. Both plugins must also report the nodes they reference
	_, _, err := store.ApplyPluginSnapshot(
		"mlflow",
		snapshotV1,
		[]NodeClaim{
			{ID: appNode},
			{ID: modelNode},
		},
		[]RelationClaim{{
			Kind:       "uses_model",
			From:       appNode,
			To:         modelNode,
			Properties: PropertyMap{"weight": "high"},
		}},
	)
	require.NoError(t, err)

	_, _, err = store.ApplyPluginSnapshot(
		"litellm",
		snapshotV2,
		[]NodeClaim{
			{ID: appNode},
			{ID: modelNode},
		},
		[]RelationClaim{{
			Kind:       "uses_model",
			From:       appNode,
			To:         modelNode,
			Properties: PropertyMap{"virtual_key": "vk-001"},
		}},
	)
	require.NoError(t, err)

	// 2. Verify both claims are stored on the same relation
	assert.Equal(t, []Relation{
		{
			Kind: "uses_model",
			From: appNode,
			To:   modelNode,
			PluginClaims: map[string]PluginClaim{
				"mlflow": {
					SnapshotID: snapshotV1,
					Properties: map[string]string{"weight": "high"},
				},
				"litellm": {
					SnapshotID: snapshotV2,
					Properties: map[string]string{"virtual_key": "vk-001"},
				},
			},
		},
	}, store.ListRelations())

	// 3. mlflow stops reporting this relation
	_, _, err = store.ApplyPluginSnapshot(
		"mlflow",
		snapshotV3,
		[]NodeClaim{
			{ID: appNode},
			{ID: modelNode},
		},
		nil, // no relations
	)
	require.NoError(t, err)

	// 4. Relation survives because litellm still claims it
	assert.Equal(t, []Relation{
		{
			Kind: "uses_model", // unchanged
			From: appNode,      // unchanged
			To:   modelNode,    // unchanged
			PluginClaims: map[string]PluginClaim{
				"litellm": { // unchanged
					SnapshotID: snapshotV2,
					Properties: map[string]string{"virtual_key": "vk-001"},
				},
				// mlflow claim pruned
			},
		},
	}, store.ListRelations())

	// 5. litellm also stops reporting the relation
	_, _, err = store.ApplyPluginSnapshot(
		"litellm",
		snapshotV4,
		[]NodeClaim{
			{ID: appNode},
			{ID: modelNode},
		},
		nil, // no relations
	)
	require.NoError(t, err)

	// 6. Relation is gone now
	assert.Empty(t, store.ListRelations())
}

func TestRunPluginAsyncCreatesPendingOperation(t *testing.T) {
	store := NewMemoryStore()
	opStore := NewMemoryOperationStore()
	block := make(chan struct{})
	service := NewService(t.Context(), store, map[string]Plugin{
		"mlflow": blockingStubPlugin{block: block},
	}, 5*time.Minute, nil, opStore)

	op, err := service.RunPluginAsync(t.Context(), "mlflow")
	require.NoError(t, err)
	assert.Equal(t, OperationStatePending, op.State)
	assert.Equal(t, "mlflow", op.Plugin)
	assert.NotEmpty(t, op.Name)

	// The operation must be immediately retrievable from the store.
	got, err := opStore.Get(op.Name)
	require.NoError(t, err)
	assert.Equal(t, OperationStatePending, got.State)

	// Unblock and wait so the goroutine does not leak.
	close(block)
	service.Wait()
}

func TestRunPluginAsyncSucceeds(t *testing.T) {
	store := NewMemoryStore()
	opStore := NewMemoryOperationStore()
	service := NewService(t.Context(), store, map[string]Plugin{
		"mlflow": stubPlugin{response: CollectResponse{
			Nodes: []NodeClaim{{
				ID:         NodeID{Kind: "model", Path: "mlflow/demo-model"},
				Properties: PropertyMap{"source": "mlflow"},
			}},
		}},
	}, 5*time.Minute, nil, opStore)

	op, err := service.RunPluginAsync(t.Context(), "mlflow")
	require.NoError(t, err)

	completed := waitForState(t, opStore, op.Name, OperationStateSucceeded)
	assert.NotNil(t, completed.EndTime)
	assert.Nil(t, completed.Error)
	assert.Equal(t, 1, completed.NodesUpserted)
	assert.Equal(t, 0, completed.RelationsUpserted)
}

func TestRunPluginAsyncFails(t *testing.T) {
	store := NewMemoryStore()
	opStore := NewMemoryOperationStore()
	service := NewService(t.Context(), store, map[string]Plugin{
		"mlflow": stubPlugin{err: errors.New("connection refused")},
	}, 5*time.Minute, nil, opStore)

	op, err := service.RunPluginAsync(t.Context(), "mlflow")
	require.NoError(t, err)

	completed := waitForState(t, opStore, op.Name, OperationStateFailed)
	require.NotNil(t, completed.Error)
	assert.Contains(t, completed.Error.Message, "connection refused")
	assert.NotNil(t, completed.EndTime)
}

func TestRunPluginAsyncRejectsUnknownPlugin(t *testing.T) {
	service := NewService(t.Context(), NewMemoryStore(), nil, 5*time.Minute, nil)

	_, err := service.RunPluginAsync(t.Context(), "missing")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPluginNotFound))
}

func TestRunPluginAsyncRejectsParallelRun(t *testing.T) {
	store := NewMemoryStore()
	opStore := NewMemoryOperationStore()
	block := make(chan struct{})
	service := NewService(t.Context(), store, map[string]Plugin{
		"mlflow": blockingStubPlugin{block: block},
	}, 5*time.Minute, nil, opStore)

	first, err := service.RunPluginAsync(t.Context(), "mlflow")
	require.NoError(t, err)

	// Wait until the first run is in flight (RUNNING), then try again.
	waitForState(t, opStore, first.Name, OperationStateRunning)

	_, err = service.RunPluginAsync(t.Context(), "mlflow")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPluginAlreadyRunning))

	close(block)
	service.Wait()
}

func TestRunAllPluginsAsyncReturnsOperations(t *testing.T) {
	store := NewMemoryStore()
	opStore := NewMemoryOperationStore()
	service := NewService(t.Context(), store, map[string]Plugin{
		"mlflow":  stubPlugin{},
		"litellm": stubPlugin{},
	}, 5*time.Minute, nil, opStore)

	ops := service.RunAllPluginsAsync(t.Context())
	require.Len(t, ops, 2)
	assert.Equal(t, []string{"litellm", "mlflow"}, []string{ops[0].Plugin, ops[1].Plugin})

	service.Wait()
}

func TestListPluginsReturnsSortedNames(t *testing.T) {
	service := NewService(t.Context(), NewMemoryStore(), map[string]Plugin{
		"mlflow":  stubPlugin{},
		"litellm": stubPlugin{},
		"fluxcd":  stubPlugin{},
	}, 5*time.Minute, nil)

	assert.Equal(t, []string{"fluxcd", "litellm", "mlflow"}, service.ListPlugins())
}

func TestGetOperationNotFound(t *testing.T) {
	service := NewService(t.Context(), NewMemoryStore(), nil, 5*time.Minute, nil)

	_, err := service.GetOperation(t.Context(), "operations/missing")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrOperationNotFound))
}
