package repositoryidentity

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseGitHubRepository(t *testing.T) {
	tests := []struct {
		name      string
		rawURL    string
		wantOwner string
		wantName  string
		wantOK    bool
	}{
		// --- Valid cases ---
		{
			name:      "Standard HTTPS",
			rawURL:    "https://github.com/octocat/Hello-World",
			wantOwner: "octocat",
			wantName:  "Hello-World",
			wantOK:    true,
		},
		{
			name:      "HTTPS with .git suffix",
			rawURL:    "https://github.com/octocat/Hello-World.git",
			wantOwner: "octocat",
			wantName:  "Hello-World",
			wantOK:    true,
		},
		{
			name:      "Missing scheme (defaults to HTTPS)",
			rawURL:    "github.com/octocat/Hello-World",
			wantOwner: "octocat",
			wantName:  "Hello-World",
			wantOK:    true,
		},
		{
			name:      "SSH SCP-like syntax",
			rawURL:    "git@github.com:octocat/Hello-World.git",
			wantOwner: "octocat",
			wantName:  "Hello-World",
			wantOK:    true,
		},
		{
			name:      "SSH URL",
			rawURL:    "ssh://git@github.com/octocat/Hello-World.git",
			wantOwner: "octocat",
			wantName:  "Hello-World",
			wantOK:    true,
		},
		{
			name:      "Valid owner with a single hyphen",
			rawURL:    "https://github.com/good-handle/repo",
			wantOwner: "good-handle",
			wantName:  "repo",
			wantOK:    true,
		},
		{
			name:      "Owner with multiple non-consecutive hyphens",
			rawURL:    "https://github.com/not-allowed-handle/repo",
			wantOwner: "not-allowed-handle",
			wantName:  "repo",
			wantOK:    true,
		},
		{
			name:      "Valid repo name with . _ - characters",
			rawURL:    "https://github.com/owner/my.repo_name-v1",
			wantOwner: "owner",
			wantName:  "my.repo_name-v1",
			wantOK:    true,
		},

		// --- Owner (Handle) validation ---
		{
			name:   "Owner with consecutive hyphens (invalid)",
			rawURL: "https://github.com/octo--cat/repo",
			wantOK: false,
		},
		{
			name:   "Owner starting with a hyphen",
			rawURL: "https://github.com/-notallowed/repo",
			wantOK: false,
		},
		{
			name:   "Owner ending with a hyphen",
			rawURL: "https://github.com/notallowed-/repo",
			wantOK: false,
		},
		{
			name:   "Owner with special characters",
			rawURL: "https://github.com/owner_name/repo",
			wantOK: false,
		},
		{
			name:   "Owner exceeding 39 characters",
			rawURL: "https://github.com/" + strings.Repeat("a", 40) + "/repo",
			wantOK: false,
		},

		// --- Repository name validation ---
		{
			name:   "Repo dot (.)",
			rawURL: "https://github.com/owner/.",
			wantOK: false,
		},
		{
			name:   "Repo double dot (..)",
			rawURL: "https://github.com/owner/..",
			wantOK: false,
		},
		{
			name:   "Repo with invalid special characters ($/!)",
			rawURL: "https://github.com/owner/repo$name",
			wantOK: false,
		},
		{
			name:   "Repo exceeding 100 characters",
			rawURL: "https://github.com/owner/" + strings.Repeat("a", 101),
			wantOK: false,
		},

		// --- Invalid URLs ---
		{
			name:   "Empty input",
			rawURL: "",
			wantOK: false,
		},
		{
			name:   "Invalid domain",
			rawURL: "https://gitlab.com/owner/repo",
			wantOK: false,
		},
		{
			name:   "URL with port",
			rawURL: "https://github.com:8080/owner/repo",
			wantOK: false,
		},
		{
			name:   "URL with query parameters",
			rawURL: "https://github.com/owner/repo?ref=main",
			wantOK: false,
		},
		{
			name:   "SSH URL with a non-git user",
			rawURL: "ssh://alice@github.com/owner/repo",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOwner, gotName, gotOK := ParseGitHubRepository(tt.rawURL)

			assert.Equal(t, tt.wantOK, gotOK)
			assert.Equal(t, tt.wantOwner, gotOwner)
			assert.Equal(t, tt.wantName, gotName)
		})
	}
}
