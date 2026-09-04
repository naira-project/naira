package sourcerepository

import (
	"testing"
)

func TestInferGitHubRepository(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		expected string
	}{
		// --- Success Cases ---
		{
			name:     "valid standard image",
			image:    "ghcr.io/owner/repo",
			expected: "https://github.com/owner/repo",
		},
		{
			name:     "valid image with tag",
			image:    "ghcr.io/owner/repo:v1.0.0",
			expected: "https://github.com/owner/repo",
		},
		{
			name:     "valid image with digest",
			image:    "ghcr.io/owner/repo@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			expected: "https://github.com/owner/repo",
		},
		{
			name:     "valid image with tag and digest",
			image:    "ghcr.io/owner/repo:v1.0.0@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			expected: "https://github.com/owner/repo",
		},

		// --- Invalid / Non-GHCR Registries ---
		{
			name:     "docker hub image",
			image:    "docker.io/owner/repo",
			expected: "",
		},
		{
			name:     "short official docker hub image",
			image:    "ubuntu:latest",
			expected: "",
		},
		{
			name:     "custom private registry",
			image:    "registry.example.com/owner/repo",
			expected: "",
		},

		// --- Security & Domain Spoofing Attempts ---
		{
			name:     "subdomain spoofing ghcr.io.evil.com",
			image:    "ghcr.io.evil.com/owner/repo",
			expected: "",
		},
		{
			name:     "prefix spoofing fakeghcr.io",
			image:    "fakeghcr.io/owner/repo",
			expected: "",
		},

		// --- Structure & Subgroup Violations ---
		{
			name:     "missing repository name",
			image:    "ghcr.io/owner",
			expected: "",
		},
		{
			name:     "nested subgroup path (len > 2)",
			image:    "ghcr.io/owner/subgroup/repo",
			expected: "",
		},
		{
			name:     "empty owner and repo",
			image:    "ghcr.io/",
			expected: "",
		},

		// --- Malformed Inputs ---
		{
			name:     "empty string",
			image:    "",
			expected: "",
		},
		{
			name:     "invalid characters in image reference",
			image:    "ghcr.io/owner/repo:tag with spaces",
			expected: "",
		},
		{
			name:     "uppercase registry",
			image:    "GHCR.IO/owner/repo",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inferGitHubRepository(tt.image)
			if got != tt.expected {
				t.Errorf("inferGitHubRepository(%q) = %q; want %q", tt.image, got, tt.expected)
			}
		})
	}
}
