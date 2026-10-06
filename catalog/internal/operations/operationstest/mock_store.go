// Package operationstest provides a lightweight in-memory mock of
// operations.Store for tests. It intentionally implements only the behavior
// (create/get/list/update) for runner and httpapi tests to observe
// operations transitioning between states. Every method can be overridden
// via its *Func field for error injection or custom behavior.
package operationstest

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/naira-project/naira/catalog/internal/operations"
)

type MockStore struct {
	CreateFunc          func(context.Context, operations.Operation) error
	GetFunc             func(context.Context, string) (operations.Operation, error)
	ListFunc            func(context.Context, operations.Filter) ([]operations.Operation, error)
	UpdateStateFunc     func(context.Context, string, operations.State, *operations.StatusError, int, int) error
	MarkInterruptedFunc func(context.Context) (int, error)

	mu   sync.Mutex
	data map[string]operations.Operation
}

func NewMockStore() *MockStore {
	return &MockStore{data: make(map[string]operations.Operation)}
}

func (m *MockStore) Create(ctx context.Context, op operations.Operation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, op)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = make(map[string]operations.Operation)
	}
	if _, exists := m.data[op.Name]; exists {
		return fmt.Errorf("operation %q: %w", op.Name, operations.ErrAlreadyExists)
	}
	m.data[op.Name] = op
	return nil
}

func (m *MockStore) Get(ctx context.Context, name string) (operations.Operation, error) {
	if err := ctx.Err(); err != nil {
		return operations.Operation{}, err
	}
	if m.GetFunc != nil {
		return m.GetFunc(ctx, name)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	op, ok := m.data[name]
	if !ok {
		return operations.Operation{}, fmt.Errorf("operation %q: %w", name, operations.ErrNotFound)
	}
	return op, nil
}

func (m *MockStore) List(ctx context.Context, filter operations.Filter) ([]operations.Operation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m.ListFunc != nil {
		return m.ListFunc(ctx, filter)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]operations.Operation, 0, len(m.data))
	for _, op := range m.data {
		if filter.Plugin != "" && op.Plugin != filter.Plugin {
			continue
		}
		if filter.State != "" && op.State != filter.State {
			continue
		}
		result = append(result, op)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result, nil
}

func (m *MockStore) UpdateState(ctx context.Context, name string, state operations.State, statusErr *operations.StatusError, nodesUpserted, relationsUpserted int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.UpdateStateFunc != nil {
		return m.UpdateStateFunc(ctx, name, state, statusErr, nodesUpserted, relationsUpserted)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	op, ok := m.data[name]
	if !ok {
		return fmt.Errorf("operation %q: %w", name, operations.ErrNotFound)
	}

	now := time.Now()
	op.State = state
	op.Error = statusErr

	if state == operations.StateRunning && op.StartTime.IsZero() {
		op.StartTime = now
	}
	if state == operations.StateSucceeded {
		op.NodesUpserted = nodesUpserted
		op.RelationsUpserted = relationsUpserted
	}
	if state == operations.StateSucceeded || state == operations.StateFailed || state == operations.StateInterrupted {
		op.EndTime = &now
	}

	m.data[name] = op
	return nil
}

func (m *MockStore) MarkInterrupted(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if m.MarkInterruptedFunc != nil {
		return m.MarkInterruptedFunc(ctx)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	count := 0
	for name, op := range m.data {
		if op.State != operations.StatePending && op.State != operations.StateRunning {
			continue
		}
		op.State = operations.StateInterrupted
		op.EndTime = &now
		m.data[name] = op
		count++
	}
	return count, nil
}
