package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *log.Logger { return log.New(io.Discard, "", 0) }

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func newTestPlugin(t *testing.T, configPath string) *Plugin {
	t.Helper()
	p, err := New(config{ConfigPath: configPath, HTTPTimeout: time.Second}, testLogger())
	require.NoError(t, err)
	p.now = func() time.Time { return testTo }
	return p
}

func nodesByID(response pluginapi.CollectResponse) map[pluginapi.NodeID]pluginapi.PropertyMap {
	result := make(map[pluginapi.NodeID]pluginapi.PropertyMap, len(response.Nodes))
	for _, node := range response.Nodes {
		result[node.ID] = node.Properties
	}
	return result
}

// configFor renders a one-project config pointed at a test server.
func configFor(host string) string {
	return fmt.Sprintf(`
schema_version: 1
projects:
  - id: demo
    host: %s
    project_id: cm0demo
    public_key: pk-lf-demo
    secret_key: sk-lf-demo
    score_name: user_feedback
    observes:
      - kind: deployment
        path: naira-idp/default/rag-assistant
`, host)
}

func TestNewValidatesConfigPath(t *testing.T) {
	_, err := New(config{ConfigPath: ""}, testLogger())
	assert.ErrorContains(t, err, "LANGFUSE_CONFIG_PATH is empty")
}

func TestCollectFailsWhenConfigIsMissing(t *testing.T) {
	p := newTestPlugin(t, filepath.Join(t.TempDir(), "absent.yaml"))

	_, err := p.Collect(t.Context())
	assert.ErrorContains(t, err, "reading langfuse config")
}

// An invalid file must fail the run so the catalog keeps the previous snapshot
// rather than pruning every node this plugin owns.
func TestCollectFailsWhenConfigIsInvalid(t *testing.T) {
	p := newTestPlugin(t, writeConfig(t, "schema_version: 99\nprojects: []\n"))

	_, err := p.Collect(t.Context())
	assert.ErrorContains(t, err, "loading langfuse config")
}

func TestCollectEmitsMetricsNodeAndRelation(t *testing.T) {
	server, _ := capturingServer(t, `{"data":[{"sum_totalCost":1.25,"avg_latency":842,"count_count":15204,"avg_value":0.82}]}`)
	p := newTestPlugin(t, writeConfig(t, configFor(server.URL)))

	response, err := p.Collect(t.Context())
	require.NoError(t, err)

	nodes := nodesByID(response)

	metricsID := pluginapi.NodeID{Kind: pluginapi.NodeKindLangfuseProject, Path: "demo"}
	require.Contains(t, nodes, metricsID)
	metrics := nodes[metricsID]
	assert.Equal(t, "1.25", metrics[propertyKeyTotalCost])
	assert.Equal(t, "842", metrics[propertyKeyAvgLatency])
	assert.Equal(t, "ms", metrics[propertyKeyLatencyUnit])
	assert.Equal(t, "15204", metrics[propertyKeyObservationCount])
	assert.Equal(t, "user_feedback", metrics[propertyKeyScoreName])
	assert.Equal(t, "0.82", metrics[propertyKeyScoreMean])
	assert.Equal(t, server.URL+"/project/cm0demo/traces", metrics[propertyKeyTracesURL])
	assert.NotContains(t, metrics, propertyKeyStale)

	// Without the window and the collection time a cost figure cannot be read.
	assert.Equal(t, "24h0m0s", metrics[propertyKeyWindow])
	assert.Equal(t, "2026-09-23T12:00:00Z", metrics[propertyKeyWindowStart])
	assert.Equal(t, "2026-09-24T12:00:00Z", metrics[propertyKeyCollectedAt])

	require.Len(t, response.Relations, 1)
	assert.Equal(t, pluginapi.RelationClaim{
		Kind: pluginapi.RelationKindObservedBy,
		From: pluginapi.NodeID{Kind: "deployment", Path: "naira-idp/default/rag-assistant"},
		To:   metricsID,
	}, response.Relations[0])
}

// The catalog rejects a snapshot whose relations reach undeclared nodes, so
// every bound app must be claimed even though this plugin knows nothing else
// about it.
func TestCollectDeclaresBoundAppNodes(t *testing.T) {
	server, _ := capturingServer(t, `{"data":[]}`)
	p := newTestPlugin(t, writeConfig(t, configFor(server.URL)))

	response, err := p.Collect(t.Context())
	require.NoError(t, err)

	nodes := nodesByID(response)
	appID := pluginapi.NodeID{Kind: "deployment", Path: "naira-idp/default/rag-assistant"}
	require.Contains(t, nodes, appID)
	assert.Empty(t, nodes[appID], "the app claim carries no properties of its own")

	declared := make(map[pluginapi.NodeID]bool, len(response.Nodes))
	for _, node := range response.Nodes {
		declared[node.ID] = true
	}
	for _, relation := range response.Relations {
		assert.True(t, declared[relation.From], "relation source %v must be declared", relation.From)
		assert.True(t, declared[relation.To], "relation target %v must be declared", relation.To)
	}
}

