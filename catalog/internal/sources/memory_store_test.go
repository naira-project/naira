package sources

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryStoreCreateGetAndList(t *testing.T) {
	store := NewMemoryStore()
	registrations := make([]Registration, 0, 3)
	for _, params := range []CreateParams{
		{
			Name:          "production",
			WorkspaceID:   "workspace-a",
			Kind:          KindKubernetes,
			CredentialRef: "secret:production",
			Kubernetes:    &KubernetesConnection{APIEndpoint: "https://production.example.com"},
		},
		{
			Name:          "development",
			WorkspaceID:   "workspace-a",
			Kind:          KindKubernetes,
			CredentialRef: "secret:development",
			Kubernetes:    &KubernetesConnection{APIEndpoint: "https://development.example.com"},
		},
		{
			Name:          "production",
			WorkspaceID:   "workspace-b",
			Kind:          KindKubernetes,
			CredentialRef: "secret:other-production",
			Kubernetes:    &KubernetesConnection{APIEndpoint: "https://other-production.example.com"},
		},
	} {
		registration, err := NewRegistration(params)
		require.NoError(t, err)
		require.NoError(t, store.Create(t.Context(), registration))
		registrations = append(registrations, registration)
		got, err := store.Get(t.Context(), registration.WorkspaceID, registration.ID)
		require.NoError(t, err)
		assert.Equal(t, registration, got)
	}

	got, err := store.List(t.Context(), "workspace-a")
	require.NoError(t, err)
	assert.Equal(t, []Registration{registrations[1], registrations[0]}, got)
	got, err = store.List(t.Context(), "workspace-b")
	require.NoError(t, err)
	assert.Equal(t, []Registration{registrations[2]}, got)
	got, err = store.List(t.Context(), "unknown-workspace")
	require.NoError(t, err)
	assert.Equal(t, []Registration{}, got)

	got, err = NewMemoryStore().List(t.Context(), "workspace-a")
	require.NoError(t, err)
	assert.Empty(t, got, "new store instances do not share state")
}

func TestMemoryStoreCreateRejectsDuplicatesAndInvalidRegistrations(t *testing.T) {
	base, err := NewRegistration(CreateParams{
		Name:          "production",
		WorkspaceID:   "workspace-a",
		Kind:          KindKubernetes,
		CredentialRef: "secret:production",
		Kubernetes:    &KubernetesConnection{APIEndpoint: "https://production.example.com"},
	})
	require.NoError(t, err)
	tests := []struct {
		name    string
		change  func(*Registration)
		wantErr error
	}{
		{
			name:    "duplicate ID",
			change:  func(r *Registration) { r.Name = "different-name" },
			wantErr: ErrAlreadyExists,
		},
		{
			name:    "duplicate ID across workspaces",
			change:  func(r *Registration) { r.WorkspaceID = "workspace-b" },
			wantErr: ErrAlreadyExists,
		},
		{
			name:    "duplicate name within workspace",
			change:  func(r *Registration) { r.ID = uuid.New() },
			wantErr: ErrAlreadyExists,
		},
		{
			name:    "invalid registration",
			change:  func(r *Registration) { r.Name = "" },
			wantErr: ErrInvalidRegistration,
		},
		{
			name:    "missing workspace",
			change:  func(r *Registration) { r.WorkspaceID = "" },
			wantErr: ErrInvalidRegistration,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewMemoryStore()
			require.NoError(t, store.Create(t.Context(), base))
			candidate := base
			tt.change(&candidate)
			err := store.Create(t.Context(), candidate)
			require.ErrorIs(t, err, tt.wantErr)
			got, err := store.List(t.Context(), base.WorkspaceID)
			require.NoError(t, err)
			assert.Equal(t, []Registration{base}, got)
		})
	}
}

