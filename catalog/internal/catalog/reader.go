package catalog

import (
	"context"
	"fmt"
)

// Service exposes read-only access to the catalog graph. It intentionally
// knows nothing about plugins or async operations - see the pluginrun
// package for the code that populates the graph via SnapshotCommitter.
type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) GetNode(ctx context.Context, id NodeID) (Node, error) {
	return s.store.GetNode(ctx, id)
}

func (s *Service) ListNodes(ctx context.Context) ([]Node, error) {
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}
	return nodes, nil
}

func (s *Service) ListRelations(ctx context.Context) ([]Relation, error) {
	relations, err := s.store.ListRelations(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing relations: %w", err)
	}
	return relations, nil
}
