// Package catalogtest provides a mock of catalog.Store
package catalogtest

import "github.com/naira-project/naira/catalog/internal/catalog"

type MockStore struct {
	ListNodesFunc     func() ([]catalog.Node, error)
	GetNodeFunc       func(id catalog.NodeID) (catalog.Node, error)
	ListRelationsFunc func() ([]catalog.Relation, error)
}

func (m *MockStore) ListNodes() ([]catalog.Node, error) {
	if m.ListNodesFunc != nil {
		return m.ListNodesFunc()
	}
	return nil, nil
}

func (m *MockStore) GetNode(id catalog.NodeID) (catalog.Node, error) {
	if m.GetNodeFunc != nil {
		return m.GetNodeFunc(id)
	}
	return catalog.Node{}, catalog.ErrNodeNotFound
}

func (m *MockStore) ListRelations() ([]catalog.Relation, error) {
	if m.ListRelationsFunc != nil {
		return m.ListRelationsFunc()
	}
	return nil, nil
}
