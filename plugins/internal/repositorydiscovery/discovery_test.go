package repositorydiscovery

import "testing"

func TestParseGitHubRepository(t *testing.T) {
	tests := []struct {
		name  string
		input string
		owner string
		repo  string
		ok    bool
	}{
		{name: "https URL", input: "https://github.com/acme/service", owner: "acme", repo: "service", ok: true},
		{name: "git suffix", input: "https://github.com/acme/service.git", owner: "acme", repo: "service", ok: true},
		{name: "scp URL", input: "git@github.com:acme/service.git", owner: "acme", repo: "service", ok: true},
		{name: "host without scheme", input: "github.com/acme/service", owner: "acme", repo: "service", ok: true},
		{name: "other host", input: "https://gitlab.com/acme/service", ok: false},
		{name: "different owner", input: "https://github.com/other/service", owner: "other", repo: "service", ok: true},
		{name: "missing repository", input: "https://github.com/acme", ok: false},
		{name: "nested path", input: "https://github.com/acme/platform/service", ok: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			owner, repo, ok := ParseGitHubRepository(test.input)
			if owner != test.owner || repo != test.repo || ok != test.ok {
				t.Fatalf("ParseGitHubRepository(%q) = (%q, %q, %t), want (%q, %q, %t)", test.input, owner, repo, ok, test.owner, test.repo, test.ok)
			}
		})
	}
}
