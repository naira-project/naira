package workspaces

import (
	"context"

	"github.com/google/uuid"
)

// Store scopes reads to an owner and enforces unique names per owner and globally unique IDs.
// A missing workspace and a workspace belonging to someone else both return ErrNotFound.
// Implementations must not permit changing ownership. Durability is implementation-specific.
type Store interface {
	Create(context.Context, Workspace) error
	Get(context.Context, Owner, uuid.UUID) (Workspace, error)
	List(context.Context, Owner) ([]Workspace, error)
}
