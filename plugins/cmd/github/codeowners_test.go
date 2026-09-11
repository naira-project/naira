package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseCodeowners(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "empty input content",
			content:  "",
			expected: []string{},
		},
		{
			name: "only comments and empty lines",
			content: `
# This is a comment
  # Indented comment

# Another comment with empty line above
`,
			expected: []string{},
		},
		{
			name: "single wildcard rule with one owner",
			content: `
* @global-owner
`,
			expected: []string{"@global-owner"},
		},
		{
			name: "single wildcard rule with multiple owners",
			content: `
* @owner1 @owner2 user@example.com
`,
			expected: []string{"@owner1", "@owner2", "user@example.com"},
		},
		{
			name: "ignore path-specific rules, extract only global asterisk rules",
			content: `
/docs/ @docs-team
*.go @golang-team
* @global-owner
/scripts/ @devops-team
`,
			expected: []string{"@global-owner"},
		},
		{
			name: "multiple wildcard lines combined and deduped",
			content: `
* @global-owner @backend-team
* @backend-team @frontend-team
`,
			expected: []string{"@global-owner", "@backend-team", "@frontend-team"},
		},
		{
			name: "wildcard line without owners ignored",
			content: `
# Pattern without owners
*
* @valid-owner
`,
			expected: []string{"@valid-owner"},
		},
		{
			name:     "inline whitespace, tabs, and Windows CRLF endings",
			content:  "*\t@tabbed-owner-1\t@tabbed-owner-2\r\n*    @spaced-owner\r\n",
			expected: []string{"@tabbed-owner-1", "@tabbed-owner-2", "@spaced-owner"},
		},
		{
			name: "duplicate owners on the same wildcard line",
			content: `
* @alice @bob @alice @alice
`,
			expected: []string{"@alice", "@bob"},
		},
		{
			name: "non-asterisk path wildcards are ignored",
			content: `
* @naira-project/dev @naira-project/maintainer
/.github/* @naira-project/auto
`,
			expected: []string{"@naira-project/dev", "@naira-project/maintainer"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractDefaultCodeowners(tt.content)

			assert.Equal(t, tt.expected, got)
		})
	}
}
