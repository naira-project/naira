package main

import (
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const supportedSchemaVersion = 1

// defaultWindow is the lookback each project aggregates over when it declares
// no window of its own.
const defaultWindow = 24 * time.Hour

// minWindow guards against windows so short that a slow collect would report a
// mostly-empty bucket.
const minWindow = time.Minute

// idPattern keeps ids usable as node path segments and stable across syncs.
var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// maxIDLength bounds id fields. Ids become node paths, and clipping one would
// corrupt references, so oversized ids fail validation instead.
const maxIDLength = 100

type langfuseConfig struct {
	SchemaVersion int             `yaml:"schema_version"`
	Projects      []projectConfig `yaml:"projects"`

	line int
}

// projectConfig is one Langfuse project and the credentials to read it. Key
// pairs are project-scoped, so every project carries its own.
type projectConfig struct {
	ID string `yaml:"id"`
	// Host is the base URL the plugin calls the API on.
	Host string `yaml:"host"`
	// UIHost is the base URL deep links are built from, for when the browser
	// cannot reach the API address the plugin uses. Defaults to Host.
	UIHost    string        `yaml:"ui_host"`
	ProjectID string        `yaml:"project_id"`
	PublicKey string        `yaml:"public_key"`
	SecretKey string        `yaml:"secret_key"`
	Window    string        `yaml:"window"`
	ScoreName string        `yaml:"score_name"`
	Observes  []observedApp `yaml:"observes"`

	// window holds Window parsed, so callers never re-parse a validated config.
	window time.Duration

	line int
}

// observedApp binds a Langfuse project to the catalog node whose detail page
// shows these metrics. Apps sharing an unscoped project share one set of
// numbers; give each a TraceName to measure them apart.
type observedApp struct {
	Kind      string `yaml:"kind"`
	Path      string `yaml:"path"`
	TraceName string `yaml:"trace_name"`

	line int
}

// These UnmarshalYAML methods reject unknown fields and capture each node's
// line for validation errors. The alias must stay per-method: decoding into
// the original type would recurse forever, and generics cannot derive a
// method-free copy of a struct type.

func (c *langfuseConfig) UnmarshalYAML(node *yaml.Node) error {
	type alias langfuseConfig
	decoded, line, err := decodeStrict[alias](node)
	if err != nil {
		return fmt.Errorf("in langfuse config: %w", err)
	}
	*c = langfuseConfig(decoded)
	c.line = line
	return nil
}

func (p *projectConfig) UnmarshalYAML(node *yaml.Node) error {
	type alias projectConfig
	decoded, line, err := decodeStrict[alias](node)
	if err != nil {
		return fmt.Errorf("in project: %w", err)
	}
	*p = projectConfig(decoded)
	p.line = line
	return nil
}

func (o *observedApp) UnmarshalYAML(node *yaml.Node) error {
	type alias observedApp
	decoded, line, err := decodeStrict[alias](node)
	if err != nil {
		return fmt.Errorf("in observes entry: %w", err)
	}
	*o = observedApp(decoded)
	o.line = line
	return nil
}

// decodeStrict decodes a mapping node into T, rejecting keys matching no
// yaml-tagged field so typos surface with their line instead of being dropped.
// Returns the node's line for validation errors.
func decodeStrict[T any](node *yaml.Node) (T, int, error) {
	var decoded T
	if err := checkKnownFields(node, yamlFieldNames[T]()); err != nil {
		return decoded, 0, fmt.Errorf("checking fields: %w", err)
	}
	if err := node.Decode(&decoded); err != nil {
		return decoded, 0, fmt.Errorf("decoding node: %w", err)
	}
	return decoded, node.Line, nil
}

// yamlFieldNames derives the accepted mapping keys from T's yaml struct tags,
// keeping the struct definition the single source of truth.
func yamlFieldNames[T any]() map[string]bool {
	t := reflect.TypeFor[T]()
	allowed := make(map[string]bool, t.NumField())
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = strings.ToLower(field.Name)
		}
		allowed[name] = true
	}
	return allowed
}

// checkKnownFields rejects mapping keys outside the allowed set. The callers'
// wraps name the mapping the keys belong to.
func checkKnownFields(node *yaml.Node, allowed map[string]bool) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: expected a mapping", node.Line)
	}

	var errs []error
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		if !allowed[key.Value] {
			errs = append(errs, fmt.Errorf("line %d: unknown field %q", key.Line, key.Value))
		}
	}
	return errors.Join(errs...)
}

