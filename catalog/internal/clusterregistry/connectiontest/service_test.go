package connectiontest

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/naira-project/naira/catalog/internal/clusterregistry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Any store call panics through the nil embedded interface.
type inaccessibleStore struct{ clusterregistry.Store }

func TestAccessIsCheckedBeforeLoadingCredentialsOrClusters(t *testing.T) {
	store := inaccessibleStore{}
	registry := clusterregistry.NewService(store)
	service := NewService(registry, store, func(context.Context, clusterregistry.Cluster) clusterregistry.ValidationResult {
		t.Fatal("connection test must not run without access")
		return clusterregistry.ValidationResult{}
	})
	caller := clusterregistry.Caller{
		Issuer:  "https://issuer.example.com",
		Subject: "alice",
	}
	workspace := clusterregistry.Workspace{ID: clusterregistry.TestWorkspaceID}
	_, err := service.TestConnection(t.Context(), clusterregistry.Caller{}, workspace, uuid.New())
	require.ErrorIs(t, err, clusterregistry.ErrUnauthenticated)
	_, err = service.TestConnection(t.Context(), caller, clusterregistry.Workspace{ID: "other"}, uuid.New())
	require.ErrorIs(t, err, clusterregistry.ErrWorkspaceNotFound)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = service.TestConnection(ctx, caller, workspace, uuid.New())
	require.ErrorIs(t, err, context.Canceled)
}

func validationFixture(t *testing.T, test Func) (*clusterregistry.MemoryStore, *Service, clusterregistry.Caller, clusterregistry.Cluster) {
	t.Helper()
	store := clusterregistry.NewMemoryStore()
	registry := clusterregistry.NewService(store)
	service := NewService(registry, store, test)
	caller := clusterregistry.Caller{
		Issuer:  "https://issuer.example.com",
		Subject: "alice",
	}
	cluster, err := registry.Create(t.Context(), caller, clusterregistry.CreateParams{
		Name:          "production",
		Workspace:     clusterregistry.Workspace{ID: clusterregistry.TestWorkspaceID},
		CredentialRef: "approved-reference",
		Kubernetes:    clusterregistry.KubernetesConnection{APIEndpoint: "https://cluster.example.com"},
	})
	require.NoError(t, err)
	return store, service, caller, cluster
}

func TestConnectionResultsAndContinuity(t *testing.T) {
	report := clusterregistry.ValidationResult{
		KubeSystemUID: "installation-a",
		Checks: []clusterregistry.AccessCheck{{
			Name:   "connection",
			Passed: true,
			Code:   "OK",
		}},
	}
	store, service, caller, cluster := validationFixture(t, func(context.Context, clusterregistry.Cluster) clusterregistry.ValidationResult { return report })
	result, err := service.TestConnection(t.Context(), caller, cluster.Workspace, cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, clusterregistry.ValidationReady, result.Status)
	got, err := store.Get(t.Context(), cluster.Workspace, cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, cluster.ID, got.ID)
	assert.Equal(t, cluster.CreatedAt, got.CreatedAt)
	assert.Equal(t, "installation-a", got.Kubernetes.KubeSystemUID)
	assert.Equal(t, result, *got.LastValidation)
	assert.Equal(t, result.CheckedAt, *got.LastValidationAt)
	result.Checks[0].Message = "mutated outside store"
	got.LastValidation.Checks[0].Message = "mutated read"
	list, err := store.List(t.Context(), cluster.Workspace)
	require.NoError(t, err)
	assert.Empty(t, list[0].LastValidation.Checks[0].Message)
	list[0].LastValidation.Checks[0].Message = "mutated list"

	report.Checks = []clusterregistry.AccessCheck{{
		Name:    "credential",
		Code:    "CREDENTIAL_UNAVAILABLE",
		Message: "Credential unavailable.",
	}}
	report.KubeSystemUID = ""
	result, err = service.TestConnection(t.Context(), caller, cluster.Workspace, cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, clusterregistry.ValidationFailed, result.Status)
	got, err = store.Get(t.Context(), cluster.Workspace, cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, "installation-a", got.Kubernetes.KubeSystemUID)
	assert.Equal(t, clusterregistry.ValidationFailed, got.ValidationStatus)

	report.KubeSystemUID = "installation-b"
	report.Checks = []clusterregistry.AccessCheck{{
		Name:   "connection",
		Passed: true,
		Code:   "OK",
	}}
	result, err = service.TestConnection(t.Context(), caller, cluster.Workspace, cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, clusterregistry.ValidationFailed, result.Status)
	require.Len(t, result.Checks, 2)
	assert.Equal(t, "CLUSTER_CHANGED", result.Checks[1].Code)
	got, err = store.Get(t.Context(), cluster.Workspace, cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, "installation-a", got.Kubernetes.KubeSystemUID)
	assert.Empty(t, got.LastValidation.Checks[0].Message)

	report.KubeSystemUID = "installation-a"
	result, err = service.TestConnection(t.Context(), caller, cluster.Workspace, cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, clusterregistry.ValidationReady, result.Status)
	require.NoError(t, service.registry.Update(t.Context(), caller, cluster.Workspace, cluster.ID, clusterregistry.UpdateParams{
		Name:          cluster.Name,
		CredentialRef: "replacement-reference",
		Kubernetes:    cluster.Kubernetes.Connection,
	}))
	got, err = store.Get(t.Context(), cluster.Workspace, cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, clusterregistry.ValidationPending, got.ValidationStatus)
	assert.Nil(t, got.LastValidation)
	assert.Nil(t, got.LastValidationAt)
	assert.Equal(t, "installation-a", got.Kubernetes.KubeSystemUID)
}

