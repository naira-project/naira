package pgstore

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/naira-project/naira/catalog/internal/catalog"
)

// ListKinds returns the distinct node kinds in sorted order.
func (s *GraphStore) ListKinds(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT kind
		FROM nodes
		ORDER BY kind
	`)
	if err != nil {
		return nil, fmt.Errorf("querying node kinds: %w", err)
	}

	kinds, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("reading node kinds: %w", err)
	}

	return kinds, nil
}

// ListNodes returns all nodes with their plugin claims.
func (s *GraphStore) ListNodes(ctx context.Context) ([]catalog.Node, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			n.kind, n.path, c.plugin_name, c.snapshot_id, c.properties
		FROM nodes n
		LEFT JOIN node_plugin_claims c
			ON c.node_id = n.id
		ORDER BY n.kind, n.path
	`)
	if err != nil {
		return nil, fmt.Errorf("querying nodes: %w", err)
	}

	result := make([]catalog.Node, 0)

	var id catalog.NodeID
	var pluginName *string
	var snapshotID *uuid.UUID
	var propsRaw []byte

	// Rows are ordered by the full node key, so one node's rows are adjacent.
	_, err = pgx.ForEachRow(rows, []any{&id.Kind, &id.Path, &pluginName, &snapshotID, &propsRaw}, func() error {
		if len(result) == 0 || result[len(result)-1].ID != id {
			result = append(result, catalog.Node{ID: id, PluginClaims: make(map[string]catalog.PluginClaim)})
		}
		node := &result[len(result)-1]

		if err := addClaim(node.PluginClaims, pluginName, snapshotID, propsRaw); err != nil {
			return fmt.Errorf("node %q/%q: %w", id.Kind, id.Path, err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading nodes: %w", err)
	}

	return result, nil
}

// GetNode returns the node identified by id, or an error if it does not exist.
func (s *GraphStore) GetNode(ctx context.Context, id catalog.NodeID) (catalog.Node, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			c.plugin_name, c.snapshot_id, c.properties
		FROM nodes n
		LEFT JOIN node_plugin_claims c
			ON c.node_id = n.id
		WHERE n.kind = $1
		  AND n.path = $2
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
		return addClaim(node.PluginClaims, pluginName, snapshotID, propsRaw)
	})
	if err != nil {
		return catalog.Node{}, fmt.Errorf("reading node %q/%q: %w", id.Kind, id.Path, err)
	}
	if !found {
		return catalog.Node{}, fmt.Errorf("node %q/%q: %w", id.Kind, id.Path, catalog.ErrNodeNotFound)
	}

	return node, nil
}

// ListRelations returns all relations with their plugin claims.
func (s *GraphStore) ListRelations(ctx context.Context) ([]catalog.Relation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			r.kind, f.kind, f.path, t.kind, t.path,
			c.plugin_name, c.snapshot_id, c.properties
		FROM relations r
		JOIN nodes f
			ON f.id = r.from_node_id
		JOIN nodes t
			ON t.id = r.to_node_id
		LEFT JOIN relation_plugin_claims c
			ON c.relation_id = r.id
		ORDER BY r.kind, f.kind, f.path, t.kind, t.path
	`)
	if err != nil {
		return nil, fmt.Errorf("querying relations: %w", err)
	}

	result := make([]catalog.Relation, 0)

	var id catalog.RelationID
	var pluginName *string
	var snapshotID *uuid.UUID
	var propsRaw []byte

	// Rows are ordered by the full relation key, so one relation's rows are adjacent.
	_, err = pgx.ForEachRow(rows, []any{
		&id.Kind, &id.From.Kind, &id.From.Path, &id.To.Kind, &id.To.Path,
		&pluginName, &snapshotID, &propsRaw,
	}, func() error {
		if n := len(result); n == 0 || result[n-1].Kind != id.Kind || result[n-1].From != id.From || result[n-1].To != id.To {
			result = append(result, catalog.Relation{
				Kind:         id.Kind,
				From:         id.From,
				To:           id.To,
				PluginClaims: make(map[string]catalog.PluginClaim),
			})
		}
		relation := &result[len(result)-1]

		if err := addClaim(relation.PluginClaims, pluginName, snapshotID, propsRaw); err != nil {
			return fmt.Errorf("relation %q: %w", id.Kind, err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading relations: %w", err)
	}

	return result, nil
}

// addClaim decodes raw and stores it in claims under name. A nil name
// (LEFT JOIN without a claim) is a no-op.
func addClaim(claims map[string]catalog.PluginClaim, name *string, snap *uuid.UUID, raw []byte) error {
	if name == nil {
		return nil
	}
	props, err := decodeProperties(raw)
	if err != nil {
		return fmt.Errorf("decoding properties for plugin %q: %w", *name, err)
	}
	claims[*name] = catalog.PluginClaim{
		SnapshotID: *snap,
		Properties: props,
	}
	return nil
}
