package sources

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegistration(t *testing.T) {
	params := CreateParams{
		Name:          "production",
		WorkspaceID:   "workspace-a",
		Kind:          KindKubernetes,
		CredentialRef: "kubernetes-secret:naira/production#kubeconfig",
		Kubernetes: &KubernetesConnection{
			APIEndpoint: "https://kubernetes.example.com:6443",
			Context:     "production",
		},
	}
	before := time.Now().UTC()
	registration, err := NewRegistration(params)
	require.NoError(t, err)
	after := time.Now().UTC()

	assert.NotEqual(t, uuid.Nil, registration.ID)
	assert.Equal(t, uuid.Version(4), registration.ID.Version())
	assert.WithinRange(t, registration.CreatedAt, before, after)
	assert.Equal(t, time.UTC, registration.CreatedAt.Location())
	assert.Equal(t, Registration{
		ID:               registration.ID,
		Name:             "production",
		WorkspaceID:      "workspace-a",
		Kind:             KindKubernetes,
		CredentialRef:    "kubernetes-secret:naira/production#kubeconfig",
		ValidationStatus: ValidationPending,
		CreatedAt:        registration.CreatedAt,
		UpdatedAt:        registration.CreatedAt,
		Kubernetes: &KubernetesDetails{
			Connection: KubernetesConnection{
				APIEndpoint: "https://kubernetes.example.com:6443",
				Context:     "production",
			},
		},
	}, registration)

	other, err := NewRegistration(params)
	require.NoError(t, err)
	assert.NotEqual(t, registration.ID, other.ID)

	params.Kubernetes.Context = "changed-input"
	assert.Equal(t, "production", registration.Kubernetes.Connection.Context)
	other.Kubernetes.Connection.Context = "changed-other-registration"
	assert.Equal(t, "production", registration.Kubernetes.Connection.Context)
}

func TestNewRegistrationRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		change  func(*CreateParams)
		wantErr string
	}{
		{
			name:    "missing workspace",
			change:  func(p *CreateParams) { p.WorkspaceID = "" },
			wantErr: "workspace ID",
		},
		{
			name:    "missing kind",
			change:  func(p *CreateParams) { p.Kind = "" },
			wantErr: "unsupported source kind",
		},
		{
			name:    "unknown kind with Kubernetes configuration",
			change:  func(p *CreateParams) { p.Kind = "github" },
			wantErr: "unsupported source kind",
		},
		{
			name:    "missing Kubernetes configuration",
			change:  func(p *CreateParams) { p.Kubernetes = nil },
			wantErr: "Kubernetes details are required",
		},
		{
			name:    "invalid Kubernetes configuration",
			change:  func(p *CreateParams) { p.Kubernetes.APIEndpoint = "" },
			wantErr: "API endpoint",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := CreateParams{
				Name:          "production",
				WorkspaceID:   "workspace-a",
				Kind:          KindKubernetes,
				CredentialRef: "opaque-reference",
				Kubernetes:    &KubernetesConnection{APIEndpoint: "https://kubernetes.example.com"},
			}
			tt.change(&params)
			registration, err := NewRegistration(params)
			require.ErrorIs(t, err, ErrInvalidRegistration)
			assert.ErrorContains(t, err, tt.wantErr)
			assert.Equal(t, Registration{}, registration)
		})
	}
}

