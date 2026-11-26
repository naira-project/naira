package clusterconnection

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/naira-project/naira/catalog/internal/clusterregistry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestApprovedFilesOnly(t *testing.T) {
	directory := t.TempDir()
	file := filepath.Join(directory, "reader.kubeconfig")
	require.NoError(t, os.WriteFile(file, []byte("private-credential"), 0o600))
	outside := filepath.Join(t.TempDir(), "outside")
	require.NoError(t, os.WriteFile(outside, []byte("outside-credential"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(directory, "escape")))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "empty"), nil, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "large"), make([]byte, maxCredentialBytes+1), 0o600))
	data, err := resolve(t.Context(), directory, nil, nil, "kubeconfig-file:"+file)
	require.NoError(t, err)
	assert.Equal(t, "private-credential", string(data))
	for _, ref := range []string{"kubeconfig-file:" + outside, "kubeconfig-file:" + filepath.Join(directory, "escape"), "kubeconfig-file:relative", "kubeconfig-file:" + directory, "kubeconfig-file:" + filepath.Join(directory, "empty"), "kubeconfig-file:" + filepath.Join(directory, "large"), "kubeconfig-file:" + filepath.Join(directory, "missing"), "unsupported:reference"} {
		_, err := resolve(t.Context(), directory, nil, nil, ref)
		require.ErrorIs(t, err, errCredentialUnavailable)
		assert.NotContains(t, err.Error(), "private-credential")
	}
	_, err = resolve(t.Context(), "", nil, nil, "kubeconfig-file:"+file)
	require.ErrorIs(t, err, errCredentialUnavailable)
	test, err := New(Config{Directory: directory})
	require.NoError(t, err)
	result := test(t.Context(), clusterregistry.Cluster{CredentialRef: "kubeconfig-file:" + file})
	require.Len(t, result.Checks, 2)
	assert.Equal(t, "INVALID_CONFIG", result.Checks[1].Code)
	result = test(t.Context(), clusterregistry.Cluster{CredentialRef: "kubeconfig-file:" + outside})
	require.Len(t, result.Checks, 1)
	assert.Equal(t, "CREDENTIAL_UNAVAILABLE", result.Checks[0].Code)
}

func TestApprovedSecretsOnly(t *testing.T) {
	client := fake.NewClientset(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "naira", Name: "approved"}, Data: map[string][]byte{"kubeconfig": []byte("private-credential")}})
	allowed := map[string]struct{}{"naira/approved#kubeconfig": {}}
	data, err := resolve(t.Context(), "", allowed, client, "kubernetes-secret:naira/approved#kubeconfig")
	require.NoError(t, err)
	assert.Equal(t, "private-credential", string(data))
	client.ClearActions()
	_, err = resolve(t.Context(), "", allowed, client, "kubernetes-secret:naira/unapproved#kubeconfig")
	require.ErrorIs(t, err, errCredentialUnavailable)
	assert.Empty(t, client.Actions(), "unapproved references must not query Secrets")
	for _, ref := range []string{"naira/approved#missing", "naira/missing#kubeconfig", "invalid"} {
		_, err := resolve(t.Context(), "", map[string]struct{}{ref: {}}, client, "kubernetes-secret:"+ref)
		require.ErrorIs(t, err, errCredentialUnavailable)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	test, err := New(Config{})
	require.NoError(t, err)
	assert.Equal(t, "CREDENTIAL_UNAVAILABLE", test(ctx, clusterregistry.Cluster{}).Checks[0].Code)
}

func TestSecretReferenceConfiguration(t *testing.T) {
	for _, ref := range []string{"", "name", "naira/name", "naira/name#", "naira/name#key#extra", "naira/name/extra#key", "naira/name#bad key"} {
		_, err := New(Config{Secrets: []string{ref}})
		require.Error(t, err)
	}
	_, _, _, ok := secretParts("naira/approved#kubeconfig")
	assert.True(t, ok)
}
