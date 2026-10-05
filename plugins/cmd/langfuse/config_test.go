package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleConfig(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("sample_config.yaml"))
	require.NoError(t, err)
	return string(data)
}

const minimalConfig = `
schema_version: 1
projects:
  - id: demo
    host: https://langfuse.example.com
    project_id: cm0demo
    public_key: pk-lf-demo
    secret_key: sk-lf-demo
`

func TestParseConfigAcceptsSample(t *testing.T) {
	cfg, err := parseConfig([]byte(sampleConfig(t)))
	require.NoError(t, err)

	require.Len(t, cfg.Projects, 2)
	assert.Equal(t, "rag-assistant", cfg.Projects[0].ID)
	assert.Equal(t, 24*time.Hour, cfg.Projects[0].window)
	assert.Equal(t, 168*time.Hour, cfg.Projects[1].window)
}

func TestParseConfigDefaultsWindow(t *testing.T) {
	cfg, err := parseConfig([]byte(minimalConfig))
	require.NoError(t, err)

	assert.Equal(t, defaultWindow, cfg.Projects[0].window)
}

// The deep link is followed by a browser, which may not reach the address the
// plugin calls the API on.
func TestParseConfigDefaultsUIHostToHost(t *testing.T) {
	cfg, err := parseConfig([]byte(minimalConfig))
	require.NoError(t, err)

	assert.Equal(t, "https://langfuse.example.com", cfg.Projects[0].UIHost)
}

func TestParseConfigKeepsSeparateUIHost(t *testing.T) {
	cfg, err := parseConfig([]byte(`
schema_version: 1
projects:
  - id: demo
    host: http://langfuse-web.langfuse.svc.cluster.local:3000
    ui_host: http://localhost:3000
    project_id: cm0demo
    public_key: pk-lf-demo
    secret_key: sk-lf-demo
`))
	require.NoError(t, err)

	project := cfg.Projects[0]
	assert.Equal(t, "http://langfuse-web.langfuse.svc.cluster.local:3000", project.Host)
	assert.Equal(t, "http://localhost:3000", project.UIHost)
	assert.Equal(t, "http://localhost:3000/project/cm0demo/traces", tracesURL(project))
}

func TestParseConfigRejectsRelativeUIHost(t *testing.T) {
	_, err := parseConfig([]byte(`
schema_version: 1
projects:
  - id: demo
    host: https://langfuse.example.com
    ui_host: /langfuse
    project_id: cm0demo
    public_key: pk-lf-demo
    secret_key: sk-lf-demo
`))
	assert.ErrorContains(t, err, `field "ui_host" must be an absolute URL`)
}

func TestParseConfigRejectsEmptyFile(t *testing.T) {
	_, err := parseConfig([]byte("# only a comment\n"))
	assert.ErrorContains(t, err, "empty or contains no YAML mapping")
}

func TestParseConfigRejectsUnknownField(t *testing.T) {
	_, err := parseConfig([]byte(`
schema_version: 1
projects:
  - id: demo
    host: https://langfuse.example.com
    project_id: cm0demo
    public_key: pk-lf-demo
    secret_key: sk-lf-demo
    windwo: 24h
`))
	assert.ErrorContains(t, err, `unknown field "windwo"`)
}

func TestParseConfigRejectsMissingProjectsKey(t *testing.T) {
	_, err := parseConfig([]byte("schema_version: 1\n"))
	assert.ErrorContains(t, err, "projects: field is required")
}

func TestParseConfigAcceptsExplicitlyEmptyProjects(t *testing.T) {
	cfg, err := parseConfig([]byte("schema_version: 1\nprojects: []\n"))
	require.NoError(t, err)
	assert.Empty(t, cfg.Projects)
}

func TestParseConfigRejectsUnsupportedSchemaVersion(t *testing.T) {
	_, err := parseConfig([]byte("schema_version: 2\nprojects: []\n"))
	assert.ErrorContains(t, err, "schema_version: 2 is not supported")
}

func TestParseConfigReportsEveryFinding(t *testing.T) {
	_, err := parseConfig([]byte(`
schema_version: 1
projects:
  - id: Bad_ID
    host: not-a-url
    project_id: ""
    public_key: pk-lf-demo
    secret_key: sk-lf-demo
    window: yesterday
`))
	require.Error(t, err)

	message := err.Error()
	assert.Contains(t, message, `field "id" must match`)
	assert.Contains(t, message, `field "host" must be an absolute URL`)
	assert.Contains(t, message, `field "project_id" must not be empty`)
	assert.Contains(t, message, `field "window" is not a duration`)
}

// The config file holds API secrets, so its errors must not quote values.
func TestParseConfigErrorsDoNotLeakSecrets(t *testing.T) {
	_, err := parseConfig([]byte(`
schema_version: 1
projects:
  - id: demo
    host: not-a-url
    project_id: cm0demo
    public_key: pk-lf-supersecret
    secret_key: sk-lf-supersecret
    window: yesterday
`))
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "supersecret")
}

func TestParseConfigRejectsDuplicateProjectID(t *testing.T) {
	_, err := parseConfig([]byte(`
schema_version: 1
projects:
  - id: demo
    host: https://langfuse.example.com
    project_id: cm0demo
    public_key: pk-lf-demo
    secret_key: sk-lf-demo
  - id: demo
    host: https://langfuse.example.com
    project_id: cm0other
    public_key: pk-lf-other
    secret_key: sk-lf-other
`))
	assert.ErrorContains(t, err, `duplicate project id "demo"`)
}

func TestParseConfigRejectsWindowBelowMinimum(t *testing.T) {
	_, err := parseConfig([]byte(`
schema_version: 1
projects:
  - id: demo
    host: https://langfuse.example.com
    project_id: cm0demo
    public_key: pk-lf-demo
    secret_key: sk-lf-demo
    window: 1s
`))
	assert.ErrorContains(t, err, `field "window" must be at least`)
}

func TestParseConfigRejectsSeparatorsInPathSegments(t *testing.T) {
	_, err := parseConfig([]byte(`
schema_version: 1
projects:
  - id: demo
    host: https://langfuse.example.com
    project_id: cm0demo
    public_key: pk-lf-demo
    secret_key: sk-lf-demo
    observes:
      - kind: deployment/v1
        path: ns/app
        trace_name: a/b
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `field "kind" must not contain '/'`)
	assert.Contains(t, err.Error(), `field "trace_name" must not contain '/'`)
}

// Forgetting to bind a project should surface an unattached metrics node, not
// silently collect nothing.
func TestScopesCoverAnUnboundProject(t *testing.T) {
	cfg, err := parseConfig([]byte(minimalConfig))
	require.NoError(t, err)

	assert.Equal(t, []string{""}, cfg.Projects[0].scopes())
}

func TestScopesAreDistinctAndOrdered(t *testing.T) {
	cfg, err := parseConfig([]byte(`
schema_version: 1
projects:
  - id: demo
    host: https://langfuse.example.com
    project_id: cm0demo
    public_key: pk-lf-demo
    secret_key: sk-lf-demo
    observes:
      - kind: deployment
        path: ns/first
        trace_name: agent
      - kind: deployment
        path: ns/second
        trace_name: agent
      - kind: deployment
        path: ns/third
`))
	require.NoError(t, err)

	assert.Equal(t, []string{"agent", ""}, cfg.Projects[0].scopes())
}
