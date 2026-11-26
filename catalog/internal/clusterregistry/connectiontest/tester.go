package connectiontest

import (
	"context"

	"github.com/naira-project/naira/catalog/internal/clusterregistry"
	"github.com/naira-project/naira/plugins/pkg/kubeaccess"
)

// KubernetesTester checks external clusters using kubeconfigs from approved Secrets.
type KubernetesTester struct {
	credentials *SecretStore
}

// NewKubernetesTester requires a configured Secret store, even with an empty allowlist.
func NewKubernetesTester(credentials *SecretStore) *KubernetesTester {
	return &KubernetesTester{credentials: credentials}
}

// Test loads the credential and checks access without persisting or collecting resources.
func (t *KubernetesTester) Test(ctx context.Context, cluster clusterregistry.Cluster) clusterregistry.ValidationResult {
	data, err := t.credentials.Load(ctx, cluster.Workspace, cluster.CredentialRef)
	if err != nil {
		return clusterregistry.ValidationResult{Checks: []clusterregistry.AccessCheck{{
			Name:    "credential",
			Code:    "CREDENTIAL_UNAVAILABLE",
			Message: "Cannot read this credential. Check the approved Secret reference and Secret permissions.",
		}}}
	}
	defer clear(data)
	connection := cluster.Kubernetes.Connection
	result := kubeaccess.CheckAccess(ctx, data, connection.APIEndpoint, connection.Context)
	checks := make([]clusterregistry.AccessCheck, 0, len(result.Checks)+1)
	checks = append(checks, clusterregistry.AccessCheck{
		Name:    "credential",
		Passed:  true,
		Code:    "OK",
		Message: "Approved credential loaded.",
	})
	for _, check := range result.Checks {
		checks = append(checks, clusterregistry.AccessCheck{
			Name:    check.Name,
			Passed:  check.Passed,
			Code:    check.Code,
			Message: check.Message,
		})
	}
	return clusterregistry.ValidationResult{
		Checks:        checks,
		KubeSystemUID: result.KubeSystemUID,
	}
}
