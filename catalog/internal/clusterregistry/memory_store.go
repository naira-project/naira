package clusterregistry

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryStore stores clusters in process memory with concurrent access protection.
// Data is not shared between instances and is lost on restart. Use NewMemoryStore to initialize it.
type MemoryStore struct {
	mu       sync.RWMutex
	clusters map[uuid.UUID]Cluster
}

var _ Store = (*MemoryStore)(nil)

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{clusters: make(map[uuid.UUID]Cluster)}
}

func (s *MemoryStore) Create(ctx context.Context, cluster Cluster) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("create cluster: %w", err)
	}
	if err := cluster.Validate(); err != nil {
		return fmt.Errorf("validate cluster: %w", err)
	}
	if _, exists := s.clusters[cluster.ID]; exists {
		return fmt.Errorf("create cluster: %w", ErrAlreadyExists)
	}
	if s.nameTaken(cluster.Workspace, cluster.ID, cluster.Name) {
		return fmt.Errorf("create cluster: %w", ErrAlreadyExists)
	}
	s.clusters[cluster.ID] = cloneCluster(cluster)
	return nil
}

func (s *MemoryStore) Get(ctx context.Context, workspace Workspace, id uuid.UUID) (Cluster, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := ctx.Err(); err != nil {
		return Cluster{}, fmt.Errorf("get cluster: %w", err)
	}
	if err := workspace.Validate(); err != nil {
		return Cluster{}, fmt.Errorf("validate workspace ID: %w", err)
	}
	cluster, exists := s.clusters[id]
	if !exists || cluster.Workspace != workspace {
		return Cluster{}, fmt.Errorf("get cluster: %w", ErrNotFound)
	}
	return cloneCluster(cluster), nil
}

func (s *MemoryStore) List(ctx context.Context, workspace Workspace) ([]Cluster, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list clusters: %w", err)
	}
	if err := workspace.Validate(); err != nil {
		return nil, fmt.Errorf("validate workspace ID: %w", err)
	}
	result := make([]Cluster, 0)
	for _, cluster := range s.clusters {
		if cluster.Workspace == workspace {
			result = append(result, cloneCluster(cluster))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func (s *MemoryStore) Update(ctx context.Context, workspace Workspace, id uuid.UUID, params UpdateParams) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("update cluster: %w", err)
	}
	if err := workspace.Validate(); err != nil {
		return fmt.Errorf("validate workspace ID: %w", err)
	}
	cluster, exists := s.clusters[id]
	if !exists || cluster.Workspace != workspace {
		return fmt.Errorf("update cluster: %w", ErrNotFound)
	}
	if err := cluster.ApplyUpdate(params); err != nil {
		return fmt.Errorf("apply cluster update: %w", err)
	}
	if s.nameTaken(workspace, id, cluster.Name) {
		return fmt.Errorf("update cluster: %w", ErrAlreadyExists)
	}
	s.clusters[id] = cluster
	return nil
}

func (s *MemoryStore) SaveValidation(ctx context.Context, workspace Workspace, id uuid.UUID, expectedUpdate time.Time, result ValidationResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("save validation: %w", err)
	}
	if err := workspace.Validate(); err != nil {
		return fmt.Errorf("validate workspace: %w", err)
	}
	cluster, exists := s.clusters[id]
	if !exists || cluster.Workspace != workspace {
		return fmt.Errorf("get cluster: %w", ErrNotFound)
	}
	if !cluster.UpdatedAt.Equal(expectedUpdate) {
		return fmt.Errorf("compare validation revision: %w", ErrValidationConflict)
	}
	if err := cluster.applyValidation(result); err != nil {
		return fmt.Errorf("apply connection result: %w", err)
	}
	s.clusters[id] = cloneCluster(cluster)
	return nil
}

// nameTaken requires the store lock to be held.
func (s *MemoryStore) nameTaken(workspace Workspace, id uuid.UUID, name string) bool {
	for _, cluster := range s.clusters {
		if cluster.ID != id && cluster.Workspace == workspace && cluster.Name == name {
			return true
		}
	}
	return false
}

func cloneCluster(cluster Cluster) Cluster {
	if cluster.LastValidationAt != nil {
		cluster.LastValidationAt = new(*cluster.LastValidationAt)
	}
	if cluster.LastValidation != nil {
		result := *cluster.LastValidation
		result.Checks = append([]AccessCheck(nil), result.Checks...)
		cluster.LastValidation = &result
	}
	return cluster
}
