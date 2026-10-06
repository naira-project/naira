package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrInvalidIngestion = errors.New("invalid ingestion payload")
	ErrNodeNotFound     = errors.New("node not found")
)

type Store interface {
	ListNodes(ctx context.Context) ([]Node, error)
	GetNode(ctx context.Context, id NodeID) (Node, error)
	ListRelations(ctx context.Context) ([]Relation, error)
}

type PluginClaim struct {
	SnapshotID uuid.UUID
	Properties map[string]string
}

type Node struct {
	ID           NodeID
	PluginClaims map[string]PluginClaim
}

type Relation struct {
	Kind         string
	From         NodeID
	To           NodeID
	PluginClaims map[string]PluginClaim
}

type RelationID struct {
	Kind string
	From NodeID
	To   NodeID
}

func ValidateSnapshotInput(pluginName string, snapshotID uuid.UUID, nodes []NodeClaim, relations []RelationClaim) error {
	if pluginName == "" {
		return fmt.Errorf("%w: plugin name is empty", ErrInvalidIngestion)
	}

	if snapshotID == uuid.Nil {
		return fmt.Errorf("%w: snapshot ID is empty", ErrInvalidIngestion)
	}

	availableNodes := make(map[NodeID]struct{}, len(nodes))

	for _, node := range nodes {
		id := node.ID
		if id.Kind == "" || id.Path == "" {
			return fmt.Errorf("%w: node ID with empty kind or path", ErrInvalidIngestion)
		}
		if strings.Contains(id.Kind, "/") {
			return fmt.Errorf("%w: node kind %q contains reserved separator '/'", ErrInvalidIngestion, id.Kind)
		}

		availableNodes[id] = struct{}{}
	}

	for _, relation := range relations {
		fromID := relation.From
		toID := relation.To

		switch {
		case relation.Kind == "" || fromID.Kind == "" || fromID.Path == "" || toID.Kind == "" || toID.Path == "":
			return fmt.Errorf("%w: relation ID with empty kind, source, or target", ErrInvalidIngestion)
		case strings.Contains(relation.Kind, "/"):
			return fmt.Errorf("%w: relation kind %q contains reserved separator '/'", ErrInvalidIngestion, relation.Kind)
		case strings.Contains(fromID.Kind, "/"):
			return fmt.Errorf("%w: source node kind %q contains reserved separator '/'", ErrInvalidIngestion, fromID.Kind)
		case strings.Contains(toID.Kind, "/"):
			return fmt.Errorf("%w: target node kind %q contains reserved separator '/'", ErrInvalidIngestion, toID.Kind)
		}
		if _, ok := availableNodes[fromID]; !ok {
			return fmt.Errorf("%w: source node %q/%q not found", ErrInvalidIngestion, fromID.Kind, fromID.Path)
		}
		if _, ok := availableNodes[toID]; !ok {
			return fmt.Errorf("%w: target node %q/%q not found", ErrInvalidIngestion, toID.Kind, toID.Path)
		}
	}

	return nil
}
