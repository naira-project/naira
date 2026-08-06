package catalog

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

const (
	// maxOperationsPerPlugin is the maximum number of operations kept in
	// memory for a single plugin. When this limit is reached the oldest
	// operation for that plugin is evicted on insert.
	maxOperationsPerPlugin = 5

	// operationTTL is the maximum age of an operation before it is eligible
	// for lazy eviction during List or Get calls.
	operationTTL = 3 * 24 * time.Hour
)

// MemoryOperationStore is an in-memory implementation of OperationStore.
// Operations are stored in a map keyed by operation name and protected by
// a read-write mutex.
type MemoryOperationStore struct {
	mu         sync.RWMutex
	operations map[string]Operation
}

func NewMemoryOperationStore() *MemoryOperationStore {
	return &MemoryOperationStore{
		operations: make(map[string]Operation),
	}
}

func (s *MemoryOperationStore) Create(op Operation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.operations[op.Name]; exists {
		return fmt.Errorf("operation %q: %w", op.Name, ErrOperationNotFound)
	}

	s.evictOldestForPluginLocked(op.Plugin)
	s.operations[op.Name] = op
	return nil
}

// Get retrieves a single operation by name. Stale operations (beyond TTL)
// are lazily evicted.
func (s *MemoryOperationStore) Get(name string) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	op, ok := s.operations[name]
	if !ok {
		return Operation{}, fmt.Errorf("operation %q: %w", name, ErrOperationNotFound)
	}

	if s.isExpiredLocked(op) {
		delete(s.operations, name)
		return Operation{}, fmt.Errorf("operation %q: %w", name, ErrOperationNotFound)
	}

	return op, nil
}

// List returns all non-expired operations ordered by creation time descending
// (most recent first). The optional filter narrows the result set by plugin
// name and/or operation state.
func (s *MemoryOperationStore) List(filter OperationFilter) ([]Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneExpiredLocked()

	result := make([]Operation, 0, len(s.operations))
	for _, op := range s.operations {
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

// UpdateState atomically transitions an operation to a new state and applies
// the associated outcome: the error for FAILED, the upserted result counts
// for SUCCEEDED, and the start time for RUNNING. If the operation does not
// exist it returns ErrOperationNotFound.
func (s *MemoryOperationStore) UpdateState(name string, state OperationState, err *StatusError, nodesUpserted, relationsUpserted int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	op, ok := s.operations[name]
	if !ok {
		return fmt.Errorf("operation %q: %w", name, ErrOperationNotFound)
	}

	now := time.Now()
	op.State = state
	op.Error = err

	if state == OperationStateRunning && op.StartTime.IsZero() {
		op.StartTime = now
	}

	if state == OperationStateSucceeded {
		op.NodesUpserted = nodesUpserted
		op.RelationsUpserted = relationsUpserted
	}

	if state == OperationStateSucceeded || state == OperationStateFailed {
		op.EndTime = &now
	}

	s.operations[name] = op
	return nil
}

// evictOldestForPluginLocked removes the oldest operation for the given
// plugin if the per-plugin cap has been reached. Must be called with the
// write lock held.
func (s *MemoryOperationStore) evictOldestForPluginLocked(plugin string) {
	var pluginOps []Operation
	for _, op := range s.operations {
		if op.Plugin == plugin {
			pluginOps = append(pluginOps, op)
		}
	}

	if len(pluginOps) < maxOperationsPerPlugin {
		return
	}

	sort.Slice(pluginOps, func(i, j int) bool {
		return pluginOps[i].CreatedAt.Before(pluginOps[j].CreatedAt)
	})

	delete(s.operations, pluginOps[0].Name)
}

// isExpiredLocked returns true if the operation is older than the TTL.
// Must be called with at least a read lock held.
func (s *MemoryOperationStore) isExpiredLocked(op Operation) bool {
	return time.Since(op.CreatedAt) > operationTTL
}

// pruneExpiredLocked removes all operations that have exceeded the TTL.
// Must be called with the write lock held.
func (s *MemoryOperationStore) pruneExpiredLocked() {
	for name, op := range s.operations {
		if s.isExpiredLocked(op) {
			delete(s.operations, name)
		}
	}
}
