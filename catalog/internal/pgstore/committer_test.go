package pgstore_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/operations"
)

// The scenarios below are stories: plugins send snapshots one after another
// and we check, after each step, what the graph looks like.

func TestSnapshotCommitter_NodeSurvivesWhileAnyPluginClaimsIt(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()

	a := nodeID("service", "a")
	b := nodeID("service", "b")
	c := nodeID("service", "c")

	// Step 1: k8s reports a and b, plus a relation between them.
	result, err := e.apply(t, "k8s", snap1,
		[]catalog.NodeClaim{
			nodeClaim(a, nil),
			nodeClaim(b, map[string]string{"tier": "backend"}),
		},
		[]catalog.RelationClaim{
			relationClaim("calls", a, b, nil),
		},
	)
	require.NoError(t, err)
	assert.Equal(t, 2, result.NodesUpserted)
	assert.Equal(t, 1, result.RelationsUpserted)

	// Step 2: docs also claims b.
	e.mustApply(t, "docs", snap2,
		[]catalog.NodeClaim{
			nodeClaim(b, map[string]string{"owner": "team-x"}),
		},
		nil,
	)

	nodes, err := e.graph.ListNodes(ctx)
	require.NoError(t, err)
	assert.Equal(t, []catalog.Node{
		{
			ID: a,
			PluginClaims: map[string]catalog.PluginClaim{
				"k8s": pluginClaim(snap1, map[string]string{}),
			},
		},
		{
			ID: b,
			PluginClaims: map[string]catalog.PluginClaim{
				"k8s":  pluginClaim(snap1, map[string]string{"tier": "backend"}),
				"docs": pluginClaim(snap2, map[string]string{"owner": "team-x"}),
			},
		},
	}, nodes)

	// Step 3: k8s's next snapshot only contains a. Its claim on b and on the
	// relation are pruned; b stays alive because docs still claims it.
	e.mustApply(t, "k8s", snap3,
		[]catalog.NodeClaim{
			nodeClaim(a, nil),
		},
		nil,
	)

	nodes, err = e.graph.ListNodes(ctx)
	require.NoError(t, err)
	assert.Equal(t, []catalog.Node{
		{
			ID: a,
			PluginClaims: map[string]catalog.PluginClaim{
				"k8s": pluginClaim(snap3, map[string]string{}),
			},
		},
		{
			ID: b,
			PluginClaims: map[string]catalog.PluginClaim{
				"docs": pluginClaim(snap2, map[string]string{"owner": "team-x"}),
			},
		},
	}, nodes)
	assert.Equal(t, 0, e.countRows(t, "relations"), "relation without claims must be deleted")
	assert.Equal(t, 0, e.countRows(t, "relation_plugin_claims"))

	// Step 4: docs moves on to c. Now nobody claims b, so it must disappear
	// from the nodes table.
	e.mustApply(t, "docs", snap4,
		[]catalog.NodeClaim{
			nodeClaim(c, nil),
		},
		nil,
	)

	nodes, err = e.graph.ListNodes(ctx)
	require.NoError(t, err)
	assert.Equal(t, []catalog.Node{
		{
			ID: a,
			PluginClaims: map[string]catalog.PluginClaim{
				"k8s": pluginClaim(snap3, map[string]string{}),
			},
		},
		{
			ID: c,
			PluginClaims: map[string]catalog.PluginClaim{
				"docs": pluginClaim(snap4, map[string]string{}),
			},
		},
	}, nodes)
	assert.Equal(t, 2, e.countRows(t, "nodes"))
	assert.Equal(t, 2, e.countRows(t, "node_plugin_claims"))
}

func TestSnapshotCommitter_RelationSurvivesWhileAnyPluginClaimsIt(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := t.Context()

	a := nodeID("service", "a")
	b := nodeID("service", "b")
	bothNodes := []catalog.NodeClaim{nodeClaim(a, nil), nodeClaim(b, nil)}

	e.mustApply(t, "k8s", snap1, bothNodes, []catalog.RelationClaim{
		relationClaim("calls", a, b, map[string]string{"protocol": "grpc"}),
	})
	e.mustApply(t, "docs", snap2, bothNodes, []catalog.RelationClaim{
		relationClaim("calls", a, b, map[string]string{"documented": "true"}),
	})
	assert.Equal(t, 1, e.countRows(t, "relations"), "both plugins must share one relation row")
	assert.Equal(t, 2, e.countRows(t, "relation_plugin_claims"))

	// k8s stops reporting the relation; docs still does.
	e.mustApply(t, "k8s", snap3, bothNodes, nil)

	relations, err := e.graph.ListRelations(ctx)
	require.NoError(t, err)
	assert.Equal(t, []catalog.Relation{
		{
			Kind: "calls",
			From: a,
			To:   b,
			PluginClaims: map[string]catalog.PluginClaim{
				"docs": pluginClaim(snap2, map[string]string{"documented": "true"}),
			},
		},
	}, relations)

	// docs stops too: the relation row goes away, the nodes stay.
	e.mustApply(t, "docs", snap4, bothNodes, nil)

	assert.Equal(t, 0, e.countRows(t, "relations"))
	assert.Equal(t, 2, e.countRows(t, "nodes"))
}

