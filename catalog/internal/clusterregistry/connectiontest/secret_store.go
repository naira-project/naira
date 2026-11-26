package connectiontest

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/naira-project/naira/catalog/internal/clusterregistry"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

const maxCredentialBytes = 1 << 20

var errCredentialUnavailable = errors.New("credential is unavailable or not approved")

// SecretStore reads kubeconfigs from Kubernetes Secret keys approved per workspace.
type SecretStore struct {
	client  corev1.SecretsGetter
	allowed map[credentialScope]secretReference
}

type credentialScope struct {
	workspace clusterregistry.Workspace
	reference string
}

type secretReference struct {
	namespace string
	name      string
	key       string
}

// NewSecretStore accepts exact namespace/name#key entries grouped by workspace ID.
// The client must address their cluster. Workspaces without entries have no credential access.
func NewSecretStore(client corev1.SecretsGetter, references map[string][]string) (*SecretStore, error) {
	allowed := make(map[credentialScope]secretReference)
	for workspaceID, entries := range references {
		workspace := clusterregistry.Workspace{ID: workspaceID}
		if err := workspace.Validate(); err != nil {
			return nil, fmt.Errorf("validate credential workspace: %w", err)
		}
		for _, reference := range entries {
			secret, err := parseSecretReference(reference)
			if err != nil {
				return nil, fmt.Errorf("parse allowed Secret reference: %w", err)
			}
			allowed[credentialScope{
				workspace: workspace,
				reference: "kubernetes-secret:" + reference,
			}] = secret
		}
	}
	if len(allowed) != 0 && client == nil {
		return nil, fmt.Errorf("a Kubernetes Secret client is required for approved credentials")
	}
	return &SecretStore{
		client:  client,
		allowed: allowed,
	}, nil
}

func parseSecretReference(reference string) (secretReference, error) {
	resource, key, found := strings.Cut(reference, "#")
	namespace, name, split := strings.Cut(resource, "/")
	if !found || !split || namespace == "" || name == "" || key == "" ||
		strings.ContainsAny(namespace+name+key, "/# \t\r\n") {
		return secretReference{}, fmt.Errorf("invalid allowed Secret reference; expected namespace/name#key")
	}
	return secretReference{
		namespace: namespace,
		name:      name,
		key:       key,
	}, nil
}

// Load returns an independent copy of a kubeconfig approved for the workspace, at most 1 MiB.
// Unapproved workspace/reference pairs are rejected before contacting Kubernetes.
func (s *SecretStore) Load(ctx context.Context, workspace clusterregistry.Workspace, reference string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("load credential: %w", err)
	}
	ref, allowed := s.allowed[credentialScope{
		workspace: workspace,
		reference: reference,
	}]
	if !allowed {
		return nil, fmt.Errorf("check Secret allowlist: %w", errCredentialUnavailable)
	}
	secret, err := s.client.Secrets(ref.namespace).Get(ctx, ref.name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get approved Secret: %w", errCredentialUnavailable)
	}
	data := secret.Data[ref.key]
	if len(data) == 0 || len(data) > maxCredentialBytes {
		return nil, fmt.Errorf("read Secret key: %w", errCredentialUnavailable)
	}
	return append([]byte(nil), data...), nil
}