func TestRegistrationValidate(t *testing.T) {
	createdAt := time.Date(2026, time.October, 24, 10, 0, 0, 0, time.UTC)
	validatedAt := createdAt.Add(time.Minute)
	updatedAt := createdAt.Add(2 * time.Minute)
	base := Registration{
		ID:               uuid.MustParse("af3a70f0-6abc-4e2c-a467-9ba0dd84a77c"),
		Name:             "production",
		WorkspaceID:      "workspace-a",
		Kind:             KindKubernetes,
		CredentialRef:    "opaque-reference",
		ValidationStatus: ValidationPending,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
		Kubernetes: &KubernetesDetails{
			Connection: KubernetesConnection{APIEndpoint: "https://kubernetes.example.com:6443"},
		},
	}
	tests := []struct {
		name    string
		change  func(*Registration)
		wantErr string
	}{
		{
			name:   "pending without fingerprint",
			change: func(*Registration) {},
		},
		{
			name:   "pending retains verified fingerprint",
			change: func(r *Registration) { r.Kubernetes.KubeSystemUID = "installation-uid" },
		},
		{
			name: "ready",
			change: func(r *Registration) {
				r.ValidationStatus = ValidationReady
				r.Kubernetes.KubeSystemUID = "installation-uid"
				r.LastValidationAt = &validatedAt
			},
		},
		{
			name: "failed before fingerprint was available",
			change: func(r *Registration) {
				r.ValidationStatus = ValidationFailed
				r.LastValidationAt = &validatedAt
			},
		},
		{
			name:    "missing identity",
			change:  func(r *Registration) { r.ID = uuid.Nil },
			wantErr: "registration ID",
		},
		{
			name:    "empty name",
			change:  func(r *Registration) { r.Name = "" },
			wantErr: "name",
		},
		{
			name:    "name has surrounding whitespace",
			change:  func(r *Registration) { r.Name = " production " },
			wantErr: "name",
		},
		{
			name:    "empty workspace",
			change:  func(r *Registration) { r.WorkspaceID = "" },
			wantErr: "workspace ID",
		},
		{
			name:    "whitespace workspace",
			change:  func(r *Registration) { r.WorkspaceID = " " },
			wantErr: "workspace ID",
		},
		{
			name:    "empty credential reference",
			change:  func(r *Registration) { r.CredentialRef = "" },
			wantErr: "credential reference",
		},
		{
			name:    "multiline credential reference",
			change:  func(r *Registration) { r.CredentialRef = "apiVersion: v1\nkind: Config" },
			wantErr: "credential reference",
		},
		{
			name:    "unsupported kind",
			change:  func(r *Registration) { r.Kind = "github" },
			wantErr: "unsupported source kind",
		},
		{
			name:    "missing Kubernetes details",
			change:  func(r *Registration) { r.Kubernetes = nil },
			wantErr: "Kubernetes details are required",
		},
		{
			name:    "unknown status",
			change:  func(r *Registration) { r.ValidationStatus = "UNKNOWN" },
			wantErr: "unknown validation status",
		},
		{
			name:    "pending with result timestamp",
			change:  func(r *Registration) { r.LastValidationAt = &validatedAt },
			wantErr: "pending validation",
		},
		{
			name: "ready without fingerprint",
			change: func(r *Registration) {
				r.ValidationStatus = ValidationReady
				r.LastValidationAt = &validatedAt
			},
			wantErr: "ready Kubernetes registration requires a kube-system UID",
		},
		{
			name: "ready without timestamp",
			change: func(r *Registration) {
				r.ValidationStatus = ValidationReady
				r.Kubernetes.KubeSystemUID = "installation-uid"
			},
			wantErr: "ready validation",
		},
		{
			name:    "failed without timestamp",
			change:  func(r *Registration) { r.ValidationStatus = ValidationFailed },
			wantErr: "failed validation",
		},
		{
			name:    "missing creation timestamp",
			change:  func(r *Registration) { r.CreatedAt = time.Time{} },
			wantErr: "creation and update timestamps",
		},
		{
			name:    "missing update timestamp",
			change:  func(r *Registration) { r.UpdatedAt = time.Time{} },
			wantErr: "creation and update timestamps",
		},
		{
			name:    "update before creation",
			change:  func(r *Registration) { r.UpdatedAt = createdAt.Add(-time.Second) },
			wantErr: "creation and update timestamps",
		},
		{
			name: "zero validation timestamp",
			change: func(r *Registration) {
				r.ValidationStatus = ValidationFailed
				r.LastValidationAt = new(time.Time)
			},
			wantErr: "last validation timestamp",
		},
		{
			name: "validation before creation",
			change: func(r *Registration) {
				r.ValidationStatus = ValidationFailed
				r.LastValidationAt = new(createdAt.Add(-time.Second))
			},
			wantErr: "last validation timestamp",
		},
		{
			name: "validation after update",
			change: func(r *Registration) {
				r.ValidationStatus = ValidationFailed
				r.LastValidationAt = new(updatedAt.Add(time.Second))
			},
			wantErr: "last validation timestamp",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registration := base
			registration.Kubernetes = new(*base.Kubernetes)
			tt.change(&registration)
			err := registration.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrInvalidRegistration)
			assert.ErrorContains(t, err, tt.wantErr)
			assert.NotContains(t, err.Error(), "sensitive-value")
			assert.NotContains(t, err.Error(), "apiVersion")
		})
	}
}

