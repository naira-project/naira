package pgstore_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/naira-project/naira/catalog/internal/operations"
)

var t0 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

// inUTC normalizes the time zone pgx returns so whole operations can be
// compared with assert.Equal.
func inUTC(op operations.Operation) operations.Operation {
	op.CreatedAt = op.CreatedAt.UTC()
	if op.StartTime != nil {
		start := op.StartTime.UTC()
		op.StartTime = &start
	}
	if op.EndTime != nil {
		end := op.EndTime.UTC()
		op.EndTime = &end
	}
	return op
}

func operationNames(ops []operations.Operation) []string {
	names := make([]string, 0, len(ops))
	for _, op := range ops {
		names = append(names, op.Name)
	}
	return names
}

func TestOperationStore_CreateAndGet(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	err := e.operations.Create(t.Context(), pendingOperation("op-1", "litellm", t0))
	require.NoError(t, err)

	got, err := e.operations.Get(t.Context(), "op-1")
	require.NoError(t, err)

	assert.Equal(t, operations.Operation{
		Name:      "op-1",
		Plugin:    "litellm",
		State:     operations.StatePending,
		CreatedAt: t0,
	}, inUTC(got))
}

func TestOperationStore_CreateDuplicate(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	require.NoError(t, e.operations.Create(t.Context(), pendingOperation("op-1", "litellm", t0)))

	err := e.operations.Create(t.Context(), pendingOperation("op-1", "other", t0.Add(time.Minute)))

	assert.ErrorIs(t, err, operations.ErrAlreadyExists)

	// The original row is untouched.
	got, err := e.operations.Get(t.Context(), "op-1")
	require.NoError(t, err)
	assert.Equal(t, "litellm", got.Plugin)
}

func TestOperationStore_GetNotFound(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	_, err := e.operations.Get(t.Context(), "missing")

	assert.ErrorIs(t, err, operations.ErrNotFound)
}

func TestOperationStore_CreateEvictsOldestPerPlugin(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	// maxOperationsPerPlugin is 5: after 7 creates only the newest 5 remain.
	for i := range 7 {
		name := fmt.Sprintf("a-%d", i)
		require.NoError(t, e.operations.Create(t.Context(), pendingOperation(name, "a", t0.Add(time.Duration(i)*time.Minute))))
	}
	require.NoError(t, e.operations.Create(t.Context(), pendingOperation("b-0", "b", t0.Add(-time.Hour))))

	gotA, err := e.operations.List(t.Context(), operations.Filter{Plugin: "a"})
	require.NoError(t, err)
	gotB, err := e.operations.List(t.Context(), operations.Filter{Plugin: "b"})
	require.NoError(t, err)

	assert.Equal(t, []string{"a-6", "a-5", "a-4", "a-3", "a-2"}, operationNames(gotA))
	assert.Equal(t, []string{"b-0"}, operationNames(gotB))
}

func TestOperationStore_ListUsesNameAsWhenCreatedAtIsEqual(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	createdAt := t0
	for _, name := range []string{"op-a", "op-c", "op-b"} {
		require.NoError(t, e.operations.Create(t.Context(), pendingOperation(name, "plugin", createdAt)))
	}

	got, err := e.operations.List(t.Context(), operations.Filter{})
	require.NoError(t, err)
	assert.Equal(t, []string{"op-c", "op-b", "op-a"}, operationNames(got))
}

func TestOperationStore_List(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	for _, op := range []operations.Operation{
		{
			Name:      "a-1",
			Plugin:    "a",
			State:     operations.StatePending,
			CreatedAt: t0.Add(1 * time.Minute),
		},
		{
			Name:      "a-2",
			Plugin:    "a",
			State:     operations.StateSucceeded,
			CreatedAt: t0.Add(2 * time.Minute),
		},
		{
			Name:      "b-1",
			Plugin:    "b",
			State:     operations.StatePending,
			CreatedAt: t0.Add(3 * time.Minute),
		},
	} {
		require.NoError(t, e.operations.Create(t.Context(), op))
	}

	tests := []struct {
		name   string
		filter operations.Filter
		want   []string
	}{
		{
			name:   "no filter returns newest first",
			filter: operations.Filter{},
			want:   []string{"b-1", "a-2", "a-1"},
		},
		{
			name:   "by plugin",
			filter: operations.Filter{Plugin: "a"},
			want:   []string{"a-2", "a-1"},
		},
		{
			name:   "by state",
			filter: operations.Filter{State: operations.StatePending},
			want:   []string{"b-1", "a-1"},
		},
		{
			name: "by plugin and state",
			filter: operations.Filter{
				Plugin: "a",
				State:  operations.StatePending,
			},
			want: []string{"a-1"},
		},
		{
			name:   "no match",
			filter: operations.Filter{Plugin: "zzz"},
			want:   []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := e.operations.List(t.Context(), tt.filter)
			require.NoError(t, err)

			assert.Equal(t, tt.want, operationNames(got))
		})
	}
}

