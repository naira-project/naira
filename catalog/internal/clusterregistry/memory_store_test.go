package clusterregistry

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
	clusters := make([]Cluster, 0, 3)
	for _, params := range []CreateParams{
		{
			Name:          "production",
			Workspace:     Workspace{ID: "workspace-a"},
			CredentialRef: "secret:production",
			Kubernetes:    KubernetesConnection{APIEndpoint: "https://production.example.com"},
		},
		{
			Name:          "development",
			Workspace:     Workspace{ID: "workspace-a"},
			CredentialRef: "secret:development",
			Kubernetes:    KubernetesConnection{APIEndpoint: "https://development.example.com"},
		},
		{
			Name:          "production",
			Workspace:     Workspace{ID: "workspace-b"},
			CredentialRef: "secret:other-production",
			Kubernetes:    KubernetesConnection{APIEndpoint: "https://other-production.example.com"},
		},
	} {
		cluster, err := NewCluster(params)
		require.NoError(t, err)
		require.NoError(t, store.Create(t.Context(), cluster))
		clusters = append(clusters, cluster)
		got, err := store.Get(t.Context(), cluster.Workspace, cluster.ID)
		require.NoError(t, err)
		assert.Equal(t, cluster, got)
	}

	got, err := store.List(t.Context(), Workspace{ID: "workspace-a"})
	require.NoError(t, err)
	assert.Equal(t, []Cluster{clusters[1], clusters[0]}, got)
	got, err = store.List(t.Context(), Workspace{ID: "workspace-b"})
	require.NoError(t, err)
	assert.Equal(t, []Cluster{clusters[2]}, got)
	got, err = store.List(t.Context(), Workspace{ID: "unknown-workspace"})
	require.NoError(t, err)
	assert.Equal(t, []Cluster{}, got)

	got, err = NewMemoryStore().List(t.Context(), Workspace{ID: "workspace-a"})
	require.NoError(t, err)
	assert.Empty(t, got, "new store instances do not share state")
}

func TestMemoryStoreCreateRejectsDuplicatesAndInvalidClusters(t *testing.T) {
	base, err := NewCluster(CreateParams{
		Name:          "production",
		Workspace:     Workspace{ID: "workspace-a"},
		CredentialRef: "secret:production",
		Kubernetes:    KubernetesConnection{APIEndpoint: "https://production.example.com"},
	})
	require.NoError(t, err)
	tests := []struct {
		name    string
		change  func(*Cluster)
		wantErr error
	}{
		{
			name:    "duplicate ID",
			change:  func(r *Cluster) { r.Name = "different-name" },
			wantErr: ErrAlreadyExists,
		},
		{
			name:    "duplicate ID across workspaces",
			change:  func(r *Cluster) { r.Workspace.ID = "workspace-b" },
			wantErr: ErrAlreadyExists,
		},
		{
			name:    "duplicate name within workspace",
			change:  func(r *Cluster) { r.ID = uuid.New() },
			wantErr: ErrAlreadyExists,
		},
		{
			name:    "invalid cluster",
			change:  func(r *Cluster) { r.Name = "" },
			wantErr: ErrInvalidCluster,
		},
		{
			name:    "missing workspace",
			change:  func(r *Cluster) { r.Workspace.ID = "" },
			wantErr: ErrInvalidCluster,
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
			got, err := store.List(t.Context(), base.Workspace)
			require.NoError(t, err)
			assert.Equal(t, []Cluster{base}, got)
		})
	}
}

func TestMemoryStoreRejectsCrossWorkspaceAccess(t *testing.T) {
	store := NewMemoryStore()
	cluster, err := NewCluster(CreateParams{
		Name:          "private-name",
		Workspace:     Workspace{ID: "workspace-a"},
		CredentialRef: "private-reference",
		Kubernetes:    KubernetesConnection{APIEndpoint: "https://private-endpoint.example.com"},
	})
	require.NoError(t, err)
	require.NoError(t, store.Create(t.Context(), cluster))

	got, denied := store.Get(t.Context(), Workspace{ID: "workspace-b"}, cluster.ID)
	require.ErrorIs(t, denied, ErrNotFound)
	assert.Equal(t, Cluster{}, got)
	_, missing := store.Get(t.Context(), Workspace{ID: "workspace-b"}, uuid.New())
	require.ErrorIs(t, missing, ErrNotFound)
	assert.EqualError(t, denied, missing.Error())

	params := UpdateParams{
		Name:          "changed",
		CredentialRef: "replacement-reference",
		Kubernetes:    KubernetesConnection{APIEndpoint: "https://replacement.example.com"},
	}
	denied = store.Update(t.Context(), Workspace{ID: "workspace-b"}, cluster.ID, params)
	require.ErrorIs(t, denied, ErrNotFound)
	missing = store.Update(t.Context(), Workspace{ID: "workspace-b"}, uuid.New(), params)
	require.ErrorIs(t, missing, ErrNotFound)
	assert.EqualError(t, denied, missing.Error())
	assert.NotContains(t, denied.Error(), cluster.Name)
	assert.NotContains(t, denied.Error(), cluster.CredentialRef)
	assert.NotContains(t, denied.Error(), cluster.Kubernetes.Connection.APIEndpoint)
	got, err = store.Get(t.Context(), cluster.Workspace, cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, cluster, got)
}