func TestMemoryStoreRejectsCrossWorkspaceAccess(t *testing.T) {
	store := NewMemoryStore()
	registration, err := NewRegistration(CreateParams{
		Name:          "private-name",
		WorkspaceID:   "workspace-a",
		Kind:          KindKubernetes,
		CredentialRef: "private-reference",
		Kubernetes:    &KubernetesConnection{APIEndpoint: "https://private-endpoint.example.com"},
	})
	require.NoError(t, err)
	require.NoError(t, store.Create(t.Context(), registration))

	got, denied := store.Get(t.Context(), "workspace-b", registration.ID)
	require.ErrorIs(t, denied, ErrNotFound)
	assert.Equal(t, Registration{}, got)
	_, missing := store.Get(t.Context(), "workspace-b", uuid.New())
	require.ErrorIs(t, missing, ErrNotFound)
	assert.EqualError(t, denied, missing.Error())

	params := UpdateParams{
		Name:          "changed",
		CredentialRef: "replacement-reference",
		Kubernetes:    &KubernetesConnection{APIEndpoint: "https://replacement.example.com"},
	}
	denied = store.Update(t.Context(), "workspace-b", registration.ID, params)
	require.ErrorIs(t, denied, ErrNotFound)
	missing = store.Update(t.Context(), "workspace-b", uuid.New(), params)
	require.ErrorIs(t, missing, ErrNotFound)
	assert.EqualError(t, denied, missing.Error())
	assert.NotContains(t, denied.Error(), registration.Name)
	assert.NotContains(t, denied.Error(), registration.CredentialRef)
	assert.NotContains(t, denied.Error(), registration.Kubernetes.Connection.APIEndpoint)
	got, err = store.Get(t.Context(), registration.WorkspaceID, registration.ID)
	require.NoError(t, err)
	assert.Equal(t, registration, got)
}

func TestMemoryStoreRejectsInvalidWorkspace(t *testing.T) {
	store := NewMemoryStore()
	for _, workspace := range []string{"", " ", " workspace-a", "workspace-a ", "workspace\na", "workspace\ta"} {
		t.Run(fmt.Sprintf("%q", workspace), func(t *testing.T) {
			got, err := store.Get(t.Context(), workspace, uuid.New())
			require.ErrorIs(t, err, ErrInvalidRegistration)
			assert.Equal(t, Registration{}, got)
			list, err := store.List(t.Context(), workspace)
			require.ErrorIs(t, err, ErrInvalidRegistration)
			assert.Nil(t, list)
			err = store.Update(t.Context(), workspace, uuid.New(), UpdateParams{})
			require.ErrorIs(t, err, ErrInvalidRegistration)
		})
	}
}

func TestMemoryStoreUpdate(t *testing.T) {
	createdAt := time.Now().UTC().Add(-time.Hour)
	validatedAt := createdAt.Add(time.Minute)
	base := Registration{
		ID:               uuid.New(),
		Name:             "production",
		WorkspaceID:      "workspace-a",
		Kind:             KindKubernetes,
		CredentialRef:    "secret:production",
		ValidationStatus: ValidationReady,
		LastValidationAt: &validatedAt,
		CreatedAt:        createdAt,
		UpdatedAt:        validatedAt,
		Kubernetes: &KubernetesDetails{
			Connection:    KubernetesConnection{APIEndpoint: "https://production.example.com"},
			KubeSystemUID: "installation-uid",
		},
	}
	tests := []struct {
		name       string
		change     func(*UpdateParams)
		invalidate bool
	}{
		{
			name:   "rename preserves validation",
			change: func(p *UpdateParams) { p.Name = "renamed" },
		},
		{
			name:       "credential reference change invalidates validation",
			change:     func(p *UpdateParams) { p.CredentialRef = "replacement-reference" },
			invalidate: true,
		},
		{
			name:       "endpoint change invalidates validation",
			change:     func(p *UpdateParams) { p.Kubernetes.APIEndpoint = "https://replacement.example.com" },
			invalidate: true,
		},
		{
			name:       "context change invalidates validation",
			change:     func(p *UpdateParams) { p.Kubernetes.Context = "another-context" },
			invalidate: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewMemoryStore()
			require.NoError(t, store.Create(t.Context(), base))
			params := UpdateParams{
				Name:          base.Name,
				CredentialRef: base.CredentialRef,
				Kubernetes:    new(base.Kubernetes.Connection),
			}
			tt.change(&params)
			before := time.Now().UTC()
			require.NoError(t, store.Update(t.Context(), base.WorkspaceID, base.ID, params))
			after := time.Now().UTC()
			got, err := store.Get(t.Context(), base.WorkspaceID, base.ID)
			require.NoError(t, err)
			assert.WithinRange(t, got.UpdatedAt, before, after)
			want := base
			want.Name = params.Name
			want.CredentialRef = params.CredentialRef
			want.UpdatedAt = got.UpdatedAt
			want.Kubernetes = &KubernetesDetails{
				Connection:    *params.Kubernetes,
				KubeSystemUID: "installation-uid",
			}
			if tt.invalidate {
				want.ValidationStatus = ValidationPending
				want.LastValidationAt = nil
			}
			assert.Equal(t, want, got)
			params.Kubernetes.APIEndpoint = "https://mutated-input.example.com"
			again, err := store.Get(t.Context(), base.WorkspaceID, base.ID)
			require.NoError(t, err)
			assert.Equal(t, want, again)
		})
	}
}