func TestValidationRevisionAndAccess(t *testing.T) {
	store, service, caller, cluster := validationFixture(t, nil)
	_, err := service.TestConnection(t.Context(), caller, cluster.Workspace, cluster.ID)
	require.ErrorIs(t, err, ErrUnavailable)
	called := false
	service.connectionTest = func(context.Context, clusterregistry.Cluster) clusterregistry.ValidationResult {
		called = true
		return clusterregistry.ValidationResult{}
	}
	_, err = service.TestConnection(t.Context(), clusterregistry.Caller{}, cluster.Workspace, cluster.ID)
	require.ErrorIs(t, err, clusterregistry.ErrUnauthenticated)
	_, err = service.TestConnection(t.Context(), caller, clusterregistry.Workspace{ID: "other"}, cluster.ID)
	require.ErrorIs(t, err, clusterregistry.ErrWorkspaceNotFound)
	_, err = service.TestConnection(t.Context(), caller, cluster.Workspace, uuid.New())
	require.ErrorIs(t, err, clusterregistry.ErrNotFound)
	assert.False(t, called)
	started, finish := make(chan struct{}), make(chan struct{})
	service.connectionTest = func(ctx context.Context, _ clusterregistry.Cluster) clusterregistry.ValidationResult {
		_, hasDeadline := ctx.Deadline()
		assert.True(t, hasDeadline)
		close(started)
		<-finish
		return clusterregistry.ValidationResult{
			KubeSystemUID: "installation-a",
			Checks: []clusterregistry.AccessCheck{{
				Name:   "connection",
				Passed: true,
			}},
		}
	}
	done := make(chan error, 1)
	go func() {
		_, err := service.TestConnection(t.Context(), caller, cluster.Workspace, cluster.ID)
		done <- err
	}()
	<-started
	require.NoError(t, service.registry.Update(t.Context(), caller, cluster.Workspace, cluster.ID, clusterregistry.UpdateParams{
		Name:          "changed",
		CredentialRef: cluster.CredentialRef,
		Kubernetes:    cluster.Kubernetes.Connection,
	}))
	close(finish)
	require.ErrorIs(t, <-done, clusterregistry.ErrValidationConflict)
	got, err := store.Get(t.Context(), cluster.Workspace, cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, clusterregistry.ValidationPending, got.ValidationStatus)
	assert.Nil(t, got.LastValidation)
	assert.Equal(t, "changed", got.Name)
}

func TestEmptyValidationResultFailsClosed(t *testing.T) {
	store, service, caller, cluster := validationFixture(t, func(context.Context, clusterregistry.Cluster) clusterregistry.ValidationResult {
		return clusterregistry.ValidationResult{}
	})
	result, err := service.TestConnection(t.Context(), caller, cluster.Workspace, cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, clusterregistry.ValidationFailed, result.Status)
	require.Len(t, result.Checks, 1)
	assert.Equal(t, "NO_RESULT", result.Checks[0].Code)
	stored, err := store.Get(t.Context(), cluster.Workspace, cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, result, *stored.LastValidation)
}

