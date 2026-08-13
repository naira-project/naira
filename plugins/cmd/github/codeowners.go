package main

import (
	"bufio"
	"strings"
)

// parseCodeowners extracts owners from a CODEOWNERS file.
//
// We only care about the *default* owners for now (the "*" pattern), since
// that's the one directly answerable question we want out of this: "who do
// I contact about this repository as a whole?". Per-path ownership rules
// exist in real CODEOWNERS files but modelling per-path ownership in the
// catalog graph is a different (and much bigger) problem than what a
// software-catalog overview needs, so it's intentionally out of scope here.
//
// CODEOWNERS syntax: lines are "<pattern> <owner> [<owner> ...]", '#'
// starts a comment, blank lines are ignored. Owners are either
// "@user", "@org/team", or an email address.
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
