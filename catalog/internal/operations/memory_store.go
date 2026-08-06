package operations

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

const (
	// maxPerPlugin is the maximum number of operations kept in memory for a
	// single plugin. When this limit is reached the oldest operation for
	// that plugin is evicted on insert.
	maxPerPlugin = 5

	// ttl is the maximum age of an operation before it is eligible for lazy
	// eviction during List or Get calls.
	ttl = 3 * 24 * time.Hour
)

var (
	ErrNotFound      = errors.New("operation not found")
	ErrAlreadyExists = errors.New("operation already exists")
)

// MemoryStore is a Store implementation backed by a map, keyed by operation
// name and protected by a read-write mutex.
type MemoryStore struct {
	mu         sync.RWMutex
	operations map[string]Operation
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		operations: make(map[string]Operation),
	}
}

func (s *MemoryStore) Create(op Operation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.operations[op.Name]; exists {
		return fmt.Errorf("operation %q: %w", op.Name, ErrAlreadyExists)
	}

	s.evictOldestForPlugin(op.Plugin)
	s.operations[op.Name] = op
	return nil
}

func (s *MemoryStore) Get(name string) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	op, ok := s.operations[name]
	if !ok {
		return Operation{}, fmt.Errorf("operation %q: %w", name, ErrNotFound)
	}

	if s.isExpired(op) {
		delete(s.operations, name)
		return Operation{}, fmt.Errorf("operation %q: %w", name, ErrNotFound)
	}

	return op, nil
}

// List returns all non-expired operations ordered by creation time
// descending (most recent first). The optional filter narrows the result
// set by plugin name and/or operation state.
func (s *MemoryStore) List(filter Filter) ([]Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneExpired()

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

func (s *MemoryStore) UpdateState(name string, state State, err *StatusError, nodesUpserted, relationsUpserted int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	op, ok := s.operations[name]
	if !ok {
		return fmt.Errorf("operation %q: %w", name, ErrNotFound)
	}

	now := time.Now()
	op.State = state
	op.Error = err

	if state == StateRunning && op.StartTime.IsZero() {
		op.StartTime = now
	}

	if state == StateSucceeded {
		op.NodesUpserted = nodesUpserted
		op.RelationsUpserted = relationsUpserted
	}

	if state == StateSucceeded || state == StateFailed {
		op.EndTime = &now
	}

	s.operations[name] = op
	return nil
}

// evictOldestForPlugin removes the oldest operation for the given plugin if
// the per-plugin cap has been reached.
func (s *MemoryStore) evictOldestForPlugin(plugin string) {
	var oldestOp Operation
	var oldestName string
	count := 0

	for name, op := range s.operations {
		if op.Plugin != plugin {
			continue
		}
		count++
		if oldestName == "" || op.CreatedAt.Before(oldestOp.CreatedAt) {
			oldestOp = op
			oldestName = name
		}
	}

	if count >= maxPerPlugin && oldestName != "" {
		delete(s.operations, oldestName)
	}
}

func (s *MemoryStore) isExpired(op Operation) bool {
	return time.Since(op.CreatedAt) > ttl
}

func (s *MemoryStore) pruneExpired() {
	for name, op := range s.operations {
		if s.isExpired(op) {
			delete(s.operations, name)
		}
	}
}
