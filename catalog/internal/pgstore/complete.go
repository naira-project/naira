package pgstore

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/operations"
)

// CompleteSnapshotOperation applies a plugin snapshot and marks the
// corresponding operation as succeeded in one transaction.
func (s *SnapshotStore) CompleteSnapshotOperation(
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

	now := time.Now()
	tag, err := tx.Exec(ctx, `
		UPDATE operations
		SET state = $1, end_time = $2, error_message = NULL,
		    nodes_upserted = $3, relations_upserted = $4
		WHERE name = $5
	`, operations.StateSucceeded, now, upsertedNodes, upsertedRelations, operationName)
	if err != nil {
		return 0, 0, fmt.Errorf("marking operation %q succeeded: %w", operationName, err)
	}
	if tag.RowsAffected() == 0 {
		return 0, 0, fmt.Errorf("marking operation %q succeeded: %w", operationName, operations.ErrNotFound)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("committing transaction: %w", err)
	}
	return upsertedNodes, upsertedRelations, nil
}

// --- operations.Store delegation -------------------------------------------

func (s *SnapshotStore) Create(op operations.Operation) error {
	return s.ops.Create(op)
}

func (s *SnapshotStore) Get(name string) (operations.Operation, error) {
	return s.ops.Get(name)
}

func (s *SnapshotStore) List(filter operations.Filter) ([]operations.Operation, error) {
	return s.ops.List(filter)
}

func (s *SnapshotStore) UpdateState(name string, state operations.State, statusErr *operations.StatusError, nodesUpserted, relationsUpserted int) error {
	return s.ops.UpdateState(name, state, statusErr, nodesUpserted, relationsUpserted)
}

func (s *SnapshotStore) MarkInterrupted() (int, error) {
	return s.ops.MarkInterrupted()
}
