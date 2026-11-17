package pgstore

import "github.com/jackc/pgx/v5/pgxpool"

// GraphStore persists the catalog graph.
type GraphStore struct {
	pool *pgxpool.Pool
}

// OperationStore persists asynchronous operation state.
type OperationStore struct {
	pool *pgxpool.Pool
}

// SnapshotCommitter atomically persists a graph snapshot and updates the
// corresponding operation in one transaction.
type SnapshotCommitter struct {
	pool *pgxpool.Pool
}

func NewGraphStore(pool *pgxpool.Pool) *GraphStore {
	return &GraphStore{pool: pool}
}

func NewOperationStore(pool *pgxpool.Pool) *OperationStore {
	return &OperationStore{pool: pool}
}

func NewSnapshotCommitter(pool *pgxpool.Pool) *SnapshotCommitter {
	return &SnapshotCommitter{pool: pool}
}
