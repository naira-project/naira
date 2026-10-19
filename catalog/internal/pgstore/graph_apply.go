package pgstore

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/naira-project/naira/catalog/internal/catalog"
)

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
) (int, int, error) {
	// Lock serialize applySnapshot calls so orphan cleanup cannot race with another
	// plugin claiming the same node.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('catalog.apply_snapshot'))`); err != nil {
		return 0, 0, fmt.Errorf("acquiring snapshot lock: %w", err)
	}

	nodes = dedupeNodeClaims(nodes)
	relations = dedupeRelationClaims(relations)

	nodeIDs, err := upsertNodes(ctx, tx, pluginName, snapshotID, nodes)
	if err != nil {
		return 0, 0, err
	}

	if err := upsertRelations(ctx, tx, pluginName, snapshotID, relations, nodeIDs); err != nil {
		return 0, 0, err
	}

	// Order matters: relations reference nodes, and claims reference both.
	statements := []struct{ what, sql string }{
		{"stale relation claims", `DELETE FROM relation_plugin_claims WHERE plugin_name = $1 AND snapshot_id <> $2`},
		{"stale node claims", `DELETE FROM node_plugin_claims WHERE plugin_name = $1 AND snapshot_id <> $2`},
	}
	for _, st := range statements {
		if _, err := tx.Exec(ctx, st.sql, pluginName, snapshotID); err != nil {
			return 0, 0, fmt.Errorf("pruning %s for plugin %q: %w", st.what, pluginName, err)
		}
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM relations r
		WHERE NOT EXISTS (SELECT 1 FROM relation_plugin_claims c WHERE c.relation_id = r.id)
	`); err != nil {
		return 0, 0, fmt.Errorf("pruning orphaned relations: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM nodes n
		WHERE NOT EXISTS (SELECT 1 FROM node_plugin_claims c WHERE c.node_id = n.id)
	`); err != nil {
		return 0, 0, fmt.Errorf("pruning orphaned nodes: %w", err)
	}

	return len(nodes), len(relations), nil
}

