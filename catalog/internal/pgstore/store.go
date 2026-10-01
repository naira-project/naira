package pgstore

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/naira-project/naira/catalog/internal/catalog"
)

// Store implements catalog.Store on top of PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) ListNodes() ([]catalog.Node, error) {
	ctx := context.Background()

	rows, err := s.pool.Query(ctx, `
		SELECT n.kind, n.path, c.plugin_name, c.snapshot_id, c.properties
		FROM nodes n
		LEFT JOIN node_plugin_claims c
		  ON c.node_kind = n.kind AND c.node_path = n.path
		ORDER BY n.kind, n.path
	`)
	if err != nil {
		return nil, fmt.Errorf("querying nodes: %w", err)
	}
	defer rows.Close()

	byID := make(map[catalog.NodeID]*catalog.Node)
	var order []catalog.NodeID

	for rows.Next() {
		var id catalog.NodeID
		var pluginName *string
		var snapshotID *uuid.UUID
		var propsRaw []byte

		if err := rows.Scan(&id.Kind, &id.Path, &pluginName, &snapshotID, &propsRaw); err != nil {
			return nil, fmt.Errorf("scanning node row: %w", err)
		}

		node, ok := byID[id]
		if !ok {
			node = &catalog.Node{ID: id, PluginClaims: make(map[string]catalog.PluginClaim)}
			byID[id] = node
			order = append(order, id)
		}

		if pluginName != nil {
			props, err := decodeProperties(propsRaw)
			if err != nil {
				return nil, fmt.Errorf("decoding properties for node %q/%q plugin %q: %w", id.Kind, id.Path, *pluginName, err)
			}
			node.PluginClaims[*pluginName] = catalog.PluginClaim{
				SnapshotID: *snapshotID,
				Properties: props,
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading nodes: %w", err)
	}

	result := make([]catalog.Node, 0, len(order))
	for _, id := range order {
		result = append(result, *byID[id])
	}
	return result, nil
}

func (s *Store) GetNode(id catalog.NodeID) (catalog.Node, error) {
	ctx := context.Background()

	rows, err := s.pool.Query(ctx, `
		SELECT c.plugin_name, c.snapshot_id, c.properties
		FROM nodes n
		LEFT JOIN node_plugin_claims c
		  ON c.node_kind = n.kind AND c.node_path = n.path
		WHERE n.kind = $1 AND n.path = $2
	`, id.Kind, id.Path)
	if err != nil {
		return catalog.Node{}, fmt.Errorf("querying node %q/%q: %w", id.Kind, id.Path, err)
	}
	defer rows.Close()

	node := catalog.Node{ID: id, PluginClaims: make(map[string]catalog.PluginClaim)}
	found := false

	for rows.Next() {
		found = true
		var pluginName *string
		var snapshotID *uuid.UUID
		var propsRaw []byte

		if err := rows.Scan(&pluginName, &snapshotID, &propsRaw); err != nil {
			return catalog.Node{}, fmt.Errorf("scanning node %q/%q: %w", id.Kind, id.Path, err)
		}
		if pluginName != nil {
			props, err := decodeProperties(propsRaw)
			if err != nil {
				return catalog.Node{}, fmt.Errorf("decoding properties for node %q/%q plugin %q: %w", id.Kind, id.Path, *pluginName, err)
			}
			node.PluginClaims[*pluginName] = catalog.PluginClaim{
				SnapshotID: *snapshotID,
				Properties: props,
			}
		}
	}
	if err := rows.Err(); err != nil {
		return catalog.Node{}, fmt.Errorf("reading node %q/%q: %w", id.Kind, id.Path, err)
	}
	if !found {
		return catalog.Node{}, fmt.Errorf("node %q/%q: %w", id.Kind, id.Path, catalog.ErrNodeNotFound)
	}

	return node, nil
}

func (s *Store) ListRelations() ([]catalog.Relation, error) {
	ctx := context.Background()

	rows, err := s.pool.Query(ctx, `
		SELECT r.kind, r.from_kind, r.from_path, r.to_kind, r.to_path,
		       c.plugin_name, c.snapshot_id, c.properties
		FROM relations r
		LEFT JOIN relation_plugin_claims c
		  ON c.relation_kind = r.kind
		 AND c.from_kind = r.from_kind
		 AND c.from_path = r.from_path
		 AND c.to_kind = r.to_kind
		 AND c.to_path = r.to_path
		ORDER BY r.kind, r.from_kind, r.from_path, r.to_kind, r.to_path
	`)
	if err != nil {
		return nil, fmt.Errorf("querying relations: %w", err)
	}
	defer rows.Close()

	byID := make(map[catalog.RelationID]*catalog.Relation)
	var order []catalog.RelationID

	for rows.Next() {
		var id catalog.RelationID
		var pluginName *string
		var snapshotID *uuid.UUID
		var propsRaw []byte

		if err := rows.Scan(
			&id.Kind, &id.From.Kind, &id.From.Path, &id.To.Kind, &id.To.Path,
			&pluginName, &snapshotID, &propsRaw,
		); err != nil {
			return nil, fmt.Errorf("scanning relation row: %w", err)
		}

		relation, ok := byID[id]
		if !ok {
			relation = &catalog.Relation{
				Kind:         id.Kind,
				From:         id.From,
				To:           id.To,
				PluginClaims: make(map[string]catalog.PluginClaim),
			}
			byID[id] = relation
			order = append(order, id)
		}

		if pluginName != nil {
			props, err := decodeProperties(propsRaw)
			if err != nil {
				return nil, fmt.Errorf("decoding properties for relation %q plugin %q: %w", id.Kind, *pluginName, err)
			}
			relation.PluginClaims[*pluginName] = catalog.PluginClaim{
				SnapshotID: *snapshotID,
				Properties: props,
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading relations: %w", err)
	}

	result := make([]catalog.Relation, 0, len(order))
	for _, id := range order {
		result = append(result, *byID[id])
	}
	return result, nil
}

func (s *Store) ApplyPluginSnapshot(
	pluginName string,
	snapshotID uuid.UUID,
	nodes []catalog.NodeClaim,
	relations []catalog.RelationClaim,
) (int, int, error) {
	ctx := context.Background()

	if err := catalog.ValidateSnapshotInput(pluginName, snapshotID, nodes, relations); err != nil {
		return 0, 0, fmt.Errorf("validate snapshot input: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	upsertedNodes, upsertedRelations, err := applySnapshot(ctx, tx, pluginName, snapshotID, nodes, relations)
	if err != nil {
		return 0, 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, fmt.Errorf("committing transaction: %w", err)
	}
	return upsertedNodes, upsertedRelations, nil
}

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

func encodeProperties(properties map[string]string) ([]byte, error) {
	if properties == nil {
		properties = map[string]string{}
	}
	return json.Marshal(properties)
}

func decodeProperties(raw []byte) (map[string]string, error) {
	if len(raw) == 0 {
		return map[string]string{}, nil
	}
	var properties map[string]string
	if err := json.Unmarshal(raw, &properties); err != nil {
		return nil, err
	}
	return properties, nil
}
