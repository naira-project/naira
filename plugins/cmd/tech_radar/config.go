package main

import (
	"errors"
	"fmt"
	"regexp"

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

func (c *radarConfig) UnmarshalYAML(node *yaml.Node) error {
	if err := checkKnownFields(node, "radar config", "schema_version", "radar", "quadrants", "rings", "entries"); err != nil {
		return err
	}
	type alias radarConfig
	var decoded alias
	if err := node.Decode(&decoded); err != nil {
		return err
	}
	*c = radarConfig(decoded)
	c.line = node.Line
	return nil
}

func (m *radarMeta) UnmarshalYAML(node *yaml.Node) error {
	if err := checkKnownFields(node, "radar", "id", "title", "edition", "owner"); err != nil {
		return err
	}
	type alias radarMeta
	var decoded alias
	if err := node.Decode(&decoded); err != nil {
		return err
	}
	*m = radarMeta(decoded)
	m.line = node.Line
	return nil
}

func (q *quadrant) UnmarshalYAML(node *yaml.Node) error {
	if err := checkKnownFields(node, "quadrant", "id", "name"); err != nil {
		return err
	}
	type alias quadrant
	var decoded alias
	if err := node.Decode(&decoded); err != nil {
		return err
	}
	*q = quadrant(decoded)
	q.line = node.Line
	return nil
}

func (r *ring) UnmarshalYAML(node *yaml.Node) error {
	if err := checkKnownFields(node, "ring", "id", "name", "description"); err != nil {
		return err
	}
	type alias ring
	var decoded alias
	if err := node.Decode(&decoded); err != nil {
		return err
	}
	*r = ring(decoded)
	r.line = node.Line
	return nil
}

func (e *entry) UnmarshalYAML(node *yaml.Node) error {
	if err := checkKnownFields(node, "entry", "id", "name", "quadrant", "ring", "moved", "owner", "rationale"); err != nil {
		return err
	}
	type alias entry
	var decoded alias
	if err := node.Decode(&decoded); err != nil {
		return err
	}
	*e = entry(decoded)
	e.line = node.Line
	return nil
}

// checkKnownFields rejects mapping keys outside the allowed set so typos
// surface with their line instead of being silently dropped.
func checkKnownFields(node *yaml.Node, context string, allowed ...string) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: %s must be a mapping", node.Line, context)
	}

	allowedKeys := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		allowedKeys[key] = true
	}

	var errs []error
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		if !allowedKeys[key.Value] {
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
		if r.Name == "" {
			report(r.line, "rings[%d] (id %q): field \"name\" must not be empty", i, r.ID)
		}
		if ringIDs[r.ID] {
			report(r.line, "rings[%d]: duplicate ring id %q", i, r.ID)
		}
		ringIDs[r.ID] = true
	}

	entryIDs := make(map[string]bool, len(c.Entries))
	for i, e := range c.Entries {
		if !idPattern.MatchString(e.ID) {
			report(e.line, "entries[%d]: field \"id\" %q must match %s", i, e.ID, idPattern)
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
