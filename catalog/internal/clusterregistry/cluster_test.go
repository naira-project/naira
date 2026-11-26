package clusterregistry

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCluster(t *testing.T) {
	params := CreateParams{
		Name:          "production",
		Workspace:     Workspace{ID: "workspace-a"},
		CredentialRef: "kubernetes-secret:naira/production#kubeconfig",
		Kubernetes: KubernetesConnection{
			APIEndpoint: "https://kubernetes.example.com:6443",
			Context:     "production",
		},
	}
	before := time.Now().UTC()
	cluster, err := NewCluster(params)
	require.NoError(t, err)
	after := time.Now().UTC()

	assert.NotEqual(t, uuid.Nil, cluster.ID)
	assert.Equal(t, uuid.Version(4), cluster.ID.Version())
	assert.WithinRange(t, cluster.CreatedAt, before, after)
	assert.Equal(t, time.UTC, cluster.CreatedAt.Location())
	assert.Equal(t, Cluster{
		ID:               cluster.ID,
		Name:             "production",
		Workspace:        Workspace{ID: "workspace-a"},
		CredentialRef:    "kubernetes-secret:naira/production#kubeconfig",
		ValidationStatus: ValidationPending,
		CreatedAt:        cluster.CreatedAt,
		UpdatedAt:        cluster.CreatedAt,
		Kubernetes: KubernetesDetails{
			Connection: KubernetesConnection{
				APIEndpoint: "https://kubernetes.example.com:6443",
				Context:     "production",
			},
		},
	}, cluster)

	other, err := NewCluster(params)
	require.NoError(t, err)
	assert.NotEqual(t, cluster.ID, other.ID)

	params.Kubernetes.Context = "changed-input"
	assert.Equal(t, "production", cluster.Kubernetes.Connection.Context)
	other.Kubernetes.Connection.Context = "changed-other-cluster"
	assert.Equal(t, "production", cluster.Kubernetes.Connection.Context)
}

func TestNewClusterRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		change  func(*CreateParams)
		wantErr string
	}{
		{
			name:    "missing workspace",
			change:  func(p *CreateParams) { p.Workspace.ID = "" },
			wantErr: "workspace ID",
		},
		{
			name:    "missing Kubernetes configuration",
			change:  func(p *CreateParams) { p.Kubernetes = KubernetesConnection{} },
			wantErr: "API endpoint",
		},
		{
			name:    "invalid Kubernetes configuration",
			change:  func(p *CreateParams) { p.Kubernetes.APIEndpoint = "not-an-endpoint" },
			wantErr: "API endpoint",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := CreateParams{
				Name:          "production",
				Workspace:     Workspace{ID: "workspace-a"},
				CredentialRef: "opaque-reference",
				Kubernetes:    KubernetesConnection{APIEndpoint: "https://kubernetes.example.com"},
			}
			tt.change(&params)
			cluster, err := NewCluster(params)
			require.ErrorIs(t, err, ErrInvalidCluster)
			assert.ErrorContains(t, err, tt.wantErr)
			assert.Equal(t, Cluster{}, cluster)
		})
	}
}

