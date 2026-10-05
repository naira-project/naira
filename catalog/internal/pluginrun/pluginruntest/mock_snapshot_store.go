// Package pluginruntest provides a configurable mock of
// pluginrun.SnapshotStore for use in pluginrun's own tests and in httpapi
// tests that wire a Runner. It embeds operationstest.MockStore for
// operations.Store and defaults CompleteSnapshotOperation to a simple
// "upsert count equals input length, then mark the operation succeeded"
// behavior - it does not model a real graph.
package pluginruntest

import (
	"context"

	"github.com/google/uuid"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/operations"
	"github.com/naira-project/naira/catalog/internal/operations/operationstest"
)

type MockSnapshotStore struct {
	*operationstest.MockStore

	CompleteSnapshotOperationFunc func(
		ctx context.Context,
		operationName, pluginName string,
		snapshotID uuid.UUID,
		nodes []catalog.NodeClaim,
		relations []catalog.RelationClaim,
	) (int, int, error)
}

func NewMockSnapshotStore() *MockSnapshotStore {
	return &MockSnapshotStore{MockStore: operationstest.NewMockStore()}
}

func (m *MockSnapshotStore) CompleteSnapshotOperation(
	ctx context.Context,
	operationName, pluginName string,
	snapshotID uuid.UUID,
	nodes []catalog.NodeClaim,
	relations []catalog.RelationClaim,
) (int, int, error) {
	if m.CompleteSnapshotOperationFunc != nil {
		return m.CompleteSnapshotOperationFunc(ctx, operationName, pluginName, snapshotID, nodes, relations)
	}

	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}

	nodesUpserted, relationsUpserted := len(nodes), len(relations)
	if err := m.MockStore.UpdateState(operationName, operations.StateSucceeded, nil, nodesUpserted, relationsUpserted); err != nil {
		return 0, 0, err
	}
	return nodesUpserted, relationsUpserted, nil
}