func TestMemoryStoreRejectsInvalidWorkspace(t *testing.T) {
	store := NewMemoryStore()
	for _, workspace := range []string{"", " ", " workspace-a", "workspace-a ", "workspace\na", "workspace\ta"} {
		t.Run(fmt.Sprintf("%q", workspace), func(t *testing.T) {
			got, err := store.Get(t.Context(), Workspace{ID: workspace}, uuid.New())
			require.ErrorIs(t, err, ErrInvalidWorkspace)
			assert.Equal(t, Cluster{}, got)
			list, err := store.List(t.Context(), Workspace{ID: workspace})
			require.ErrorIs(t, err, ErrInvalidWorkspace)
			assert.Nil(t, list)
			err = store.Update(t.Context(), Workspace{ID: workspace}, uuid.New(), UpdateParams{})
			require.ErrorIs(t, err, ErrInvalidWorkspace)
		})
	}
}

func TestMemoryStoreUpdate(t *testing.T) {
	createdAt := time.Now().UTC().Add(-time.Hour)
	validatedAt := createdAt.Add(time.Minute)
	base := Cluster{
		ID:               uuid.New(),
		Name:             "production",
		Workspace:        Workspace{ID: "workspace-a"},
		CredentialRef:    "secret:production",
		ValidationStatus: ValidationReady,
		LastValidationAt: &validatedAt,
		CreatedAt:        createdAt,
		UpdatedAt:        validatedAt,
		Kubernetes: KubernetesDetails{
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
				Kubernetes:    base.Kubernetes.Connection,
			}
			tt.change(&params)
			before := time.Now().UTC()
			require.NoError(t, store.Update(t.Context(), base.Workspace, base.ID, params))
			after := time.Now().UTC()
			got, err := store.Get(t.Context(), base.Workspace, base.ID)
			require.NoError(t, err)
			assert.WithinRange(t, got.UpdatedAt, before, after)
			want := base
			want.Name = params.Name
			want.CredentialRef = params.CredentialRef
			want.UpdatedAt = got.UpdatedAt
			want.Kubernetes = KubernetesDetails{
				Connection:    params.Kubernetes,
				KubeSystemUID: "installation-uid",
			}
			if tt.invalidate {
				want.ValidationStatus = ValidationPending
				want.LastValidationAt = nil
			}
			assert.Equal(t, want, got)
			params.Kubernetes.APIEndpoint = "https://mutated-input.example.com"
			again, err := store.Get(t.Context(), base.Workspace, base.ID)
			require.NoError(t, err)
			assert.Equal(t, want, again)
		})
	}
}

func TestMemoryStoreUpdateFailureIsAtomic(t *testing.T) {
	base, err := NewCluster(CreateParams{
		Name:          "production",
		Workspace:     Workspace{ID: "workspace-a"},
		CredentialRef: "secret:production",
		Kubernetes:    KubernetesConnection{APIEndpoint: "https://production.example.com"},
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
			wantErr: ErrInvalidCluster,
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
			err := store.Update(t.Context(), base.Workspace, base.ID, UpdateParams{
				Name:          tt.newName,
				CredentialRef: "replacement-reference",
				Kubernetes:    KubernetesConnection{APIEndpoint: "https://replacement.example.com"},
			})
			require.ErrorIs(t, err, tt.wantErr)
			got, err := store.Get(t.Context(), base.Workspace, base.ID)
			require.NoError(t, err)
			assert.Equal(t, base, got)
			got, err = store.Get(t.Context(), other.Workspace, other.ID)
			require.NoError(t, err)
			assert.Equal(t, other, got)
		})
	}
}

