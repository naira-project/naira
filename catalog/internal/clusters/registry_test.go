package clusters

import (
	"testing"

	"github.com/naira-project/naira/catalog/internal/clusterregistry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func TestNewWithoutCredentials(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	for _, tt := range []struct {
		name        string
		credentials map[string][]string
	}{
		{name: "nil allowlist"},
		{
			name:        "empty allowlist",
			credentials: map[string][]string{},
		},
		{
			name:        "empty workspace entries",
			credentials: map[string][]string{clusterregistry.TestWorkspaceID: {}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			registry, err := New(tt.credentials)
			require.NoError(t, err)
			caller := clusterregistry.Caller{
				Issuer:  "https://issuer.example.com",
				Subject: "alice",
			}
			workspace := clusterregistry.Workspace{ID: clusterregistry.TestWorkspaceID}
			cluster, err := registry.Registrations.Create(t.Context(), caller, clusterregistry.CreateParams{
				Name:          "target",
				Workspace:     workspace,
				CredentialRef: "kubernetes-secret:naira/reader#kubeconfig",
				Kubernetes:    clusterregistry.KubernetesConnection{APIEndpoint: "https://cluster.example.com"},
			})
			require.NoError(t, err)
			result, err := registry.ConnectionTests.TestConnection(t.Context(), caller, workspace, cluster.ID)
			require.NoError(t, err)
			assert.Equal(t, clusterregistry.ValidationResult{
				Status:    clusterregistry.ValidationFailed,
				CheckedAt: result.CheckedAt,
				Checks: []clusterregistry.AccessCheck{{
					Name:    "credential",
					Code:    "CREDENTIAL_UNAVAILABLE",
					Message: "Cannot read this credential. Check the approved Secret reference and Secret permissions.",
				}},
			}, result)
			stored, err := registry.Registrations.Get(t.Context(), caller, workspace, cluster.ID)
			require.NoError(t, err)
			assert.Equal(t, &result, stored.LastValidation)
			fresh, err := New(tt.credentials)
			require.NoError(t, err)
			list, err := fresh.Registrations.List(t.Context(), caller, workspace)
			require.NoError(t, err)
			assert.Empty(t, list)
		})
	}
}

func TestNewRegistryWithCredentials(t *testing.T) {
	client := fake.NewClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "naira",
			Name:      "reader",
		},
		Data: map[string][]byte{"kubeconfig": []byte("not a kubeconfig")},
	})
	registry, err := newRegistry(map[string][]string{
		clusterregistry.TestWorkspaceID: {"naira/reader#kubeconfig"},
		"other-workspace":               {"naira/other#kubeconfig"},
	}, client.CoreV1())
	require.NoError(t, err)
	assert.Empty(t, client.Actions(), "initialization must not read Secrets")
	caller := clusterregistry.Caller{
		Issuer:  "https://issuer.example.com",
		Subject: "alice",
	}
	workspace := clusterregistry.Workspace{ID: clusterregistry.TestWorkspaceID}
	cluster, err := registry.Registrations.Create(t.Context(), caller, clusterregistry.CreateParams{
		Name:          "target",
		Workspace:     workspace,
		CredentialRef: "kubernetes-secret:naira/reader#kubeconfig",
		Kubernetes:    clusterregistry.KubernetesConnection{APIEndpoint: "https://cluster.example.com"},
	})
	require.NoError(t, err)
	_, err = registry.ConnectionTests.TestConnection(t.Context(), clusterregistry.Caller{}, workspace, cluster.ID)
	require.ErrorIs(t, err, clusterregistry.ErrUnauthenticated)
	assert.Empty(t, client.Actions())
	result, err := registry.ConnectionTests.TestConnection(t.Context(), caller, workspace, cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, clusterregistry.ValidationResult{
		Status:    clusterregistry.ValidationFailed,
		CheckedAt: result.CheckedAt,
		Checks: []clusterregistry.AccessCheck{
			{
				Name:    "credential",
				Passed:  true,
				Code:    "OK",
				Message: "Approved credential loaded.",
			},
			{
				Name:    "configuration",
				Code:    "INVALID_CONFIG",
				Message: "Cannot parse kubeconfig.",
			},
		},
	}, result)
	require.Len(t, client.Actions(), 1)
	assert.Equal(t, "get", client.Actions()[0].GetVerb())
	assert.Equal(t, "secrets", client.Actions()[0].GetResource().Resource)
	assert.Equal(t, "naira", client.Actions()[0].GetNamespace())
	stored, err := registry.Registrations.Get(t.Context(), caller, workspace, cluster.ID)
	require.NoError(t, err)
	assert.Equal(t, &result, stored.LastValidation)

	require.NoError(t, registry.Registrations.Update(t.Context(), caller, workspace, cluster.ID, clusterregistry.UpdateParams{
		Name:          cluster.Name,
		CredentialRef: "kubernetes-secret:naira/other#kubeconfig",
		Kubernetes:    cluster.Kubernetes.Connection,
	}))
	result, err = registry.ConnectionTests.TestConnection(t.Context(), caller, workspace, cluster.ID)
	require.NoError(t, err)
	require.Len(t, result.Checks, 1)
	assert.Equal(t, "CREDENTIAL_UNAVAILABLE", result.Checks[0].Code)
	assert.Len(t, client.Actions(), 1, "another workspace's allowlist must not permit Secret reads")
}

func TestNewRegistryErrors(t *testing.T) {
	for _, tt := range []struct {
		name        string
		credentials map[string][]string
		wantError   error
		message     string
	}{
		{
			name:        "missing Secret client",
			credentials: map[string][]string{clusterregistry.TestWorkspaceID: {"naira/reader#kubeconfig"}},
			message:     "configure cluster credentials: a Kubernetes Secret client is required",
		},
		{
			name:        "invalid workspace",
			credentials: map[string][]string{"": {}},
			wantError:   clusterregistry.ErrInvalidWorkspace,
			message:     "configure cluster credentials: validate credential workspace",
		},
		{
			name:        "invalid Secret reference",
			credentials: map[string][]string{clusterregistry.TestWorkspaceID: {"invalid"}},
			message:     "configure cluster credentials: parse allowed Secret reference",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			registry, err := newRegistry(tt.credentials, nil)
			require.ErrorContains(t, err, tt.message)
			if tt.wantError != nil {
				assert.ErrorIs(t, err, tt.wantError)
			}
			assert.Nil(t, registry)
		})
	}
}

func TestNewRequiresInClusterConfigWithCredentials(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	registry, err := New(map[string][]string{clusterregistry.TestWorkspaceID: {"naira/reader#kubeconfig"}})
	require.ErrorIs(t, err, rest.ErrNotInCluster)
	assert.ErrorContains(t, err, "create Naira's Secret client")
	assert.ErrorContains(t, err, "load in-cluster configuration")
	assert.Nil(t, registry)
}
