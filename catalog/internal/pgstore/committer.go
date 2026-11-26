package pgstore

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/operations"
)

// snapshotLockKey is the advisory lock key ("catalog") held while applying a
// snapshot. The lock is global: snapshots of all plugins are serialized, which
// trades throughput for race-free orphan cleanup.
const snapshotLockKey int64 = 0x636174616c6f67

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

// applySnapshot upserts pluginName's claims for nodes/relations, prunes
// everything previously claimed by pluginName that isn't part of this
// snapshot, and finally removes nodes/relations left with no claims from any
// plugin.
func applySnapshot(
	ctx context.Context,
	tx pgx.Tx,
	pluginName string,
	snapshotID uuid.UUID,
	nodes []catalog.NodeClaim,
	relations []catalog.RelationClaim,
) (SnapshotResult, error) {
	if _, err := tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock($1)
	`, snapshotLockKey); err != nil {
		return SnapshotResult{}, fmt.Errorf("acquiring snapshot lock: %w", err)
	}

	if err := upsertNodes(ctx, tx, pluginName, snapshotID, nodes); err != nil {
		return SnapshotResult{}, fmt.Errorf("upserting snapshot nodes: %w", err)
	}

	if err := upsertRelations(ctx, tx, pluginName, snapshotID, relations); err != nil {
		return SnapshotResult{}, fmt.Errorf("upserting snapshot relations: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM relation_plugin_claims
		WHERE plugin_name = $1
		  AND snapshot_id <> $2
	`, pluginName, snapshotID); err != nil {
		return SnapshotResult{}, fmt.Errorf("pruning stale relation claims for plugin %q: %w", pluginName, err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM node_plugin_claims
		WHERE plugin_name = $1
		  AND snapshot_id <> $2
	`, pluginName, snapshotID); err != nil {
		return SnapshotResult{}, fmt.Errorf("pruning stale node claims for plugin %q: %w", pluginName, err)
	}

	// Relations must go before nodes: they reference nodes.
	if _, err := tx.Exec(ctx, `
		DELETE FROM relations r
		WHERE NOT EXISTS (
			SELECT 1
			FROM relation_plugin_claims c
			WHERE c.relation_id = r.id
		)
	`); err != nil {
		return SnapshotResult{}, fmt.Errorf("pruning orphaned relations: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM nodes n
		WHERE NOT EXISTS (
			SELECT 1
			FROM node_plugin_claims c
			WHERE c.node_id = n.id
		)
	`); err != nil {
		return SnapshotResult{}, fmt.Errorf("pruning orphaned nodes: %w", err)
	}

	return SnapshotResult{
		NodesUpserted:     len(nodes),
		RelationsUpserted: len(relations),
	}, nil
}

// upsertNodes makes sure every node exists (new rows get a fresh UUIDv7),
// then writes pluginName's claim for each. nodes must be validated with
// [catalog.ValidateSnapshotInput] first (no duplicate IDs).
func upsertNodes(
	ctx context.Context,
	tx pgx.Tx,
	pluginName string,
	snapshotID uuid.UUID,
	nodes []catalog.NodeClaim,
) error {
	if len(nodes) == 0 {
		return nil
	}

	newIDs := make([]uuid.UUID, len(nodes))
	kinds := make([]string, len(nodes))
	paths := make([]string, len(nodes))
	props := make([]string, len(nodes))
	for i, n := range nodes {
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("generating id for node %q/%q: %w", n.ID.Kind, n.ID.Path, err)
		}
		raw, err := encodeProperties(n.Properties)
		if err != nil {
			return fmt.Errorf("encoding properties for node %q/%q: %w", n.ID.Kind, n.ID.Path, err)
		}
		newIDs[i], kinds[i], paths[i], props[i] = id, n.ID.Kind, n.ID.Path, string(raw)
	}

	// Existing nodes keep their id; generated IDs are only used for inserts.
	if _, err := tx.Exec(ctx, `
		INSERT INTO nodes (id, kind, path)
		SELECT t.id, t.kind, t.path
		FROM unnest($1::uuid[], $2::text[], $3::text[]) AS t(id, kind, path)
		ON CONFLICT (kind, path)
		DO NOTHING
	`, newIDs, kinds, paths); err != nil {
		return fmt.Errorf("upserting nodes: %w", err)
	}

	tag, err := tx.Exec(ctx, `
		INSERT INTO node_plugin_claims (
			node_id, plugin_name, snapshot_id, properties
		)
		SELECT n.id, $4::text, $5::uuid, t.props::jsonb
		FROM unnest($1::text[], $2::text[], $3::text[]) AS t(kind, path, props)
		JOIN nodes n
		  ON n.kind = t.kind
		 AND n.path = t.path
		ON CONFLICT (node_id, plugin_name)
		DO UPDATE SET
			snapshot_id = EXCLUDED.snapshot_id,
			properties = EXCLUDED.properties
	`, kinds, paths, props, pluginName, snapshotID)
	if err != nil {
		return fmt.Errorf("upserting node claims: %w", err)
	}
	if got := tag.RowsAffected(); got != int64(len(nodes)) {
		return fmt.Errorf("upserted %d node claims, want %d", got, len(nodes))
	}

	return nil
}