func TestOperationStore_UpdateState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		state         operations.State
		statusErr     *operations.StatusError
		nodes         int
		relations     int
		wantStartTime bool
		wantEndTime   bool
		want          operations.Operation
	}{
		{
			name:          "running sets start time",
			state:         operations.StateRunning,
			wantStartTime: true,
			want: operations.Operation{
				Name:      "op-1",
				Plugin:    "litellm",
				State:     operations.StateRunning,
				CreatedAt: t0,
			},
		},
		{
			name:        "succeeded records end time and counts",
			state:       operations.StateSucceeded,
			nodes:       3,
			relations:   2,
			wantEndTime: true,
			want: operations.Operation{
				Name:              "op-1",
				Plugin:            "litellm",
				State:             operations.StateSucceeded,
				NodesUpserted:     3,
				RelationsUpserted: 2,
				CreatedAt:         t0,
			},
		},
		{
			name:        "failed records end time and error message",
			state:       operations.StateFailed,
			statusErr:   &operations.StatusError{Message: "plugin exploded"},
			wantEndTime: true,
			want: operations.Operation{
				Name:      "op-1",
				Plugin:    "litellm",
				State:     operations.StateFailed,
				Error:     &operations.StatusError{Message: "plugin exploded"},
				CreatedAt: t0,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t)
			require.NoError(t, e.operations.Create(t.Context(), pendingOperation("op-1", "litellm", t0)))

			err := e.operations.UpdateState(t.Context(), "op-1", tt.state, tt.statusErr, tt.nodes, tt.relations)
			require.NoError(t, err)

			got, err := e.operations.Get(t.Context(), "op-1")
			require.NoError(t, err)

			// Start/end times come from time.Now(), so check presence and
			// compare the rest as a whole value.
			assert.Equal(t, tt.wantStartTime, got.StartTime != nil, "start time set")
			assert.Equal(t, tt.wantEndTime, got.EndTime != nil, "end time set")
			got.StartTime = nil
			got.EndTime = nil
			assert.Equal(t, tt.want, inUTC(got))
		})
	}
}

func TestOperationStore_UpdateState_RunningKeepsFirstStartTime(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	require.NoError(t, e.operations.Create(t.Context(), pendingOperation("op-1", "litellm", t0)))

	require.NoError(t, e.operations.UpdateState(t.Context(), "op-1", operations.StateRunning, nil, 0, 0))
	first, err := e.operations.Get(t.Context(), "op-1")
	require.NoError(t, err)
	require.NotNil(t, first.StartTime)

	time.Sleep(10 * time.Millisecond)
	require.NoError(t, e.operations.UpdateState(t.Context(), "op-1", operations.StateRunning, nil, 0, 0))
	second, err := e.operations.Get(t.Context(), "op-1")
	require.NoError(t, err)

	assert.True(t, first.StartTime.Equal(*second.StartTime), "start time changed from %v to %v", first.StartTime, second.StartTime)
}

func TestOperationStore_UpdateState_UnsupportedState(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	require.NoError(t, e.operations.Create(t.Context(), pendingOperation("op-1", "litellm", t0)))

	err := e.operations.UpdateState(t.Context(), "op-1", operations.StatePending, nil, 0, 0)

	assert.ErrorContains(t, err, "not handled")

	got, err := e.operations.Get(t.Context(), "op-1")
	require.NoError(t, err)
	assert.Equal(t, operations.StatePending, got.State)
}

func TestOperationStore_UpdateState_NotFound(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	err := e.operations.UpdateState(t.Context(), "missing", operations.StateRunning, nil, 0, 0)

	assert.ErrorIs(t, err, operations.ErrNotFound)
}
