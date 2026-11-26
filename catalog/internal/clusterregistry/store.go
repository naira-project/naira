package clusterregistry

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound           = errors.New("cluster not found")
	ErrAlreadyExists      = errors.New("cluster already exists")
	ErrValidationConflict = errors.New("cluster changed during validation; retry")
)

// Store manages clusters with globally unique IDs and names unique within a workspace.
// Implementations must reject empty workspace IDs and scope every read and update to
// the supplied workspace. Workspace scoping does not authorize the caller.
// Durability depends on the implementation.
type Store interface {
	Create(ctx context.Context, cluster Cluster) error
	Get(ctx context.Context, workspace Workspace, id uuid.UUID) (Cluster, error)
	// List returns clusters ordered by name, using case-sensitive comparison.
	List(ctx context.Context, workspace Workspace) ([]Cluster, error)
	// Update atomically applies the same metadata and validation rules as ApplyUpdate.
	Update(ctx context.Context, workspace Workspace, id uuid.UUID, params UpdateParams) error
	// SaveValidation rejects stale revisions and never replaces an existing installation UID.
	SaveValidation(ctx context.Context, workspace Workspace, id uuid.UUID, expectedUpdate time.Time, result ValidationResult) error
}