func TestMemoryStoreUpdateFailureIsAtomic(t *testing.T) {
	base, err := NewRegistration(CreateParams{
		Name:          "production",
		WorkspaceID:   "workspace-a",
		Kind:          KindKubernetes,
		CredentialRef: "secret:production",
		Kubernetes:    &KubernetesConnection{APIEndpoint: "https://production.example.com"},
	})
	require.NoError(t, err)
	other := base
	other.ID = uuid.New()
	other.Name = "already-taken"
	for _, tt := range []struct {
		name    string
		newName string
		wantErr error
	}{
		{
			name:    "invalid metadata",
			newName: "",
			wantErr: ErrInvalidRegistration,
		},
		{
			name:    "name conflict",
			newName: "already-taken",
			wantErr: ErrAlreadyExists,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := NewMemoryStore()
			require.NoError(t, store.Create(t.Context(), base))
			require.NoError(t, store.Create(t.Context(), other))
			err := store.Update(t.Context(), base.WorkspaceID, base.ID, UpdateParams{
				Name:          tt.newName,
				CredentialRef: "replacement-reference",
				Kubernetes:    &KubernetesConnection{APIEndpoint: "https://replacement.example.com"},
			})
			require.ErrorIs(t, err, tt.wantErr)
			got, err := store.Get(t.Context(), base.WorkspaceID, base.ID)
			require.NoError(t, err)
			assert.Equal(t, base, got)
			got, err = store.Get(t.Context(), other.WorkspaceID, other.ID)
			require.NoError(t, err)
			assert.Equal(t, other, got)
		})
	}
}

func TestMemoryStoreCopiesMutableFields(t *testing.T) {
	store := NewMemoryStore()
	createdAt := time.Now().UTC().Add(-time.Hour)
	registration := Registration{
		ID:               uuid.New(),
		Name:             "production",
		WorkspaceID:      "workspace-a",
		Kind:             KindKubernetes,
		CredentialRef:    "secret:production",
		ValidationStatus: ValidationReady,
		LastValidationAt: new(createdAt.Add(time.Minute)),
		CreatedAt:        createdAt,
		UpdatedAt:        createdAt.Add(time.Minute),
		Kubernetes: &KubernetesDetails{
			Connection:    KubernetesConnection{APIEndpoint: "https://production.example.com"},
			KubeSystemUID: "installation-uid",
		},
	}
	want := registration
	want.Kubernetes = new(*registration.Kubernetes)
	want.LastValidationAt = new(*registration.LastValidationAt)
	require.NoError(t, store.Create(t.Context(), registration))
	registration.Kubernetes.KubeSystemUID = "mutated-input"
	*registration.LastValidationAt = time.Time{}

	got, err := store.Get(t.Context(), want.WorkspaceID, want.ID)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	got.Kubernetes.Connection.APIEndpoint = "https://mutated-output.example.com"
	*got.LastValidationAt = time.Time{}

	list, err := store.List(t.Context(), want.WorkspaceID)
	require.NoError(t, err)
	require.Equal(t, []Registration{want}, list)
	list[0].Kubernetes.KubeSystemUID = "mutated-list"
	*list[0].LastValidationAt = time.Time{}
	got, err = store.Get(t.Context(), want.WorkspaceID, want.ID)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestMemoryStoreNameUniquenessAfterRename(t *testing.T) {
	store := NewMemoryStore()
	registration, err := NewRegistration(CreateParams{
		Name:          "production",
		WorkspaceID:   "workspace-a",
		Kind:          KindKubernetes,
		CredentialRef: "secret:production",
		Kubernetes:    &KubernetesConnection{APIEndpoint: "https://production.example.com"},
	})
	require.NoError(t, err)
	require.NoError(t, store.Create(t.Context(), registration))
	other := registration
	other.ID = uuid.New()
	other.WorkspaceID = "workspace-b"
	other.Name = "renamed"
	require.NoError(t, store.Create(t.Context(), other))

	require.NoError(t, store.Update(t.Context(), registration.WorkspaceID, registration.ID, UpdateParams{
		Name:          "renamed",
		CredentialRef: registration.CredentialRef,
		Kubernetes:    new(registration.Kubernetes.Connection),
	}))

	for _, name := range []string{"production", "Renamed"} {
		candidate := registration
		candidate.ID = uuid.New()
		candidate.Name = name
		require.NoError(t, store.Create(t.Context(), candidate))
	}
	list, err := store.List(t.Context(), registration.WorkspaceID)
	require.NoError(t, err)
	require.Len(t, list, 3)
	assert.Equal(t, []string{"Renamed", "production", "renamed"}, []string{list[0].Name, list[1].Name, list[2].Name})
}

func TestMemoryStoreCanceledContext(t *testing.T) {
	store := NewMemoryStore()
	registration, err := NewRegistration(CreateParams{
		Name:          "production",
		WorkspaceID:   "workspace-a",
		Kind:          KindKubernetes,
		CredentialRef: "secret:production",
		Kubernetes:    &KubernetesConnection{APIEndpoint: "https://production.example.com"},
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, store.Create(ctx, registration), context.Canceled)
	list, err := store.List(t.Context(), registration.WorkspaceID)
	require.NoError(t, err)
	assert.Empty(t, list)
	require.NoError(t, store.Create(t.Context(), registration))
	got, err := store.Get(ctx, registration.WorkspaceID, registration.ID)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, Registration{}, got)
	list, err = store.List(ctx, registration.WorkspaceID)
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, list)
	err = store.Update(ctx, registration.WorkspaceID, registration.ID, UpdateParams{
		Name:          "renamed",
		CredentialRef: registration.CredentialRef,
		Kubernetes:    new(registration.Kubernetes.Connection),
	})
	require.ErrorIs(t, err, context.Canceled)
	got, err = store.Get(t.Context(), registration.WorkspaceID, registration.ID)
	require.NoError(t, err)
	assert.Equal(t, registration, got)
}

