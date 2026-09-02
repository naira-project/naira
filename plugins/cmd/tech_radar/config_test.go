package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validConfig = `schema_version: 1
radar:
  id: naira
  title: Naira Tech Radar
  edition: 2026-09
  owner: platform-team
quadrants:
  - id: models
    name: Models
  - id: agentic
    name: Agentic Patterns
  - id: knowledge
    name: Knowledge Techniques
  - id: others
    name: Others
rings:
  - id: adopt
    name: Adopt
    description: Proven; default choice for new work.
  - id: hold
    name: Hold
    description: Do not start new work with this.
entries:
  - id: claude-sonnet
    name: Claude Sonnet
    quadrant: models
    ring: adopt
    moved: in
    owner: ml-platform
    rationale: Default general-purpose model.
  - id: naive-rag
    name: Naive RAG
    quadrant: knowledge
    ring: hold
    owner: ai-board
    rationale: Superseded by hybrid retrieval.
`

func TestParseRadarConfigValid(t *testing.T) {
	cfg, err := parseRadarConfig([]byte(validConfig))
	require.NoError(t, err)

	assert.Equal(t, 1, cfg.SchemaVersion)
	assert.Equal(t, "naira", cfg.Radar.ID)
	assert.Equal(t, "Naira Tech Radar", cfg.Radar.Title)
	assert.Equal(t, "2026-09", cfg.Radar.Edition)

	require.Len(t, cfg.Quadrants, 4)
	assert.Equal(t, "models", cfg.Quadrants[0].ID)
	require.Len(t, cfg.Rings, 2)
	assert.Equal(t, "Proven; default choice for new work.", cfg.Rings[0].Description)

	require.Len(t, cfg.Entries, 2)
	assert.Equal(t, "in", cfg.Entries[0].Moved)
	assert.Equal(t, "none", cfg.Entries[1].Moved, "omitted moved defaults to none")
}

func TestParseRadarConfigDefaultsRadarID(t *testing.T) {
	config := `schema_version: 1
radar:
  title: Radar
  edition: v1
  owner: team
quadrants:
  - {id: a, name: A}
  - {id: b, name: B}
  - {id: c, name: C}
  - {id: d, name: D}
rings:
  - {id: adopt, name: Adopt}
entries: []
`
	cfg, err := parseRadarConfig([]byte(config))
	require.NoError(t, err)
	assert.Equal(t, "default", cfg.Radar.ID)
}

