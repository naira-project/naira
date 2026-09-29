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

// waitForState polls the operation store until op reaches the given state or
// the timeout elapses.
func waitForState(t *testing.T, store operations.Store, name string, state operations.State) operations.Operation {
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
	return operations.Operation{}
}

func TestRunPluginAsyncCreatesPendingOperation(t *testing.T) {
	store := catalog.NewMemoryStore()
	opStore := operations.NewMemoryStore()
	block := make(chan struct{})
	runner := NewRunner(t.Context(), store, opStore, map[string]Plugin{
		"mlflow": blockingStubPlugin{block: block},
	}, 5*time.Minute, nil)

	op, err := runner.RunPluginAsync(t.Context(), "mlflow")
	require.NoError(t, err)
	assert.Equal(t, operations.StatePending, op.State)
	assert.Equal(t, "mlflow", op.Plugin)
	assert.NotEmpty(t, op.Name)

	// Unblock and wait so the goroutine does not leak.
	close(block)
	runner.Wait()
}

func TestRunPluginAsyncSucceeds(t *testing.T) {
	store := catalog.NewMemoryStore()
	opStore := operations.NewMemoryStore()
	runner := NewRunner(t.Context(), store, opStore, map[string]Plugin{
		"mlflow": stubPlugin{response: catalog.CollectResponse{
			Nodes: []catalog.NodeClaim{{
				ID:         catalog.NodeID{Kind: "model", Path: "mlflow/demo-model"},
				Properties: catalog.PropertyMap{"source": "mlflow"},
			}},
		}},
	}, 5*time.Minute, nil)

	op, err := runner.RunPluginAsync(t.Context(), "mlflow")
	require.NoError(t, err)

	completed := waitForState(t, opStore, op.Name, operations.StateSucceeded)
	assert.NotNil(t, completed.EndTime)
	assert.Nil(t, completed.Error)
	assert.Equal(t, 1, completed.NodesUpserted)
	assert.Equal(t, 0, completed.RelationsUpserted)
}

func TestRunPluginAsyncFails(t *testing.T) {
	store := catalog.NewMemoryStore()
	opStore := operations.NewMemoryStore()
	runner := NewRunner(t.Context(), store, opStore, map[string]Plugin{
		"mlflow": stubPlugin{err: errors.New("connection refused")},
	}, 5*time.Minute, nil)

	op, err := runner.RunPluginAsync(t.Context(), "mlflow")
	require.NoError(t, err)

	completed := waitForState(t, opStore, op.Name, operations.StateFailed)
	require.NotNil(t, completed.Error)
	assert.Contains(t, completed.Error.Message, "connection refused")
	assert.NotNil(t, completed.EndTime)
}

func TestRunPluginAsyncRejectsUnknownPlugin(t *testing.T) {
	runner := NewRunner(t.Context(), catalog.NewMemoryStore(), operations.NewMemoryStore(), nil, 5*time.Minute, nil)

	_, err := runner.RunPluginAsync(t.Context(), "missing")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPluginNotFound))
}

func TestRunPluginAsyncRejectsParallelRun(t *testing.T) {
	store := catalog.NewMemoryStore()
	opStore := operations.NewMemoryStore()
	block := make(chan struct{})
	runner := NewRunner(t.Context(), store, opStore, map[string]Plugin{
		"mlflow": blockingStubPlugin{block: block},
	}, 5*time.Minute, nil)

	first, err := runner.RunPluginAsync(t.Context(), "mlflow")
	require.NoError(t, err)

	// Wait until the first run is in flight (RUNNING), then try again.
	waitForState(t, opStore, first.Name, operations.StateRunning)

	_, err = runner.RunPluginAsync(t.Context(), "mlflow")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrPluginAlreadyRunning))

	close(block)
	runner.Wait()
}

func TestRunAllPluginsAsyncReturnsOperations(t *testing.T) {
	store := catalog.NewMemoryStore()
	opStore := operations.NewMemoryStore()
	runner := NewRunner(t.Context(), store, opStore, map[string]Plugin{
		"mlflow":  stubPlugin{},
		"litellm": stubPlugin{},
	}, 5*time.Minute, nil)

	ops := runner.RunAllPluginsAsync(t.Context())
	require.Len(t, ops, 2)
	assert.Equal(t, []string{"litellm", "mlflow"}, []string{ops[0].Plugin, ops[1].Plugin})

	runner.Wait()
}

func TestListPluginsReturnsSortedNames(t *testing.T) {
	runner := NewRunner(t.Context(), catalog.NewMemoryStore(), operations.NewMemoryStore(), map[string]Plugin{
		"mlflow":  stubPlugin{},
		"litellm": stubPlugin{},
		"fluxcd":  stubPlugin{},
	}, 5*time.Minute, nil)

	assert.Equal(t, []string{"fluxcd", "litellm", "mlflow"}, runner.ListPlugins())
}

func TestGetOperationNotFound(t *testing.T) {
	runner := NewRunner(t.Context(), catalog.NewMemoryStore(), operations.NewMemoryStore(), nil, 5*time.Minute, nil)

	_, err := runner.GetOperation(t.Context(), "operations/missing")
	require.Error(t, err)
	assert.True(t, errors.Is(err, operations.ErrNotFound))
}
