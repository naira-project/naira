package pgstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/naira-project/naira/catalog/internal/operations"
)

const maxOperationsPerPlugin = 5

const selectOperationSQL = `
	SELECT name, plugin, state, start_time, end_time, error_message, nodes_upserted, relations_upserted, created_at
	FROM operations
`

// Create inserts op and evicts the plugin's operations beyond the newest
// [maxOperationsPerPlugin]. Ordering is by CreatedAt, so an op created with an
// old CreatedAt may be evicted immediately.
func (s *OperationStore) Create(ctx context.Context, op operations.Operation) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO operations (name, plugin, state, created_at)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (name) DO NOTHING
		`, op.Name, op.Plugin, op.State, op.CreatedAt)
		if err != nil {
			return fmt.Errorf("inserting operation %q: %w", op.Name, err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("operation %q: %w", op.Name, operations.ErrAlreadyExists)
		}

		if _, err := tx.Exec(ctx, `
			DELETE FROM operations
			WHERE name IN (
				SELECT name FROM operations
				WHERE plugin = $1
				ORDER BY created_at DESC, name DESC
				OFFSET $2
			)
		`, op.Plugin, maxOperationsPerPlugin); err != nil {
			return fmt.Errorf("evicting old operations for plugin %q: %w", op.Plugin, err)
		}
		return nil
	})
}

func (s *OperationStore) Get(ctx context.Context, name string) (operations.Operation, error) {
	op, err := scanOperation(s.pool.QueryRow(ctx, selectOperationSQL+` WHERE name = $1`, name))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return operations.Operation{}, fmt.Errorf("operation %q: %w", name, operations.ErrNotFound)
		}
		return operations.Operation{}, fmt.Errorf("getting operation %q: %w", name, err)
	}
	return op, nil
}

func (s *OperationStore) List(ctx context.Context, filter operations.Filter) ([]operations.Operation, error) {
	query := selectOperationSQL + ` WHERE 1=1`
	var args []any
	if filter.Plugin != "" {
		args = append(args, filter.Plugin)
		query += fmt.Sprintf(" AND plugin = $%d", len(args))
	}
	if filter.State != "" {
		args = append(args, filter.State)
		query += fmt.Sprintf(" AND state = $%d", len(args))
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing operations: %w", err)
	}

	result, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (operations.Operation, error) {
		return scanOperation(row)
	})
	if err != nil {
		return nil, fmt.Errorf("collecting operations: %w", err)
	}

	return result, nil
}

// UpdateState moves an operation to state. Operations that already succeeded
// or failed are final: such updates return [operations.ErrInvalidTransition].
func (s *OperationStore) UpdateState(ctx context.Context, name string, state operations.State, statusErr *operations.StatusError, nodesUpserted, relationsUpserted int) error {
	return updateOperationState(ctx, s.pool, name, state, statusErr, nodesUpserted, relationsUpserted)
}

type operationQuerier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func updateOperationState(
	ctx context.Context,
	q operationQuerier,
	name string,
	state operations.State,
	statusErr *operations.StatusError,
	nodesUpserted, relationsUpserted int,
) error {
	now := time.Now()

	var errorMessage *string
	if statusErr != nil {
		errorMessage = &statusErr.Message
	}

	var (
		query string
		args  []any
	)
	// Only non-final operations may be updated.
	switch state {
	case operations.StateRunning:
		query = `
			UPDATE operations
			SET state = $1, error_message = $2, start_time = COALESCE(start_time, $3)
			WHERE name = $4 AND state IN ('PENDING', 'RUNNING')`
		args = []any{state, errorMessage, now, name}

	case operations.StateSucceeded:
		query = `
			UPDATE operations
			SET state = $1, error_message = $2, end_time = $3,
			    nodes_upserted = $4, relations_upserted = $5
			WHERE name = $6 AND state IN ('PENDING', 'RUNNING')`
		args = []any{state, errorMessage, now, nodesUpserted, relationsUpserted, name}

	case operations.StateFailed:
		query = `
			UPDATE operations
			SET state = $1, error_message = $2, end_time = $3
			WHERE name = $4 AND state IN ('PENDING', 'RUNNING')`
		args = []any{state, errorMessage, now, name}

	default:
		return fmt.Errorf("operations state %q not handled for operation %q", state, name)
	}

	tag, err := q.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("updating operation %q: %w", name, err)
	}
	if tag.RowsAffected() > 0 {
		return nil
	}

	var current operations.State
	err = q.QueryRow(ctx, `SELECT state FROM operations WHERE name = $1`, name).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("operation %q: %w", name, operations.ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("reading state of operation %q: %w", name, err)
	}
	return fmt.Errorf("operation %q is %s, cannot move to %s: %w", name, current, state, operations.ErrInvalidTransition)
}

func scanOperation(row pgx.Row) (operations.Operation, error) {
	var op operations.Operation
	var errorMessage *string

	if err := row.Scan(
		&op.Name, &op.Plugin, &op.State, &op.StartTime, &op.EndTime, &errorMessage,
		&op.NodesUpserted, &op.RelationsUpserted, &op.CreatedAt,
	); err != nil {
		return operations.Operation{}, fmt.Errorf("scanning operation row: %w", err)
	}
	if errorMessage != nil {
		op.Error = &operations.StatusError{Message: *errorMessage}
	}
	return op, nil
}
