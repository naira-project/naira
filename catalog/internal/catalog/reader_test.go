package catalog

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	response := NewService(store).ListNodes(t.Context())

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

	service := NewService(store)

	response, err := service.GetNode(t.Context(), NodeID{Kind: "model", Path: "mlflow/fraud-detector"})
	require.NoError(t, err)
	assert.Equal(t, "model", response.ID.Kind)
	assert.Equal(t, "mlflow/fraud-detector", response.ID.Path)

	_, err = service.GetNode(t.Context(), NodeID{Kind: "model", Path: "mlflow/missing"})
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

	response := NewService(store).ListRelations(t.Context())

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
