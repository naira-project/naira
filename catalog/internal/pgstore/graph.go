package pgstore

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/naira-project/naira/catalog/internal/catalog"
)

func (s *GraphStore) ListNodes(ctx context.Context) ([]catalog.Node, error) {
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

	byID := make(map[catalog.NodeID]*catalog.Node)
	var order []catalog.NodeID

	var id catalog.NodeID
	var pluginName *string
	var snapshotID *uuid.UUID
	var propsRaw []byte

	_, err = pgx.ForEachRow(rows, []any{&id.Kind, &id.Path, &pluginName, &snapshotID, &propsRaw}, func() error {
		node, ok := byID[id]
		if !ok {
			node = &catalog.Node{ID: id, PluginClaims: make(map[string]catalog.PluginClaim)}
			byID[id] = node
			order = append(order, id)
		}

		if pluginName != nil {
			props, err := decodeProperties(propsRaw)
			if err != nil {
				return fmt.Errorf("decoding properties for node %q/%q plugin %q: %w", id.Kind, id.Path, *pluginName, err)
			}
			node.PluginClaims[*pluginName] = catalog.PluginClaim{
				SnapshotID: *snapshotID,
				Properties: props,
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading nodes: %w", err)
	}

	result := make([]catalog.Node, 0, len(order))
	for _, id := range order {
		result = append(result, *byID[id])
	}
	return result, nil
}

func (s *GraphStore) GetNode(ctx context.Context, id catalog.NodeID) (catalog.Node, error) {
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

	node := catalog.Node{ID: id, PluginClaims: make(map[string]catalog.PluginClaim)}
	found := false

	var pluginName *string
	var snapshotID *uuid.UUID
	var propsRaw []byte

	_, err = pgx.ForEachRow(rows, []any{&pluginName, &snapshotID, &propsRaw}, func() error {
		found = true
		if pluginName != nil {
			props, err := decodeProperties(propsRaw)
			if err != nil {
				return fmt.Errorf("decoding properties for node %q/%q plugin %q: %w", id.Kind, id.Path, *pluginName, err)
			}
			node.PluginClaims[*pluginName] = catalog.PluginClaim{
				SnapshotID: *snapshotID,
				Properties: props,
			}
		}
		return nil
	})
	if err != nil {
		return catalog.Node{}, fmt.Errorf("reading node %q/%q: %w", id.Kind, id.Path, err)
	}
	if !found {
		return catalog.Node{}, fmt.Errorf("node %q/%q: %w", id.Kind, id.Path, catalog.ErrNodeNotFound)
	}

	return node, nil
}

func (s *GraphStore) ListRelations(ctx context.Context) ([]catalog.Relation, error) {
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

	byID := make(map[catalog.RelationID]*catalog.Relation)
	var order []catalog.RelationID

	var id catalog.RelationID
	var pluginName *string
	var snapshotID *uuid.UUID
	var propsRaw []byte

	_, err = pgx.ForEachRow(rows, []any{
		&id.Kind, &id.From.Kind, &id.From.Path, &id.To.Kind, &id.To.Path,
		&pluginName, &snapshotID, &propsRaw,
	}, func() error {
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
				return fmt.Errorf("decoding properties for relation %q plugin %q: %w", id.Kind, *pluginName, err)
			}
			relation.PluginClaims[*pluginName] = catalog.PluginClaim{
				SnapshotID: *snapshotID,
				Properties: props,
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading relations: %w", err)
	}

	result := make([]catalog.Relation, 0, len(order))
	for _, id := range order {
		result = append(result, *byID[id])
	}
	return result, nil
}