func TestRegistrationApplyUpdate(t *testing.T) {
	createdAt := time.Now().UTC().Add(-time.Hour)
	validatedAt := createdAt.Add(time.Minute)
	base := Registration{
		ID:               uuid.MustParse("af3a70f0-6abc-4e2c-a467-9ba0dd84a77c"),
		Name:             "production",
		WorkspaceID:      "workspace-a",
		Kind:             KindKubernetes,
		CredentialRef:    "original-reference",
		ValidationStatus: ValidationReady,
		LastValidationAt: &validatedAt,
		CreatedAt:        createdAt,
		UpdatedAt:        validatedAt,
		Kubernetes: &KubernetesDetails{
			Connection:    KubernetesConnection{APIEndpoint: "https://kubernetes.example.com"},
			KubeSystemUID: "installation-uid",
		},
	}
	tests := []struct {
		name       string
		params     UpdateParams
		invalidate bool
		wantErr    bool
	}{
		{
			name: "rename preserves validation",
			params: UpdateParams{
				Name:          "renamed",
				Kubernetes:    &KubernetesConnection{APIEndpoint: "https://kubernetes.example.com"},
				CredentialRef: "original-reference",
			},
		},
		{
			name: "credential reference change preserves identity but invalidates validation",
			params: UpdateParams{
				Name:          "production",
				Kubernetes:    &KubernetesConnection{APIEndpoint: "https://kubernetes.example.com"},
				CredentialRef: "replacement-reference",
			},
			invalidate: true,
		},
		{
			name: "endpoint change preserves fingerprint but invalidates validation",
			params: UpdateParams{
				Name:          "production",
				Kubernetes:    &KubernetesConnection{APIEndpoint: "https://new-endpoint.example.com"},
				CredentialRef: "original-reference",
			},
			invalidate: true,
		},
		{
			name: "invalid update changes nothing",
			params: UpdateParams{
				Name:          "renamed",
				Kubernetes:    &KubernetesConnection{APIEndpoint: "not-an-endpoint"},
				CredentialRef: "replacement-reference",
			},
			wantErr: true,
		},
		{
			name: "context change invalidates validation",
			params: UpdateParams{
				Name:          "production",
				CredentialRef: "original-reference",
				Kubernetes: &KubernetesConnection{
					APIEndpoint: "https://kubernetes.example.com",
					Context:     "another-context",
				},
			},
			invalidate: true,
		},
		{
			name: "missing connection cannot remove source details",
			params: UpdateParams{
				Name:          "renamed",
				CredentialRef: "original-reference",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registration := base
			originalDetails := *base.Kubernetes
			before := time.Now().UTC()
			err := registration.ApplyUpdate(tt.params)
			after := time.Now().UTC()
			assert.Equal(t, originalDetails, *base.Kubernetes)
			if tt.wantErr {
				require.ErrorIs(t, err, ErrInvalidRegistration)
				assert.Equal(t, base, registration)
				return
			}
			require.NoError(t, err)
			assert.WithinRange(t, registration.UpdatedAt, before, after)
			assert.Equal(t, time.UTC, registration.UpdatedAt.Location())
			want := base
			want.Kubernetes = new(*base.Kubernetes)
			want.Name = tt.params.Name
			want.Kubernetes.Connection = *tt.params.Kubernetes
			want.CredentialRef = tt.params.CredentialRef
			want.UpdatedAt = registration.UpdatedAt
			if tt.invalidate {
				want.ValidationStatus = ValidationPending
				want.LastValidationAt = nil
			}
			assert.Equal(t, want, registration)
			tt.params.Kubernetes.Context = "mutated-after-update"
			assert.Equal(t, want, registration)
		})
	}
}

func TestRegistrationApplyUpdateRejectsInvalidExistingRegistration(t *testing.T) {
	registration := Registration{
		ID:               uuid.New(),
		Name:             "production",
		WorkspaceID:      "workspace-a",
		Kind:             "unsupported",
		CredentialRef:    "opaque-reference",
		ValidationStatus: ValidationPending,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}
	want := registration
	err := registration.ApplyUpdate(UpdateParams{
		Name:          "renamed",
		CredentialRef: "replacement-reference",
		Kubernetes:    &KubernetesConnection{APIEndpoint: "https://kubernetes.example.com"},
	})
	require.ErrorIs(t, err, ErrInvalidRegistration)
	assert.ErrorContains(t, err, "unsupported source kind")
	assert.Equal(t, want, registration)
}
