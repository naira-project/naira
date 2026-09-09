package main

import (
	"bufio"
	"strings"
)

// parseCodeowners extracts the default owners (the "*" pattern) from a CODEOWNERS file,
// ignoring path-specific rules, inline comments, and duplicates.
func parseCodeowners(content string) []string {
	var defaultOwners []string

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		pattern := fields[0]
		if pattern != "*" {
			continue
		}

		defaultOwners = append(defaultOwners, fields[1:]...)
	}

	return dedupeStrings(defaultOwners)
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
