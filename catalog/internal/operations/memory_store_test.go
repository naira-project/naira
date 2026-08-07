package operations

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
		State:     StatePending,
		CreatedAt: createdAt,
	}
}

func TestMemoryStoreCreateAndGet(t *testing.T) {
	store := NewMemoryStore()
	op := newTestOperation("operations/plugin-run-1", "mlflow", time.Now())

	require.NoError(t, store.Create(op))

	got, err := store.Get(op.Name)
	require.NoError(t, err)
	assert.Equal(t, op, got)
}

func TestMemoryStoreGetNotFound(t *testing.T) {
	store := NewMemoryStore()

	_, err := store.Get("operations/plugin-run-missing")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestMemoryStoreCreateDuplicateName(t *testing.T) {
	store := NewMemoryStore()
	op := newTestOperation("operations/plugin-run-1", "mlflow", time.Now())

	require.NoError(t, store.Create(op))
	err := store.Create(op)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrAlreadyExists))
}

func TestMemoryStoreUpdateStateSetsRunningStartTime(t *testing.T) {
	store := NewMemoryStore()
	op := newTestOperation("operations/plugin-run-1", "mlflow", time.Now())
	require.NoError(t, store.Create(op))

	require.NoError(t, store.UpdateState(op.Name, StateRunning, nil, 0, 0))

	got, err := store.Get(op.Name)
	require.NoError(t, err)
	assert.Equal(t, StateRunning, got.State)
	assert.False(t, got.StartTime.IsZero(), "start time should be set on RUNNING")
	assert.Nil(t, got.EndTime, "end time should be nil while running")
}

func TestMemoryStoreUpdateStateSucceededSetsResult(t *testing.T) {
	store := NewMemoryStore()
	op := newTestOperation("operations/plugin-run-1", "mlflow", time.Now())
	require.NoError(t, store.Create(op))

	require.NoError(t, store.UpdateState(op.Name, StateRunning, nil, 0, 0))
	require.NoError(t, store.UpdateState(op.Name, StateSucceeded, nil, 3, 5))

	got, err := store.Get(op.Name)
	require.NoError(t, err)
	assert.Equal(t, StateSucceeded, got.State)
	assert.NotNil(t, got.EndTime, "end time should be set on SUCCEEDED")
	assert.Equal(t, 3, got.NodesUpserted)
	assert.Equal(t, 5, got.RelationsUpserted)
	assert.Nil(t, got.Error)
}

func TestMemoryStoreUpdateStateFailedSetsError(t *testing.T) {
	store := NewMemoryStore()
	op := newTestOperation("operations/plugin-run-1", "mlflow", time.Now())
	require.NoError(t, store.Create(op))

	statusErr := &StatusError{Message: "boom"}
	require.NoError(t, store.UpdateState(op.Name, StateFailed, statusErr, 0, 0))

	got, err := store.Get(op.Name)
	require.NoError(t, err)
	assert.Equal(t, StateFailed, got.State)
	assert.NotNil(t, got.EndTime)
	assert.Equal(t, statusErr, got.Error)
}

func TestMemoryStoreUpdateStateNotFound(t *testing.T) {
	store := NewMemoryStore()

	err := store.UpdateState("operations/missing", StateRunning, nil, 0, 0)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestMemoryStoreListFiltersByPlugin(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now()

	require.NoError(t, store.Create(newTestOperation("operations/plugin-run-1", "mlflow", now)))
	require.NoError(t, store.Create(newTestOperation("operations/plugin-run-2", "litellm", now)))

	ops, err := store.List(Filter{Plugin: "mlflow"})
	require.NoError(t, err)
	require.Len(t, ops, 1)
	assert.Equal(t, "mlflow", ops[0].Plugin)
}

func TestMemoryStoreListFiltersByState(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now()

	op1 := newTestOperation("operations/plugin-run-1", "mlflow", now)
	require.NoError(t, store.Create(op1))
	require.NoError(t, store.UpdateState(op1.Name, StateSucceeded, nil, 1, 1))

	op2 := newTestOperation("operations/plugin-run-2", "litellm", now)
	require.NoError(t, store.Create(op2))

	ops, err := store.List(Filter{State: StateSucceeded})
	require.NoError(t, err)
	require.Len(t, ops, 1)
	assert.Equal(t, "mlflow", ops[0].Plugin)
}

func TestMemoryStoreListOrdersMostRecentFirst(t *testing.T) {
	store := NewMemoryStore()
	base := time.Now()

	oldOp := newTestOperation("operations/plugin-run-old", "mlflow", base.Add(-2*time.Hour))
	newOp := newTestOperation("operations/plugin-run-new", "mlflow", base)

	require.NoError(t, store.Create(oldOp))
	require.NoError(t, store.Create(newOp))

	ops, err := store.List(Filter{})
	require.NoError(t, err)
	require.Len(t, ops, 2)
	assert.Equal(t, newOp.Name, ops[0].Name)
	assert.Equal(t, oldOp.Name, ops[1].Name)
}

func TestMemoryStoreEvictsOldestPerPlugin(t *testing.T) {
	store := NewMemoryStore()
	base := time.Now()

	// Insert more than the per-plugin cap for the same plugin.
	for i := range maxPerPlugin + 1 {
		op := newTestOperation(
			"operations/plugin-run-"+string(rune('a'+i)),
			"mlflow",
			base.Add(time.Duration(i)*time.Minute),
		)
		require.NoError(t, store.Create(op))
	}

	ops, err := store.List(Filter{Plugin: "mlflow"})
	require.NoError(t, err)
	require.Len(t, ops, maxPerPlugin)

	// The oldest operation ("operations/plugin-run-a") must have been evicted.
	_, err = store.Get("operations/plugin-run-a")
	assert.True(t, errors.Is(err, ErrNotFound))
}
