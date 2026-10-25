package workspaces

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/google/uuid"
)

// MemoryStore is concurrency-safe, instance-local and loses all data on restart.
// Initialize it with NewMemoryStore.
type MemoryStore struct {
	mu         sync.RWMutex
	workspaces map[uuid.UUID]Workspace
}

var _ Store = (*MemoryStore)(nil)

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{workspaces: make(map[uuid.UUID]Workspace)}
}

func (s *MemoryStore) Create(ctx context.Context, workspace Workspace) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	if err := workspace.Validate(); err != nil {
		return fmt.Errorf("validate workspace: %w", err)
	}
	if _, exists := s.workspaces[workspace.ID]; exists {
		return fmt.Errorf("create workspace: %w", ErrAlreadyExists)
	}
	for _, existing := range s.workspaces {
		if existing.Owner == workspace.Owner && existing.Name == workspace.Name {
			return fmt.Errorf("create workspace: %w", ErrAlreadyExists)
		}
	}
	s.workspaces[workspace.ID] = workspace
	return nil
}

func (s *MemoryStore) Get(ctx context.Context, owner Owner, id uuid.UUID) (Workspace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return Workspace{}, fmt.Errorf("get workspace: %w", err)
	}
	if err := owner.Validate(); err != nil {
		return Workspace{}, fmt.Errorf("validate owner: %w", err)
	}
	workspace, exists := s.workspaces[id]
	if !exists || workspace.Owner != owner {
		return Workspace{}, fmt.Errorf("get workspace: %w", ErrNotFound)
	}
	return workspace, nil
}

func (s *MemoryStore) List(ctx context.Context, owner Owner) ([]Workspace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	if err := owner.Validate(); err != nil {
		return nil, fmt.Errorf("validate owner: %w", err)
	}
	result := make([]Workspace, 0)
	for _, workspace := range s.workspaces {
		if workspace.Owner == owner {
			result = append(result, workspace)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}
