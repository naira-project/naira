package catalog

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
)

var (
	ErrInvalidIngestion = errors.New("invalid ingestion payload")
)

type Store interface {
	ListNodes() []Node
	GetNode(id NodeID) (Node, error)
	ListRelations() []Relation
	ApplyPluginSnapshot(pluginName string, snapshotID uuid.UUID, nodes []NodeClaim, relations []RelationClaim) (int, int, error)
}

type MemoryStore struct {
	mu        sync.RWMutex
	nodes     map[NodeID]Node
	relations map[RelationID]Relation
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

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		nodes:     make(map[NodeID]Node),
		relations: make(map[RelationID]Relation),
	}
}

func (s *MemoryStore) ListNodes() []Node {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		result = append(result, node)
	}

	sort.Slice(result, func(i, j int) bool {
		return lessNodeID(result[i].ID, result[j].ID)
	})

	return result
}

func (s *MemoryStore) GetNode(id NodeID) (Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	node, ok := s.nodes[id]
	if !ok {
		return Node{}, fmt.Errorf("node %q/%q: %w", id.Kind, id.Path, ErrNodeNotFound)
	}

	return node, nil
}

func (s *MemoryStore) ListRelations() []Relation {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Relation, 0, len(s.relations))
	for _, relation := range s.relations {
		result = append(result, relation)
	}
	sort.Slice(result, func(i, j int) bool {
		return lessRelation(result[i], result[j])
	})

	return result
}

func (s *MemoryStore) ApplyPluginSnapshot(pluginName string, snapshotID uuid.UUID, nodes []NodeClaim, relations []RelationClaim) (int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := validateSnapshotInput(pluginName, snapshotID, nodes, relations); err != nil {
		return 0, 0, fmt.Errorf("validate snapshot input: %w", err)
	}

	upsertedNodes := 0
	for _, nodeClaim := range nodes {
		id := nodeClaim.ID

		existing, ok := s.nodes[id]
		if !ok {
			existing = Node{
				ID:           id,
				PluginClaims: make(map[string]PluginClaim),
			}
		} else if existing.PluginClaims == nil {
			existing.PluginClaims = make(map[string]PluginClaim)
		}

		existing.PluginClaims[pluginName] = PluginClaim{
			SnapshotID: snapshotID,
			Properties: nodeClaim.Properties,
		}
		s.nodes[id] = existing
		upsertedNodes++
	}

	upsertedRelations := 0
	for _, relationClaim := range relations {
		key := RelationID{
			Kind: relationClaim.Kind,
			From: relationClaim.From,
			To:   relationClaim.To,
		}

		existing, ok := s.relations[key]
		if !ok {
			existing = Relation{
				Kind:         key.Kind,
				From:         key.From,
				To:           key.To,
				PluginClaims: make(map[string]PluginClaim),
			}
		} else if existing.PluginClaims == nil {
			existing.PluginClaims = make(map[string]PluginClaim)
		}

		existing.PluginClaims[pluginName] = PluginClaim{
			SnapshotID: snapshotID,
			Properties: relationClaim.Properties,
		}
		s.relations[key] = existing
		upsertedRelations++
	}

	s.pruneNodes(pluginName, snapshotID)
	s.pruneRelations(pluginName, snapshotID)

	return upsertedNodes, upsertedRelations, nil
}

func (s *MemoryStore) pruneNodes(pluginName string, snapshotID uuid.UUID) {
	for id, node := range s.nodes {
		claim, exists := node.PluginClaims[pluginName]
		if !exists {
			continue
		}

		if claim.SnapshotID != snapshotID {
			delete(s.nodes[id].PluginClaims, pluginName)
		}

		if len(node.PluginClaims) == 0 {
			delete(s.nodes, id)
		}
	}
}

func (s *MemoryStore) pruneRelations(pluginName string, snapshotID uuid.UUID) {
	for id, relation := range s.relations {
		claim, exists := relation.PluginClaims[pluginName]
		if !exists {
			continue
		}

		if claim.SnapshotID != snapshotID {
			delete(s.relations[id].PluginClaims, pluginName)
		}

		if len(relation.PluginClaims) == 0 {
			delete(s.relations, id)
		}
	}
}

func validateSnapshotInput(pluginName string, snapshotID uuid.UUID, nodes []NodeClaim, relations []RelationClaim) error {
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

func lessNodeID(left NodeID, right NodeID) bool {
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	return left.Path < right.Path
}

func lessRelation(left Relation, right Relation) bool {
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}

	if left.From != right.From {
		return lessNodeID(left.From, right.From)
	}

	if left.To != right.To {
		return lessNodeID(left.To, right.To)
	}

	return false
}
