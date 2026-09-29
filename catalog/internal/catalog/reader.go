package catalog

import "context"

// Service exposes read-only access to the catalog graph. It intentionally
// knows nothing about plugins or async operations - see the pluginrun
// package for the code that populates the graph via Store.ApplyPluginSnapshot.
type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) GetNode(_ context.Context, id NodeID) (Node, error) {
	return s.store.GetNode(id)
}

func (s *Service) ListNodes(_ context.Context) []Node {
	return s.store.ListNodes()
}

func (s *Service) ListRelations(_ context.Context) []Relation {
	return s.store.ListRelations()
}