func TestValidationResultComplete(t *testing.T) {
	checkedAt := time.Date(2026, time.November, 4, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name     string
		knownUID string
		input    clusterregistry.ValidationResult
		want     clusterregistry.ValidationResult
	}{
		{
			name: "successful checks with a new fingerprint",
			input: clusterregistry.ValidationResult{
				KubeSystemUID: "installation-a",
				Checks:        []clusterregistry.AccessCheck{{Name: "connection", Passed: true}},
			},
			want: clusterregistry.ValidationResult{
				Status:        clusterregistry.ValidationReady,
				CheckedAt:     checkedAt,
				KubeSystemUID: "installation-a",
				Checks:        []clusterregistry.AccessCheck{{Name: "connection", Passed: true}},
			},
		},
		{
			name:     "matching known fingerprint",
			knownUID: "installation-a",
			input: clusterregistry.ValidationResult{
				KubeSystemUID: "installation-a",
				Checks:        []clusterregistry.AccessCheck{{Name: "connection", Passed: true}},
			},
			want: clusterregistry.ValidationResult{
				Status:        clusterregistry.ValidationReady,
				CheckedAt:     checkedAt,
				KubeSystemUID: "installation-a",
				Checks:        []clusterregistry.AccessCheck{{Name: "connection", Passed: true}},
			},
		},
		{
			name: "a failed check overrides adapter status",
			input: clusterregistry.ValidationResult{
				Status:        clusterregistry.ValidationReady,
				CheckedAt:     checkedAt.Add(-time.Hour),
				KubeSystemUID: "installation-a",
				Checks: []clusterregistry.AccessCheck{
					{Name: "connection", Passed: true},
					{Name: "services", Code: "FORBIDDEN"},
				},
			},
			want: clusterregistry.ValidationResult{
				Status:        clusterregistry.ValidationFailed,
				CheckedAt:     checkedAt,
				KubeSystemUID: "installation-a",
				Checks: []clusterregistry.AccessCheck{
					{Name: "connection", Passed: true},
					{Name: "services", Code: "FORBIDDEN"},
				},
			},
		},
		{
			name: "a missing fingerprint prevents readiness",
			input: clusterregistry.ValidationResult{
				Checks: []clusterregistry.AccessCheck{{Name: "connection", Passed: true}},
			},
			want: clusterregistry.ValidationResult{
				Status:    clusterregistry.ValidationFailed,
				CheckedAt: checkedAt,
				Checks:    []clusterregistry.AccessCheck{{Name: "connection", Passed: true}},
			},
		},
		{
			name: "no checks fail closed even with a fingerprint",
			input: clusterregistry.ValidationResult{
				KubeSystemUID: "installation-a",
			},
			want: clusterregistry.ValidationResult{
				Status:        clusterregistry.ValidationFailed,
				CheckedAt:     checkedAt,
				KubeSystemUID: "installation-a",
				Checks: []clusterregistry.AccessCheck{{
					Name:    "validation",
					Code:    "NO_RESULT",
					Message: "The validator did not return any access checks.",
				}},
			},
		},
		{
			name:     "different installation fails continuity",
			knownUID: "installation-a",
			input: clusterregistry.ValidationResult{
				KubeSystemUID: "installation-b",
				Checks:        []clusterregistry.AccessCheck{{Name: "connection", Passed: true}},
			},
			want: clusterregistry.ValidationResult{
				Status:        clusterregistry.ValidationFailed,
				CheckedAt:     checkedAt,
				KubeSystemUID: "installation-b",
				Checks: []clusterregistry.AccessCheck{
					{Name: "connection", Passed: true},
					{
						Name:    "continuity",
						Code:    "CLUSTER_CHANGED",
						Message: "The installation fingerprint differs from the registered cluster; it was not replaced.",
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := completeResult(tt.input, tt.knownUID, checkedAt)
			assert.Equal(t, tt.want, got)
		})
	}
}
