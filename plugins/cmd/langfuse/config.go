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
	Kind string `yaml:"kind"`
	// PathPrefix replaces the cluster id as the node path's first segment, for
	// nodes a plugin keys by its own PATH_PREFIX instead, e.g. litellm apps.
	PathPrefix string `yaml:"path_prefix"`
	Namespace  string `yaml:"namespace"`
	Name       string `yaml:"name"`
	TraceName  string `yaml:"trace_name"`

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
		return safeError{fmt.Errorf("in langfuse config: %w", err)}
	}
	*c = langfuseConfig(decoded)
	c.line = line
	return nil
}

func (p *projectConfig) UnmarshalYAML(node *yaml.Node) error {
	type alias projectConfig
	decoded, line, err := decodeStrict[alias](node)
	if err != nil {
		return safeError{fmt.Errorf("in project: %w", err)}
	}
	*p = projectConfig(decoded)
	p.line = line
	return nil
}

func (o *observedApp) UnmarshalYAML(node *yaml.Node) error {
	type alias observedApp
	decoded, line, err := decodeStrict[alias](node)
	if err != nil {
		return safeError{fmt.Errorf("in observes entry: %w", err)}
	}
	*o = observedApp(decoded)
	o.line = line
	return nil
}

// safeError marks an error whose message holds only field names and lines,
// never config values, so it may pass through decodeStrict unchanged.
type safeError struct{ error }

func (e safeError) Unwrap() error { return e.error }

// decodeStrict decodes a mapping node into T one field at a time, rejecting
// unknown and duplicate keys so typos surface with their line. Returns the
// node's line for validation errors.
func decodeStrict[T any](node *yaml.Node) (T, int, error) {
	var decoded T
	if node.Kind != yaml.MappingNode {
		return decoded, 0, fmt.Errorf("line %d: expected a mapping", node.Line)
	}

	fields := yamlFields[T]()
	target := reflect.ValueOf(&decoded).Elem()
	seen := make(map[string]bool, len(fields))

	var errs []error
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]

		index, ok := fields[key.Value]
		switch {
		case !ok:
			errs = append(errs, fmt.Errorf("line %d: unknown field %q", key.Line, key.Value))
			continue
		case seen[key.Value]:
			errs = append(errs, fmt.Errorf("line %d: duplicate field %q", key.Line, key.Value))
			continue
		}
		seen[key.Value] = true

		if err := value.Decode(target.Field(index).Addr().Interface()); err != nil {
			var safe safeError
			if errors.As(err, &safe) {
				errs = append(errs, err)
				continue
			}
			// Not wrapped: yaml.v3 quotes the offending scalar, which may be a secret.
			errs = append(errs, fmt.Errorf("line %d: field %q: invalid value", value.Line, key.Value))
		}
	}
	return decoded, node.Line, errors.Join(errs...)
}

// yamlFields maps the accepted mapping keys to T's field indexes, derived from
// its yaml struct tags so the struct definition stays the single source of
// truth.
func yamlFields[T any]() map[string]int {
	t := reflect.TypeFor[T]()
	fields := make(map[string]int, t.NumField())
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
		fields[name] = i
	}
	return fields
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
				{"namespace", o.Namespace},
				{"name", o.Name},
			} {
				if strings.TrimSpace(field.value) == "" {
					report(o.line, "projects[%d].observes[%d]: field %q must not be empty", i, j, field.name)
				}
			}
			// Each of these is a single segment of a node path or kind.
			for _, field := range []struct{ name, value string }{
				{"kind", o.Kind},
				{"path_prefix", o.PathPrefix},
				{"namespace", o.Namespace},
				{"name", o.Name},
				{"trace_name", o.TraceName},
			} {
				if strings.Contains(field.value, "/") {
					report(o.line, "projects[%d].observes[%d]: field %q must not contain '/'", i, j, field.name)
				}
			}

			key := o.Kind + "\x00" + o.PathPrefix + "\x00" + o.Namespace + "\x00" + o.Name
			if scopes[key] {
				report(o.line, "projects[%d].observes[%d]: duplicate binding for %s %q", i, j, o.Kind, o.Namespace+"/"+o.Name)
			}
			scopes[key] = true
		}
	}

	return errors.Join(errs...)
}

// needsClusterID reports whether any binding falls back to the cluster id as
// its path prefix.
func (c *langfuseConfig) needsClusterID() bool {
	for _, p := range c.Projects {
		for _, o := range p.Observes {
			if o.PathPrefix == "" {
				return true
			}
		}
	}
	return false
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
