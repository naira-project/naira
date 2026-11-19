// Package catalog defines catalog graph models and snapshot validation.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	maxIdentifierLength = 40
	maxNodePathLength   = 300
)

var (
	ErrInvalidIngestion = errors.New("invalid ingestion payload")
	ErrNodeNotFound     = errors.New("node not found")
)

// Reader provides read-only access to the catalog graph.
type Reader interface {
	GetNode(ctx context.Context, id NodeID) (Node, error)
	ListNodes(ctx context.Context) ([]Node, error)
	ListRelations(ctx context.Context) ([]Relation, error)
	ListKinds(ctx context.Context) ([]string, error)
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
	if runeLength := utf8.RuneCountInString(pluginName); runeLength > maxIdentifierLength {
		return fmt.Errorf("%w: plugin name exceeds %d characters", ErrInvalidIngestion, maxIdentifierLength)
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
		if runeLength := utf8.RuneCountInString(id.Kind); runeLength > maxIdentifierLength {
			return fmt.Errorf("%w: node kind exceeds %d characters", ErrInvalidIngestion, maxIdentifierLength)
		}
		if runeLength := utf8.RuneCountInString(id.Path); runeLength > maxNodePathLength {
			return fmt.Errorf("%w: node path exceeds %d characters", ErrInvalidIngestion, maxNodePathLength)
		}
		if strings.Contains(id.Kind, "/") {
			return fmt.Errorf("%w: node kind %q contains reserved separator '/'", ErrInvalidIngestion, id.Kind)
		}
		if _, exists := availableNodes[id]; exists {
			return fmt.Errorf("%w: duplicate node ID %q/%q", ErrInvalidIngestion, id.Kind, id.Path)
		}

		availableNodes[id] = struct{}{}
	}

	availableRelations := make(map[RelationID]struct{}, len(relations))
	for _, relation := range relations {
		fromID := relation.From
		toID := relation.To

		switch {
		case relation.Kind == "" || fromID.Kind == "" || fromID.Path == "" || toID.Kind == "" || toID.Path == "":
			return fmt.Errorf("%w: relation ID with empty kind, source, or target", ErrInvalidIngestion)
		case utf8.RuneCountInString(relation.Kind) > maxIdentifierLength:
			return fmt.Errorf("%w: relation kind exceeds %d characters", ErrInvalidIngestion, maxIdentifierLength)
		case strings.Contains(relation.Kind, "/"):
			return fmt.Errorf("%w: relation kind %q contains reserved separator '/'", ErrInvalidIngestion, relation.Kind)
		case strings.Contains(fromID.Kind, "/"):
			return fmt.Errorf("%w: source node kind %q contains reserved separator '/'", ErrInvalidIngestion, fromID.Kind)
		case strings.Contains(toID.Kind, "/"):
			return fmt.Errorf("%w: target node kind %q contains reserved separator '/'", ErrInvalidIngestion, toID.Kind)
		}

		relationID := RelationID{Kind: relation.Kind, From: fromID, To: toID}
		if _, exists := availableRelations[relationID]; exists {
			return fmt.Errorf("%w: duplicate relation ID %q %q/%q -> %q/%q",
				ErrInvalidIngestion, relation.Kind, fromID.Kind, fromID.Path, toID.Kind, toID.Path)
		}
		availableRelations[relationID] = struct{}{}

		if _, ok := availableNodes[fromID]; !ok {
			return fmt.Errorf("%w: source node %q/%q not found", ErrInvalidIngestion, fromID.Kind, fromID.Path)
		}
		if _, ok := availableNodes[toID]; !ok {
			return fmt.Errorf("%w: target node %q/%q not found", ErrInvalidIngestion, toID.Kind, toID.Path)
		}
	}

	return nil
}