// upsertRelations is the relation counterpart of [upsertNodes]. Both
// endpoints of every relation must already be upserted, and relations must
// not contain duplicate IDs.
func upsertRelations(
	ctx context.Context,
	tx pgx.Tx,
	pluginName string,
	snapshotID uuid.UUID,
	relations []catalog.RelationClaim,
) error {
	if len(relations) == 0 {
		return nil
	}

	newIDs := make([]uuid.UUID, len(relations))
	kinds := make([]string, len(relations))
	fromKinds := make([]string, len(relations))
	fromPaths := make([]string, len(relations))
	toKinds := make([]string, len(relations))
	toPaths := make([]string, len(relations))
	props := make([]string, len(relations))
	for i, r := range relations {
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("generating id for relation %q: %w", r.Kind, err)
		}
		raw, err := encodeProperties(r.Properties)
		if err != nil {
			return fmt.Errorf("encoding properties for relation %q: %w", r.Kind, err)
		}
		newIDs[i], kinds[i] = id, r.Kind
		fromKinds[i], fromPaths[i] = r.From.Kind, r.From.Path
		toKinds[i], toPaths[i] = r.To.Kind, r.To.Path
		props[i] = string(raw)
	}

	// Existing relations keep their id; generated IDs are only used for inserts.
	if _, err := tx.Exec(ctx, `
		INSERT INTO relations (id, kind, from_node_id, to_node_id)
		SELECT t.id, t.kind, f.id, tn.id
		FROM unnest(
			$1::uuid[], $2::text[], $3::text[], $4::text[], $5::text[], $6::text[]
		) AS t(id, kind, from_kind, from_path, to_kind, to_path)
		JOIN nodes f
		  ON f.kind = t.from_kind
		 AND f.path = t.from_path
		JOIN nodes tn
		  ON tn.kind = t.to_kind
		 AND tn.path = t.to_path
		ON CONFLICT (kind, from_node_id, to_node_id)
		DO NOTHING
	`, newIDs, kinds, fromKinds, fromPaths, toKinds, toPaths); err != nil {
		return fmt.Errorf("upserting relations: %w", err)
	}

	tag, err := tx.Exec(ctx, `
		INSERT INTO relation_plugin_claims (
			relation_id, plugin_name, snapshot_id, properties
		)
		SELECT r.id, $7::text, $8::uuid, t.props::jsonb
		FROM unnest(
			$1::text[], $2::text[], $3::text[], $4::text[], $5::text[], $6::text[]
		) AS t(kind, from_kind, from_path, to_kind, to_path, props)
		JOIN nodes f
		  ON f.kind = t.from_kind
		 AND f.path = t.from_path
		JOIN nodes tn
		  ON tn.kind = t.to_kind
		 AND tn.path = t.to_path
		JOIN relations r
		  ON r.kind = t.kind
		 AND r.from_node_id = f.id
		 AND r.to_node_id = tn.id
		ON CONFLICT (relation_id, plugin_name)
		DO UPDATE SET
			snapshot_id = EXCLUDED.snapshot_id,
			properties = EXCLUDED.properties
	`, kinds, fromKinds, fromPaths, toKinds, toPaths, props, pluginName, snapshotID)
	if err != nil {
		return fmt.Errorf("upserting relation claims: %w", err)
	}
	if got := tag.RowsAffected(); got != int64(len(relations)) {
		return fmt.Errorf("upserted %d relation claims, want %d", got, len(relations))
	}

	return nil
}
