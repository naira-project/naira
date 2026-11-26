package pgstore_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/naira-project/naira/catalog/internal/catalog"
)

func TestGraphStore_EmptyDatabase(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	kinds, err := e.graph.ListKinds(t.Context())
	require.NoError(t, err)
	nodes, err := e.graph.ListNodes(t.Context())
	require.NoError(t, err)
	relations, err := e.graph.ListRelations(t.Context())
	require.NoError(t, err)

	assert.Equal(t, []string{}, kinds)
	assert.Equal(t, []catalog.Node{}, nodes)
	assert.Equal(t, []catalog.Relation{}, relations)
}

func TestGraphStore_ListKinds(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	e.mustApply(t, "k8s", snap1,
		[]catalog.NodeClaim{
			nodeClaim(nodeID("service", "prod/api"), nil),
			nodeClaim(nodeID("deployment", "prod/api"), nil),
			nodeClaim(nodeID("service", "prod/web"), nil),
		},
		nil,
	)

	got, err := e.graph.ListKinds(t.Context())
	require.NoError(t, err)

	assert.Equal(t, []string{"deployment", "service"}, got)
}

func TestGraphStore_ListNodes(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	// given: k8s knows both nodes, docs adds an owner to the service
	e.mustApply(t, "k8s", snap1,
		[]catalog.NodeClaim{
			nodeClaim(nodeID("service", "prod/api"), map[string]string{"port": "80"}),
			nodeClaim(nodeID("deployment", "prod/api"), map[string]string{"replicas": "3"}),
		},
		nil,
	)
	e.mustApply(t, "docs", snap2,
		[]catalog.NodeClaim{
			nodeClaim(nodeID("service", "prod/api"), map[string]string{"owner": "team-a"}),
		},
		nil,
	)

	got, err := e.graph.ListNodes(t.Context())
	require.NoError(t, err)

	// expected: sorted by kind; claims merged per node
	assert.Equal(t, []catalog.Node{
		{
			ID: nodeID("deployment", "prod/api"),
			PluginClaims: map[string]catalog.PluginClaim{
				"k8s": pluginClaim(snap1, map[string]string{"replicas": "3"}),
			},
		},
		{
			ID: nodeID("service", "prod/api"),
			PluginClaims: map[string]catalog.PluginClaim{
				"k8s":  pluginClaim(snap1, map[string]string{"port": "80"}),
				"docs": pluginClaim(snap2, map[string]string{"owner": "team-a"}),
			},
		},
	}, got)
}

func TestGraphStore_GetNode(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	// given: one node claimed by two plugins, plus an unrelated node
	e.mustApply(t, "k8s", snap1,
		[]catalog.NodeClaim{
			nodeClaim(nodeID("service", "prod/api"), map[string]string{"port": "80"}),
			nodeClaim(nodeID("service", "prod/web"), nil),
		},
		nil,
	)
	e.mustApply(t, "docs", snap2,
		[]catalog.NodeClaim{
			nodeClaim(nodeID("service", "prod/api"), map[string]string{"owner": "team-a"}),
		},
		nil,
	)

	got, err := e.graph.GetNode(t.Context(), nodeID("service", "prod/api"))
	require.NoError(t, err)

	// expected: only the requested node, with both claims
	assert.Equal(t, catalog.Node{
		ID: nodeID("service", "prod/api"),
		PluginClaims: map[string]catalog.PluginClaim{
			"k8s":  pluginClaim(snap1, map[string]string{"port": "80"}),
			"docs": pluginClaim(snap2, map[string]string{"owner": "team-a"}),
		},
	}, got)
}

func TestGraphStore_GetNode_NotFound(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	e.mustApply(t, "k8s", snap1,
		[]catalog.NodeClaim{
			nodeClaim(nodeID("service", "prod/api"), nil),
		},
		nil,
	)

	tests := []struct {
		name string
		id   catalog.NodeID
	}{
		{
			name: "unknown path",
			id:   nodeID("service", "prod/missing"),
		},
		{
			name: "unknown kind",
			id:   nodeID("database", "prod/api"),
		},
		{
			name: "path is not a prefix match",
			id:   nodeID("service", "prod"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := e.graph.GetNode(t.Context(), tt.id)

			assert.ErrorIs(t, err, catalog.ErrNodeNotFound)
		})
	}
}

func TestGraphStore_ListRelations(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	e.mustApply(t, "k8s", snap1,
		[]catalog.NodeClaim{
			nodeClaim(nodeID("deployment", "prod/api"), nil),
			nodeClaim(nodeID("service", "prod/api"), nil),
		},
		[]catalog.RelationClaim{
			relationClaim("exposes", nodeID("deployment", "prod/api"), nodeID("service", "prod/api"), nil),
		},
	)

	got, err := e.graph.ListRelations(t.Context())
	require.NoError(t, err)

	assert.Equal(t, []catalog.Relation{
		{
			Kind: "exposes",
			From: nodeID("deployment", "prod/api"),
			To:   nodeID("service", "prod/api"),
			PluginClaims: map[string]catalog.PluginClaim{
				"k8s": pluginClaim(snap1, map[string]string{}),
			},
		},
	}, got)
}
