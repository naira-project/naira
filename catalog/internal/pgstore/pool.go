// Package pgstore is the Postgres-backed persistence layer for the catalog
// graph and plugin-run operations.
package pgstore

import (
	"context"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaFS embed.FS

// Connect opens a connection pool to Postgres and ensures the schema exists.
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("reading embedded schema: %w", err)
	}

	if _, err := pool.Exec(ctx, string(schema)); err != nil {
		pool.Close()
		return nil, fmt.Errorf("applying schema: %w", err)
	}

	return pool, nil
}