func TestCollectMeasuresScopesApart(t *testing.T) {
	server, queries := capturingServer(t, `{"data":[{"sum_totalCost":2}]}`)
	p := newTestPlugin(t, writeConfig(t, fmt.Sprintf(`
schema_version: 1
projects:
  - id: support
    host: %s
    project_id: cm0support
    public_key: pk-lf-demo
    secret_key: sk-lf-demo
    observes:
      - kind: deployment
        path: ns/triage
        trace_name: triage-agent
      - kind: deployment
        path: ns/escalation
        trace_name: escalation-agent
`, server.URL)))

	response, err := p.Collect(t.Context())
	require.NoError(t, err)

	nodes := nodesByID(response)
	assert.Contains(t, nodes, pluginapi.NodeID{Kind: pluginapi.NodeKindLangfuseProject, Path: "support/triage-agent"})
	assert.Contains(t, nodes, pluginapi.NodeID{Kind: pluginapi.NodeKindLangfuseProject, Path: "support/escalation-agent"})
	assert.Len(t, response.Relations, 2)

	// No score_name is configured, so only the observation query runs.
	assert.Len(t, *queries, 2)
}

// One unreachable project must not fail the run: a returned error discards the
// whole snapshot, blanking every other project's metrics too.
func TestCollectDegradesFailingProjectWithoutFailingRun(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(failing.Close)
	healthy, _ := capturingServer(t, `{"data":[{"sum_totalCost":3}]}`)

	p := newTestPlugin(t, writeConfig(t, fmt.Sprintf(`
schema_version: 1
projects:
  - id: broken
    host: %s
    project_id: cm0broken
    public_key: pk-lf-demo
    secret_key: sk-lf-demo
  - id: healthy
    host: %s
    project_id: cm0healthy
    public_key: pk-lf-demo
    secret_key: sk-lf-demo
`, failing.URL, healthy.URL)))

	response, err := p.Collect(t.Context())
	require.NoError(t, err, "a failing project must not discard the snapshot")

	nodes := nodesByID(response)
	broken := nodes[pluginapi.NodeID{Kind: pluginapi.NodeKindLangfuseProject, Path: "broken"}]
	assert.Equal(t, "true", broken[propertyKeyStale])
	assert.Contains(t, broken[propertyKeyLastError], "500")
	assert.Equal(t, failing.URL+"/project/cm0broken/traces", broken[propertyKeyTracesURL],
		"a stale node still deep-links, so the user can check Langfuse directly")

	assert.Equal(t, "3", nodes[pluginapi.NodeID{Kind: pluginapi.NodeKindLangfuseProject, Path: "healthy"}][propertyKeyTotalCost])
}

// The catalog replaces a plugin's whole claim per snapshot, so a transient
// failure would blank the widget unless the last good values are re-emitted.
func TestCollectRetainsLastGoodValuesWhenAProjectFails(t *testing.T) {
	var fail bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"sum_totalCost":4.5}]}`))
	}))
	t.Cleanup(server.Close)

	p := newTestPlugin(t, writeConfig(t, configFor(server.URL)))

	first, err := p.Collect(t.Context())
	require.NoError(t, err)
	metricsID := pluginapi.NodeID{Kind: pluginapi.NodeKindLangfuseProject, Path: "demo"}
	require.Equal(t, "4.5", nodesByID(first)[metricsID][propertyKeyTotalCost])

	fail = true
	second, err := p.Collect(t.Context())
	require.NoError(t, err)

	degraded := nodesByID(second)[metricsID]
	assert.Equal(t, "4.5", degraded[propertyKeyTotalCost], "the last good cost survives a failed collect")
	assert.Equal(t, "true", degraded[propertyKeyStale])
	assert.Equal(t, "2026-09-24T12:00:00Z", degraded[propertyKeyStaleSince])
}

// A score query failing must not throw away the cost and latency that landed.
func TestCollectKeepsObservationsWhenScoreQueryFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if query := r.URL.Query().Get("query"); query != "" && containsView(query, viewScoresNumeric) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"sum_totalCost":5.5}]}`))
	}))
	t.Cleanup(server.Close)

	p := newTestPlugin(t, writeConfig(t, configFor(server.URL)))

	response, err := p.Collect(t.Context())
	require.NoError(t, err)

	metrics := nodesByID(response)[pluginapi.NodeID{Kind: pluginapi.NodeKindLangfuseProject, Path: "demo"}]
	assert.Equal(t, "5.5", metrics[propertyKeyTotalCost])
	assert.NotContains(t, metrics, propertyKeyStale, "the scope is not stale, only its score is missing")
	assert.NotContains(t, metrics, propertyKeyScoreMean)
}

func containsView(query, view string) bool {
	return len(query) > 0 && jsonContains(query, `"view":"`+view+`"`)
}

func jsonContains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func TestCollectWithoutProjectsEmitsNothing(t *testing.T) {
	p := newTestPlugin(t, writeConfig(t, "schema_version: 1\nprojects: []\n"))

	response, err := p.Collect(t.Context())
	require.NoError(t, err)
	assert.Empty(t, response.Nodes)
	assert.Empty(t, response.Relations)
}
