// Package catalogtest provides a mock of catalog.Store
package catalogtest

import (
	"context"

	"github.com/naira-project/naira/catalog/internal/catalog"
)

type MockStore struct {
	ListNodesFunc     func(context.Context) ([]catalog.Node, error)
	GetNodeFunc       func(context.Context, catalog.NodeID) (catalog.Node, error)
	ListRelationsFunc func(context.Context) ([]catalog.Relation, error)
}

func (m *MockStore) ListNodes(ctx context.Context) ([]catalog.Node, error) {
	if m.ListNodesFunc != nil {
		return m.ListNodesFunc(ctx)
	}
	return nil, nil
}

func (m *MockStore) GetNode(ctx context.Context, id catalog.NodeID) (catalog.Node, error) {
	if m.GetNodeFunc != nil {
		return m.GetNodeFunc(ctx, id)
	}
	return catalog.Node{}, catalog.ErrNodeNotFound
}

func (m *MockStore) ListRelations(ctx context.Context) ([]catalog.Relation, error) {
	if m.ListRelationsFunc != nil {
		return m.ListRelationsFunc(ctx)
	}
	return nil, nil
}
