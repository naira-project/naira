// Package sources models workspace-owned source registrations,
// independently of discovered catalog resources and collector configuration.
package sources

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Kind string

const KindKubernetes Kind = "kubernetes"

type ValidationStatus string

const (
	ValidationPending ValidationStatus = "PENDING"
	ValidationReady   ValidationStatus = "READY"
	ValidationFailed  ValidationStatus = "FAILED"
)

var ErrInvalidRegistration = errors.New("invalid source registration")

// Registration identifies a source connection owned by one workspace.
// ID, WorkspaceID, Kind and CreatedAt must remain unchanged after creation.
type Registration struct {
	ID          uuid.UUID
	Name        string
	WorkspaceID string
	Kind        Kind

	// CredentialRef is an opaque locator, never a kubeconfig, token or private key.
	CredentialRef string

	// Kubernetes must be set exactly when Kind is KindKubernetes.
	Kubernetes *KubernetesDetails

	ValidationStatus ValidationStatus
	// LastValidationAt describes the current connection configuration, including failures.
	// Nil means that configuration has not been validated.
	LastValidationAt *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type CreateParams struct {
	Name          string
	WorkspaceID   string
	Kind          Kind
	CredentialRef string
	Kubernetes    *KubernetesConnection
}

// UpdateParams replaces editable metadata and connection configuration.
// Identity, ownership, source kind and validation results cannot be supplied.
type UpdateParams struct {
	Name          string
	CredentialRef string
	Kubernetes    *KubernetesConnection
}

// NewRegistration generates an identity and initializes an unvalidated registration.
// It neither connects to the source nor persists the result.
func NewRegistration(params CreateParams) (Registration, error) {
	now := time.Now().UTC()
	registration := Registration{
		ID:               uuid.New(),
		Name:             params.Name,
		WorkspaceID:      params.WorkspaceID,
		Kind:             params.Kind,
		CredentialRef:    params.CredentialRef,
		ValidationStatus: ValidationPending,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if params.Kubernetes != nil {
		registration.Kubernetes = &KubernetesDetails{Connection: *params.Kubernetes}
	}
	if err := registration.Validate(); err != nil {
		return Registration{}, fmt.Errorf("validate new registration: %w", err)
	}
	return registration, nil
}

// ApplyUpdate replaces metadata in memory and invalidates validation on connection changes.
// It preserves identity, workspace, kind and verified source details. Invalid updates change nothing.
// Changes to a credential's value at an unchanged reference require separate revalidation.
func (r *Registration) ApplyUpdate(params UpdateParams) error {
	if err := r.Validate(); err != nil {
		return fmt.Errorf("validate existing registration: %w", err)
	}
	updated := *r
	updated.Name = params.Name
	updated.CredentialRef = params.CredentialRef
	updated.UpdatedAt = time.Now().UTC()
	connectionChanged := r.CredentialRef != params.CredentialRef
	if r.Kind == KindKubernetes {
		if params.Kubernetes == nil {
			return fmt.Errorf("Kubernetes connection is required: %w", ErrInvalidRegistration)
		}
		details := *r.Kubernetes
		details.Connection = *params.Kubernetes
		updated.Kubernetes = &details
		connectionChanged = connectionChanged || r.Kubernetes.Connection != details.Connection
	}
	if connectionChanged {
		updated.ValidationStatus = ValidationPending
		updated.LastValidationAt = nil
	}
	if err := updated.Validate(); err != nil {
		return fmt.Errorf("validate updated registration: %w", err)
	}
	*r = updated
	return nil
}

// Validate checks structural invariants only, without verifying access or resolving credentials.
func (r Registration) Validate() error {
	if r.ID == uuid.Nil {
		return fmt.Errorf("registration ID must not be empty: %w", ErrInvalidRegistration)
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
			name:  "workspace ID",
			value: r.WorkspaceID,
		},
		{
			name:  "credential reference",
			value: r.CredentialRef,
		},
	} {
		if field.value == "" || field.value != strings.TrimSpace(field.value) || strings.ContainsAny(field.value, "\r\n\t") {
			return fmt.Errorf("%s must be nonempty without surrounding whitespace or control separators: %w", field.name, ErrInvalidRegistration)
		}
	}

	if r.CreatedAt.IsZero() || r.UpdatedAt.IsZero() || r.UpdatedAt.Before(r.CreatedAt) {
		return fmt.Errorf("creation and update timestamps must be set and ordered: %w", ErrInvalidRegistration)
	}
	if r.LastValidationAt != nil && (r.LastValidationAt.IsZero() || r.LastValidationAt.Before(r.CreatedAt) || r.LastValidationAt.After(r.UpdatedAt)) {
		return fmt.Errorf("last validation timestamp must be between creation and update: %w", ErrInvalidRegistration)
	}
	switch r.ValidationStatus {
	case ValidationPending:
		if r.LastValidationAt != nil {
			return fmt.Errorf("pending validation must not have a result timestamp: %w", ErrInvalidRegistration)
		}
	case ValidationReady:
		if r.LastValidationAt == nil {
			return fmt.Errorf("ready validation requires a result timestamp: %w", ErrInvalidRegistration)
		}
	case ValidationFailed:
		if r.LastValidationAt == nil {
			return fmt.Errorf("failed validation requires a result timestamp: %w", ErrInvalidRegistration)
		}
	default:
		return fmt.Errorf("unknown validation status: %w", ErrInvalidRegistration)
	}
	switch r.Kind {
	case KindKubernetes:
		if r.Kubernetes == nil {
			return fmt.Errorf("Kubernetes details are required: %w", ErrInvalidRegistration)
		}
		if err := r.Kubernetes.validate(r.ValidationStatus); err != nil {
			return fmt.Errorf("validate Kubernetes details: %w", err)
		}
	default:
		return fmt.Errorf("unsupported source kind: %w", ErrInvalidRegistration)
	}
	return nil
}
