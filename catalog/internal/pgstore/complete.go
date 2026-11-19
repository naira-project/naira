package pgstore

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

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
) (SnapshotResult, error) {
	if err := catalog.ValidateSnapshotInput(pluginName, snapshotID, nodes, relations); err != nil {
		return SnapshotResult{}, fmt.Errorf("validate snapshot input: %w", err)
	}

	var result SnapshotResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		result, err = applySnapshot(ctx, tx, pluginName, snapshotID, nodes, relations)
		if err != nil {
			return fmt.Errorf("applying snapshot: %w", err)
		}

		if err := updateOperationState(ctx, tx, operationName, operations.StateSucceeded, nil, result.NodesUpserted, result.RelationsUpserted); err != nil {
			return fmt.Errorf("marking operation %q succeeded: %w", operationName, err)
		}
		return nil
	})
	if err != nil {
		return SnapshotResult{}, fmt.Errorf("completing snapshot operation %q: %w", operationName, err)
	}
	return result, nil
}
