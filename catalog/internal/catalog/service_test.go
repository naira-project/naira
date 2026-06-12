package catalog

import (
	"context"
	"errors"
	"testing"

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
	name    string
	request IngestionRequest
	err     error
}

func (p stubPlugin) Name() string {
	return p.name
}

func (p stubPlugin) Collect(context.Context) (IngestionRequest, error) {
	return p.request, p.err
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

	response := NewService(store, nil).ListNodes(t.Context())

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

	response, err := NewService(store, nil).GetNode(t.Context(), NodeID{Kind: "model", Path: "mlflow/fraud-detector"})
	require.NoError(t, err)
	assert.Equal(t, "model", response.ID.Kind)
	assert.Equal(t, "mlflow/fraud-detector", response.ID.Path)

	_, err = NewService(store, nil).GetNode(t.Context(), NodeID{Kind: "model", Path: "mlflow/missing"})
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

	response := NewService(store, nil).ListRelations(t.Context())

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
	service := NewService(store, nil, stubPlugin{
		name: "mlflow",
		request: IngestionRequest{
			Nodes: []NodeClaim{{
				ID:         NodeID{Kind: "model", Path: "mlflow/demo-model"},
				Properties: PropertyMap{"source": "mlflow"},
			}},
		},
	})

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
