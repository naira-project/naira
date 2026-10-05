package catalog_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/naira-project/naira/catalog/internal/catalog"
)

var snapshotID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

func TestValidateSnapshotInput(t *testing.T) {
	validNode := catalog.NodeID{Kind: "model", Path: "mlflow/demo"}

	tests := []struct {
		name       string
		pluginName string
		snapshotID uuid.UUID
		nodes      []catalog.NodeClaim
		relations  []catalog.RelationClaim
		wantErr    bool
	}{
		{
			name:       "accepts empty snapshot",
			pluginName: "mlflow",
			snapshotID: snapshotID,
		},
		{
			name:       "accepts nodes and relations referencing them",
			pluginName: "mlflow",
			snapshotID: snapshotID,
			nodes: []catalog.NodeClaim{
				{ID: validNode},
				{ID: catalog.NodeID{Kind: "dataset", Path: "mlflow/data"}},
			},
			relations: []catalog.RelationClaim{{
				Kind: "trained_on",
				From: validNode,
				To:   catalog.NodeID{Kind: "dataset", Path: "mlflow/data"},
			}},
		},
		{
			name:       "rejects empty plugin name",
			pluginName: "",
			snapshotID: snapshotID,
			wantErr:    true,
		},
		{
			name:       "rejects nil snapshot ID",
			pluginName: "mlflow",
			snapshotID: uuid.Nil,
			wantErr:    true,
		},
		{
			name:       "rejects node with empty path",
			pluginName: "mlflow",
			snapshotID: snapshotID,
			nodes:      []catalog.NodeClaim{{ID: catalog.NodeID{Kind: "model", Path: ""}}},
			wantErr:    true,
		},
		{
			name:       "rejects node kind containing separator",
			pluginName: "mlflow",
			snapshotID: snapshotID,
			nodes:      []catalog.NodeClaim{{ID: catalog.NodeID{Kind: "model/x", Path: "p"}}},
			wantErr:    true,
		},
		{
			name:       "rejects relation referencing node outside the snapshot",
			pluginName: "mlflow",
			snapshotID: snapshotID,
			nodes:      []catalog.NodeClaim{{ID: validNode}},
			relations: []catalog.RelationClaim{{
				Kind: "trained_on",
				From: validNode,
				To:   catalog.NodeID{Kind: "dataset", Path: "missing"},
			}},
			wantErr: true,
		},
		{
			name:       "rejects relation kind containing separator",
			pluginName: "mlflow",
			snapshotID: snapshotID,
			nodes:      []catalog.NodeClaim{{ID: validNode}},
			relations: []catalog.RelationClaim{{
				Kind: "trained/on",
				From: validNode,
				To:   validNode,
			}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := catalog.ValidateSnapshotInput(tt.pluginName, tt.snapshotID, tt.nodes, tt.relations)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, catalog.ErrInvalidIngestion)
				return
			}
			require.NoError(t, err)
		})
	}
}
