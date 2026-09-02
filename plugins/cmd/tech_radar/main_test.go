package main

import (
	"bytes"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *log.Logger { return log.New(io.Discard, "", 0) }

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "radar.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func newTestPlugin(t *testing.T, configPath string, logger *log.Logger) *Plugin {
	t.Helper()
	p, err := New(config{ConfigPath: configPath}, logger)
	require.NoError(t, err)
	return p
}

func nodesByKind(response pluginapi.CollectResponse, kind string) map[string]pluginapi.PropertyMap {
	result := make(map[string]pluginapi.PropertyMap)
	for _, node := range response.Nodes {
		if node.ID.Kind == kind {
			result[node.ID.Path] = node.Properties
		}
	}
	return result
}

func TestNewValidatesConfig(t *testing.T) {
	_, err := New(config{ConfigPath: ""}, testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TECH_RADAR_CONFIG_PATH is empty")
}

func TestCollectEmitsRadarAndEntries(t *testing.T) {
	p := newTestPlugin(t, writeConfig(t, validConfig), testLogger())

	response, err := p.Collect(t.Context())
	require.NoError(t, err)

	assert.Empty(t, response.Relations, "the radar emits no relations by design")

	radars := nodesByKind(response, pluginapi.NodeKindTechRadar)
	require.Contains(t, radars, "naira")
	radar := radars["naira"]
	assert.Equal(t, "Naira Tech Radar", radar["title"])
	assert.Equal(t, "2026-09", radar["edition"])
	assert.Equal(t, "platform-team", radar["owner"])
	assert.Equal(t, "1", radar["schema_version"])
	assert.Equal(t, "2", radar["entry_count"])
	assert.Equal(t, "1", radar["moved_count"])
	assert.JSONEq(t,
		`[{"id":"models","name":"Models"},{"id":"agentic","name":"Agentic Patterns"},{"id":"knowledge","name":"Knowledge Techniques"},{"id":"others","name":"Others"}]`,
		radar["quadrants"], "quadrant order is preserved")
	assert.JSONEq(t,
		`[{"id":"adopt","name":"Adopt","description":"Proven; default choice for new work."},{"id":"hold","name":"Hold","description":"Do not start new work with this."}]`,
		radar["rings"])

	entries := nodesByKind(response, pluginapi.NodeKindTechRadarEntry)
	require.Len(t, entries, 2)
	require.Contains(t, entries, "naira/claude-sonnet")
	first := entries["naira/claude-sonnet"]
	assert.Equal(t, "Claude Sonnet", first["title"])
	assert.Equal(t, "models", first["quadrant"])
	assert.Equal(t, "adopt", first["ring"])
	assert.Equal(t, "in", first["moved"])
	assert.Equal(t, "ml-platform", first["owner"])
	assert.Equal(t, "Default general-purpose model.", first["rationale"])
	assert.Equal(t, "0", first["index"])
	assert.Equal(t, "naira", first["radar"])

	second := entries["naira/naive-rag"]
	assert.Equal(t, "none", second["moved"], "omitted moved defaults to none")
	assert.Equal(t, "1", second["index"])
}

func TestCollectFailsWithoutTouchingResponseOnMissingFile(t *testing.T) {
	p := newTestPlugin(t, filepath.Join(t.TempDir(), "does-not-exist.yaml"), testLogger())

	response, err := p.Collect(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does-not-exist.yaml")
	assert.Empty(t, response.Nodes, "a failed collect returns a zero-value response")
}

func TestCollectFailsOnInvalidConfig(t *testing.T) {
	path := writeConfig(t, "schema_version: 1\nradar:\n  bogus: field\nquadrants: []\nrings: []\nentries: []\n")
	p := newTestPlugin(t, path, testLogger())

	response, err := p.Collect(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
	assert.Contains(t, err.Error(), `unknown field "bogus" in radar`)
	assert.Contains(t, err.Error(), "line 3")
	assert.Empty(t, response.Nodes)
}

func TestCollectRereadsFileEveryRun(t *testing.T) {
	path := writeConfig(t, validConfig)
	p := newTestPlugin(t, path, testLogger())

	first, err := p.Collect(t.Context())
	require.NoError(t, err)
	require.Len(t, nodesByKind(first, pluginapi.NodeKindTechRadarEntry), 2)

	updated := strings.Replace(validConfig, "edition: 2026-09", "edition: 2026-12", 1)
	updated = strings.Replace(updated, "moved: in", "moved: out", 1)
	require.NoError(t, os.WriteFile(path, []byte(updated), 0o600))

	second, err := p.Collect(t.Context())
	require.NoError(t, err)
	radar := nodesByKind(second, pluginapi.NodeKindTechRadar)["naira"]
	assert.Equal(t, "2026-12", radar["edition"])
	entry := nodesByKind(second, pluginapi.NodeKindTechRadarEntry)["naira/claude-sonnet"]
	assert.Equal(t, "out", entry["moved"])
}

func TestCollectTruncatesLongRationale(t *testing.T) {
	longRationale := strings.Repeat("ä", maxTextLength+10)
	config := strings.Replace(validConfig, "rationale: Default general-purpose model.", "rationale: "+longRationale, 1)

	var logBuffer bytes.Buffer
	p := newTestPlugin(t, writeConfig(t, config), log.New(&logBuffer, "", 0))

	response, err := p.Collect(t.Context())
	require.NoError(t, err)

	rationale := nodesByKind(response, pluginapi.NodeKindTechRadarEntry)["naira/claude-sonnet"]["rationale"]
	runes := []rune(rationale)
	assert.Len(t, runes, maxTextLength+1, "truncated to the limit plus ellipsis")
	assert.Equal(t, '…', runes[len(runes)-1])
	assert.Contains(t, logBuffer.String(), `WARN: entry "claude-sonnet": rationale truncated`)
}
