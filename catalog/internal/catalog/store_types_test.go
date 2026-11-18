package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateSnapshotInput_DuplicateIDs(t *testing.T) {
	t.Parallel()

	a := NodeID{Kind: "service", Path: "a"}
	b := NodeID{Kind: "service", Path: "b"}

	tests := []struct {
		name      string
		nodes     []NodeClaim
		relations []RelationClaim
		wantErr   string
	}{
		{
			name: "duplicate node ID",
			nodes: []NodeClaim{
				{ID: a},
				{ID: b},
				{ID: a},
			},
			wantErr: `duplicate node ID "service"/"a"`,
		},
		{
			name: "duplicate relation ID",
			nodes: []NodeClaim{
				{ID: a},
				{ID: b},
			},
			relations: []RelationClaim{
				{Kind: "calls", From: a, To: b},
				{Kind: "calls", From: a, To: b},
			},
			wantErr: `duplicate relation ID "calls" "service"/"a" -> "service"/"b"`,
		},
		{
			name: "same endpoints with different relation kinds are distinct",
			nodes: []NodeClaim{
				{ID: a},
				{ID: b},
			},
			relations: []RelationClaim{
				{Kind: "calls", From: a, To: b},
				{Kind: "uses", From: a, To: b},
			},
		},
		{
			name: "reversed relation direction is distinct",
			nodes: []NodeClaim{
				{ID: a},
				{ID: b},
			},
			relations: []RelationClaim{
				{Kind: "calls", From: a, To: b},
				{Kind: "calls", From: b, To: a},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateSnapshotInput("k8s", snapshotV1, tt.nodes, tt.relations)

			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, ErrInvalidIngestion)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}
