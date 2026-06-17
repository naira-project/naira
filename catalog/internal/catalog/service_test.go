package catalog

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubPlugin struct {
	request CollectResponse
	err     error
}

func (p stubPlugin) Collect(context.Context) (CollectResponse, error) {
	return p.request, p.err
}

func applyPluginSnapshot(t *testing.T, store *MemoryStore, nodes []NodeClaim, relations []RelationClaim) {
	t.Helper()

	_, _, err := store.ApplyPluginSnapshot("test-plugin", uuid.MustParse("00000000-0000-0000-0000-000000000001"), nodes, relations)
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

	response := NewService(store, nil, nil).ListNodes(t.Context())
	require.Len(t, response, 1)

	node := response[0]
	assert.Equal(t, "model", node.ID.Kind)
	assert.Equal(t, "mlflow/fraud-detector", node.ID.Path)
	assert.Equal(t, map[string]string{
		"source":      "mlflow",
		"description": "registry model",
		"owner":       "risk-platform",
	}, node.Properties)
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

	response, err := NewService(store, nil, nil).GetNode(t.Context(), NodeID{Kind: "model", Path: "mlflow/fraud-detector"})
	require.NoError(t, err)
	assert.Equal(t, "model", response.ID.Kind)
	assert.Equal(t, "mlflow/fraud-detector", response.ID.Path)

	_, err = NewService(store, nil, nil).GetNode(t.Context(), NodeID{Kind: "model", Path: "mlflow/missing"})
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
				Kind: "uses_model",
				From: NodeID{Kind: "application", Path: "litellm/fraud-assistant"},
				To:   NodeID{Kind: "model", Path: "mlflow/fraud-detector"},
			},
			{
				Kind: "trained_on",
				From: NodeID{Kind: "model", Path: "mlflow/fraud-detector"},
				To:   NodeID{Kind: "dataset", Path: "mlflow/transactions-v1"},
			},
		},
	)

	response := NewService(store, nil, nil).ListRelations(t.Context())
	assert.Len(t, response, 2)
	assert.Equal(t, "trained_on", response[0].Kind)
	assert.Equal(t, NodeID{Kind: "model", Path: "mlflow/fraud-detector"}, response[0].From)
	assert.Equal(t, NodeID{Kind: "dataset", Path: "mlflow/transactions-v1"}, response[0].To)
	assert.Equal(t, "uses_model", response[1].Kind)
	assert.Equal(t, NodeID{Kind: "application", Path: "litellm/fraud-assistant"}, response[1].From)
	assert.Equal(t, NodeID{Kind: "model", Path: "mlflow/fraud-detector"}, response[1].To)
}

func TestRunAllPluginsUpsertsCollectedGraph(t *testing.T) {
	store := NewMemoryStore()
	service := NewService(store, nil, map[string]Plugin{
		"mlflow": stubPlugin{
			request: CollectResponse{
				Nodes: []NodeClaim{{
					ID:         NodeID{Kind: "model", Path: "mlflow/demo-model"},
					Properties: PropertyMap{"source": "mlflow"},
				}},
			},
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
		uuid.MustParse("00000000-0000-0000-0000-000000000001"),
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
		uuid.MustParse("00000000-0000-0000-0000-000000000010"),
		[]NodeClaim{{
			ID:         NodeID{Kind: "application", Path: "litellm/current-app"},
			Properties: PropertyMap{"source": "litellm"},
		}},
		nil,
	)
	require.NoError(t, err)

	_, _, err = store.ApplyPluginSnapshot(
		"mlflow",
		uuid.MustParse("00000000-0000-0000-0000-000000000002"),
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
