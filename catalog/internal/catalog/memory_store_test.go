package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func listKinds(t *testing.T, store *MemoryStore) []string {
	t.Helper()
	result, err := store.ListKinds(t.Context())
	require.NoError(t, err)
	return result
}

func listNodes(t *testing.T, store *MemoryStore) []Node {
	t.Helper()
	result, err := store.ListNodes(t.Context())
	require.NoError(t, err)
	return result
}

func listRelations(t *testing.T, store *MemoryStore) []Relation {
	t.Helper()
	result, err := store.ListRelations(t.Context())
	require.NoError(t, err)
	return result
}

func TestListKinds(t *testing.T) {
	store := NewMemoryStore()
	assert.Empty(t, listKinds(t, store))

	appNode := NodeID{
		Kind: "application",
		Path: "app-a",
	}
	otherAppNode := NodeID{
		Kind: "application",
		Path: "app-b",
	}
	modelNode := NodeID{
		Kind: "model",
		Path: "shared-model",
	}

	_, _, err := store.ApplyPluginSnapshot(t.Context(),
		"mlflow",
		snapshotV1,
		[]NodeClaim{
			{ID: appNode},
			{ID: otherAppNode},
			{ID: modelNode},
		},
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"application", "model"}, listKinds(t, store))

	_, _, err = store.ApplyPluginSnapshot(t.Context(),
		"mlflow",
		snapshotV2,
		[]NodeClaim{
			{ID: modelNode},
		},
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"model"}, listKinds(t, store))
}

func TestApplyPluginSnapshotPrunesPreviousPluginSnapshot(t *testing.T) {
	store := NewMemoryStore()

	_, _, err := store.ApplyPluginSnapshot(t.Context(),
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

	_, _, err = store.ApplyPluginSnapshot(t.Context(),
		"litellm",
		snapshotV2,
		[]NodeClaim{{
			ID:         NodeID{Kind: "application", Path: "litellm/current-app"},
			Properties: PropertyMap{"source": "litellm"},
		}},
		nil,
	)
	require.NoError(t, err)

	_, _, err = store.ApplyPluginSnapshot(t.Context(),
		"mlflow",
		snapshotV3,
		[]NodeClaim{{
			ID:         NodeID{Kind: "model", Path: "mlflow/new-model"},
			Properties: PropertyMap{"source": "mlflow"},
		}},
		nil,
	)
	require.NoError(t, err)

	nodes := listNodes(t, store)
	require.Len(t, nodes, 2)
	assert.Equal(t, NodeID{Kind: "application", Path: "litellm/current-app"}, nodes[0].ID)
	assert.Equal(t, NodeID{Kind: "model", Path: "mlflow/new-model"}, nodes[1].ID)
	assert.Empty(t, listRelations(t, store))
}

func TestMultiplePluginsContributingToSameNode(t *testing.T) {
	store := NewMemoryStore()

	// 1. Ingest from plugin mlflow
	_, _, err := store.ApplyPluginSnapshot(t.Context(),
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
	_, _, err = store.ApplyPluginSnapshot(t.Context(),
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
	}, listNodes(t, store))

	// 4. Update mlflow with a snapshot that doesn't contain the shared-model
	_, _, err = store.ApplyPluginSnapshot(t.Context(),
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
	}, listNodes(t, store))

	// 6. Update litellm with a snapshot that doesn't contain the shared-model
	_, _, err = store.ApplyPluginSnapshot(t.Context(),
		"litellm",
		snapshotV4,
		[]NodeClaim{}, // empty
		nil,
	)
	require.NoError(t, err)

	// 7. Verify the node is deleted now
	assert.Empty(t, listNodes(t, store))
}

func TestMultiplePluginsContributingToSameRelation(t *testing.T) {
	store := NewMemoryStore()

	appNode := NodeID{Kind: "application", Path: "shared-app"}
	modelNode := NodeID{Kind: "model", Path: "shared-model"}

	// 1. Both plugins must also report the nodes they reference
	_, _, err := store.ApplyPluginSnapshot(t.Context(),
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

	_, _, err = store.ApplyPluginSnapshot(t.Context(),
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
	}, listRelations(t, store))

	// 3. mlflow stops reporting this relation
	_, _, err = store.ApplyPluginSnapshot(t.Context(),
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
	}, listRelations(t, store))

	// 5. litellm also stops reporting the relation
	_, _, err = store.ApplyPluginSnapshot(t.Context(),
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
	assert.Empty(t, listRelations(t, store))
}
