// Package catalogtest provides configurable stubs for catalog.Store.
package catalogtest

import (
	"context"
	"errors"
	"fmt"

	"github.com/naira-project/naira/catalog/internal/catalog"
)

var ErrUnexpectedCall = errors.New("unexpected call to unconfigured catalog store method")

type StubStore struct {
	ListNodesFunc     func(context.Context) ([]catalog.Node, error)
	GetNodeFunc       func(context.Context, catalog.NodeID) (catalog.Node, error)
	ListRelationsFunc func(context.Context) ([]catalog.Relation, error)
}

func (s *StubStore) ListNodes(ctx context.Context) ([]catalog.Node, error) {
	if s.ListNodesFunc == nil {
		return nil, fmt.Errorf("ListNodes: %w", ErrUnexpectedCall)
	}
	return s.ListNodesFunc(ctx)
}

func (s *StubStore) GetNode(ctx context.Context, id catalog.NodeID) (catalog.Node, error) {
	if s.GetNodeFunc == nil {
		return catalog.Node{}, fmt.Errorf("GetNode: %w", ErrUnexpectedCall)
	}
	return s.GetNodeFunc(ctx, id)
}

func (s *StubStore) ListRelations(ctx context.Context) ([]catalog.Relation, error) {
	if s.ListRelationsFunc == nil {
		return nil, fmt.Errorf("ListRelations: %w", ErrUnexpectedCall)
	}
	return s.ListRelationsFunc(ctx)
}
