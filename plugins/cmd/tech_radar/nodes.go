package main

import (
	"encoding/json"
	"strconv"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
)

const (
	propertyKeyTitle         = "title"
	propertyKeyEdition       = "edition"
	propertyKeyOwner         = "owner"
	propertyKeySchemaVersion = "schema_version"
	propertyKeyQuadrants     = "quadrants"
	propertyKeyRings         = "rings"
	propertyKeyEntryCount    = "entry_count"
	propertyKeyMovedCount    = "moved_count"
	propertyKeyQuadrant      = "quadrant"
	propertyKeyRing          = "ring"
	propertyKeyMoved         = "moved"
	propertyKeyRationale     = "rationale"
	propertyKeyIndex         = "index"
	propertyKeyRadar         = "radar"
)

const maxTextLength = 2000

// buildResponse turns a validated radar config into catalog claims: one
// tech_radar node carrying the taxonomy plus one tech_radar_entry node per
// entry. The radar emits no relations by design — it describes the target
// vision, not what is currently deployed.
func (p *Plugin) buildResponse(cfg *radarConfig) pluginapi.CollectResponse {
	radarID := pluginapi.NodeID{Kind: pluginapi.NodeKindTechRadar, Path: cfg.Radar.ID}

	movedCount := 0
	for _, e := range cfg.Entries {
		if e.Moved != movedNone {
			movedCount++
		}
	}

	// Marshalling the already-validated taxonomy cannot fail; errors are ignored.
	quadrantsJSON, _ := json.Marshal(cfg.Quadrants)
	ringsJSON, _ := json.Marshal(cfg.Rings)

	nodes := make([]pluginapi.NodeClaim, 0, len(cfg.Entries)+1)
	nodes = append(nodes, pluginapi.NodeClaim{
		ID: radarID,
		Properties: pluginapi.PropertyMap{
			propertyKeyTitle:         cfg.Radar.Title,
			propertyKeyEdition:       cfg.Radar.Edition,
			propertyKeyOwner:         cfg.Radar.Owner,
			propertyKeySchemaVersion: strconv.Itoa(cfg.SchemaVersion),
			propertyKeyQuadrants:     string(quadrantsJSON),
			propertyKeyRings:         string(ringsJSON),
			propertyKeyEntryCount:    strconv.Itoa(len(cfg.Entries)),
			propertyKeyMovedCount:    strconv.Itoa(movedCount),
		},
	})

	for i, e := range cfg.Entries {
		rationale := truncate(e.Rationale)
		if rationale != e.Rationale {
			p.logger.Printf("WARN: entry %q: rationale truncated to %d runes", e.ID, maxTextLength)
		}

		nodes = append(nodes, pluginapi.NodeClaim{
			ID: pluginapi.NodeID{Kind: pluginapi.NodeKindTechRadarEntry, Path: cfg.Radar.ID + "/" + e.ID},
			Properties: pluginapi.PropertyMap{
				propertyKeyTitle:     e.Name,
				propertyKeyQuadrant:  e.Quadrant,
				propertyKeyRing:      e.Ring,
				propertyKeyMoved:     e.Moved,
				propertyKeyOwner:     e.Owner,
				propertyKeyRationale: rationale,
				propertyKeyIndex:     strconv.Itoa(i),
				propertyKeyRadar:     cfg.Radar.ID,
			},
		})
	}

	return pluginapi.CollectResponse{Nodes: nodes, Relations: []pluginapi.RelationClaim{}}
}

// truncate cuts on a rune boundary so clipping never produces invalid UTF-8.
func truncate(value string) string {
	runes := []rune(value)
	if len(runes) <= maxTextLength {
		return value
	}

	return string(runes[:maxTextLength]) + "…"
}
