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

const (
	maxOperationsPerPlugin      = 5
	interruptedOperationMessage = "operation was interrupted because the catalog process restarted before it could complete"
)

const selectOperationSQL = `
	SELECT name, plugin, state, start_time, end_time, error_message, nodes_upserted, relations_upserted, created_at
	FROM operations
`

func (s *OperationStore) Create(ctx context.Context, op operations.Operation) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

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

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
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
	defer rows.Close()

	result := make([]operations.Operation, 0)
	for rows.Next() {
		op, err := scanOperation(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning operation: %w", err)
		}
		result = append(result, op)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading operations: %w", err)
	}
	return result, nil
}

func (s *OperationStore) UpdateState(ctx context.Context, name string, state operations.State, statusErr *operations.StatusError, nodesUpserted, relationsUpserted int) error {
	return updateOperationState(ctx, s.pool, name, state, statusErr, nodesUpserted, relationsUpserted)
}

// MarkInterrupted transitions every operation currently PENDING or RUNNING
// to StateInterrupted
func (s *OperationStore) MarkInterrupted(ctx context.Context) (int, error) {
	now := time.Now()

	tag, err := s.pool.Exec(ctx, `
		UPDATE operations
		SET state = $1, end_time = $2, error_message = $3
		WHERE state IN ($4, $5)
	`, operations.StateInterrupted, now, interruptedOperationMessage, operations.StatePending, operations.StateRunning)
	if err != nil {
		return 0, fmt.Errorf("marking interrupted operations: %w", err)
	}

	return int(tag.RowsAffected()), nil
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

	query := `UPDATE operations SET state = $1, error_message = $2`
	args := []any{state, errorMessage}
	if state == operations.StateRunning {
		query += fmt.Sprintf(", start_time = COALESCE(start_time, $%d)", len(args)+1)
		args = append(args, now)
	}
	if state == operations.StateSucceeded || state == operations.StateFailed {
		query += fmt.Sprintf(", end_time = $%d", len(args)+1)
		args = append(args, now)
	}
	if state == operations.StateSucceeded {
		query += fmt.Sprintf(", nodes_upserted = $%d, relations_upserted = $%d", len(args)+1, len(args)+2)
		args = append(args, nodesUpserted, relationsUpserted)
	}
	query += fmt.Sprintf(" WHERE name = $%d", len(args)+1)
	args = append(args, name)

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
