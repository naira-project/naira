package sources

import (
	"fmt"
	"net/url"
	"strings"
)

// KubernetesConnection describes the expected API endpoint and kubeconfig context.
// It contains no credentials and does not open a connection.
type KubernetesConnection struct {
	APIEndpoint string
	// Context selects a context in the referenced kubeconfig; empty means its current context.
	Context string
}

// KubernetesDetails separates connection configuration from verified installation identity.
type KubernetesDetails struct {
	Connection KubernetesConnection
	// KubeSystemUID is a verified installation fingerprint, not the registration ID.
	// Empty means unknown. Ordinary updates must not replace it.
	KubeSystemUID string
}

// Validate checks connection syntax without reading a kubeconfig or contacting Kubernetes.
func (c KubernetesConnection) Validate() error {
	if c.APIEndpoint == "" || c.APIEndpoint != strings.TrimSpace(c.APIEndpoint) || strings.ContainsAny(c.APIEndpoint, "\r\n\t") {
		return fmt.Errorf("API endpoint must be nonempty without surrounding whitespace or control separators: %w", ErrInvalidRegistration)
	}
	endpoint, err := url.Parse(c.APIEndpoint)
	if err != nil || (endpoint.Scheme != "https" && endpoint.Scheme != "http") || endpoint.Hostname() == "" ||
		endpoint.User != nil || endpoint.RawQuery != "" || endpoint.ForceQuery || strings.Contains(c.APIEndpoint, "#") {
		// Do not echo a malformed endpoint: it may contain inline credentials.
		return fmt.Errorf("API endpoint must be an absolute HTTP(S) URL without user info, query or fragment: %w", ErrInvalidRegistration)
	}
	if c.Context != strings.TrimSpace(c.Context) || strings.ContainsAny(c.Context, "\r\n\t") {
		return fmt.Errorf("kubeconfig context must not contain surrounding whitespace or control separators: %w", ErrInvalidRegistration)
	}
	return nil
}

func (d KubernetesDetails) validate(status ValidationStatus) error {
	if err := d.Connection.Validate(); err != nil {
		return fmt.Errorf("validate Kubernetes connection: %w", err)
	}
	if d.KubeSystemUID != strings.TrimSpace(d.KubeSystemUID) || strings.ContainsAny(d.KubeSystemUID, "\r\n\t") {
		return fmt.Errorf("kube-system UID must not contain surrounding whitespace or control separators: %w", ErrInvalidRegistration)
	}
	if status == ValidationReady && d.KubeSystemUID == "" {
		return fmt.Errorf("ready Kubernetes registration requires a kube-system UID: %w", ErrInvalidRegistration)
	}
	return nil
}
