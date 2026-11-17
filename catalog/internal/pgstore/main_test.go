package pgstore_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/operations"
	"github.com/naira-project/naira/catalog/internal/pgstore"
)

// One Postgres container serves the whole package. Migrations are applied
// once (through the production pgstore.Connect) to templateDB; every test
// then gets its own database cloned from that template, which is much
// cheaper than starting a container or re-running migrations per test and
// lets tests run in parallel without sharing state.
const templateDB = "catalog_template"

var (
	// serverURL points at the container; its path is replaced to address a
	// specific database.
	serverURL *url.URL
	// adminPool is connected to the "postgres" maintenance database and is
	// used to create and drop per-test databases.
	adminPool *pgxpool.Pool
	dbCounter atomic.Int64
)

var (
	snap1 = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	snap2 = uuid.MustParse("00000000-0000-0000-0000-000000000002")
	snap3 = uuid.MustParse("00000000-0000-0000-0000-000000000003")
	snap4 = uuid.MustParse("00000000-0000-0000-0000-000000000004")
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:17",
		postgres.WithDatabase(templateDB),
		postgres.WithUsername("catalog"),
		postgres.WithPassword("catalog"),
		postgres.BasicWaitStrategies(),
	)
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			fmt.Fprintf(os.Stderr, "terminating postgres container: %v\n", err)
		}
	}()
	if err != nil {
		fmt.Fprintf(os.Stderr, "starting postgres container: %v\n", err)
		return 1
	}

	connString, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "getting postgres connection string: %v\n", err)
		return 1
	}
	serverURL, err = url.Parse(connString)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parsing postgres connection string: %v\n", err)
		return 1
	}

	templatePool, err := pgstore.Connect(ctx, databaseDSN(templateDB))
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrating template database: %v\n", err)
		return 1
	}
	templatePool.Close()

	adminPool, err = pgxpool.New(ctx, databaseDSN("postgres"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "connecting to maintenance database: %v\n", err)
		return 1
	}
	defer adminPool.Close()

	// CREATE DATABASE ... TEMPLATE fails while anyone is connected to the
	// template, and server-side sessions can outlive templatePool.Close() briefly.
	if err := waitForNoSessions(ctx, templateDB); err != nil {
		fmt.Fprintf(os.Stderr, "waiting for template database to become idle: %v\n", err)
		return 1
	}

	return m.Run()
}

func databaseDSN(database string) string {
	u := *serverURL
	u.Path = "/" + database
	return u.String()
}

func waitForNoSessions(ctx context.Context, database string) error {
	deadline := time.Now().Add(10 * time.Second)
	for {
		var sessions int
		err := adminPool.QueryRow(ctx,
			`SELECT count(*) FROM pg_stat_activity WHERE datname = $1`, database,
		).Scan(&sessions)
		if err != nil {
			return fmt.Errorf("counting sessions of database %q: %w", database, err)
		}
		if sessions == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%d sessions still connected to database %q", sessions, database)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// newDatabase clones the template into a fresh database that is dropped when
// the test finishes, and returns its DSN.
func newDatabase(t *testing.T) string {
	t.Helper()

	name := fmt.Sprintf("test_%d", dbCounter.Add(1))
	_, err := adminPool.Exec(t.Context(), fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s", name, templateDB))
	require.NoError(t, err)

	t.Cleanup(func() {
		// t.Context() is already cancelled when cleanups run.
		_, err := adminPool.Exec(context.Background(), fmt.Sprintf("DROP DATABASE %s WITH (FORCE)", name))
		assert.NoError(t, err)
	})

	return databaseDSN(name)
}

// env bundles all public stores over one isolated database.
type env struct {
	pool       *pgxpool.Pool
	graph      *pgstore.GraphStore
	operations *pgstore.OperationStore
	committer  *pgstore.SnapshotCommitter
	opSeq      atomic.Int64
}

// newEnv goes through pgstore.Connect (migrations are a no-op on an already
// migrated clone), so every test also exercises the production path.
func newEnv(t *testing.T) *env {
	t.Helper()

	pool, err := pgstore.Connect(t.Context(), newDatabase(t))
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return &env{
		pool:       pool,
		graph:      pgstore.NewGraphStore(pool),
		operations: pgstore.NewOperationStore(pool),
		committer:  pgstore.NewSnapshotCommitter(pool),
	}
}

// apply creates a pending operation and completes it with the given snapshot.
// It reports errors instead of failing the test.
func (e *env) apply(
	t *testing.T,
	plugin string,
	snapshotID uuid.UUID,
	nodes []catalog.NodeClaim,
	relations []catalog.RelationClaim,
) (int, int, error) {
	t.Helper()

	name := fmt.Sprintf("op-%d", e.opSeq.Add(1))
	if err := e.operations.Create(t.Context(), pendingOperation(name, plugin, time.Now())); err != nil {
		return 0, 0, fmt.Errorf("creating operation %q: %w", name, err)
	}

	upsertedNodes, upsertedRelations, err := e.committer.CompleteSnapshotOperation(
		t.Context(), name, plugin, snapshotID, nodes, relations,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("completing snapshot operation %q: %w", name, err)
	}
	return upsertedNodes, upsertedRelations, nil
}

func (e *env) mustApply(
	t *testing.T,
	plugin string,
	snapshotID uuid.UUID,
	nodes []catalog.NodeClaim,
	relations []catalog.RelationClaim,
) {
	t.Helper()

	_, _, err := e.apply(t, plugin, snapshotID, nodes, relations)
	require.NoError(t, err)
}

func (e *env) countRows(t *testing.T, table string) int {
	t.Helper()

	var count int
	err := e.pool.QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&count)
	require.NoError(t, err)
	return count
}

func (e *env) nodeRowID(t *testing.T, id catalog.NodeID) uuid.UUID {
	t.Helper()

	var rowID uuid.UUID
	err := e.pool.QueryRow(t.Context(),
		`SELECT id FROM nodes WHERE kind = $1 AND path = $2`, id.Kind, id.Path,
	).Scan(&rowID)
	require.NoError(t, err)
	return rowID
}

func pendingOperation(name, plugin string, createdAt time.Time) operations.Operation {
	return operations.Operation{
		Name:      name,
		Plugin:    plugin,
		State:     operations.StatePending,
		CreatedAt: createdAt,
	}
}

func nodeID(kind, path string) catalog.NodeID {
	return catalog.NodeID{
		Kind: kind,
		Path: path,
	}
}

func nodeClaim(id catalog.NodeID, properties map[string]string) catalog.NodeClaim {
	return catalog.NodeClaim{
		ID:         id,
		Properties: properties,
	}
}

func relationClaim(kind string, from, to catalog.NodeID, properties map[string]string) catalog.RelationClaim {
	return catalog.RelationClaim{
		Kind:       kind,
		From:       from,
		To:         to,
		Properties: properties,
	}
}

func pluginClaim(snapshotID uuid.UUID, properties map[string]string) catalog.PluginClaim {
	return catalog.PluginClaim{
		SnapshotID: snapshotID,
		Properties: properties,
	}
}