func TestMemoryStoreCopiesMutableFields(t *testing.T) {
	store := NewMemoryStore()
	createdAt := time.Now().UTC().Add(-time.Hour)
	cluster := Cluster{
		ID:               uuid.New(),
		Name:             "production",
		Workspace:        Workspace{ID: "workspace-a"},
		CredentialRef:    "secret:production",
		ValidationStatus: ValidationReady,
		LastValidationAt: new(createdAt.Add(time.Minute)),
		CreatedAt:        createdAt,
		UpdatedAt:        createdAt.Add(time.Minute),
		Kubernetes: KubernetesDetails{
			Connection:    KubernetesConnection{APIEndpoint: "https://production.example.com"},
			KubeSystemUID: "installation-uid",
		},
	}
	want := cluster
	want.LastValidationAt = new(*cluster.LastValidationAt)
	require.NoError(t, store.Create(t.Context(), cluster))
	cluster.Kubernetes.KubeSystemUID = "mutated-input"
	*cluster.LastValidationAt = time.Time{}

	got, err := store.Get(t.Context(), want.Workspace, want.ID)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	got.Kubernetes.Connection.APIEndpoint = "https://mutated-output.example.com"
	*got.LastValidationAt = time.Time{}

	list, err := store.List(t.Context(), want.Workspace)
	require.NoError(t, err)
	require.Equal(t, []Cluster{want}, list)
	list[0].Kubernetes.KubeSystemUID = "mutated-list"
	*list[0].LastValidationAt = time.Time{}
	got, err = store.Get(t.Context(), want.Workspace, want.ID)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestMemoryStoreNameUniquenessAfterRename(t *testing.T) {
	store := NewMemoryStore()
	cluster, err := NewCluster(CreateParams{
		Name:          "production",
		Workspace:     Workspace{ID: "workspace-a"},
		CredentialRef: "secret:production",
		Kubernetes:    KubernetesConnection{APIEndpoint: "https://production.example.com"},
	})
	require.NoError(t, err)
	require.NoError(t, store.Create(t.Context(), cluster))
	other := cluster
	other.ID = uuid.New()
	other.Workspace.ID = "workspace-b"
	other.Name = "renamed"
	require.NoError(t, store.Create(t.Context(), other))

	require.NoError(t, store.Update(t.Context(), cluster.Workspace, cluster.ID, UpdateParams{
		Name:          "renamed",
		CredentialRef: cluster.CredentialRef,
		Kubernetes:    cluster.Kubernetes.Connection,
	}))

	for _, name := range []string{"production", "Renamed"} {
		candidate := cluster
		candidate.ID = uuid.New()
		candidate.Name = name
		require.NoError(t, store.Create(t.Context(), candidate))
	}
	list, err := store.List(t.Context(), cluster.Workspace)
	require.NoError(t, err)
	require.Len(t, list, 3)
	assert.Equal(t, []string{"Renamed", "production", "renamed"}, []string{list[0].Name, list[1].Name, list[2].Name})
}

func TestMemoryStoreCanceledContext(t *testing.T) {
	store := NewMemoryStore()
	cluster, err := NewCluster(CreateParams{
		Name:          "production",
		Workspace:     Workspace{ID: "workspace-a"},
		CredentialRef: "secret:production",
		Kubernetes:    KubernetesConnection{APIEndpoint: "https://production.example.com"},
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, store.Create(ctx, cluster), context.Canceled)
	list, err := store.List(t.Context(), cluster.Workspace)
	require.NoError(t, err)
	assert.Empty(t, list)
	require.NoError(t, store.Create(t.Context(), cluster))
	got, err := store.Get(ctx, cluster.Workspace, cluster.ID)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, Cluster{}, got)
	list, err = store.List(ctx, cluster.Workspace)
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, list)
	err = store.Update(ctx, cluster.Workspace, cluster.ID, UpdateParams{
		Name:          "renamed",
		CredentialRef: cluster.CredentialRef,
		Kubernetes:    cluster.Kubernetes.Connection,
	})
	require.ErrorIs(t, err, context.Canceled)
	got, err = store.Get(t.Context(), cluster.Workspace, cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, cluster, got)
}

