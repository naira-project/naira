package catalog

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestOperation(name, plugin string, createdAt time.Time) Operation {
	return Operation{
		Name:      name,
		Plugin:    plugin,
		State:     OperationStatePending,
		CreatedAt: createdAt,
	}
}

func TestMemoryOperationStoreCreateAndGet(t *testing.T) {
	store := NewMemoryOperationStore()
	op := newTestOperation("operations/plugin-run-1", "mlflow", time.Now())

	require.NoError(t, store.Create(op))

	got, err := store.Get(op.Name)
	require.NoError(t, err)
	assert.Equal(t, op, got)
}

func TestMemoryOperationStoreGetNotFound(t *testing.T) {
	store := NewMemoryOperationStore()

	_, err := store.Get("operations/plugin-run-missing")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrOperationNotFound))
}

func TestMemoryOperationStoreCreateDuplicateName(t *testing.T) {
	store := NewMemoryOperationStore()
	op := newTestOperation("operations/plugin-run-1", "mlflow", time.Now())

	require.NoError(t, store.Create(op))
	err := store.Create(op)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrOperationNotFound))
}

func TestMemoryOperationStoreUpdateStateSetsRunningStartTime(t *testing.T) {
	store := NewMemoryOperationStore()
	op := newTestOperation("operations/plugin-run-1", "mlflow", time.Now())
	require.NoError(t, store.Create(op))

	require.NoError(t, store.UpdateState(op.Name, OperationStateRunning, nil, 0, 0))

	got, err := store.Get(op.Name)
	require.NoError(t, err)
	assert.Equal(t, OperationStateRunning, got.State)
	assert.False(t, got.StartTime.IsZero(), "start time should be set on RUNNING")
	assert.Nil(t, got.EndTime, "end time should be nil while running")
}

func TestMemoryOperationStoreUpdateStateSucceededSetsResult(t *testing.T) {
	store := NewMemoryOperationStore()
	op := newTestOperation("operations/plugin-run-1", "mlflow", time.Now())
	require.NoError(t, store.Create(op))

	require.NoError(t, store.UpdateState(op.Name, OperationStateRunning, nil, 0, 0))
	require.NoError(t, store.UpdateState(op.Name, OperationStateSucceeded, nil, 3, 5))

	got, err := store.Get(op.Name)
	require.NoError(t, err)
	assert.Equal(t, OperationStateSucceeded, got.State)
	assert.NotNil(t, got.EndTime, "end time should be set on SUCCEEDED")
	assert.Equal(t, 3, got.NodesUpserted)
	assert.Equal(t, 5, got.RelationsUpserted)
	assert.Nil(t, got.Error)
}

func TestMemoryOperationStoreUpdateStateFailedSetsError(t *testing.T) {
	store := NewMemoryOperationStore()
	op := newTestOperation("operations/plugin-run-1", "mlflow", time.Now())
	require.NoError(t, store.Create(op))

	statusErr := &StatusError{Code: 13, Message: "boom"}
	require.NoError(t, store.UpdateState(op.Name, OperationStateFailed, statusErr, 0, 0))

	got, err := store.Get(op.Name)
	require.NoError(t, err)
	assert.Equal(t, OperationStateFailed, got.State)
	assert.NotNil(t, got.EndTime)
	assert.Equal(t, statusErr, got.Error)
}

func TestMemoryOperationStoreUpdateStateNotFound(t *testing.T) {
	store := NewMemoryOperationStore()

	err := store.UpdateState("operations/missing", OperationStateRunning, nil, 0, 0)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrOperationNotFound))
}

func TestMemoryOperationStoreListFiltersByPlugin(t *testing.T) {
	store := NewMemoryOperationStore()
	now := time.Now()

	require.NoError(t, store.Create(newTestOperation("operations/plugin-run-1", "mlflow", now)))
	require.NoError(t, store.Create(newTestOperation("operations/plugin-run-2", "litellm", now)))

	ops, err := store.List(OperationFilter{Plugin: "mlflow"})
	require.NoError(t, err)
	require.Len(t, ops, 1)
	assert.Equal(t, "mlflow", ops[0].Plugin)
}

func TestMemoryOperationStoreListFiltersByState(t *testing.T) {
	store := NewMemoryOperationStore()
	now := time.Now()

	op1 := newTestOperation("operations/plugin-run-1", "mlflow", now)
	require.NoError(t, store.Create(op1))
	require.NoError(t, store.UpdateState(op1.Name, OperationStateSucceeded, nil, 1, 1))

	op2 := newTestOperation("operations/plugin-run-2", "litellm", now)
	require.NoError(t, store.Create(op2))

	ops, err := store.List(OperationFilter{State: OperationStateSucceeded})
	require.NoError(t, err)
	require.Len(t, ops, 1)
	assert.Equal(t, "mlflow", ops[0].Plugin)
}

func TestMemoryOperationStoreListOrdersMostRecentFirst(t *testing.T) {
	store := NewMemoryOperationStore()
	base := time.Now()

	oldOp := newTestOperation("operations/plugin-run-old", "mlflow", base.Add(-2*time.Hour))
	newOp := newTestOperation("operations/plugin-run-new", "mlflow", base)

	require.NoError(t, store.Create(oldOp))
	require.NoError(t, store.Create(newOp))

	ops, err := store.List(OperationFilter{})
	require.NoError(t, err)
	require.Len(t, ops, 2)
	assert.Equal(t, newOp.Name, ops[0].Name)
	assert.Equal(t, oldOp.Name, ops[1].Name)
}

func TestMemoryOperationStoreEvictsOldestPerPlugin(t *testing.T) {
	store := NewMemoryOperationStore()
	base := time.Now()

	// Insert more than the per-plugin cap (5) for the same plugin.
	for i := 0; i < maxOperationsPerPlugin+1; i++ {
		op := newTestOperation(
			"operations/plugin-run-"+string(rune('a'+i)),
			"mlflow",
			base.Add(time.Duration(i)*time.Minute),
		)
		require.NoError(t, store.Create(op))
	}

	ops, err := store.List(OperationFilter{Plugin: "mlflow"})
	require.NoError(t, err)
	require.Len(t, ops, maxOperationsPerPlugin)

	// The oldest operation ("operations/plugin-run-a") must have been evicted.
	_, err = store.Get("operations/plugin-run-a")
	assert.True(t, errors.Is(err, ErrOperationNotFound))
}

func TestMemoryOperationStoreDelete(t *testing.T) {
	store := NewMemoryOperationStore()
	op := newTestOperation("operations/plugin-run-1", "mlflow", time.Now())
	require.NoError(t, store.Create(op))

	require.NoError(t, store.Delete(op.Name))

	_, err := store.Get(op.Name)
	assert.True(t, errors.Is(err, ErrOperationNotFound))
}

func TestMemoryOperationStoreDeleteNotFound(t *testing.T) {
	store := NewMemoryOperationStore()

	err := store.Delete("operations/missing")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrOperationNotFound))
}
