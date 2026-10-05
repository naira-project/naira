package main

import (
	"strconv"
	"sync"
	"time"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
)

const (
	propertyKeyProjectID        = "project_id"
	propertyKeyHost             = "host"
	propertyKeyScope            = "scope"
	propertyKeyTotalCost        = "total_cost_usd"
	propertyKeyAvgLatency       = "avg_latency"
	propertyKeyLatencyUnit      = "latency_unit"
	propertyKeyObservationCount = "observation_count"
	propertyKeyScoreName        = "score_name"
	propertyKeyScoreMean        = "score_mean"
	propertyKeyScoreCount       = "score_count"
	propertyKeyWindow           = "window"
	propertyKeyWindowStart      = "window_start"
	propertyKeyWindowEnd        = "window_end"
	propertyKeyCollectedAt      = "collected_at"
	propertyKeyTracesURL        = "traces_url"
	propertyKeyStale            = "stale"
	propertyKeyStaleSince       = "stale_since"
	propertyKeyLastError        = "last_error"
)

// latencyUnit labels avg_latency so the UI never has to guess. It is a
// property rather than part of the metric name because it is the likeliest
// part of this schema to change between Langfuse versions.
const latencyUnit = "ms"

// maxErrorLength bounds last_error, which reaches a node property and then a
// browser.
const maxErrorLength = 300

// scopeNodeID is the catalog identity of one project-and-scope pair. Bindings
// sharing a scope share this node, and so share its numbers.
func scopeNodeID(project projectConfig, traceName string) pluginapi.NodeID {
	path := project.ID
	if traceName != "" {
		path += "/" + traceName
	}
	return pluginapi.NodeID{Kind: pluginapi.NodeKindLangfuseProject, Path: path}
}

// identityProperties are the properties a node carries whether or not its
// metrics could be read: what it points at, and where to go for the detail.
func identityProperties(project projectConfig, traceName string) pluginapi.PropertyMap {
	return pluginapi.PropertyMap{
		propertyKeyProjectID: project.ProjectID,
		propertyKeyHost:      project.Host,
		propertyKeyScope:     traceName,
		propertyKeyTracesURL: tracesURL(project),
	}
}

// measuredProperties renders one successful collect for one scope. The window
// bounds and collection time ship with it because the catalog keeps no
// history, and a cost figure means nothing without the span it covers.
func measuredProperties(
	project projectConfig,
	traceName string,
	observations observationMetrics,
	scores scoreMetrics,
	from, to time.Time,
) pluginapi.PropertyMap {
	properties := identityProperties(project, traceName)

	properties[propertyKeyTotalCost] = observations.TotalCost
	properties[propertyKeyAvgLatency] = observations.AvgLatency
	properties[propertyKeyLatencyUnit] = latencyUnit
	properties[propertyKeyObservationCount] = observations.Count

	properties[propertyKeyWindow] = project.window.String()
	properties[propertyKeyWindowStart] = formatTimestamp(from)
	properties[propertyKeyWindowEnd] = formatTimestamp(to)
	properties[propertyKeyCollectedAt] = formatTimestamp(to)

	if scores.Name != "" {
		properties[propertyKeyScoreName] = scores.Name
		properties[propertyKeyScoreMean] = scores.Mean
		properties[propertyKeyScoreCount] = scores.Count
	}

	return properties
}

// relationsFor links every app bound to this scope to its metrics node. The
// edge runs app -> metrics node, the direction the UI reads.
func relationsFor(project projectConfig, traceName string) []pluginapi.RelationClaim {
	nodeID := scopeNodeID(project, traceName)

	relations := make([]pluginapi.RelationClaim, 0, len(project.Observes))
	for _, app := range project.Observes {
		if app.TraceName != traceName {
			continue
		}
		relations = append(relations, pluginapi.RelationClaim{
			Kind: pluginapi.RelationKindObservedBy,
			From: pluginapi.NodeID{Kind: app.Kind, Path: app.Path},
			To:   nodeID,
		})
	}
	return relations
}

// lastGood remembers the most recent successful properties per node. The
// catalog replaces a plugin's whole claim each snapshot, so without this a
// single failed collect would blank the widget until the next success.
type lastGood struct {
	mu         sync.Mutex
	properties map[string]pluginapi.PropertyMap
	staleSince map[string]time.Time
}

func newLastGood() *lastGood {
	return &lastGood{
		properties: make(map[string]pluginapi.PropertyMap),
		staleSince: make(map[string]time.Time),
	}
}

func (l *lastGood) record(path string, properties pluginapi.PropertyMap) {
	l.mu.Lock()
	defer l.mu.Unlock()

	stored := make(pluginapi.PropertyMap, len(properties))
	for key, value := range properties {
		stored[key] = value
	}
	l.properties[path] = stored
	delete(l.staleSince, path)
}

// degrade returns what to emit for a failed scope: the last good values marked
// stale, or identity alone if there are none. The first failure after a
// success sets stale_since; later ones preserve it.
func (l *lastGood) degrade(
	project projectConfig,
	traceName string,
	collectErr error,
	now time.Time,
) pluginapi.PropertyMap {
	path := scopeNodeID(project, traceName).Path

	l.mu.Lock()
	defer l.mu.Unlock()

	properties := identityProperties(project, traceName)
	for key, value := range l.properties[path] {
		properties[key] = value
	}

	since, ok := l.staleSince[path]
	if !ok {
		since = now
		l.staleSince[path] = since
	}

	properties[propertyKeyStale] = strconv.FormatBool(true)
	properties[propertyKeyStaleSince] = formatTimestamp(since)
	properties[propertyKeyLastError] = clipError(collectErr)

	return properties
}

func clipError(err error) string {
	if err == nil {
		return ""
	}

	message := []rune(err.Error())
	if len(message) <= maxErrorLength {
		return string(message)
	}
	return string(message[:maxErrorLength]) + "…"
}
