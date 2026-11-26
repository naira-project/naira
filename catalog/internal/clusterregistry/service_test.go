package clusterregistry

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Any store call panics through the nil embedded interface.
type inaccessibleStore struct{ Store }

func TestServiceDeniesBeforeClusterAccess(t *testing.T) {
	caller := Caller{
		Issuer:  "https://issuer.example",
		Subject: "alice",
	}
	service := NewService(inaccessibleStore{})
	for _, tt := range []struct {
		name      string
		caller    Caller
		workspace Workspace
		want      error
	}{
		{
			name:      "unauthenticated",
			workspace: Workspace{ID: TestWorkspaceID},
			want:      ErrUnauthenticated,
		},
		{
			name:      "missing issuer",
			caller:    Caller{Subject: "alice"},
			workspace: Workspace{ID: TestWorkspaceID},
			want:      ErrUnauthenticated,
		},
		{
			name:      "missing subject",
			caller:    Caller{Issuer: caller.Issuer},
			workspace: Workspace{ID: TestWorkspaceID},
			want:      ErrUnauthenticated,
		},
		{
			name:      "unknown workspace",
			caller:    caller,
			workspace: Workspace{ID: "other-workspace"},
			want:      ErrWorkspaceNotFound,
		},
		{
			name:      "empty workspace",
			caller:    caller,
			workspace: Workspace{},
			want:      ErrWorkspaceNotFound,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cluster, err := service.Create(t.Context(), tt.caller, CreateParams{Workspace: tt.workspace})
			require.ErrorIs(t, err, tt.want)
			assert.Equal(t, Cluster{}, cluster)
			cluster, err = service.Get(t.Context(), tt.caller, tt.workspace, uuid.New())
			require.ErrorIs(t, err, tt.want)
			assert.Equal(t, Cluster{}, cluster)
			list, err := service.List(t.Context(), tt.caller, tt.workspace)
			require.ErrorIs(t, err, tt.want)
			assert.Nil(t, list)
			require.ErrorIs(t, service.Update(t.Context(), tt.caller, tt.workspace, uuid.New(), UpdateParams{}), tt.want)
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	workspace := Workspace{ID: TestWorkspaceID}
	_, err := service.Create(ctx, caller, CreateParams{Workspace: workspace})
	require.ErrorIs(t, err, context.Canceled)
	_, err = service.Get(ctx, caller, workspace, uuid.New())
	require.ErrorIs(t, err, context.Canceled)
	_, err = service.List(ctx, caller, workspace)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, service.Update(ctx, caller, workspace, uuid.New(), UpdateParams{}), context.Canceled)
}

func TestServiceSharedTestWorkspace(t *testing.T) {
	alice := Caller{
		Issuer:  "https://issuer.example",
		Subject: "alice",
	}
	bob := Caller{
		Issuer:  alice.Issuer,
		Subject: "bob",
	}
	workspace := Workspace{ID: TestWorkspaceID}
	service := NewService(NewMemoryStore())
	params := CreateParams{
		Name:          "cluster-a",
		Workspace:     workspace,
		CredentialRef: "secret:production",
		Kubernetes:    KubernetesConnection{APIEndpoint: "https://production.example.com"},
	}
	first, err := service.Create(t.Context(), alice, params)
	require.NoError(t, err)
	_, err = service.Create(t.Context(), bob, params)
	require.ErrorIs(t, err, ErrAlreadyExists)
	params.Name = "cluster-b"
	second, err := service.Create(t.Context(), bob, params)
	require.NoError(t, err)
	for _, caller := range []Caller{alice, bob} {
		list, err := service.List(t.Context(), caller, workspace)
		require.NoError(t, err)
		assert.Equal(t, []Cluster{first, second}, list)
	}
	_, err = service.Get(t.Context(), bob, workspace, uuid.New())
	require.ErrorIs(t, err, ErrNotFound)
	require.ErrorIs(t, service.Update(t.Context(), bob, workspace, uuid.New(), UpdateParams{}), ErrNotFound)
	require.NoError(t, service.Update(t.Context(), bob, workspace, first.ID, UpdateParams{
		Name:          "renamed",
		CredentialRef: "secret:replacement",
		Kubernetes:    params.Kubernetes,
	}))
	got, err := service.Get(t.Context(), alice, workspace, first.ID)
	require.NoError(t, err)
	first.Name = "renamed"
	first.CredentialRef = "secret:replacement"
	first.UpdatedAt = got.UpdatedAt
	assert.Equal(t, first, got)
	params.Name = ""
	_, err = service.Create(t.Context(), alice, params)
	require.ErrorIs(t, err, ErrInvalidCluster)
	list, err := NewService(NewMemoryStore()).List(t.Context(), alice, workspace)
	require.NoError(t, err)
	assert.Empty(t, list, "a fresh process starts with no registered clusters")
}
