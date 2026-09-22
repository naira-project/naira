package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
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

// ghAttestationEntry is used with `gh attestation verify --format json` output
//
// ghAttestationEntry uses signature.certificate, which contains values that cannot be
// manipulated, even if someone gains access to the GitHub Actions workflow creating
// attestations. Before trusting other fields from the verify output (like statement.predicate),
// see the security notes in: `gh help attestation verify`
type ghAttestationEntry struct {
	VerificationResult struct {
		Signature struct {
			Certificate ghCertificate `json:"certificate"`
		} `json:"signature"`
	} `json:"verificationResult"`
}

type ghCertificate struct {
	SourceRepositoryURI      string `json:"sourceRepositoryURI"`
	SourceRepositoryOwnerURI string `json:"sourceRepositoryOwnerURI"`
}

var ErrAttestationMissing = errors.New("attestation not found for given image")

// Verify checks whether an image has a trusted GitHub artifact attestation from a repository
// in the organization and, if valid, returns the repository owner and name.
func (v *attestationVerifier) Verify(ctx context.Context, image, org string) (ownerAndName, error) {
	ctx, cancel := context.WithTimeout(ctx, v.timeout)
	defer cancel()

	imageURI := "oci://" + image

	if _, err := url.Parse(imageURI); err != nil {
		return ownerAndName{}, fmt.Errorf("invalid image reference %q: %w", image, err)
	}

	// passing gh an oci:// reference (rather than a resolved digest) means
	// gh itself talks to the registry to resolve the digest and fetch
	// the attestation bundle
	cmd := exec.CommandContext(ctx, v.ghPath, "attestation", "verify",
		imageURI,
		"--owner", org,
		"--format", "json",
	)
	if v.token != "" {
		cmd.Env = append(os.Environ(), "GH_TOKEN="+v.token)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if runErr := cmd.Run(); runErr != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](runErr); ok {
			// the command started, but verification failed
			return ownerAndName{}, fmt.Errorf(
				"gh attestation verify failed (exit code %d) for image %q: %s",
				exitErr.ExitCode(), image, strings.TrimSpace(stderr.String()),
			)
		}
		// anything else: gh binary missing, failed to start, etc.
		return ownerAndName{}, fmt.Errorf(
			"failed to run gh attestation verify for image %q: %w (stderr: %s)",
			image, runErr, strings.TrimSpace(stderr.String()),
		)
	}

	var entries []ghAttestationEntry
	if err := json.Unmarshal(stdout.Bytes(), &entries); err != nil {
		return ownerAndName{}, fmt.Errorf("parsing gh attestation verify output: %w", err)
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
		return ownerAndName{owner: ownerFromCert, name: nameFromCert}, nil
	}
	return ownerAndName{}, ErrAttestationMissing
}

func (v *attestationVerifier) CheckGhAvailable(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, v.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, v.ghPath, "--version")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gh CLI not available at %q: %w (output: %s)", v.ghPath, err, strings.TrimSpace(string(out)))
	}

	return nil
}
