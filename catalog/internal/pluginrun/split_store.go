package pluginrun

import (
	"context"

	"github.com/google/uuid"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/operations"
)

// SplitStore combines separate graph and operation stores for environments
// that do not provide a transaction-capable persistence backend. Production
// PostgreSQL wiring should use pgstore.Store directly so snapshot completion
// remains atomic.
type SplitStore struct {
	Catalog    catalog.Store
	Operations operations.Store
}

func (s SplitStore) ListNodes() ([]catalog.Node, error) { return s.Catalog.ListNodes() }

func (s SplitStore) GetNode(id catalog.NodeID) (catalog.Node, error) {
	return s.Catalog.GetNode(id)
}

func (s SplitStore) ListRelations() ([]catalog.Relation, error) {
	return s.Catalog.ListRelations()
}

func (s SplitStore) ApplyPluginSnapshot(pluginName string, snapshotID uuid.UUID, nodes []catalog.NodeClaim, relations []catalog.RelationClaim) (int, int, error) {
	return s.Catalog.ApplyPluginSnapshot(pluginName, snapshotID, nodes, relations)
}

func (s SplitStore) Create(op operations.Operation) error { return s.Operations.Create(op) }

func (s SplitStore) Get(name string) (operations.Operation, error) { return s.Operations.Get(name) }

func (s SplitStore) List(filter operations.Filter) ([]operations.Operation, error) {
	return s.Operations.List(filter)
}

func (s SplitStore) UpdateState(name string, state operations.State, statusErr *operations.StatusError, nodesUpserted, relationsUpserted int) error {
	return s.Operations.UpdateState(name, state, statusErr, nodesUpserted, relationsUpserted)
}

func (s SplitStore) CompleteSnapshotOperation(_ context.Context, operationName, pluginName string, snapshotID uuid.UUID, nodes []catalog.NodeClaim, relations []catalog.RelationClaim) (int, int, error) {
	nodesUpserted, relationsUpserted, err := s.Catalog.ApplyPluginSnapshot(pluginName, snapshotID, nodes, relations)
	if err != nil {
		return 0, 0, err
	}
	if err := s.Operations.UpdateState(operationName, operations.StateSucceeded, nil, nodesUpserted, relationsUpserted); err != nil {
		return 0, 0, err
	}
	return nodesUpserted, relationsUpserted, nil
}
