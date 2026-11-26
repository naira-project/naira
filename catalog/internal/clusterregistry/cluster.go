// Package clusterregistry manages workspace-scoped Kubernetes clusters,
// independently of discovered catalog resources and collector configuration.
package clusterregistry

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrInvalidCluster = errors.New("invalid cluster")

// Cluster identifies a registered Kubernetes connection in one workspace.
// ID, Workspace and CreatedAt must remain unchanged after creation.
type Cluster struct {
	ID        uuid.UUID
	Name      string
	Workspace Workspace

	// CredentialRef is an opaque locator, never a kubeconfig, token or private key.
	CredentialRef string

	Kubernetes KubernetesDetails

	ValidationStatus ValidationStatus
	// LastValidationAt describes the current connection configuration, including failures.
	// Nil means that configuration has not been validated.
	LastValidationAt *time.Time
	LastValidation   *ValidationResult
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type CreateParams struct {
	Name          string
	Workspace     Workspace
	CredentialRef string
	Kubernetes    KubernetesConnection
}

// UpdateParams replaces editable metadata and connection configuration.
// Identity, workspace and validation results cannot be supplied.
type UpdateParams struct {
	Name          string
	CredentialRef string
	Kubernetes    KubernetesConnection
}

// NewCluster generates an identity and initializes an unvalidated cluster.
// It neither connects to the cluster nor persists the result.
func NewCluster(params CreateParams) (Cluster, error) {
	now := time.Now().UTC()
	cluster := Cluster{
		ID:               uuid.New(),
		Name:             params.Name,
		Workspace:        params.Workspace,
		CredentialRef:    params.CredentialRef,
		Kubernetes:       KubernetesDetails{Connection: params.Kubernetes},
		ValidationStatus: ValidationPending,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := cluster.Validate(); err != nil {
		return Cluster{}, fmt.Errorf("validate new cluster: %w", err)
	}
	return cluster, nil
}

// ApplyUpdate replaces metadata in memory and invalidates validation on connection changes.
// It preserves identity, workspace and verified cluster details. Invalid updates change nothing.
// Changes to a credential's value at an unchanged reference require separate revalidation.
func (r *Cluster) ApplyUpdate(params UpdateParams) error {
	if err := r.Validate(); err != nil {
		return fmt.Errorf("validate existing cluster: %w", err)
	}
	updated := *r
	updated.Name = params.Name
	updated.CredentialRef = params.CredentialRef
	updated.UpdatedAt = time.Now().UTC()
	updated.Kubernetes.Connection = params.Kubernetes
	connectionChanged := r.CredentialRef != params.CredentialRef || r.Kubernetes.Connection != params.Kubernetes
	if connectionChanged {
		updated.ValidationStatus = ValidationPending
		updated.LastValidationAt = nil
		updated.LastValidation = nil
	}
	if err := updated.Validate(); err != nil {
		return fmt.Errorf("validate updated cluster: %w", err)
	}
	*r = updated
	return nil
}

// applyValidation preserves a known installation UID. Invalid results change nothing.
func (r *Cluster) applyValidation(result ValidationResult) error {
	if err := result.validate(); err != nil {
		return fmt.Errorf("validate connection result: %w", err)
	}
	knownUID := r.Kubernetes.KubeSystemUID
	if result.Status == ValidationReady && knownUID != "" && knownUID != result.KubeSystemUID {
		return fmt.Errorf("preserve installation UID: %w", ErrInvalidCluster)
	}
	updated := *r
	updated.ValidationStatus = result.Status
	updated.LastValidationAt = new(result.CheckedAt)
	updated.LastValidation = &result
	updated.UpdatedAt = result.CheckedAt
	if result.Status == ValidationReady && knownUID == "" {
		updated.Kubernetes.KubeSystemUID = result.KubeSystemUID
	}
	if err := updated.Validate(); err != nil {
		return fmt.Errorf("validate result state: %w", err)
	}
	*r = updated
	return nil
}

// Validate checks structural invariants only, without verifying access or resolving credentials.
func (r Cluster) Validate() error {
	if r.ID == uuid.Nil {
		return fmt.Errorf("cluster ID must not be empty: %w", ErrInvalidCluster)
	}
	if err := r.Workspace.Validate(); err != nil {
		return fmt.Errorf("%w: validate workspace: %w", ErrInvalidCluster, err)
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{
			name:  "name",
			value: r.Name,
		},
		{
			name:  "credential reference",
			value: r.CredentialRef,
		},
	} {
		if field.value == "" || field.value != strings.TrimSpace(field.value) || strings.ContainsAny(field.value, "\r\n\t") {
			return fmt.Errorf("%s must be nonempty without surrounding whitespace or control separators: %w", field.name, ErrInvalidCluster)
		}
	}

	if r.CreatedAt.IsZero() || r.UpdatedAt.IsZero() || r.UpdatedAt.Before(r.CreatedAt) {
		return fmt.Errorf("creation and update timestamps must be set and ordered: %w", ErrInvalidCluster)
	}
	if r.LastValidationAt != nil && (r.LastValidationAt.IsZero() || r.LastValidationAt.Before(r.CreatedAt) || r.LastValidationAt.After(r.UpdatedAt)) {
		return fmt.Errorf("last validation timestamp must be between creation and update: %w", ErrInvalidCluster)
	}
	if report := r.LastValidation; report != nil {
		if r.LastValidationAt == nil || !report.CheckedAt.Equal(*r.LastValidationAt) || report.Status != r.ValidationStatus || r.ValidationStatus == ValidationPending {
			return fmt.Errorf("validation report must match its status and timestamp: %w", ErrInvalidCluster)
		}
	}
	switch r.ValidationStatus {
	case ValidationPending:
		if r.LastValidationAt != nil {
			return fmt.Errorf("pending validation must not have a result timestamp: %w", ErrInvalidCluster)
		}
	case ValidationReady:
		if r.LastValidationAt == nil {
			return fmt.Errorf("ready validation requires a result timestamp: %w", ErrInvalidCluster)
		}
	case ValidationFailed:
		if r.LastValidationAt == nil {
			return fmt.Errorf("failed validation requires a result timestamp: %w", ErrInvalidCluster)
		}
	default:
		return fmt.Errorf("unknown validation status: %w", ErrInvalidCluster)
	}
	if err := r.Kubernetes.validate(r.ValidationStatus); err != nil {
		return fmt.Errorf("validate Kubernetes details: %w", err)
	}
	return nil
}
