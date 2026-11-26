// Package connectiontest tests saved cluster connections without running discovery.
package connectiontest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/naira-project/naira/catalog/internal/clusterregistry"
)

var ErrUnavailable = errors.New("connection validation is not configured")

const connectionTestTimeout = 30 * time.Second

// Func checks a cluster snapshot without persisting or collecting resources.
// Return safe, non-secret messages. Status and CheckedAt are ignored.
type Func func(context.Context, clusterregistry.Cluster) clusterregistry.ValidationResult

// Service runs connection tests and saves results only for the tested revision.
type Service struct {
	registry       *clusterregistry.Service
	store          clusterregistry.Store
	connectionTest Func
}

// NewService requires the registry and its backing store. A nil connectionTest disables tests.
func NewService(registry *clusterregistry.Service, store clusterregistry.Store, connectionTest Func) *Service {
	return &Service{
		registry:       registry,
		store:          store,
		connectionTest: connectionTest,
	}
}

// TestConnection checks access before loading the cluster and never imports resources.
func (s *Service) TestConnection(ctx context.Context, caller clusterregistry.Caller, workspace clusterregistry.Workspace, id uuid.UUID) (clusterregistry.ValidationResult, error) {
	cluster, err := s.registry.Get(ctx, caller, workspace, id)
	if err != nil {
		return clusterregistry.ValidationResult{}, fmt.Errorf("get cluster for validation: %w", err)
	}
	if s.connectionTest == nil {
		return clusterregistry.ValidationResult{}, fmt.Errorf("initialize validation: %w", ErrUnavailable)
	}
	testCtx, cancel := context.WithTimeout(ctx, connectionTestTimeout)
	defer cancel()
	result := s.connectionTest(testCtx, cluster)
	result = completeResult(result, cluster.Kubernetes.KubeSystemUID, time.Now().UTC())
	if err := s.store.SaveValidation(ctx, workspace, id, cluster.UpdatedAt, result); err != nil {
		return clusterregistry.ValidationResult{}, fmt.Errorf("save validation result: %w", err)
	}
	return result, nil
}

func completeResult(result clusterregistry.ValidationResult, knownUID string, checkedAt time.Time) clusterregistry.ValidationResult {
	if len(result.Checks) == 0 {
		result.Checks = []clusterregistry.AccessCheck{{
			Name:    "validation",
			Code:    "NO_RESULT",
			Message: "The validator did not return any access checks.",
		}}
	}
	if knownUID != "" && result.KubeSystemUID != "" && knownUID != result.KubeSystemUID {
		result.Checks = append(result.Checks, clusterregistry.AccessCheck{
			Name:    "continuity",
			Code:    "CLUSTER_CHANGED",
			Message: "The installation fingerprint differs from the registered cluster; it was not replaced.",
		})
	}
	result.CheckedAt = checkedAt
	result.Status = clusterregistry.ValidationFailed
	if result.Successful() {
		result.Status = clusterregistry.ValidationReady
	}
	return result
}
