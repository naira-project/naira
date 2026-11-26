package connectiontest

import (
	"context"
	"fmt"
	"testing"

	"github.com/naira-project/naira/catalog/internal/clusterregistry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestSecretStoreLoadsOnlyApprovedKeys(t *testing.T) {
	client := fake.NewClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "naira", Name: "approved"},
		Data:       map[string][]byte{"kubeconfig": []byte("private-credential")},
	})
	workspace := clusterregistry.Workspace{ID: "workspace-a"}
	store, err := NewSecretStore(client.CoreV1(), map[string][]string{workspace.ID: {"naira/approved#kubeconfig"}})
	require.NoError(t, err)
	data, err := store.Load(t.Context(), workspace, "kubernetes-secret:naira/approved#kubeconfig")
	require.NoError(t, err)
	assert.Equal(t, "private-credential", string(data))
	clear(data)
	data, err = store.Load(t.Context(), workspace, "kubernetes-secret:naira/approved#kubeconfig")
	require.NoError(t, err)
	assert.Equal(t, "private-credential", string(data))

	client.ClearActions()
	for _, reference := range []string{
		"kubernetes-secret:naira/unapproved#kubeconfig",
		"kubernetes-secret:naira/approved#other",
		"kubeconfig-file:/tmp/reader.kubeconfig",
		"invalid",
	} {
		_, err := store.Load(t.Context(), workspace, reference)
		require.ErrorIs(t, err, errCredentialUnavailable)
		assert.NotContains(t, err.Error(), "private-credential")
	}
	assert.Empty(t, client.Actions(), "unapproved references must not query Secrets")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = store.Load(ctx, workspace, "kubernetes-secret:naira/approved#kubeconfig")
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, client.Actions())
}

func TestSecretStoreRejectsUnavailableData(t *testing.T) {
	client := fake.NewClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "naira", Name: "approved"},
		Data: map[string][]byte{
			"empty": nil,
			"large": make([]byte, maxCredentialBytes+1),
		},
	})
	for _, reference := range []string{"naira/missing#kubeconfig", "naira/approved#missing", "naira/approved#empty", "naira/approved#large"} {
		workspace := clusterregistry.Workspace{ID: "workspace-a"}
		store, err := NewSecretStore(client.CoreV1(), map[string][]string{workspace.ID: {reference}})
		require.NoError(t, err)
		_, err = store.Load(t.Context(), workspace, "kubernetes-secret:"+reference)
		require.ErrorIs(t, err, errCredentialUnavailable)
	}
}

func TestParseSecretReference(t *testing.T) {
	for _, tt := range []struct {
		reference string
		want      secretReference
	}{
		{
			reference: "naira/reader#kubeconfig",
			want: secretReference{
				namespace: "naira",
				name:      "reader",
				key:       "kubeconfig",
			},
		},
		{
			reference: "idp-system/cluster.reader#config.yaml",
			want: secretReference{
				namespace: "idp-system",
				name:      "cluster.reader",
				key:       "config.yaml",
			},
		},
	} {
		t.Run(tt.reference, func(t *testing.T) {
			reference, err := parseSecretReference(tt.reference)
			require.NoError(t, err)
			assert.Equal(t, tt.want, reference)
		})
	}
}

func TestSecretStoreConfiguration(t *testing.T) {
	for _, ref := range []string{"", "name", "naira/name", "naira/name#", "naira/name#key#extra", "naira/name/extra#key", "naira/name#bad key"} {
		_, err := NewSecretStore(nil, map[string][]string{"workspace-a": {ref}})
		require.Error(t, err)
	}
	for _, workspaceID := range []string{"", " workspace-a", "workspace-a\n"} {
		_, err := NewSecretStore(nil, map[string][]string{workspaceID: {}})
		require.ErrorIs(t, err, clusterregistry.ErrInvalidWorkspace)
	}
	_, err := NewSecretStore(nil, map[string][]string{"workspace-a": {"naira/approved#kubeconfig"}})
	require.Error(t, err)
	store, err := NewSecretStore(nil, nil)
	require.NoError(t, err)
	_, err = store.Load(t.Context(), clusterregistry.Workspace{ID: "workspace-a"}, "kubernetes-secret:naira/approved#kubeconfig")
	require.ErrorIs(t, err, errCredentialUnavailable)
	_, err = NewSecretStore(nil, map[string][]string{"workspace-a": {}})
	require.NoError(t, err)
}

func TestSecretStoreDoesNotExposeAPIErrors(t *testing.T) {
	client := fake.NewClientset()
	client.PrependReactor("get", "secrets", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "approved", fmt.Errorf("private-upstream-error"))
	})
	workspace := clusterregistry.Workspace{ID: "workspace-a"}
	store, err := NewSecretStore(client.CoreV1(), map[string][]string{workspace.ID: {"naira/approved#kubeconfig"}})
	require.NoError(t, err)
	_, err = store.Load(t.Context(), workspace, "kubernetes-secret:naira/approved#kubeconfig")
	require.ErrorIs(t, err, errCredentialUnavailable)
	assert.NotContains(t, err.Error(), "private-upstream-error")
}

func TestSecretStoreWorkspaceIsolation(t *testing.T) {
	client := fake.NewClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "naira", Name: "access"},
		Data: map[string][]byte{
			"a":      []byte("private-a"),
			"b":      []byte("private-b"),
			"shared": []byte("private-shared"),
		},
	})
	store, err := NewSecretStore(client.CoreV1(), map[string][]string{
		"workspace-a": {"naira/access#a", "naira/access#shared"},
		"workspace-b": {"naira/access#b", "naira/access#shared"},
		"empty":       {},
	})
	require.NoError(t, err)
	for _, tt := range []struct {
		workspace string
		key       string
		want      string
	}{
		{workspace: "workspace-a", key: "a", want: "private-a"},
		{workspace: "workspace-b", key: "b", want: "private-b"},
		{workspace: "workspace-a", key: "shared", want: "private-shared"},
		{workspace: "workspace-b", key: "shared", want: "private-shared"},
		{workspace: "workspace-a", key: "b"},
		{workspace: "workspace-b", key: "a"},
		{workspace: "unknown", key: "shared"},
		{workspace: "empty", key: "shared"},
		{workspace: " workspace-a", key: "a"},
		{key: "a"},
	} {
		t.Run(tt.workspace+"/"+tt.key, func(t *testing.T) {
			client.ClearActions()
			data, err := store.Load(t.Context(), clusterregistry.Workspace{ID: tt.workspace}, "kubernetes-secret:naira/access#"+tt.key)
			if tt.want == "" {
				require.ErrorIs(t, err, errCredentialUnavailable)
				assert.Nil(t, data)
				assert.Empty(t, client.Actions(), "unapproved workspace/reference pairs must not query Secrets")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(data))
			require.Len(t, client.Actions(), 1)
		})
	}
}
