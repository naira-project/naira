package pgstore

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/operations"
)

// CompleteSnapshotOperation applies a plugin snapshot and marks the
// corresponding operation as succeeded in one transaction.
func (s *SnapshotCommitter) CompleteSnapshotOperation(
	ctx context.Context,
	operationName, pluginName string,
	snapshotID uuid.UUID,
	nodes []catalog.NodeClaim,
	relations []catalog.RelationClaim,
) (int, int, error) {
	if err := catalog.ValidateSnapshotInput(pluginName, snapshotID, nodes, relations); err != nil {
		return 0, 0, fmt.Errorf("validate snapshot input: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	upsertedNodes, upsertedRelations, err := applySnapshot(ctx, tx, pluginName, snapshotID, nodes, relations)
	if err != nil {
		return 0, 0, err
	}

	if err := updateOperationState(ctx, tx, operationName, operations.StateSucceeded, nil, upsertedNodes, upsertedRelations); err != nil {
		return 0, 0, fmt.Errorf("marking operation %q succeeded: %w", operationName, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("committing transaction: %w", err)
	}
	return upsertedNodes, upsertedRelations, nil
}