func TestClusterValidate(t *testing.T) {
	createdAt := time.Date(2026, time.October, 24, 10, 0, 0, 0, time.UTC)
	validatedAt := createdAt.Add(time.Minute)
	updatedAt := createdAt.Add(2 * time.Minute)
	base := Cluster{
		ID:               uuid.MustParse("af3a70f0-6abc-4e2c-a467-9ba0dd84a77c"),
		Name:             "production",
		Workspace:        Workspace{ID: "workspace-a"},
		CredentialRef:    "opaque-reference",
		ValidationStatus: ValidationPending,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
		Kubernetes: KubernetesDetails{
			Connection: KubernetesConnection{APIEndpoint: "https://kubernetes.example.com:6443"},
		},
	}
	tests := []struct {
		name    string
		change  func(*Cluster)
		wantErr string
	}{
		{
			name:   "pending without fingerprint",
			change: func(*Cluster) {},
		},
		{
			name:   "pending retains verified fingerprint",
			change: func(r *Cluster) { r.Kubernetes.KubeSystemUID = "installation-uid" },
		},
		{
			name: "ready",
			change: func(r *Cluster) {
				r.ValidationStatus = ValidationReady
				r.Kubernetes.KubeSystemUID = "installation-uid"
				r.LastValidationAt = &validatedAt
			},
		},
		{
			name: "failed before fingerprint was available",
			change: func(r *Cluster) {
				r.ValidationStatus = ValidationFailed
				r.LastValidationAt = &validatedAt
			},
		},
		{
			name:    "missing identity",
			change:  func(r *Cluster) { r.ID = uuid.Nil },
			wantErr: "cluster ID",
		},
		{
			name:    "empty name",
			change:  func(r *Cluster) { r.Name = "" },
			wantErr: "name",
		},
		{
			name:    "name has surrounding whitespace",
			change:  func(r *Cluster) { r.Name = " production " },
			wantErr: "name",
		},
		{
			name:    "empty workspace",
			change:  func(r *Cluster) { r.Workspace.ID = "" },
			wantErr: "workspace ID",
		},
		{
			name:    "whitespace workspace",
			change:  func(r *Cluster) { r.Workspace.ID = " " },
			wantErr: "workspace ID",
		},
		{
			name:    "empty credential reference",
			change:  func(r *Cluster) { r.CredentialRef = "" },
			wantErr: "credential reference",
		},
		{
			name:    "multiline credential reference",
			change:  func(r *Cluster) { r.CredentialRef = "apiVersion: v1\nkind: Config" },
			wantErr: "credential reference",
		},
		{
			name:    "missing Kubernetes details",
			change:  func(r *Cluster) { r.Kubernetes = KubernetesDetails{} },
			wantErr: "API endpoint",
		},
		{
			name:    "unknown status",
			change:  func(r *Cluster) { r.ValidationStatus = "UNKNOWN" },
			wantErr: "unknown validation status",
		},
		{
			name:    "pending with result timestamp",
			change:  func(r *Cluster) { r.LastValidationAt = &validatedAt },
			wantErr: "pending validation",
		},
		{
			name: "ready without fingerprint",
			change: func(r *Cluster) {
				r.ValidationStatus = ValidationReady
				r.LastValidationAt = &validatedAt
			},
			wantErr: "ready Kubernetes cluster requires a kube-system UID",
		},
		{
			name: "ready without timestamp",
			change: func(r *Cluster) {
				r.ValidationStatus = ValidationReady
				r.Kubernetes.KubeSystemUID = "installation-uid"
			},
			wantErr: "ready validation",
		},
		{
			name:    "failed without timestamp",
			change:  func(r *Cluster) { r.ValidationStatus = ValidationFailed },
			wantErr: "failed validation",
		},
		{
			name:    "missing creation timestamp",
			change:  func(r *Cluster) { r.CreatedAt = time.Time{} },
			wantErr: "creation and update timestamps",
		},
		{
			name:    "missing update timestamp",
			change:  func(r *Cluster) { r.UpdatedAt = time.Time{} },
			wantErr: "creation and update timestamps",
		},
		{
			name:    "update before creation",
			change:  func(r *Cluster) { r.UpdatedAt = createdAt.Add(-time.Second) },
			wantErr: "creation and update timestamps",
		},
		{
			name: "zero validation timestamp",
			change: func(r *Cluster) {
				r.ValidationStatus = ValidationFailed
				r.LastValidationAt = new(time.Time)
			},
			wantErr: "last validation timestamp",
		},
		{
			name: "validation before creation",
			change: func(r *Cluster) {
				r.ValidationStatus = ValidationFailed
				r.LastValidationAt = new(createdAt.Add(-time.Second))
			},
			wantErr: "last validation timestamp",
		},
		{
			name: "validation after update",
			change: func(r *Cluster) {
				r.ValidationStatus = ValidationFailed
				r.LastValidationAt = new(updatedAt.Add(time.Second))
			},
			wantErr: "last validation timestamp",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := base

			tt.change(&cluster)
			err := cluster.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrInvalidCluster)
			assert.ErrorContains(t, err, tt.wantErr)
			assert.NotContains(t, err.Error(), "sensitive-value")
			assert.NotContains(t, err.Error(), "apiVersion")
		})
	}
}

