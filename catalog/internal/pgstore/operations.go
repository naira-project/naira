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
				ORDER BY created_at DESC
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

func (s *OperationStore) UpdateState(ctx context.Context, name string, state operations.State, statusErr *operations.StatusError, nodesUpserted, relationsUpserted int) error {
	return updateOperationState(ctx, s.pool, name, state, statusErr, nodesUpserted, relationsUpserted)
}

type operationQuerier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
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
	switch state {
	case operations.StateRunning:
		query = `
            UPDATE operations
            SET state = $1, error_message = $2, start_time = COALESCE(start_time, $3)
            WHERE name = $4`
		args = []any{state, errorMessage, now, name}

	case operations.StateSucceeded:
		query = `
            UPDATE operations
            SET state = $1, error_message = $2, end_time = $3,
                nodes_upserted = $4, relations_upserted = $5
            WHERE name = $6`
		args = []any{state, errorMessage, now, nodesUpserted, relationsUpserted, name}

	case operations.StateFailed:
		query = `
            UPDATE operations
            SET state = $1, error_message = $2, end_time = $3
            WHERE name = $4`
		args = []any{state, errorMessage, now, name}

	default:
		return fmt.Errorf("operations state %q not handled for operation %q", state, name)
	}

	tag, err := q.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("updating operation %q: %w", name, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("operation %q: %w", name, operations.ErrNotFound)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanOperation(row rowScanner) (operations.Operation, error) {
	var op operations.Operation
	var startTime *time.Time
	var errorMessage *string

	if err := row.Scan(
		&op.Name, &op.Plugin, &op.State, &startTime, &op.EndTime, &errorMessage,
		&op.NodesUpserted, &op.RelationsUpserted, &op.CreatedAt,
	); err != nil {
		return operations.Operation{}, err
	}

	if startTime != nil {
		op.StartTime = *startTime
	}
	if errorMessage != nil {
		op.Error = &operations.StatusError{Message: *errorMessage}
	}
	return op, nil
}
