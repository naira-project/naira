package catalog

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestValidateSnapshotInput(t *testing.T) {
	t.Parallel()

	a := NodeID{Kind: "service", Path: "a"}
	b := NodeID{Kind: "service", Path: "b"}
	validNodes := []NodeClaim{
		{ID: a},
		{ID: NodeID{Kind: "target", Path: "b"}},
	}

	tests := []struct {
		name      string
		plugin    string
		nodes     []NodeClaim
		relations []RelationClaim
		wantErr   string
	}{
		{
			name: "empty snapshot is valid",
		},
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
			name:  "duplicate relation ID",
			nodes: []NodeClaim{{ID: a}, {ID: b}},
			relations: []RelationClaim{
				{Kind: "calls", From: a, To: b},
				{Kind: "calls", From: a, To: b},
			},
			wantErr: `duplicate relation ID "calls" "service"/"a" -> "service"/"b"`,
		},
		{
			name:  "same endpoints with different relation kinds are distinct",
			nodes: []NodeClaim{{ID: a}, {ID: b}},
			relations: []RelationClaim{
				{Kind: "calls", From: a, To: b},
				{Kind: "uses", From: a, To: b},
			},
		},
		{
			name:  "reversed relation direction is distinct",
			nodes: []NodeClaim{{ID: a}, {ID: b}},
			relations: []RelationClaim{
				{Kind: "calls", From: a, To: b},
				{Kind: "calls", From: b, To: a},
			},
		},
		{
			name:    "node with empty kind",
			nodes:   []NodeClaim{{ID: NodeID{Path: "a"}}},
			wantErr: "node ID with empty kind or path",
		},
		{
			name:    "node kind contains reserved separator",
			nodes:   []NodeClaim{{ID: NodeID{Kind: "service/api", Path: "a"}}},
			wantErr: `node kind "service/api" contains reserved separator`,
		},
		{
			name:  "relation kind contains reserved separator",
			nodes: []NodeClaim{{ID: a}, {ID: b}},
			relations: []RelationClaim{
				{Kind: "calls/api", From: a, To: b},
			},
			wantErr: `relation kind "calls/api" contains reserved separator`,
		},
		{
			name:  "relation references missing node",
			nodes: []NodeClaim{{ID: a}},
			relations: []RelationClaim{
				{Kind: "calls", From: a, To: b},
			},
			wantErr: `target node "service"/"b" not found`,
		},
		{
			name:   "plugin name at 40 multibyte characters",
			plugin: strings.Repeat("ą", 40),
		},
		{
			name:   "plugin name at 40 four-byte code points",
			plugin: strings.Repeat("🤗", 40),
		},
		{
			// Each 😵‍💫 grapheme contains three code points, so 40 graphemes exceed the 40-rune limit.
			name:    "plugin name with 40 multi-code-point emoji graphemes exceeds limit",
			plugin:  strings.Repeat("😵‍💫", 40),
			wantErr: "plugin name exceeds 40 characters",
		},
		{
			name:    "plugin name exceeds 40 characters",
			plugin:  strings.Repeat("ą", 41),
			wantErr: "plugin name exceeds 40 characters",
		},
		{
			name:  "node kind at 40 multibyte characters",
			nodes: []NodeClaim{{ID: NodeID{Kind: strings.Repeat("ą", 40), Path: "a"}}},
		},
		{
			name:    "node kind exceeds 40 characters",
			nodes:   []NodeClaim{{ID: NodeID{Kind: strings.Repeat("ą", 41), Path: "a"}}},
			wantErr: "node kind exceeds 40 characters",
		},
		{
			name:  "node path at 300 multibyte characters",
			nodes: []NodeClaim{{ID: NodeID{Kind: "service", Path: strings.Repeat("ą", 300)}}},
		},
		{
			name:    "node path exceeds 300 characters",
			nodes:   []NodeClaim{{ID: NodeID{Kind: "service", Path: strings.Repeat("ą", 301)}}},
			wantErr: "node path exceeds 300 characters",
		},
		{
			name:  "relation kind at 40 multibyte characters",
			nodes: validNodes,
			relations: []RelationClaim{
				{Kind: strings.Repeat("ą", 40), From: validNodes[0].ID, To: validNodes[1].ID},
			},
		},
		{
			name:  "relation kind exceeds 40 characters",
			nodes: validNodes,
			relations: []RelationClaim{
				{Kind: strings.Repeat("ą", 41), From: validNodes[0].ID, To: validNodes[1].ID},
			},
			wantErr: "relation kind exceeds 40 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			plugin := tt.plugin
			if plugin == "" {
				plugin = "k8s"
			}

			err := ValidateSnapshotInput(plugin, snapshotV1, tt.nodes, tt.relations)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, ErrInvalidIngestion)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestValidateSnapshotInput_RequiresPluginAndSnapshotID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		pluginName string
		snapshotID uuid.UUID
		wantErr    string
	}{
		{
			name:       "empty plugin name",
			pluginName: "",
			snapshotID: snapshotV1,
			wantErr:    "plugin name is empty",
		},
		{
			name:       "empty snapshot ID",
			pluginName: "k8s",
			wantErr:    "snapshot ID is empty",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateSnapshotInput(tt.pluginName, tt.snapshotID, nil, nil)

			assert.ErrorIs(t, err, ErrInvalidIngestion)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}
