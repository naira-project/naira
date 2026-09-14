package main

import (
	"context"
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
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s' %q\nexit %d\n", stdout, exitCode)
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

func attestationJSON(repositoryURL string) string {
	return fmt.Sprintf(`[{"verificationResult":{"signature":{"certificate":{"sourceRepositoryURI":%q}}}}]`, repositoryURL)
}

func TestAttestationVerifier_Verify(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		exitCode   int
		org        string
		wantOwner  string
		wantName   string
		wantVerify bool
		wantErr    string
	}{
		{
			name:       "returns repository from a valid attestation",
			output:     attestationJSON("https://github.com/naira-project/service"),
			org:        "naira-project",
			wantOwner:  "naira-project",
			wantName:   "service",
			wantVerify: true,
		},
		{
			name:       "matches organization case insensitively",
			output:     attestationJSON("https://github.com/Naira-Project/service"),
			org:        "naira-project",
			wantOwner:  "Naira-Project",
			wantName:   "service",
			wantVerify: true,
		},
		{
			name:   "rejects a different organization",
			output: attestationJSON("https://github.com/other-org/service"),
			org:    "naira-project",
		},
		{
			name:   "rejects an empty result",
			output: "[]",
			org:    "naira-project",
		},
		{
			name:     "returns an error when gh fails",
			exitCode: 1,
			org:      "naira-project",
			wantErr:  "gh attestation verify failed",
		},
		{
			name:    "returns an error for malformed JSON",
			output:  "not-json",
			org:     "naira-project",
			wantErr: "parsing gh attestation verify output",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verifier := newAttestationVerifier(
				fakeGh(t, tt.output, tt.exitCode),
				"token",
				5*time.Second,
			)

			owner, name, verified, err := verifier.Verify(
				context.Background(),
				"ghcr.io/naira-project/service:latest",
				tt.org,
			)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				assert.False(t, verified)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantVerify, verified)
			assert.Equal(t, tt.wantOwner, owner)
			assert.Equal(t, tt.wantName, name)
		})
	}
}
