package sources

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// MemoryStore stores registrations in process memory with concurrent access protection.
// Data is not shared between instances and is lost on restart. Use NewMemoryStore to initialize it.
type MemoryStore struct {
	mu            sync.RWMutex
	registrations map[uuid.UUID]Registration
}

var _ Store = (*MemoryStore)(nil)

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{registrations: make(map[uuid.UUID]Registration)}
}

func (s *MemoryStore) Create(ctx context.Context, registration Registration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("create source registration: %w", err)
	}
	if err := registration.Validate(); err != nil {
		return fmt.Errorf("validate source registration: %w", err)
	}
	if _, exists := s.registrations[registration.ID]; exists {
		return fmt.Errorf("create source registration: %w", ErrAlreadyExists)
	}
	for _, existing := range s.registrations {
		if existing.WorkspaceID == registration.WorkspaceID && existing.Name == registration.Name {
			return fmt.Errorf("create source registration: %w", ErrAlreadyExists)
		}
	}
	s.registrations[registration.ID] = cloneRegistration(registration)
	return nil
}

func (s *MemoryStore) Get(ctx context.Context, workspaceID string, id uuid.UUID) (Registration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := ctx.Err(); err != nil {
		return Registration{}, fmt.Errorf("get source registration: %w", err)
	}
	if err := validateWorkspaceID(workspaceID); err != nil {
		return Registration{}, fmt.Errorf("validate workspace ID: %w", err)
	}
	registration, exists := s.registrations[id]
	if !exists || registration.WorkspaceID != workspaceID {
		return Registration{}, fmt.Errorf("get source registration: %w", ErrNotFound)
	}
	return cloneRegistration(registration), nil
}

func (s *MemoryStore) List(ctx context.Context, workspaceID string) ([]Registration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list source registrations: %w", err)
	}
	if err := validateWorkspaceID(workspaceID); err != nil {
		return nil, fmt.Errorf("validate workspace ID: %w", err)
	}
	result := make([]Registration, 0)
	for _, registration := range s.registrations {
		if registration.WorkspaceID == workspaceID {
			result = append(result, cloneRegistration(registration))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func (s *MemoryStore) Update(ctx context.Context, workspaceID string, id uuid.UUID, params UpdateParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("update source registration: %w", err)
	}
	if err := validateWorkspaceID(workspaceID); err != nil {
		return fmt.Errorf("validate workspace ID: %w", err)
	}
	registration, exists := s.registrations[id]
	if !exists || registration.WorkspaceID != workspaceID {
		return fmt.Errorf("update source registration: %w", ErrNotFound)
	}
	updated := cloneRegistration(registration)
	if err := updated.ApplyUpdate(params); err != nil {
		return fmt.Errorf("apply source registration update: %w", err)
	}
	for _, existing := range s.registrations {
		if existing.ID != id && existing.WorkspaceID == workspaceID && existing.Name == updated.Name {
			return fmt.Errorf("update source registration: %w", ErrAlreadyExists)
		}
	}
	s.registrations[id] = updated
	return nil
}

func validateWorkspaceID(workspaceID string) error {
	if workspaceID == "" || workspaceID != strings.TrimSpace(workspaceID) || strings.ContainsAny(workspaceID, "\r\n\t") {
		return fmt.Errorf("workspace ID must be nonempty without surrounding whitespace or control separators: %w", ErrInvalidRegistration)
	}
	return nil
}

func cloneRegistration(registration Registration) Registration {
	if registration.Kubernetes != nil {
		registration.Kubernetes = new(*registration.Kubernetes)
	}
	if registration.LastValidationAt != nil {
		registration.LastValidationAt = new(*registration.LastValidationAt)
	}
	return registration
}
