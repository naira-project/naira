package clusterregistry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceValidate(t *testing.T) {
	for _, id := range []string{
		"workspace-a",
		"af3a70f0-6abc-4e2c-a467-9ba0dd84a77c",
	} {
		t.Run(id, func(t *testing.T) {
			workspace := Workspace{ID: id}
			require.NoError(t, workspace.Validate())
			assert.Equal(t, Workspace{ID: id}, workspace)
		})
	}
	for _, id := range []string{"", " ", " workspace-a", "workspace-a ", "workspace\na", "workspace\ra", "workspace\ta"} {
		t.Run("invalid:"+id, func(t *testing.T) {
			workspace := Workspace{ID: id}
			err := workspace.Validate()
			require.ErrorIs(t, err, ErrInvalidWorkspace)
			assert.ErrorContains(t, err, "workspace ID")
			assert.Equal(t, Workspace{ID: id}, workspace, "validation does not normalize the identity")
		})
	}
}

func TestClusterRejectsInvalidWorkspace(t *testing.T) {
	cluster, err := NewCluster(CreateParams{
		Name:          "production",
		Workspace:     Workspace{},
		CredentialRef: "secret:production",
		Kubernetes:    KubernetesConnection{APIEndpoint: "https://production.example.com"},
	})
	require.ErrorIs(t, err, ErrInvalidCluster)
	require.ErrorIs(t, err, ErrInvalidWorkspace)
	assert.Equal(t, Cluster{}, cluster)
}

func TestWorkspaceValueIsolation(t *testing.T) {
	workspace := Workspace{ID: "workspace-a"}
	params := CreateParams{
		Name:          "production",
		Workspace:     workspace,
		CredentialRef: "secret:production",
		Kubernetes:    KubernetesConnection{APIEndpoint: "https://production.example.com"},
	}
	cluster, err := NewCluster(params)
	require.NoError(t, err)
	want := cluster
	params.Workspace.ID = "workspace-b"
	assert.Equal(t, want, cluster)

	store := NewMemoryStore()
	require.NoError(t, store.Create(t.Context(), cluster))
	cluster.Workspace.ID = "workspace-c"
	got, err := store.Get(t.Context(), workspace, cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, want, got)

	got.Workspace.ID = "workspace-d"
	list, err := store.List(t.Context(), workspace)
	require.NoError(t, err)
	require.Equal(t, []Cluster{want}, list)
	list[0].Workspace.ID = "workspace-e"
	got, err = store.Get(t.Context(), workspace, cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
