package sources

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/naira-project/naira/catalog/internal/workspaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Any store call panics through the nil embedded interface.
type inaccessibleStore struct{ Store }

func TestServiceDeniesBeforeSourceAccess(t *testing.T) {
	workspaceService := workspaces.NewService(workspaces.NewMemoryStore())
	owner := workspaces.Owner{
		Issuer:  "https://issuer.example",
		Subject: "alice",
	}
	workspace, err := workspaceService.Create(t.Context(), owner, "production")
	require.NoError(t, err)
	service := NewService(inaccessibleStore{}, workspaceService)
	for _, caller := range []workspaces.Owner{
		{
			Issuer:  owner.Issuer,
			Subject: "bob",
		},
		{
			Issuer:  "https://other.example",
			Subject: owner.Subject,
		},
		{},
	} {
		want := workspaces.ErrForbidden
		if caller == (workspaces.Owner{}) {
			want = workspaces.ErrUnauthenticated
		}
		registration, err := service.Create(t.Context(), caller, CreateParams{WorkspaceID: workspace.ID.String()})
		require.ErrorIs(t, err, want)
		assert.Equal(t, Registration{}, registration)
		registration, err = service.Get(t.Context(), caller, workspace.ID.String(), uuid.New())
		require.ErrorIs(t, err, want)
		assert.Equal(t, Registration{}, registration)
		list, err := service.List(t.Context(), caller, workspace.ID.String())
		require.ErrorIs(t, err, want)
		assert.Nil(t, list)
		require.ErrorIs(t, service.Update(t.Context(), caller, workspace.ID.String(), uuid.New(), UpdateParams{}), want)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = service.Create(ctx, owner, CreateParams{WorkspaceID: workspace.ID.String()})
	require.ErrorIs(t, err, context.Canceled)
	_, err = service.Get(ctx, owner, workspace.ID.String(), uuid.New())
	require.ErrorIs(t, err, context.Canceled)
	_, err = service.List(ctx, owner, workspace.ID.String())
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, service.Update(ctx, owner, workspace.ID.String(), uuid.New(), UpdateParams{}), context.Canceled)
}

func TestServiceOwnedRegistrations(t *testing.T) {
	workspaceService := workspaces.NewService(workspaces.NewMemoryStore())
	owner := workspaces.Owner{
		Issuer:  "https://issuer.example",
		Subject: "alice",
	}
	workspace, err := workspaceService.Create(t.Context(), owner, "production")
	require.NoError(t, err)
	other, err := workspaceService.Create(t.Context(), owner, "development")
	require.NoError(t, err)
	service := NewService(NewMemoryStore(), workspaceService)
	params := CreateParams{
		Name:          "cluster-a",
		WorkspaceID:   workspace.ID.String(),
		Kind:          KindKubernetes,
		CredentialRef: "secret:production",
		Kubernetes:    &KubernetesConnection{APIEndpoint: "https://production.example.com"},
	}
	first, err := service.Create(t.Context(), owner, params)
	require.NoError(t, err)
	_, err = service.Create(t.Context(), owner, params)
	require.ErrorIs(t, err, ErrAlreadyExists)
	params.Name = "cluster-b"
	second, err := service.Create(t.Context(), owner, params)
	require.NoError(t, err)
	list, err := service.List(t.Context(), owner, workspace.ID.String())
	require.NoError(t, err)
	assert.Equal(t, []Registration{first, second}, list)
	_, err = service.Get(t.Context(), owner, other.ID.String(), first.ID)
	require.ErrorIs(t, err, ErrNotFound)
	require.ErrorIs(t, service.Update(t.Context(), owner, other.ID.String(), first.ID, UpdateParams{}), ErrNotFound)
	require.NoError(t, service.Update(t.Context(), owner, workspace.ID.String(), first.ID, UpdateParams{
		Name:          "renamed",
		CredentialRef: "secret:replacement",
		Kubernetes:    params.Kubernetes,
	}))
	got, err := service.Get(t.Context(), owner, workspace.ID.String(), first.ID)
	require.NoError(t, err)
	first.Name = "renamed"
	first.CredentialRef = "secret:replacement"
	first.UpdatedAt = got.UpdatedAt
	assert.Equal(t, first, got)
	params.Name = ""
	_, err = service.Create(t.Context(), owner, params)
	require.ErrorIs(t, err, ErrInvalidRegistration)
}
