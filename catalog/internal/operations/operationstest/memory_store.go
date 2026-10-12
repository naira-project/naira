// Package operationstest provides a lightweight in-memory fake of
// operations.Store for tests that need to observe operation state transitions.
package operationstest

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/naira-project/naira/catalog/internal/operations"
)

type MemoryStore struct {
	mu   sync.Mutex
	data map[string]operations.Operation
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]operations.Operation)}
}

func (m *MemoryStore) Create(ctx context.Context, op operations.Operation) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("creating operation: %w", err)
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

func (m *MemoryStore) Get(ctx context.Context, name string) (operations.Operation, error) {
	if err := ctx.Err(); err != nil {
		return operations.Operation{}, fmt.Errorf("getting operation %q: %w", name, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	op, ok := m.data[name]
	if !ok {
		return operations.Operation{}, fmt.Errorf("operation %q: %w", name, operations.ErrNotFound)
	}
	return op, nil
}

func (m *MemoryStore) List(ctx context.Context, filter operations.Filter) ([]operations.Operation, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("listing operations: %w", err)
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

func (m *MemoryStore) UpdateState(ctx context.Context, name string, state operations.State, statusErr *operations.StatusError, nodesUpserted, relationsUpserted int) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("updating operation %q: %w", name, err)
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

func (m *MemoryStore) MarkInterrupted(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("marking operations interrupted: %w", err)
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
