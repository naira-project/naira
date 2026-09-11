package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/naira-project/naira/plugins/internal/repositoryidentity"
)

// attestationVerifier verifies GitHub artifact attestations for container
// images by shelling out to the `gh` CLI (`gh attestation verify`).
type attestationVerifier struct {
	ghPath  string
	token   string
	timeout time.Duration
}

func newAttestationVerifier(ghPath, token string, timeout time.Duration) *attestationVerifier {
	return &attestationVerifier{ghPath: ghPath, token: token, timeout: timeout}
}

// `gh attestation verify --format json` output
type ghAttestationEntry struct {
	VerificationResult struct {
		Signature struct {
			Certificate ghCertificate `json:"certificate"`
		} `json:"signature"`
	} `json:"verificationResult"`
}

// signature.certificate is populated directly from the OpenID Connect token that GitHub has generated
// so it's properties contain values that cannot be manipulated by the workflow that originated the attestation.
// see more "gh help attestation verify"
type ghCertificate struct {
	SourceRepositoryURI      string `json:"sourceRepositoryURI"`
	SourceRepositoryOwnerURI string `json:"sourceRepositoryOwnerURI"`
}

// Verify checks whether image has a valid, trusted GitHub artifact
// attestation whose source repository belongs to org, and if so returns
// that repository's owner and name.
func (v *attestationVerifier) Verify(ctx context.Context, image, org string) (owner, name string, verified bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, v.timeout)
	defer cancel()

	// passing gh an oci:// reference (rather than a resolved digest) means
	// gh itself talks to the registry to resolve the digest and fetch
	// the attestation bundle

	// Question: should I use imageID from k8s?
	cmd := exec.CommandContext(ctx, v.ghPath, "attestation", "verify",
		"oci://"+image,
		"--owner", org,
		"--format", "json",
	)
	if v.token != "" {
		cmd.Env = append(cmd.Env, "GH_TOKEN="+v.token)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if runErr := cmd.Run(); runErr != nil {
		if _, ok := errors.AsType[*exec.ExitError](runErr); ok {
			// the command starts and finishes but verification fails
			return "", "", false, nil
		}
		// anything else (gh binary missing, ctx cancelled ...)
		return "", "", false, fmt.Errorf("running gh attestation verify: %w (stderr: %s)", runErr, strings.TrimSpace(stderr.String()))
	}

	var entries []ghAttestationEntry
	if err := json.Unmarshal(stdout.Bytes(), &entries); err != nil {
		return "", "", false, fmt.Errorf("parsing gh attestation verify output: %w", err)
	}

	for _, entry := range entries {
		cert := entry.VerificationResult.Signature.Certificate
		if cert.SourceRepositoryURI == "" {
			continue
		}
		ownerFromCert, nameFromCert, ok := repositoryidentity.ParseGitHubRepository(cert.SourceRepositoryURI)
		if !ok {
			continue
		}
		if !strings.EqualFold(ownerFromCert, org) {
			continue
		}
		return ownerFromCert, nameFromCert, true, nil
	}
	return "", "", false, nil
}
