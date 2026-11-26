// Package clusters configures cluster registrations and connection testing.
package clusters

import (
	"fmt"
	"time"

	"github.com/naira-project/naira/catalog/internal/clusterregistry"
	"github.com/naira-project/naira/catalog/internal/clusterregistry/connectiontest"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

// Registry groups cluster management and connection tests sharing one store.
// Both services must be non-nil and use the same backing store.
type Registry struct {
	Registrations   *clusterregistry.Service
	ConnectionTests *connectiontest.Service
}

// New creates an in-memory registry with workspace-scoped credential allowlists.
// Nonempty allowlists require in-cluster configuration for reading Naira's Secrets.
func New(credentialSecrets map[string][]string) (*Registry, error) {
	var secretClient corev1.SecretsGetter
	for _, references := range credentialSecrets {
		if len(references) == 0 {
			continue
		}
		client, err := newSecretClient()
		if err != nil {
			return nil, fmt.Errorf("create Naira's Secret client: %w", err)
		}
		secretClient = client
		break
	}
	registry, err := newRegistry(credentialSecrets, secretClient)
	if err != nil {
		return nil, fmt.Errorf("initialize cluster services: %w", err)
	}
	return registry, nil
}

func newRegistry(credentialSecrets map[string][]string, secretClient corev1.SecretsGetter) (*Registry, error) {
	credentials, err := connectiontest.NewSecretStore(secretClient, credentialSecrets)
	if err != nil {
		return nil, fmt.Errorf("configure cluster credentials: %w", err)
	}
	store := clusterregistry.NewMemoryStore()
	registrations := clusterregistry.NewService(store)
	tester := connectiontest.NewKubernetesTester(credentials)
	return &Registry{
		Registrations:   registrations,
		ConnectionTests: connectiontest.NewService(registrations, store, tester.Test),
	}, nil
}

func newSecretClient() (corev1.SecretsGetter, error) {
	// This identity reads Secrets in Naira's cluster, never the target cluster.
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("load in-cluster configuration: %w", err)
	}
	config.Timeout = 8 * time.Second
	client, err := corev1.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("initialize Kubernetes client: %w", err)
	}
	return client, nil
}