func TestMemoryStoreConcurrentCreate(t *testing.T) {
	store := NewMemoryStore()
	const writers = 20
	results := make(chan error, writers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range writers {
		registration, err := NewRegistration(CreateParams{
			Name:          "production",
			WorkspaceID:   "workspace-a",
			Kind:          KindKubernetes,
			CredentialRef: "secret:production",
			Kubernetes:    &KubernetesConnection{APIEndpoint: "https://production.example.com"},
		})
		require.NoError(t, err)
		wg.Go(func() {
			<-start
			results <- store.Create(t.Context(), registration)
		})
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, ErrAlreadyExists)
		}
	}
	assert.Equal(t, 1, successes)
	list, err := store.List(t.Context(), "workspace-a")
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestMemoryStoreConcurrentRenameAndRead(t *testing.T) {
	store := NewMemoryStore()
	const writers = 20
	results := make(chan error, writers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range writers {
		registration, err := NewRegistration(CreateParams{
			Name:          fmt.Sprintf("source-%02d", i),
			WorkspaceID:   "workspace-a",
			Kind:          KindKubernetes,
			CredentialRef: "secret:production",
			Kubernetes:    &KubernetesConnection{APIEndpoint: "https://production.example.com"},
		})
		require.NoError(t, err)
		require.NoError(t, store.Create(t.Context(), registration))
		wg.Go(func() {
			<-start
			results <- store.Update(t.Context(), registration.WorkspaceID, registration.ID, UpdateParams{
				Name:          "shared-name",
				CredentialRef: registration.CredentialRef,
				Kubernetes:    new(registration.Kubernetes.Connection),
			})
			got, err := store.Get(t.Context(), registration.WorkspaceID, registration.ID)
			if assert.NoError(t, err) {
				got.Kubernetes.KubeSystemUID = "local-only"
			}
			list, err := store.List(t.Context(), registration.WorkspaceID)
			if assert.NoError(t, err) {
				for _, item := range list {
					assert.Empty(t, item.Kubernetes.KubeSystemUID)
					item.Kubernetes.KubeSystemUID = "local-only"
				}
			}
		})
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, ErrAlreadyExists)
		}
	}
	assert.Equal(t, 1, successes)
	list, err := store.List(t.Context(), "workspace-a")
	require.NoError(t, err)
	assert.Len(t, list, writers)
}