func TestMemoryStoreConcurrentCreate(t *testing.T) {
	store := NewMemoryStore()
	const writers = 20
	results := make(chan error, writers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range writers {
		cluster, err := NewCluster(CreateParams{
			Name:          "production",
			Workspace:     Workspace{ID: "workspace-a"},
			CredentialRef: "secret:production",
			Kubernetes:    KubernetesConnection{APIEndpoint: "https://production.example.com"},
		})
		require.NoError(t, err)
		wg.Go(func() {
			<-start
			results <- store.Create(t.Context(), cluster)
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
	list, err := store.List(t.Context(), Workspace{ID: "workspace-a"})
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
		cluster, err := NewCluster(CreateParams{
			Name:          fmt.Sprintf("cluster-%02d", i),
			Workspace:     Workspace{ID: "workspace-a"},
			CredentialRef: "secret:production",
			Kubernetes:    KubernetesConnection{APIEndpoint: "https://production.example.com"},
		})
		require.NoError(t, err)
		require.NoError(t, store.Create(t.Context(), cluster))
		wg.Go(func() {
			<-start
			results <- store.Update(t.Context(), cluster.Workspace, cluster.ID, UpdateParams{
				Name:          "shared-name",
				CredentialRef: cluster.CredentialRef,
				Kubernetes:    cluster.Kubernetes.Connection,
			})
			got, err := store.Get(t.Context(), cluster.Workspace, cluster.ID)
			if assert.NoError(t, err) {
				got.Kubernetes.KubeSystemUID = "local-only"
			}
			list, err := store.List(t.Context(), cluster.Workspace)
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
	list, err := store.List(t.Context(), Workspace{ID: "workspace-a"})
	require.NoError(t, err)
	assert.Len(t, list, writers)
}

func TestSaveValidationRejectsInvalidState(t *testing.T) {
	store := NewMemoryStore()
	cluster, err := NewCluster(CreateParams{
		Name:          "production",
		Workspace:     Workspace{ID: TestWorkspaceID},
		CredentialRef: "approved-reference",
		Kubernetes:    KubernetesConnection{APIEndpoint: "https://cluster.example.com"},
	})
	require.NoError(t, err)
	require.NoError(t, store.Create(t.Context(), cluster))
	result := ValidationResult{Status: ValidationReady, CheckedAt: time.Now().UTC()}
	for _, tt := range []struct {
		name      string
		workspace Workspace
		id        uuid.UUID
		result    ValidationResult
		want      error
	}{
		{
			name:      "missing cluster",
			workspace: cluster.Workspace,
			id:        uuid.New(),
			result:    result,
			want:      ErrNotFound,
		},
		{
			name:      "other workspace",
			workspace: Workspace{ID: "other"},
			id:        cluster.ID,
			result:    result,
			want:      ErrNotFound,
		},
		{
			name:   "empty workspace",
			id:     cluster.ID,
			result: result,
			want:   ErrInvalidWorkspace,
		},
		{
			name:      "ready missing UID",
			workspace: cluster.Workspace,
			id:        cluster.ID,
			result:    result,
			want:      ErrInvalidCluster,
		},
		{
			name:      "pending result",
			workspace: cluster.Workspace,
			id:        cluster.ID,
			result:    ValidationResult{Status: ValidationPending},
			want:      ErrInvalidCluster,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.ErrorIs(t, store.SaveValidation(t.Context(), tt.workspace, tt.id, cluster.UpdatedAt, tt.result), tt.want)
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, store.SaveValidation(ctx, cluster.Workspace, cluster.ID, cluster.UpdatedAt, result), context.Canceled)
	got, err := store.Get(t.Context(), cluster.Workspace, cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, cluster, got)
}

func TestMemoryStoreRenamePreservesValidation(t *testing.T) {
	store := NewMemoryStore()
	cluster, err := NewCluster(CreateParams{
		Name:          "production",
		Workspace:     Workspace{ID: TestWorkspaceID},
		CredentialRef: "approved-reference",
		Kubernetes:    KubernetesConnection{APIEndpoint: "https://cluster.example.com"},
	})
	require.NoError(t, err)
	require.NoError(t, store.Create(t.Context(), cluster))
	result := ValidationResult{
		Status:        ValidationReady,
		CheckedAt:     time.Now().UTC(),
		KubeSystemUID: "installation-a",
		Checks: []AccessCheck{{
			Name:   "connection",
			Passed: true,
		}},
	}
	require.NoError(t, store.SaveValidation(t.Context(), cluster.Workspace, cluster.ID, cluster.UpdatedAt, result))
	want, err := store.Get(t.Context(), cluster.Workspace, cluster.ID)
	require.NoError(t, err)
	params := UpdateParams{
		Name:          "renamed",
		CredentialRef: cluster.CredentialRef,
		Kubernetes:    cluster.Kubernetes.Connection,
	}
	require.NoError(t, store.Update(t.Context(), cluster.Workspace, cluster.ID, params))
	got, err := store.Get(t.Context(), cluster.Workspace, cluster.ID)
	require.NoError(t, err)
	want.Name = params.Name
	want.UpdatedAt = got.UpdatedAt
	assert.Equal(t, want, got)

	got.LastValidation.Checks[0].Message = "changed outside store"
	*got.LastValidationAt = time.Time{}
	again, err := store.Get(t.Context(), cluster.Workspace, cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, want, again)
	assert.Empty(t, result.Checks[0].Message)
}
