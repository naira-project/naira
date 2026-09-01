package repositorydiscovery

import "testing"

func TestParseGitHubRepository(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantOwner string
		wantRepo  string
		wantOK    bool
	}{
		// valid cases
		{"https", "https://github.com/acme/service", "acme", "service", true},
		{"http", "http://github.com/acme/service", "acme", "service", true},
		{"git suffix", "https://github.com/acme/service.git", "acme", "service", true},
		{"scp", "git@github.com:acme/service.git", "acme", "service", true},
		{"scp without .git", "git@github.com:acme/service", "acme", "service", true},
		{"missing scheme", "github.com/acme/service", "acme", "service", true},
		{"dots and underscores in repo", "https://github.com/acme/my.service_v2", "acme", "my.service_v2", true},
		{"whitespace padding", "  https://github.com/acme/service.git\n", "acme", "service", true},

		// host / spoofing
		{"different host", "https://gitlab.com/acme/service", "", "", false},
		{"subdomain spoofing", "https://github.com.evil.com/acme/service", "", "", false},
		{"IP instead of host", "https://192.168.1.1/acme/service", "", "", false},

		// injections
		{"query string", "https://github.com/acme/service?x=1", "", "", false},
		{"fragment", "https://github.com/acme/service#x", "", "", false},
		{"port", "https://github.com:8443/acme/service", "", "", false},
		{"credentials", "https://admin:secret@github.com/acme/service", "", "", false},
		{"user without password", "https://git@github.com/acme/service", "", "", false},

		// path traversal / path structure
		{"traversal as repo", "https://github.com/acme/..", "", "", false},
		{"dot as repo", "https://github.com/acme/.", "", "", false},
		{"too many segments", "https://github.com/acme/platform/service", "", "", false},
		{"too few segments", "https://github.com/acme", "", "", false},
		{"empty segments", "https://github.com//", "", "", false},
		{"scp traversal", "git@github.com:acme/../service.git", "", "", false},
		{"scp too many segments", "git@github.com:acme/repo/extra.git", "", "", false},

		// forbidden characters
		{"null byte", "https://github.com/acme/service\x00.git", "", "", false},

		// schemes
		{"file scheme", "file:///acme/service", "", "", false},
		{"javascript scheme", "javascript://github.com/acme/service", "", "", false},
		{"ssh scheme", "ssh://git@github.com/acme/service.git", "", "", false},

		// junk input
		{"malformed", ":::not-a-url:::", "", "", false},
		{"empty", "", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, ok := parseGitHubRepository(tt.input)
			if owner != tt.wantOwner || repo != tt.wantRepo || ok != tt.wantOK {
				t.Errorf("parseGitHubRepository(%q) = (%q, %q, %t), want (%q, %q, %t)",
					tt.input, owner, repo, ok, tt.wantOwner, tt.wantRepo, tt.wantOK)
			}
		})
	}
}
