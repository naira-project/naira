package catalog

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/google/uuid"
)

type Store interface {
	ListNodes(ctx context.Context) ([]Node, error)
	GetNode(ctx context.Context, id NodeID) (Node, error)
	ListRelations(ctx context.Context) ([]Relation, error)
	ApplyPluginSnapshot(ctx context.Context, pluginName string, snapshotID uuid.UUID, nodes []NodeClaim, relations []RelationClaim) (int, int, error)
	ListKinds(ctx context.Context) ([]string, error)
}

type MemoryStore struct {
	mu        sync.RWMutex
	nodes     map[NodeID]Node
	relations map[RelationID]Relation
	kinds     []string
}

func (s *MemoryStore) ListKinds(_ context.Context) ([]string, error) {
	kinds := make(map[string]struct{})
	s.mu.RLock()
	for _, node := range s.nodes {
		kinds[node.ID.Kind] = struct{}{}
	}
	s.mu.RUnlock()

	result := make([]string, 0, len(kinds))
	for kind := range kinds {
		result = append(result, kind)
	}
	sort.Strings(result)

	return result, nil
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		nodes:     make(map[NodeID]Node),
		relations: make(map[RelationID]Relation),
	}
}

func (s *MemoryStore) ListNodes(_ context.Context) ([]Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		result = append(result, node)
	}

	sort.Slice(result, func(i, j int) bool {
		return lessNodeID(result[i].ID, result[j].ID)
	})

	return result, nil
}

func (s *MemoryStore) GetNode(_ context.Context, id NodeID) (Node, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	node, ok := s.nodes[id]
	if !ok {
		return Node{}, fmt.Errorf("node %q/%q: %w", id.Kind, id.Path, ErrNodeNotFound)
	}

	return node, nil
}

func (s *MemoryStore) ListRelations(_ context.Context) ([]Relation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Relation, 0, len(s.relations))
	for _, relation := range s.relations {
		result = append(result, relation)
	}
	sort.Slice(result, func(i, j int) bool {
		return lessRelation(result[i], result[j])
	})

	return result, nil
}

func (s *MemoryStore) ApplyPluginSnapshot(_ context.Context, pluginName string, snapshotID uuid.UUID, nodes []NodeClaim, relations []RelationClaim) (int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ValidateSnapshotInput(pluginName, snapshotID, nodes, relations); err != nil {
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

		if len(s.nodes[id].PluginClaims) == 0 {
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

		if len(s.relations[id].PluginClaims) == 0 {
			delete(s.relations, id)
		}
	}
}

func lessNodeID(left, right NodeID) bool {
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	return left.Path < right.Path
}

func lessRelation(left, right Relation) bool {
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
