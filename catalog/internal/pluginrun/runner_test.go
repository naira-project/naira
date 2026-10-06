package pluginrun

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/operations"
	"github.com/naira-project/naira/catalog/internal/pluginrun/pluginruntest"
)

type stubPlugin struct {
	response catalog.CollectResponse
	err      error
}

func (p stubPlugin) Collect(context.Context) (catalog.CollectResponse, error) {
	return p.response, p.err
}

// blockingStubPlugin blocks Collect until the block channel is closed,
// allowing tests to deterministically hold a plugin run in the RUNNING state.
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

// waitForState polls the mock store until op reaches the given state or the
// timeout elapses.
func waitForState(t *testing.T, store *pluginruntest.MockSnapshotStore, name string, state operations.State) operations.Operation {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		op, err := store.Get(t.Context(), name)
		if err == nil && op.State == state {
			return op
		}
		time.Sleep(10 * time.Millisecond)
	}

	op, err := store.Get(t.Context(), name)
	if err != nil {
		t.Fatalf("operation %q: %v", name, err)
	}
	t.Fatalf("operation %q state = %s, want %s", name, op.State, state)
	return operations.Operation{}
}

func TestRunPluginAsyncCreatesPendingOperation(t *testing.T) {
	store := pluginruntest.NewMockSnapshotStore()
	block := make(chan struct{})
	runner := NewRunner(t.Context(), store, store, map[string]Plugin{
		"mlflow": blockingStubPlugin{block: block},
	}, 5*time.Minute, nil)

	op, err := runner.RunPluginAsync(t.Context(), "mlflow")
	require.NoError(t, err)
	assert.Equal(t, operations.StatePending, op.State)
	assert.Equal(t, "mlflow", op.Plugin)
	assert.NotEmpty(t, op.Name)

	close(block)
	runner.Wait()
}

func TestRunPluginAsyncSucceeds(t *testing.T) {
	store := pluginruntest.NewMockSnapshotStore()
	runner := NewRunner(t.Context(), store, store, map[string]Plugin{
		"mlflow": stubPlugin{response: catalog.CollectResponse{
			Nodes: []catalog.NodeClaim{{
				ID:         catalog.NodeID{Kind: "model", Path: "mlflow/demo-model"},
				Properties: catalog.PropertyMap{"source": "mlflow"},
			}},
		}},
	}, 5*time.Minute, nil)

	op, err := runner.RunPluginAsync(t.Context(), "mlflow")
	require.NoError(t, err)

	completed := waitForState(t, store, op.Name, operations.StateSucceeded)
	assert.NotNil(t, completed.EndTime)
	assert.Nil(t, completed.Error)
	assert.Equal(t, 1, completed.NodesUpserted)
	assert.Equal(t, 0, completed.RelationsUpserted)
}

func TestRunPluginAsyncFails(t *testing.T) {
	store := pluginruntest.NewMockSnapshotStore()
	runner := NewRunner(t.Context(), store, store, map[string]Plugin{
		"mlflow": stubPlugin{err: errors.New("connection refused")},
	}, 5*time.Minute, nil)

	op, err := runner.RunPluginAsync(t.Context(), "mlflow")
	require.NoError(t, err)

	completed := waitForState(t, store, op.Name, operations.StateFailed)
	require.NotNil(t, completed.Error)
	assert.Contains(t, completed.Error.Message, "connection refused")
	assert.NotNil(t, completed.EndTime)
}

func TestRunPluginAsyncRejectsUnknownPlugin(t *testing.T) {
	store := pluginruntest.NewMockSnapshotStore()
	runner := NewRunner(t.Context(), store, store, nil, 5*time.Minute, nil)

	_, err := runner.RunPluginAsync(t.Context(), "missing")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPluginNotFound))
}

func TestRunPluginAsyncRejectsParallelRun(t *testing.T) {
	store := pluginruntest.NewMockSnapshotStore()
	block := make(chan struct{})
	runner := NewRunner(t.Context(), store, store, map[string]Plugin{
		"mlflow": blockingStubPlugin{block: block},
	}, 5*time.Minute, nil)

	first, err := runner.RunPluginAsync(t.Context(), "mlflow")
	require.NoError(t, err)

	waitForState(t, store, first.Name, operations.StateRunning)

	_, err = runner.RunPluginAsync(t.Context(), "mlflow")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPluginAlreadyRunning))

	close(block)
	runner.Wait()
}

func TestRunAllPluginsAsyncReturnsOperations(t *testing.T) {
	store := pluginruntest.NewMockSnapshotStore()
	runner := NewRunner(t.Context(), store, store, map[string]Plugin{
		"mlflow":  stubPlugin{},
		"litellm": stubPlugin{},
	}, 5*time.Minute, nil)

	ops := runner.RunAllPluginsAsync(t.Context())
	require.Len(t, ops, 2)
	assert.Equal(t, []string{"litellm", "mlflow"}, []string{ops[0].Plugin, ops[1].Plugin})

	runner.Wait()
}

func TestGetOperationNotFound(t *testing.T) {
	store := pluginruntest.NewMockSnapshotStore()
	runner := NewRunner(t.Context(), store, store, nil, 5*time.Minute, nil)

	_, err := runner.GetOperation(t.Context(), "operations/missing")
	require.Error(t, err)
	assert.True(t, errors.Is(err, operations.ErrNotFound))
}
