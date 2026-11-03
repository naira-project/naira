package catalog

import "context"

// Reader provides read-only access to the catalog graph.
type Reader interface {
	GetNode(ctx context.Context, id NodeID) (Node, error)
	ListNodes(ctx context.Context) ([]Node, error)
	ListRelations(ctx context.Context) ([]Relation, error)
	ListKinds(ctx context.Context) ([]string, error)
}

// Service exposes read-only access to the catalog graph.
type Service struct {
	store Reader
}

func NewService(store Reader) *Service {
	return &Service{store: store}
}

func (s *Service) GetNode(ctx context.Context, id NodeID) (Node, error) {
	return s.store.GetNode(ctx, id)
}

func (s *Service) ListNodes(ctx context.Context) ([]Node, error) {
	return s.store.ListNodes(ctx)
}

func (s *Service) ListRelations(ctx context.Context) ([]Relation, error) {
	return s.store.ListRelations(ctx)
}

func (s *Service) ListKinds(ctx context.Context) ([]string, error) {
	return s.store.ListKinds(ctx)
}
