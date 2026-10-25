package sources

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var (
	ErrNotFound      = errors.New("source registration not found")
	ErrAlreadyExists = errors.New("source registration already exists")
)

// Store manages registrations with globally unique IDs and names unique within a workspace.
// Implementations must reject empty workspace IDs and scope every read and update to
// the supplied workspace. Workspace scoping does not authorize the caller.
// Durability depends on the implementation.
type Store interface {
	Create(ctx context.Context, registration Registration) error
	Get(ctx context.Context, workspaceID string, id uuid.UUID) (Registration, error)
	// List returns registrations ordered by name, using case-sensitive comparison.
	List(ctx context.Context, workspaceID string) ([]Registration, error)
	// Update atomically applies the same metadata and validation rules as ApplyUpdate.
	Update(ctx context.Context, workspaceID string, id uuid.UUID, params UpdateParams) error
}
