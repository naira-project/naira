package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeGh creates a tiny gh replacement.
func fakeGh(t *testing.T, stdout string, exitCode int) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "gh")
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s' %q
exit %d
`, stdout, exitCode)
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

func attestationJSON(repositoryURL string) string {
	return fmt.Sprintf(`[{"verificationResult":{"signature":{"certificate":{"sourceRepositoryURI":%q}}}}]`, repositoryURL)
}

func TestAttestationVerifier_Verify(t *testing.T) {
	const testGithubOrg = "naira-project"

	tests := []struct {
		name      string
		ghPath    string
		output    string
		exitCode  int
		wantOwner string
		wantName  string
		wantErr   string
	}{
		{
			name:      "returns repository from a valid attestation",
			output:    attestationJSON("https://github.com/naira-project/service"),
			wantOwner: "naira-project",
			wantName:  "service",
		},
		{
			name:      "matches organization case insensitively",
			output:    attestationJSON("https://github.com/Naira-Project/service"),
			wantOwner: "Naira-Project",
			wantName:  "service",
		},
		{
			name:    "rejects a different organization",
			output:  attestationJSON("https://github.com/other-org/service"),
			wantErr: ErrAttestationMissing.Error(),
		},
		{
			name:    "rejects an empty result",
			output:  "[]",
			wantErr: ErrAttestationMissing.Error(),
		},
		{
			name:    "rejects an entry with an empty source repository URI",
			output:  attestationJSON(""),
			wantErr: ErrAttestationMissing.Error(),
		},
		{
			name:    "rejects a non-github source repository URI",
			output:  attestationJSON("https://gitlab.com/naira-project/service"),
			wantErr: ErrAttestationMissing.Error(),
		},
		{
			name:     "returns an error when gh fails",
			exitCode: 1,
			wantErr:  "gh attestation verify failed",
		},
		{
			name:    "returns an error for malformed JSON",
			output:  "not-json",
			wantErr: "parsing gh attestation verify output",
		},
		{
			name:    "returns an error when gh cannot be started",
			ghPath:  filepath.Join("/tmp", "missing-gh"),
			wantErr: "failed to run gh attestation verify for image",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ghPath := tt.ghPath
			if ghPath == "" {
				ghPath = fakeGh(t, tt.output, tt.exitCode)
			}
			verifier := newAttestationVerifier(ghPath, "token", 5*time.Second)

			repo, err := verifier.Verify(t.Context(), "ghcr.io/naira-project/service:latest", testGithubOrg)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantOwner, repo.owner)
			assert.Equal(t, tt.wantName, repo.name)
		})
	}
}
