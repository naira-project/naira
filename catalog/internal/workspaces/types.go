// Package workspaces models flat, single-owner workspaces.
package workspaces

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrUnauthenticated = errors.New("authenticated identity required")
	ErrForbidden       = errors.New("workspace access denied")
	ErrInvalid         = errors.New("invalid workspace")
	ErrNotFound        = errors.New("workspace not found")
	ErrAlreadyExists   = errors.New("workspace already exists")
)

// Owner identifies a verified OIDC subject within its issuer.
// Construct it from trusted authentication, never from request bodies.
type Owner struct {
	Issuer  string
	Subject string
}

func (o Owner) Validate() error {
	if strings.TrimSpace(o.Issuer) == "" || strings.TrimSpace(o.Subject) == "" {
		return fmt.Errorf("validate workspace owner: %w", ErrUnauthenticated)
	}
	return nil
}

// Workspace has one immutable owner and no parent or membership hierarchy.
type Workspace struct {
	ID        uuid.UUID
	Name      string
	Owner     Owner
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (w Workspace) Validate() error {
	if err := w.Owner.Validate(); err != nil {
		return fmt.Errorf("validate owner: %w", err)
	}
	if w.ID == uuid.Nil || w.Name == "" || w.Name != strings.TrimSpace(w.Name) || strings.ContainsAny(w.Name, "\r\n\t") {
		return fmt.Errorf("workspace requires an ID and a nonempty name without surrounding whitespace or control separators: %w", ErrInvalid)
	}
	if w.CreatedAt.IsZero() || w.UpdatedAt.IsZero() || w.UpdatedAt.Before(w.CreatedAt) {
		return fmt.Errorf("workspace timestamps must be set and ordered: %w", ErrInvalid)
	}
	return nil
}
