package pgstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/naira-project/naira/catalog/internal/pgstore"
)

func TestConnect_IsIdempotent(t *testing.T) {
	t.Parallel()

	dsn := newDatabase(t)

	first, err := pgstore.Connect(t.Context(), dsn)
	require.NoError(t, err)
	t.Cleanup(first.Close)

	second, err := pgstore.Connect(t.Context(), dsn)
	require.NoError(t, err)
	t.Cleanup(second.Close)

	var tables int
	err = second.QueryRow(t.Context(), `
		SELECT count(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_name IN (
			  'nodes', 'node_plugin_claims', 'relations',
			  'relation_plugin_claims', 'operations'
		  )
	`).Scan(&tables)
	require.NoError(t, err)
	assert.Equal(t, 5, tables)

}

func TestConnect_UnreachableDatabase(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	pool, err := pgstore.Connect(ctx, "postgres://catalog:catalog@127.0.0.1:1/catalog?sslmode=disable&connect_timeout=2")

	assert.Nil(t, pool)
	assert.ErrorContains(t, err, "migrating database")
}
