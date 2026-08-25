// Package util parses configuration shapes that plugins share.
package util

import (
	"fmt"
	"strings"
)

// NamedValue is one "name=value" entry.
type NamedValue struct {
	Name  string
	Value string
}

// ParseNamedValues parses a comma-separated "name=value,name=value" setting,
// the shape several plugins use for their endpoint configuration.
func ParseNamedValues(raw string) ([]NamedValue, error) {
	var values []NamedValue
	seen := make(map[string]struct{})

	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			return nil, fmt.Errorf("invalid entry %q: must be in name=value format", entry)
		}
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name == "" || value == "" {
			return nil, fmt.Errorf("invalid entry %q: name and value must not be empty", entry)
		}
		if strings.Contains(name, "/") {
			return nil, fmt.Errorf("invalid name %q: must not contain '/'", name)
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("duplicate name %q", name)
		}
		seen[name] = struct{}{}

		values = append(values, NamedValue{Name: name, Value: value})
	}

	return values, nil
}
