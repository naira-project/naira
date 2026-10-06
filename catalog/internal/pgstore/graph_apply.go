package pgstore

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/naira-project/naira/catalog/internal/catalog"
)

func applySnapshot(
	ctx context.Context,
	tx pgx.Tx,
	pluginName string,
	snapshotID uuid.UUID,
	nodes []catalog.NodeClaim,
	relations []catalog.RelationClaim,
) (int, int, error) {
	upsertedNodes := 0
	for _, node := range nodes {
		props, err := encodeProperties(node.Properties)
		if err != nil {
			return 0, 0, fmt.Errorf("encoding properties for node %q/%q: %w", node.ID.Kind, node.ID.Path, err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO nodes (kind, path) VALUES ($1, $2)
			ON CONFLICT (kind, path) DO NOTHING
		`, node.ID.Kind, node.ID.Path); err != nil {
			return 0, 0, fmt.Errorf("upserting node %q/%q: %w", node.ID.Kind, node.ID.Path, err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO node_plugin_claims (node_kind, node_path, plugin_name, snapshot_id, properties)
			VALUES ($1, $2, $3, $4, $5::jsonb)
			ON CONFLICT (node_kind, node_path, plugin_name)
			DO UPDATE SET snapshot_id = EXCLUDED.snapshot_id, properties = EXCLUDED.properties
		`, node.ID.Kind, node.ID.Path, pluginName, snapshotID, props); err != nil {
			return 0, 0, fmt.Errorf("upserting claim for node %q/%q: %w", node.ID.Kind, node.ID.Path, err)
		}
		upsertedNodes++
	}

	upsertedRelations := 0
	for _, relation := range relations {
		props, err := encodeProperties(relation.Properties)
		if err != nil {
			return 0, 0, fmt.Errorf("encoding properties for relation %q %q/%q -> %q/%q: %w", relation.Kind, relation.From.Kind, relation.From.Path, relation.To.Kind, relation.To.Path, err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO relations (kind, from_kind, from_path, to_kind, to_path)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (kind, from_kind, from_path, to_kind, to_path) DO NOTHING
		`, relation.Kind, relation.From.Kind, relation.From.Path, relation.To.Kind, relation.To.Path); err != nil {
			return 0, 0, fmt.Errorf("upserting relation %q %q/%q -> %q/%q: %w", relation.Kind, relation.From.Kind, relation.From.Path, relation.To.Kind, relation.To.Path, err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO relation_plugin_claims (relation_kind, from_kind, from_path, to_kind, to_path, plugin_name, snapshot_id, properties)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)
			ON CONFLICT (relation_kind, from_kind, from_path, to_kind, to_path, plugin_name)
			DO UPDATE SET snapshot_id = EXCLUDED.snapshot_id, properties = EXCLUDED.properties
		`, relation.Kind, relation.From.Kind, relation.From.Path, relation.To.Kind, relation.To.Path, pluginName, snapshotID, props); err != nil {
			return 0, 0, fmt.Errorf("upserting claim for relation %q %q/%q -> %q/%q: %w", relation.Kind, relation.From.Kind, relation.From.Path, relation.To.Kind, relation.To.Path, err)
		}
		upsertedRelations++
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM node_plugin_claims WHERE plugin_name = $1 AND snapshot_id <> $2
	`, pluginName, snapshotID); err != nil {
		return 0, 0, fmt.Errorf("pruning stale node claims for plugin %q: %w", pluginName, err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM relation_plugin_claims WHERE plugin_name = $1 AND snapshot_id <> $2
	`, pluginName, snapshotID); err != nil {
		return 0, 0, fmt.Errorf("pruning stale relation claims for plugin %q: %w", pluginName, err)
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM nodes n
		WHERE NOT EXISTS (
			SELECT 1 FROM node_plugin_claims c
			WHERE c.node_kind = n.kind AND c.node_path = n.path
		)
	`); err != nil {
		return 0, 0, fmt.Errorf("pruning orphaned nodes: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM relations r
		WHERE NOT EXISTS (
			SELECT 1 FROM relation_plugin_claims c
			WHERE c.relation_kind = r.kind
			  AND c.from_kind = r.from_kind AND c.from_path = r.from_path
			  AND c.to_kind = r.to_kind AND c.to_path = r.to_path
		)
	`); err != nil {
		return 0, 0, fmt.Errorf("pruning orphaned relations: %w", err)
	}

	return upsertedNodes, upsertedRelations, nil
}
