// Package operationstest provides a lightweight, in-memory fake of
// operations.Store for tests. Unlike a production persistence store, it has
// no TTL or per-plugin eviction logic - just enough state tracking
// (create/get/list/update) for runner and httpapi tests to observe
// operations transitioning between states. Every method can be overridden
// via its *Func field for error injection or custom behavior.
package operationstest

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/naira-project/naira/catalog/internal/operations"
)

type MockStore struct {
	CreateFunc          func(op operations.Operation) error
	GetFunc             func(name string) (operations.Operation, error)
	ListFunc            func(filter operations.Filter) ([]operations.Operation, error)
	UpdateStateFunc     func(name string, state operations.State, err *operations.StatusError, nodesUpserted, relationsUpserted int) error
	MarkInterruptedFunc func() (int, error)

	mu   sync.Mutex
	data map[string]operations.Operation
}

func NewMockStore() *MockStore {
	return &MockStore{data: make(map[string]operations.Operation)}
}

func (m *MockStore) Create(op operations.Operation) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(op)
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

func (m *MockStore) Get(name string) (operations.Operation, error) {
	if m.GetFunc != nil {
		return m.GetFunc(name)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	op, ok := m.data[name]
	if !ok {
		return operations.Operation{}, fmt.Errorf("operation %q: %w", name, operations.ErrNotFound)
	}
	return op, nil
}

func (m *MockStore) List(filter operations.Filter) ([]operations.Operation, error) {
	if m.ListFunc != nil {
		return m.ListFunc(filter)
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

func (m *MockStore) UpdateState(name string, state operations.State, statusErr *operations.StatusError, nodesUpserted, relationsUpserted int) error {
	if m.UpdateStateFunc != nil {
		return m.UpdateStateFunc(name, state, statusErr, nodesUpserted, relationsUpserted)
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

func (m *MockStore) MarkInterrupted() (int, error) {
	if m.MarkInterruptedFunc != nil {
		return m.MarkInterruptedFunc()
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
