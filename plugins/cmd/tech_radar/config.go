package main

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const supportedSchemaVersion = 1

const defaultRadarID = "default"

const (
	movedIn   = "in"
	movedOut  = "out"
	movedNone = "none"
)

// idPattern keeps ids usable as node path segments and stable across editions.
var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// maxIDLength bounds id fields. Ids become node paths and are never clipped —
// clipping would corrupt references — so oversized ids fail validation.
const maxIDLength = 100

type radarConfig struct {
	SchemaVersion int        `yaml:"schema_version"`
	Radar         radarMeta  `yaml:"radar"`
	Quadrants     []quadrant `yaml:"quadrants"`
	Rings         []ring     `yaml:"rings"`
	Entries       []entry    `yaml:"entries"`

	line int
}

type radarMeta struct {
	ID      string `yaml:"id"`
	Title   string `yaml:"title"`
	Edition string `yaml:"edition"`
	Owner   string `yaml:"owner"`

	line int
}

type quadrant struct {
	ID   string `yaml:"id" json:"id"`
	Name string `yaml:"name" json:"name"`

	line int
}

type ring struct {
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`

	line int
}

type entry struct {
	ID        string `yaml:"id"`
	Name      string `yaml:"name"`
	Quadrant  string `yaml:"quadrant"`
	Ring      string `yaml:"ring"`
	Moved     string `yaml:"moved"`
	Owner     string `yaml:"owner"`
	Rationale string `yaml:"rationale"`

	line int
}

// The UnmarshalYAML methods exist to reject unknown fields and to capture
// each node's line for validation errors; decodeStrict does the heavy
// lifting. The alias declarations must stay per-method: decoding into the
// original type would recurse into UnmarshalYAML forever, and Go generics
// cannot derive a method-free copy of a struct type.

func (c *radarConfig) UnmarshalYAML(node *yaml.Node) error {
	type alias radarConfig
	decoded, line, err := decodeStrict[alias](node, "radar config")
	if err != nil {
		return err
	}
	*c = radarConfig(decoded)
	c.line = line
	return nil
}

func (m *radarMeta) UnmarshalYAML(node *yaml.Node) error {
	type alias radarMeta
	decoded, line, err := decodeStrict[alias](node, "radar")
	if err != nil {
		return err
	}
	*m = radarMeta(decoded)
	m.line = line
	return nil
}

func (q *quadrant) UnmarshalYAML(node *yaml.Node) error {
	type alias quadrant
	decoded, line, err := decodeStrict[alias](node, "quadrant")
	if err != nil {
		return err
	}
	*q = quadrant(decoded)
	q.line = line
	return nil
}

func (r *ring) UnmarshalYAML(node *yaml.Node) error {
	type alias ring
	decoded, line, err := decodeStrict[alias](node, "ring")
	if err != nil {
		return err
	}
	*r = ring(decoded)
	r.line = line
	return nil
}

func (e *entry) UnmarshalYAML(node *yaml.Node) error {
	type alias entry
	decoded, line, err := decodeStrict[alias](node, "entry")
	if err != nil {
		return err
	}
	*e = entry(decoded)
	e.line = line
	return nil
}

// decodeStrict decodes a mapping node into T, rejecting keys that don't match
// any yaml-tagged field of T so typos surface with their line instead of
// being silently dropped. It returns the node's line for validation errors.
func decodeStrict[T any](node *yaml.Node, context string) (T, int, error) {
	var decoded T
	if err := checkKnownFields(node, context, yamlFieldNames[T]()); err != nil {
		return decoded, 0, fmt.Errorf("checking %s fields: %w", context, err)
	}
	if err := node.Decode(&decoded); err != nil {
		return decoded, 0, fmt.Errorf("decoding %s: %w", context, err)
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

// checkKnownFields rejects mapping keys outside the allowed set.
func checkKnownFields(node *yaml.Node, context string, allowed map[string]bool) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: %s must be a mapping", node.Line, context)
	}

	var errs []error
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		if !allowed[key.Value] {
			errs = append(errs, fmt.Errorf("line %d: unknown field %q in %s", key.Line, key.Value, context))
		}
	}
	return errors.Join(errs...)
}

// parseRadarConfig unmarshals and validates the radar YAML. All validation
// findings are reported together, each naming the offending line and field.
func parseRadarConfig(data []byte) (*radarConfig, error) {
	var cfg radarConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}
	// An empty or comment-only file never invokes UnmarshalYAML, leaving the
	// zero value behind. Name the real cause instead of reporting "line 0"
	// validation errors against a document that does not exist.
	if cfg.line == 0 {
		return nil, errors.New("config file is empty or contains no YAML mapping")
	}

	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}
	return &cfg, nil
}

func (c *radarConfig) applyDefaults() {
	if c.Radar.ID == "" {
		c.Radar.ID = defaultRadarID
	}
	for i := range c.Entries {
		if c.Entries[i].Moved == "" {
			c.Entries[i].Moved = movedNone
		}
	}
}

func (c *radarConfig) validate() error {
	var errs []error
	report := func(line int, format string, args ...any) {
		errs = append(errs, fmt.Errorf("line %d: %s", line, fmt.Sprintf(format, args...)))
	}

	if c.SchemaVersion != supportedSchemaVersion {
		report(c.line, "schema_version: %d is not supported (expected %d)", c.SchemaVersion, supportedSchemaVersion)
	}

	metaLine := c.Radar.line
	if metaLine == 0 {
		metaLine = c.line
	}
	if !idPattern.MatchString(c.Radar.ID) {
		report(metaLine, "radar: field \"id\" %q must match %s", c.Radar.ID, idPattern)
	}
	if len(c.Radar.ID) > maxIDLength {
		report(metaLine, "radar: field \"id\" must be at most %d characters, got %d", maxIDLength, len(c.Radar.ID))
	}
	for _, field := range []struct{ name, value string }{
		{"title", c.Radar.Title}, {"edition", c.Radar.Edition}, {"owner", c.Radar.Owner},
	} {
		if field.value == "" {
			report(metaLine, "radar: field %q must not be empty", field.name)
		}
	}

	if len(c.Quadrants) != 4 {
		report(c.line, "quadrants: exactly 4 quadrants are required, got %d", len(c.Quadrants))
	}
	quadrantIDs := make(map[string]bool, len(c.Quadrants))
	for i, q := range c.Quadrants {
		if !idPattern.MatchString(q.ID) {
			report(q.line, "quadrants[%d]: field \"id\" %q must match %s", i, q.ID, idPattern)
		}
		if len(q.ID) > maxIDLength {
			report(q.line, "quadrants[%d]: field \"id\" must be at most %d characters, got %d", i, maxIDLength, len(q.ID))
		}
		if q.Name == "" {
			report(q.line, "quadrants[%d] (id %q): field \"name\" must not be empty", i, q.ID)
		}
		if quadrantIDs[q.ID] {
			report(q.line, "quadrants[%d]: duplicate quadrant id %q", i, q.ID)
		}
		quadrantIDs[q.ID] = true
	}

	if len(c.Rings) < 1 || len(c.Rings) > 6 {
		report(c.line, "rings: between 1 and 6 rings are required, got %d", len(c.Rings))
	}
	ringIDs := make(map[string]bool, len(c.Rings))
	for i, r := range c.Rings {
		if !idPattern.MatchString(r.ID) {
			report(r.line, "rings[%d]: field \"id\" %q must match %s", i, r.ID, idPattern)
		}
		if len(r.ID) > maxIDLength {
			report(r.line, "rings[%d]: field \"id\" must be at most %d characters, got %d", i, maxIDLength, len(r.ID))
		}
		if r.Name == "" {
			report(r.line, "rings[%d] (id %q): field \"name\" must not be empty", i, r.ID)
		}
		if ringIDs[r.ID] {
			report(r.line, "rings[%d]: duplicate ring id %q", i, r.ID)
		}
		ringIDs[r.ID] = true
	}

	// The entries key is required: distinguishing a deliberately empty radar
	// ("entries: []") from an accidentally deleted block keeps a valid-looking
	// sync from silently wiping every entry node.
	if c.Entries == nil {
		report(c.line, "entries: field is required (use [] for a radar without entries)")
	}
	entryIDs := make(map[string]bool, len(c.Entries))
	for i, e := range c.Entries {
		if !idPattern.MatchString(e.ID) {
			report(e.line, "entries[%d]: field \"id\" %q must match %s", i, e.ID, idPattern)
		}
		if len(e.ID) > maxIDLength {
			report(e.line, "entries[%d]: field \"id\" must be at most %d characters, got %d", i, maxIDLength, len(e.ID))
		}
		if entryIDs[e.ID] {
			report(e.line, "entries[%d]: duplicate entry id %q", i, e.ID)
		}
		entryIDs[e.ID] = true

		for _, field := range []struct{ name, value string }{
			{"name", e.Name}, {"owner", e.Owner}, {"rationale", e.Rationale},
		} {
			if field.value == "" {
				report(e.line, "entries[%d] (id %q): field %q must not be empty", i, e.ID, field.name)
			}
		}
		if !quadrantIDs[e.Quadrant] {
			report(e.line, "entries[%d] (id %q): field \"quadrant\" references undeclared quadrant %q", i, e.ID, e.Quadrant)
		}
		if !ringIDs[e.Ring] {
			report(e.line, "entries[%d] (id %q): field \"ring\" references undeclared ring %q", i, e.ID, e.Ring)
		}
		if e.Moved != movedIn && e.Moved != movedOut && e.Moved != movedNone {
			report(e.line, "entries[%d] (id %q): field \"moved\" must be one of in, out, none; got %q", i, e.ID, e.Moved)
		}
	}

	return errors.Join(errs...)
}