func TestSnapshotCommitter_ReapplyKeepsRowIDsAndReplacesClaim(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	a := nodeID("service", "a")

	e.mustApply(t, "k8s", snap1, []catalog.NodeClaim{nodeClaim(a, map[string]string{"v": "1", "old": "x"})}, nil)
	idBefore := e.nodeRowID(t, a)

	e.mustApply(t, "k8s", snap2, []catalog.NodeClaim{nodeClaim(a, map[string]string{"v": "2"})}, nil)

	assert.Equal(t, idBefore, e.nodeRowID(t, a), "node row id must be stable across snapshots")
	got, err := e.graph.GetNode(t.Context(), a)
	require.NoError(t, err)
	assert.Equal(t, catalog.Node{
		ID: a,
		PluginClaims: map[string]catalog.PluginClaim{
			// Properties are replaced, not merged ("old" is gone).
			"k8s": pluginClaim(snap2, map[string]string{"v": "2"}),
		},
	}, got)
}

func TestSnapshotCommitter_CompleteMarksOperationSucceeded(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	a := nodeID("service", "a")
	b := nodeID("service", "b")
	require.NoError(t, e.operations.Create(t.Context(), pendingOperation("op-1", "k8s", t0)))

	result, err := e.committer.CompleteSnapshotOperation(
		t.Context(), "op-1", "k8s", snap1,
		[]catalog.NodeClaim{nodeClaim(a, nil), nodeClaim(b, nil)},
		[]catalog.RelationClaim{relationClaim("calls", a, b, nil)},
	)
	require.NoError(t, err)
	assert.Equal(t, 2, result.NodesUpserted)
	assert.Equal(t, 1, result.RelationsUpserted)

	got, err := e.operations.Get(t.Context(), "op-1")
	require.NoError(t, err)
	require.NotNil(t, got.EndTime, "end time must be set")
	got.EndTime = nil
	assert.Equal(t, operations.Operation{
		Name:              "op-1",
		Plugin:            "k8s",
		State:             operations.StateSucceeded,
		NodesUpserted:     2,
		RelationsUpserted: 1,
		CreatedAt:         t0,
	}, inUTC(got))
}

// Snapshot application is serialized with an advisory lock so that orphan
// cleanup cannot race with another plugin claiming the same node. Without it
// concurrent runs can delete a node another transaction just claimed (or hit
// foreign-key errors).
func TestSnapshotCommitter_ConcurrentPluginsSharingNodes(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	const plugins = 8
	shared := nodeID("service", "shared")

	runAll := func(round string, withOwnNode bool) {
		var wg sync.WaitGroup
		errs := make([]error, plugins)
		for i := range plugins {
			wg.Go(func() {
				claims := []catalog.NodeClaim{nodeClaim(shared, nil)}
				if withOwnNode {
					claims = append(claims, nodeClaim(nodeID("service", fmt.Sprintf("own-%d", i)), nil))
				}
				_, errs[i] = e.apply(t, fmt.Sprintf("plugin%d", i), uuid.New(), claims, nil)
			})
		}
		wg.Wait()
		for i, err := range errs {
			require.NoError(t, err, "%s: plugin%d", round, i)
		}
	}

	runAll("first round", true)

	assert.Equal(t, plugins+1, e.countRows(t, "nodes"))
	gotShared, err := e.graph.GetNode(t.Context(), shared)
	require.NoError(t, err)
	assert.Len(t, gotShared.PluginClaims, plugins)

	// Everyone drops their own node in parallel; only the shared one remains.
	runAll("second round", false)

	assert.Equal(t, 1, e.countRows(t, "nodes"))
	assert.Equal(t, plugins, e.countRows(t, "node_plugin_claims"))
}

func TestSnapshotCommitter_EmptySnapshotRemovesAllPluginClaims(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	a := nodeID("service", "a")
	b := nodeID("service", "b")

	e.mustApply(t, "k8s", snap1,
		[]catalog.NodeClaim{nodeClaim(a, nil), nodeClaim(b, nil)},
		[]catalog.RelationClaim{relationClaim("calls", a, b, nil)},
	)
	e.mustApply(t, "docs", snap2, []catalog.NodeClaim{nodeClaim(b, nil)}, nil)

	// k8s reports nothing: its claims go away, b stays thanks to docs.
	e.mustApply(t, "k8s", snap3, nil, nil)

	nodes, err := e.graph.ListNodes(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []catalog.Node{
		{
			ID: b,
			PluginClaims: map[string]catalog.PluginClaim{
				"docs": pluginClaim(snap2, map[string]string{}),
			},
		},
	}, nodes)
	assert.Equal(t, 0, e.countRows(t, "relations"))
	assert.Equal(t, 0, e.countRows(t, "relation_plugin_claims"))
}

func TestSnapshotCommitter_FinishedOperationIsRejectedAndNothingApplied(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	a := nodeID("service", "a")
	b := nodeID("service", "b")
	require.NoError(t, e.operations.Create(t.Context(), pendingOperation("op-1", "k8s", t0)))

	_, err := e.committer.CompleteSnapshotOperation(
		t.Context(), "op-1", "k8s", snap1, []catalog.NodeClaim{nodeClaim(a, nil)}, nil,
	)
	require.NoError(t, err)

	_, err = e.committer.CompleteSnapshotOperation(
		t.Context(), "op-1", "k8s", snap2, []catalog.NodeClaim{nodeClaim(b, nil)}, nil,
	)

	require.ErrorIs(t, err, operations.ErrInvalidTransition)
	nodes, err := e.graph.ListNodes(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []catalog.Node{
		{
			ID: a,
			PluginClaims: map[string]catalog.PluginClaim{
				"k8s": pluginClaim(snap1, map[string]string{}),
			},
		},
	}, nodes)
}
