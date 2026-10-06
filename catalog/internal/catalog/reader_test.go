package catalog_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/catalog/catalogtest"
)

func TestServiceListNodesReturnsWhatTheStoreReturns(t *testing.T) {
	want := []catalog.Node{{
		ID: catalog.NodeID{Kind: "model", Path: "mlflow/fraud-detector"},
		PluginClaims: map[string]catalog.PluginClaim{
			"mlflow": {
				Properties: map[string]string{
					"source":      "mlflow",
					"description": "registry model",
					"owner":       "risk-platform",
				},
			},
		},
	}}

	store := &catalogtest.MockStore{
		ListNodesFunc: func(_ context.Context) ([]catalog.Node, error) {
			return want, nil
		},
	}

	response, err := catalog.NewService(store).ListNodes(t.Context())
	require.NoError(t, err)
	assert.Equal(t, want, response)
}

func TestServiceListNodesWrapsStoreError(t *testing.T) {
	store := &catalogtest.MockStore{
		ListNodesFunc: func(_ context.Context) ([]catalog.Node, error) {
			return nil, errors.New("connection reset")
		},
	}

	_, err := catalog.NewService(store).ListNodes(t.Context())
	require.Error(t, err)
	assert.ErrorContains(t, err, "listing nodes")
	assert.ErrorContains(t, err, "connection reset")
}

func TestServiceGetNodeReturnsWhatTheStoreReturns(t *testing.T) {
	id := catalog.NodeID{Kind: "model", Path: "mlflow/fraud-detector"}
	want := catalog.Node{ID: id}

	store := &catalogtest.MockStore{
		GetNodeFunc: func(_ context.Context, gotID catalog.NodeID) (catalog.Node, error) {
			assert.Equal(t, id, gotID)
			return want, nil
		},
	}

	response, err := catalog.NewService(store).GetNode(t.Context(), id)
	require.NoError(t, err)
	assert.Equal(t, want, response)
}

func TestServiceGetNodePropagatesNotFound(t *testing.T) {
	store := &catalogtest.MockStore{
		GetNodeFunc: func(context.Context, catalog.NodeID) (catalog.Node, error) {
			return catalog.Node{}, catalog.ErrNodeNotFound
		},
	}

	_, err := catalog.NewService(store).GetNode(t.Context(), catalog.NodeID{Kind: "model", Path: "missing"})
	assert.ErrorIs(t, err, catalog.ErrNodeNotFound)
}

func TestServiceListRelationsReturnsWhatTheStoreReturns(t *testing.T) {
	want := []catalog.Relation{{
		Kind: "uses_model",
		From: catalog.NodeID{Kind: "application", Path: "litellm/fraud-assistant"},
		To:   catalog.NodeID{Kind: "model", Path: "mlflow/fraud-detector"},
		PluginClaims: map[string]catalog.PluginClaim{
			"mlflow": {
				Properties: map[string]string{"via": "virtual-key"},
			},
		},
	}}

	store := &catalogtest.MockStore{
		ListRelationsFunc: func(_ context.Context) ([]catalog.Relation, error) {
			return want, nil
		},
	}

	response, err := catalog.NewService(store).ListRelations(t.Context())
	require.NoError(t, err)
	assert.Equal(t, want, response)
}

func TestServiceListRelationsWrapsStoreError(t *testing.T) {
	store := &catalogtest.MockStore{
		ListRelationsFunc: func(_ context.Context) ([]catalog.Relation, error) {
			return nil, errors.New("timeout")
		},
	}

	_, err := catalog.NewService(store).ListRelations(t.Context())
	require.Error(t, err)
	assert.ErrorContains(t, err, "listing relations")
	assert.ErrorContains(t, err, "timeout")
}
