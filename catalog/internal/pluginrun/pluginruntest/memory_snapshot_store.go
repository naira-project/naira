// Package pluginruntest provides in-memory stores used by plugin run tests.
package pluginruntest

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/operations"
	"github.com/naira-project/naira/catalog/internal/operations/operationstest"
)

var ErrUnexpectedSnapshotCommit = errors.New("unexpected snapshot commit")

type MemorySnapshotStore struct {
	*operationstest.MemoryStore

	CompleteSnapshotOperationFunc func(
		ctx context.Context,
		operationName, pluginName string,
		snapshotID uuid.UUID,
		nodes []catalog.NodeClaim,
		relations []catalog.RelationClaim,
	) (int, int, error)
}

// SuccessfulSnapshotCommit returns a test commit function that marks the
// operation successful and reports the number of submitted claims. It does
// not model catalog persistence.
func SuccessfulSnapshotCommit(store *operationstest.MemoryStore) func(
	context.Context,
	string,
	string,
	uuid.UUID,
	[]catalog.NodeClaim,
	[]catalog.RelationClaim,
) (int, int, error) {
	return func(
		ctx context.Context,
		operationName, _ string,
		_ uuid.UUID,
		nodes []catalog.NodeClaim,
		relations []catalog.RelationClaim,
	) (int, int, error) {
		nodesUpserted, relationsUpserted := len(nodes), len(relations)
		if err := store.UpdateState(ctx, operationName, operations.StateSucceeded, nil, nodesUpserted, relationsUpserted); err != nil {
			return 0, 0, fmt.Errorf("marking operation %q succeeded: %w", operationName, err)
		}
		return nodesUpserted, relationsUpserted, nil
	}
}

func NewMemorySnapshotStore() *MemorySnapshotStore {
	return &MemorySnapshotStore{MemoryStore: operationstest.NewMemoryStore()}
}

// NewMemorySnapshotStoreWithSuccessfulCommit marks operations succeeded and
// reports submitted claim counts. It does not model catalog persistence.
func NewMemorySnapshotStoreWithSuccessfulCommit() *MemorySnapshotStore {
	store := NewMemorySnapshotStore()
	store.CompleteSnapshotOperationFunc = SuccessfulSnapshotCommit(store.MemoryStore)
	return store
}

func (s *MemorySnapshotStore) CompleteSnapshotOperation(
	ctx context.Context,
	operationName, pluginName string,
	snapshotID uuid.UUID,
	nodes []catalog.NodeClaim,
	relations []catalog.RelationClaim,
) (int, int, error) {
	if s.CompleteSnapshotOperationFunc == nil {
		return 0, 0, fmt.Errorf("operation %q: %w", operationName, ErrUnexpectedSnapshotCommit)
	}
	return s.CompleteSnapshotOperationFunc(ctx, operationName, pluginName, snapshotID, nodes, relations)
}