// parseConfig unmarshals and validates the Langfuse YAML, reporting all
// findings together. Messages name fields, never values: the file holds API
// secrets and its errors reach the operation log.
func parseConfig(data []byte) (*langfuseConfig, error) {
	var cfg langfuseConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}
	// An empty or comment-only file never invokes UnmarshalYAML, so name the
	// real cause rather than reporting "line 0" errors against nothing.
	if cfg.line == 0 {
		return nil, errors.New("config file is empty or contains no YAML mapping")
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}
	return &cfg, nil
}

func (c *langfuseConfig) validate() error {
	var errs []error
	report := func(line int, format string, args ...any) {
		errs = append(errs, fmt.Errorf("line %d: %s", line, fmt.Sprintf(format, args...)))
	}

	if c.SchemaVersion != supportedSchemaVersion {
		report(c.line, "schema_version: %d is not supported (expected %d)", c.SchemaVersion, supportedSchemaVersion)
	}

	// Required so a deliberately empty config ("projects: []") is distinct
	// from a deleted block, which would silently wipe every metrics node.
	if c.Projects == nil {
		report(c.line, "projects: field is required (use [] to disable collection)")
	}

	projectIDs := make(map[string]bool, len(c.Projects))
	for i := range c.Projects {
		p := &c.Projects[i]

		if !idPattern.MatchString(p.ID) {
			report(p.line, "projects[%d]: field \"id\" must match %s", i, idPattern)
		}
		if len(p.ID) > maxIDLength {
			report(p.line, "projects[%d]: field \"id\" must be at most %d characters, got %d", i, maxIDLength, len(p.ID))
		}
		if projectIDs[p.ID] {
			report(p.line, "projects[%d]: duplicate project id %q", i, p.ID)
		}
		projectIDs[p.ID] = true

		for _, field := range []struct{ name, value string }{
			{"host", p.Host},
			{"project_id", p.ProjectID},
			{"public_key", p.PublicKey},
			{"secret_key", p.SecretKey},
		} {
			if strings.TrimSpace(field.value) == "" {
				report(p.line, "projects[%d] (id %q): field %q must not be empty", i, p.ID, field.name)
			}
		}

		for _, field := range []struct{ name, value string }{
			{"host", p.Host},
			{"ui_host", p.UIHost},
		} {
			if field.value == "" {
				continue
			}
			if parsed, err := url.Parse(field.value); err != nil || parsed.Scheme == "" || parsed.Host == "" {
				report(p.line, "projects[%d] (id %q): field %q must be an absolute URL, e.g. https://langfuse.example.com", i, p.ID, field.name)
			}
		}

		if p.UIHost == "" {
			p.UIHost = p.Host
		}

		p.window = defaultWindow
		if p.Window != "" {
			parsed, err := time.ParseDuration(p.Window)
			switch {
			case err != nil:
				report(p.line, "projects[%d] (id %q): field \"window\" is not a duration (e.g. 24h, 30m)", i, p.ID)
			case parsed < minWindow:
				report(p.line, "projects[%d] (id %q): field \"window\" must be at least %s", i, p.ID, minWindow)
			default:
				p.window = parsed
			}
		}

		scopes := make(map[string]bool, len(p.Observes))
		for j, o := range p.Observes {
			for _, field := range []struct{ name, value string }{
				{"kind", o.Kind},
				{"path", o.Path},
			} {
				if strings.TrimSpace(field.value) == "" {
					report(o.line, "projects[%d].observes[%d]: field %q must not be empty", i, j, field.name)
				}
			}
			// Node kinds are a single URL path segment in the catalog API.
			if strings.Contains(o.Kind, "/") {
				report(o.line, "projects[%d].observes[%d]: field \"kind\" must not contain '/'", i, j)
			}
			// The trace name becomes a node path segment when set.
			if strings.Contains(o.TraceName, "/") {
				report(o.line, "projects[%d].observes[%d]: field \"trace_name\" must not contain '/'", i, j)
			}

			key := o.Kind + "\x00" + o.Path
			if scopes[key] {
				report(o.line, "projects[%d].observes[%d]: duplicate binding for %s %q", i, j, o.Kind, o.Path)
			}
			scopes[key] = true
		}
	}

	return errors.Join(errs...)
}

// scopes returns the distinct scopes to query: one per trace name across the
// bindings, where "" means the whole project. Apps sharing a scope share one
// node and one query. An unbound project still yields the whole-project scope,
// so a forgotten binding shows an unattached node rather than nothing.
func (p *projectConfig) scopes() []string {
	if len(p.Observes) == 0 {
		return []string{""}
	}

	seen := make(map[string]bool, len(p.Observes))
	result := make([]string, 0, len(p.Observes))
	for _, o := range p.Observes {
		if seen[o.TraceName] {
			continue
		}
		seen[o.TraceName] = true
		result = append(result, o.TraceName)
	}
	return result
}