func TestClusterApplyUpdate(t *testing.T) {
	createdAt := time.Now().UTC().Add(-time.Hour)
	validatedAt := createdAt.Add(time.Minute)
	base := Cluster{
		ID:               uuid.MustParse("af3a70f0-6abc-4e2c-a467-9ba0dd84a77c"),
		Name:             "production",
		Workspace:        Workspace{ID: "workspace-a"},
		CredentialRef:    "original-reference",
		ValidationStatus: ValidationReady,
		LastValidationAt: &validatedAt,
		CreatedAt:        createdAt,
		UpdatedAt:        validatedAt,
		Kubernetes: KubernetesDetails{
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
				Kubernetes:    KubernetesConnection{APIEndpoint: "https://kubernetes.example.com"},
				CredentialRef: "original-reference",
			},
		},
		{
			name: "credential reference change preserves identity but invalidates validation",
			params: UpdateParams{
				Name:          "production",
				Kubernetes:    KubernetesConnection{APIEndpoint: "https://kubernetes.example.com"},
				CredentialRef: "replacement-reference",
			},
			invalidate: true,
		},
		{
			name: "endpoint change preserves fingerprint but invalidates validation",
			params: UpdateParams{
				Name:          "production",
				Kubernetes:    KubernetesConnection{APIEndpoint: "https://new-endpoint.example.com"},
				CredentialRef: "original-reference",
			},
			invalidate: true,
		},
		{
			name: "invalid update changes nothing",
			params: UpdateParams{
				Name:          "renamed",
				Kubernetes:    KubernetesConnection{APIEndpoint: "not-an-endpoint"},
				CredentialRef: "replacement-reference",
			},
			wantErr: true,
		},
		{
			name: "context change invalidates validation",
			params: UpdateParams{
				Name:          "production",
				CredentialRef: "original-reference",
				Kubernetes: KubernetesConnection{
					APIEndpoint: "https://kubernetes.example.com",
					Context:     "another-context",
				},
			},
			invalidate: true,
		},
		{
			name: "missing connection cannot remove cluster details",
			params: UpdateParams{
				Name:          "renamed",
				CredentialRef: "original-reference",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := base
			originalDetails := base.Kubernetes
			before := time.Now().UTC()
			err := cluster.ApplyUpdate(tt.params)
			after := time.Now().UTC()
			assert.Equal(t, originalDetails, base.Kubernetes)
			if tt.wantErr {
				require.ErrorIs(t, err, ErrInvalidCluster)
				assert.Equal(t, base, cluster)
				return
			}
			require.NoError(t, err)
			assert.WithinRange(t, cluster.UpdatedAt, before, after)
			assert.Equal(t, time.UTC, cluster.UpdatedAt.Location())
			want := base
			want.Name = tt.params.Name
			want.Kubernetes.Connection = tt.params.Kubernetes
			want.CredentialRef = tt.params.CredentialRef
			want.UpdatedAt = cluster.UpdatedAt
			if tt.invalidate {
				want.ValidationStatus = ValidationPending
				want.LastValidationAt = nil
			}
			assert.Equal(t, want, cluster)
			tt.params.Kubernetes.Context = "mutated-after-update"
			assert.Equal(t, want, cluster)
		})
	}
}