// upsertNodes makes sure every node exists (new rows get a fresh UUIDv7),
// then writes pluginName's claim for each. It returns the id of every node
// in the snapshot.
func upsertNodes(
	ctx context.Context,
	tx pgx.Tx,
	pluginName string,
	snapshotID uuid.UUID,
	nodes []catalog.NodeClaim,
) (map[catalog.NodeID]uuid.UUID, error) {
	ids := make(map[catalog.NodeID]uuid.UUID, len(nodes))
	if len(nodes) == 0 {
		return ids, nil
	}

	newIDs := make([]string, len(nodes))
	kinds := make([]string, len(nodes))
	paths := make([]string, len(nodes))
	props := make([]string, len(nodes))
	for i, n := range nodes {
		id, err := uuid.NewV7()
		if err != nil {
			return nil, fmt.Errorf("generating id for node %q/%q: %w", n.ID.Kind, n.ID.Path, err)
		}
		raw, err := encodeProperties(n.Properties)
		if err != nil {
			return nil, fmt.Errorf("encoding properties for node %q/%q: %w", n.ID.Kind, n.ID.Path, err)
		}
		newIDs[i], kinds[i], paths[i], props[i] = id.String(), n.ID.Kind, n.ID.Path, string(raw)
	}

	// Existing nodes keep their id; the freshly generated one is discarded.
	if _, err := tx.Exec(ctx, `
		INSERT INTO nodes (id, kind, path)
		SELECT * FROM unnest($1::uuid[], $2::text[], $3::text[])
		ON CONFLICT (kind, path) DO NOTHING
	`, newIDs, kinds, paths); err != nil {
		return nil, fmt.Errorf("upserting nodes: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT n.id, n.kind, n.path
		FROM nodes n
		JOIN unnest($1::text[], $2::text[]) AS t(kind, path)
		  ON n.kind = t.kind AND n.path = t.path
	`, kinds, paths)
	if err != nil {
		return nil, fmt.Errorf("resolving node ids: %w", err)
	}
	for rows.Next() {
		var id uuid.UUID
		var nodeID catalog.NodeID
		if err := rows.Scan(&id, &nodeID.Kind, &nodeID.Path); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scanning node id: %w", err)
		}
		ids[nodeID] = id
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("resolving node ids: %w", err)
	}

	claimNodeIDs := make([]string, len(nodes))
	for i, n := range nodes {
		id, ok := ids[n.ID]
		if !ok {
			return nil, fmt.Errorf("node %q/%q missing after upsert", n.ID.Kind, n.ID.Path)
		}
		claimNodeIDs[i] = id.String()
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO node_plugin_claims (node_id, plugin_name, snapshot_id, properties)
		SELECT t.node_id, $2, $3, t.props::jsonb
		FROM unnest($1::uuid[], $4::text[]) AS t(node_id, props)
		ON CONFLICT (node_id, plugin_name)
		DO UPDATE SET snapshot_id = EXCLUDED.snapshot_id, properties = EXCLUDED.properties
	`, claimNodeIDs, pluginName, snapshotID, props); err != nil {
		return nil, fmt.Errorf("upserting node claims: %w", err)
	}

	return ids, nil
}

// upsertRelations is the relation counterpart of [upsertNodes].
func upsertRelations(
	ctx context.Context,
	tx pgx.Tx,
	pluginName string,
	snapshotID uuid.UUID,
	relations []catalog.RelationClaim,
	nodeIDs map[catalog.NodeID]uuid.UUID,
) error {
	if len(relations) == 0 {
		return nil
	}

	newIDs := make([]string, len(relations))
	kinds := make([]string, len(relations))
	fromIDs := make([]string, len(relations))
	toIDs := make([]string, len(relations))
	props := make([]string, len(relations))
	for i, r := range relations {
		from, okFrom := nodeIDs[r.From]
		to, okTo := nodeIDs[r.To]
		if !okFrom || !okTo {
			return fmt.Errorf("relation %q %q/%q -> %q/%q references a node outside the snapshot",
				r.Kind, r.From.Kind, r.From.Path, r.To.Kind, r.To.Path)
		}
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("generating id for relation %q: %w", r.Kind, err)
		}
		raw, err := encodeProperties(r.Properties)
		if err != nil {
			return fmt.Errorf("encoding properties for relation %q: %w", r.Kind, err)
		}
		newIDs[i], kinds[i], fromIDs[i], toIDs[i], props[i] = id.String(), r.Kind, from.String(), to.String(), string(raw)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO relations (id, kind, from_node_id, to_node_id)
		SELECT * FROM unnest($1::uuid[], $2::text[], $3::uuid[], $4::uuid[])
		ON CONFLICT (kind, from_node_id, to_node_id) DO NOTHING
	`, newIDs, kinds, fromIDs, toIDs); err != nil {
		return fmt.Errorf("upserting relations: %w", err)
	}

	type relationKey struct {
		kind     string
		from, to uuid.UUID
	}
	resolved := make(map[relationKey]uuid.UUID, len(relations))

	rows, err := tx.Query(ctx, `
		SELECT r.id, r.kind, r.from_node_id, r.to_node_id
		FROM relations r
		JOIN unnest($1::text[], $2::uuid[], $3::uuid[]) AS t(kind, from_id, to_id)
		  ON r.kind = t.kind AND r.from_node_id = t.from_id AND r.to_node_id = t.to_id
	`, kinds, fromIDs, toIDs)
	if err != nil {
		return fmt.Errorf("resolving relation ids: %w", err)
	}
	for rows.Next() {
		var id uuid.UUID
		var key relationKey
		if err := rows.Scan(&id, &key.kind, &key.from, &key.to); err != nil {
			rows.Close()
			return fmt.Errorf("scanning relation id: %w", err)
		}
		resolved[key] = id
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("resolving relation ids: %w", err)
	}

	claimRelationIDs := make([]string, len(relations))
	for i, r := range relations {
		key := relationKey{kind: r.Kind, from: nodeIDs[r.From], to: nodeIDs[r.To]}
		id, ok := resolved[key]
		if !ok {
			return fmt.Errorf("relation %q missing after upsert", r.Kind)
		}
		claimRelationIDs[i] = id.String()
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO relation_plugin_claims (relation_id, plugin_name, snapshot_id, properties)
		SELECT t.relation_id, $2, $3, t.props::jsonb
		FROM unnest($1::uuid[], $4::text[]) AS t(relation_id, props)
		ON CONFLICT (relation_id, plugin_name)
		DO UPDATE SET snapshot_id = EXCLUDED.snapshot_id, properties = EXCLUDED.properties
	`, claimRelationIDs, pluginName, snapshotID, props); err != nil {
		return fmt.Errorf("upserting relation claims: %w", err)
	}

	return nil
}

// dedupeNodeClaims collapses repeated node IDs within one snapshot, last
// claim wins.
func dedupeNodeClaims(nodes []catalog.NodeClaim) []catalog.NodeClaim {
	index := make(map[catalog.NodeID]int, len(nodes))
	out := make([]catalog.NodeClaim, 0, len(nodes))
	for _, n := range nodes {
		if i, ok := index[n.ID]; ok {
			out[i] = n
			continue
		}
		index[n.ID] = len(out)
		out = append(out, n)
	}
	return out
}

func dedupeRelationClaims(relations []catalog.RelationClaim) []catalog.RelationClaim {
	index := make(map[catalog.RelationID]int, len(relations))
	out := make([]catalog.RelationClaim, 0, len(relations))
	for _, r := range relations {
		key := catalog.RelationID{Kind: r.Kind, From: r.From, To: r.To}
		if i, ok := index[key]; ok {
			out[i] = r
			continue
		}
		index[key] = len(out)
		out = append(out, r)
	}
	return out
}
