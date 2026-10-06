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

// SnapshotStore composes operation persistence with the atomic cross-aggregate
// commit used by plugin runs. It wraps an OperationStore for operations.Store
// methods and adds CompleteSnapshotOperation for the two-phase commit.
type SnapshotStore struct {
	pool *pgxpool.Pool
	ops  *OperationStore
}

func NewGraphStore(pool *pgxpool.Pool) *GraphStore {
	return &GraphStore{pool: pool}
}

func NewOperationStore(pool *pgxpool.Pool) *OperationStore {
	return &OperationStore{pool: pool}
}

func NewSnapshotStore(pool *pgxpool.Pool) *SnapshotStore {
	return &SnapshotStore{
		pool: pool,
		ops:  NewOperationStore(pool),
	}
}