func TestClusterApplyUpdateRejectsInvalidExistingCluster(t *testing.T) {
	cluster := Cluster{
		ID:               uuid.New(),
		Name:             "production",
		Workspace:        Workspace{ID: "workspace-a"},
		CredentialRef:    "opaque-reference",
		ValidationStatus: ValidationPending,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}
	want := cluster
	err := cluster.ApplyUpdate(UpdateParams{
		Name:          "renamed",
		CredentialRef: "replacement-reference",
		Kubernetes:    KubernetesConnection{APIEndpoint: "https://kubernetes.example.com"},
	})
	require.ErrorIs(t, err, ErrInvalidCluster)
	assert.ErrorContains(t, err, "API endpoint")
	assert.Equal(t, want, cluster)
}

func TestClusterApplyValidation(t *testing.T) {
	createdAt := time.Date(2026, time.November, 4, 12, 0, 0, 0, time.UTC)
	checkedAt := createdAt.Add(time.Minute)
	base := Cluster{
		ID:            uuid.New(),
		Name:          "production",
		Workspace:     Workspace{ID: TestWorkspaceID},
		CredentialRef: "approved-reference",
		Kubernetes: KubernetesDetails{
			Connection: KubernetesConnection{APIEndpoint: "https://production.example.com"},
		},
		ValidationStatus: ValidationPending,
		CreatedAt:        createdAt,
		UpdatedAt:        createdAt,
	}
	for _, tt := range []struct {
		name     string
		knownUID string
		result   ValidationResult
		wantUID  string
		wantErr  bool
	}{
		{
			name: "ready records the first installation UID",
			result: ValidationResult{
				Status:        ValidationReady,
				CheckedAt:     checkedAt,
				KubeSystemUID: "installation-a",
				Checks:        []AccessCheck{{Name: "connection", Passed: true}},
			},
			wantUID: "installation-a",
		},
		{
			name:     "failed preserves the verified installation UID",
			knownUID: "installation-a",
			result: ValidationResult{
				Status:        ValidationFailed,
				CheckedAt:     checkedAt,
				KubeSystemUID: "installation-b",
				Checks:        []AccessCheck{{Name: "continuity", Code: "CLUSTER_CHANGED"}},
			},
			wantUID: "installation-a",
		},
		{
			name: "failed does not trust an initial observed UID",
			result: ValidationResult{
				Status:        ValidationFailed,
				CheckedAt:     checkedAt,
				KubeSystemUID: "installation-a",
				Checks:        []AccessCheck{{Name: "services", Code: "FORBIDDEN"}},
			},
		},
		{
			name:     "ready cannot replace an installation UID",
			knownUID: "installation-a",
			result: ValidationResult{
				Status:        ValidationReady,
				CheckedAt:     checkedAt,
				KubeSystemUID: "installation-b",
				Checks:        []AccessCheck{{Name: "connection", Passed: true}},
			},
			wantErr: true,
		},
		{
			name: "an invalid result does not change the cluster",
			result: ValidationResult{
				Status:    ValidationPending,
				CheckedAt: checkedAt,
			},
			wantErr: true,
		},
		{
			name: "result timestamp cannot predate creation",
			result: ValidationResult{
				Status:    ValidationFailed,
				CheckedAt: createdAt.Add(-time.Second),
			},
			wantErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cluster := base
			cluster.Kubernetes.KubeSystemUID = tt.knownUID
			want := cluster
			err := cluster.applyValidation(tt.result)
			if tt.wantErr {
				require.ErrorIs(t, err, ErrInvalidCluster)
				assert.Equal(t, want, cluster)
				return
			}
			require.NoError(t, err)
			want.ValidationStatus = tt.result.Status
			want.LastValidationAt = new(tt.result.CheckedAt)
			want.LastValidation = &tt.result
			want.UpdatedAt = tt.result.CheckedAt
			want.Kubernetes.KubeSystemUID = tt.wantUID
			assert.Equal(t, want, cluster)
		})
	}
}