func TestParseRadarConfigErrors(t *testing.T) {
	tests := []struct {
		name     string
		config   string
		wantErrs []string
	}{
		{
			name:     "yaml syntax error",
			config:   "schema_version: 1\nradar:\n  title: [broken",
			wantErrs: []string{"parsing YAML", "line"},
		},
		{
			name:     "unsupported schema version",
			config:   "schema_version: 2\n" + validConfig[len("schema_version: 1\n"):],
			wantErrs: []string{`schema_version: 2 is not supported (expected 1)`},
		},
		{
			name: "unknown field names field and line",
			config: `schema_version: 1
radar:
  title: Radar
  edition: v1
  owner: team
  color: blue
quadrants: []
rings: []
entries: []
`,
			wantErrs: []string{`line 6`, `unknown field "color" in radar`},
		},
		{
			name: "missing required radar fields",
			config: `schema_version: 1
radar:
  title: Radar
quadrants:
  - {id: a, name: A}
  - {id: b, name: B}
  - {id: c, name: C}
  - {id: d, name: D}
rings:
  - {id: adopt, name: Adopt}
entries: []
`,
			wantErrs: []string{`radar: field "edition" must not be empty`, `radar: field "owner" must not be empty`},
		},
		{
			name: "wrong quadrant count",
			config: `schema_version: 1
radar: {title: Radar, edition: v1, owner: team}
quadrants:
  - {id: a, name: A}
  - {id: b, name: B}
rings:
  - {id: adopt, name: Adopt}
entries: []
`,
			wantErrs: []string{"quadrants: exactly 4 quadrants are required, got 2"},
		},
		{
			name: "too many rings",
			config: `schema_version: 1
radar: {title: Radar, edition: v1, owner: team}
quadrants:
  - {id: a, name: A}
  - {id: b, name: B}
  - {id: c, name: C}
  - {id: d, name: D}
rings:
  - {id: r1, name: R1}
  - {id: r2, name: R2}
  - {id: r3, name: R3}
  - {id: r4, name: R4}
  - {id: r5, name: R5}
  - {id: r6, name: R6}
  - {id: r7, name: R7}
entries: []
`,
			wantErrs: []string{"rings: between 1 and 6 rings are required, got 7"},
		},
		{
			name: "duplicate ids",
			config: `schema_version: 1
radar: {title: Radar, edition: v1, owner: team}
quadrants:
  - {id: a, name: A}
  - {id: a, name: A again}
  - {id: c, name: C}
  - {id: d, name: D}
rings:
  - {id: adopt, name: Adopt}
  - {id: adopt, name: Adopt again}
entries:
  - {id: e1, name: E1, quadrant: a, ring: adopt, owner: o, rationale: r}
  - {id: e1, name: E1 again, quadrant: c, ring: adopt, owner: o, rationale: r}
`,
			wantErrs: []string{`duplicate quadrant id "a"`, `duplicate ring id "adopt"`, `duplicate entry id "e1"`},
		},
		{
			name: "entry references undeclared taxonomy with entry id and line",
			config: `schema_version: 1
radar: {title: Radar, edition: v1, owner: team}
quadrants:
  - {id: a, name: A}
  - {id: b, name: B}
  - {id: c, name: C}
  - {id: d, name: D}
rings:
  - {id: adopt, name: Adopt}
entries:
  - id: e1
    name: E1
    quadrant: nope
    ring: gone
    owner: o
    rationale: r
`,
			wantErrs: []string{
				`line 11`,
				`entries[0] (id "e1"): field "quadrant" references undeclared quadrant "nope"`,
				`entries[0] (id "e1"): field "ring" references undeclared ring "gone"`,
			},
		},
		{
			name: "invalid moved value",
			config: `schema_version: 1
radar: {title: Radar, edition: v1, owner: team}
quadrants:
  - {id: a, name: A}
  - {id: b, name: B}
  - {id: c, name: C}
  - {id: d, name: D}
rings:
  - {id: adopt, name: Adopt}
entries:
  - {id: e1, name: E1, quadrant: a, ring: adopt, moved: sideways, owner: o, rationale: r}
`,
			wantErrs: []string{`field "moved" must be one of in, out, none; got "sideways"`},
		},
		{
			name: "invalid id pattern",
			config: `schema_version: 1
radar: {title: Radar, edition: v1, owner: team}
quadrants:
  - {id: "Bad/Id", name: A}
  - {id: b, name: B}
  - {id: c, name: C}
  - {id: d, name: D}
rings:
  - {id: adopt, name: Adopt}
entries: []
`,
			wantErrs: []string{`quadrants[0]: field "id" "Bad/Id" must match`},
		},
		{
			name:     "empty file",
			config:   "",
			wantErrs: []string{"schema_version: 0 is not supported"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseRadarConfig([]byte(test.config))
			require.Error(t, err)
			for _, want := range test.wantErrs {
				assert.Contains(t, err.Error(), want)
			}
		})
	}
}

func TestParseRadarConfigReportsAllErrorsAtOnce(t *testing.T) {
	config := `schema_version: 3
radar:
  title: Radar
quadrants:
  - {id: a, name: A}
rings: []
entries:
  - {id: e1, name: E1, quadrant: nope, ring: nope, owner: o, rationale: r}
`
	_, err := parseRadarConfig([]byte(config))
	require.Error(t, err)

	for _, want := range []string{
		"schema_version: 3 is not supported",
		`radar: field "edition" must not be empty`,
		"quadrants: exactly 4 quadrants are required, got 1",
		"rings: between 1 and 6 rings are required, got 0",
		`references undeclared quadrant "nope"`,
	} {
		assert.Contains(t, err.Error(), want)
	}
}
