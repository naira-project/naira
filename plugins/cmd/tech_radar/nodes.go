package main

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
)

const (
	propertyKeyEdition       = "edition"
	propertyKeyEntryCount    = "entry_count"
	propertyKeyIndex         = "index"
	propertyKeyMoved         = "moved"
	propertyKeyMovedCount    = "moved_count"
	propertyKeyOwner         = "owner"
	propertyKeyQuadrant      = "quadrant"
	propertyKeyQuadrants     = "quadrants"
	propertyKeyRadar         = "radar"
	propertyKeyRationale     = "rationale"
	propertyKeyRing          = "ring"
	propertyKeyRings         = "rings"
	propertyKeySchemaVersion = "schema_version"
	propertyKeyTitle         = "title"
)

// Free-form text is clipped rather than rejected so an oversized value never
// blocks a sync: short labels (titles, names, owners, the edition) are capped
// at maxLabelLength runes and long-form text (an entry's rationale, a ring's
// description) at maxTextLength runes, each with a trailing ellipsis and a
// logged warning (see clip). Ids are exempt: they become node paths, so
// oversized ids are rejected during validation instead (see maxIDLength).
const (
	maxLabelLength = 200
	maxTextLength  = 2000
)

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

	quadrants := make([]quadrant, len(cfg.Quadrants))
	for i, q := range cfg.Quadrants {
		q.Name = p.clip(fmt.Sprintf("quadrant %q: name", q.ID), q.Name, maxLabelLength)
		quadrants[i] = q
	}
	rings := make([]ring, len(cfg.Rings))
	for i, r := range cfg.Rings {
		r.Name = p.clip(fmt.Sprintf("ring %q: name", r.ID), r.Name, maxLabelLength)
		r.Description = p.clip(fmt.Sprintf("ring %q: description", r.ID), r.Description, maxTextLength)
		rings[i] = r
	}

	// Marshalling the already-validated taxonomy cannot fail; errors are ignored.
	quadrantsJSON, _ := json.Marshal(quadrants)
	ringsJSON, _ := json.Marshal(rings)

	nodes := make([]pluginapi.NodeClaim, 0, len(cfg.Entries)+1)
	nodes = append(nodes, pluginapi.NodeClaim{
		ID: radarID,
		Properties: pluginapi.PropertyMap{
			propertyKeyTitle:         p.clip(fmt.Sprintf("radar %q: title", cfg.Radar.ID), cfg.Radar.Title, maxLabelLength),
			propertyKeyEdition:       p.clip(fmt.Sprintf("radar %q: edition", cfg.Radar.ID), cfg.Radar.Edition, maxLabelLength),
			propertyKeyOwner:         p.clip(fmt.Sprintf("radar %q: owner", cfg.Radar.ID), cfg.Radar.Owner, maxLabelLength),
			propertyKeySchemaVersion: strconv.Itoa(cfg.SchemaVersion),
			propertyKeyQuadrants:     string(quadrantsJSON),
			propertyKeyRings:         string(ringsJSON),
			propertyKeyEntryCount:    strconv.Itoa(len(cfg.Entries)),
			propertyKeyMovedCount:    strconv.Itoa(movedCount),
		},
	})

	for i, e := range cfg.Entries {
		nodes = append(nodes, pluginapi.NodeClaim{
			ID: pluginapi.NodeID{Kind: pluginapi.NodeKindTechRadarEntry, Path: cfg.Radar.ID + "/" + e.ID},
			Properties: pluginapi.PropertyMap{
				propertyKeyTitle:     p.clip(fmt.Sprintf("entry %q: name", e.ID), e.Name, maxLabelLength),
				propertyKeyQuadrant:  e.Quadrant,
				propertyKeyRing:      e.Ring,
				propertyKeyMoved:     e.Moved,
				propertyKeyOwner:     p.clip(fmt.Sprintf("entry %q: owner", e.ID), e.Owner, maxLabelLength),
				propertyKeyRationale: p.clip(fmt.Sprintf("entry %q: rationale", e.ID), e.Rationale, maxTextLength),
				propertyKeyIndex:     strconv.Itoa(i),
				propertyKeyRadar:     cfg.Radar.ID,
			},
		})
	}

	return pluginapi.CollectResponse{Nodes: nodes}
}

// clip truncates value to limit runes, cutting on a rune boundary so clipping
// never produces invalid UTF-8, and logs a warning naming the clipped field.
func (p *Plugin) clip(field, value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}

	p.logger.Printf("WARN: %s truncated to %d runes", field, limit)
	return string(runes[:limit]) + "…"
}
