package workspaces

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Service permits authenticated users to create and access only their own workspaces.
// Ownership is immutable; missing and foreign workspaces are indistinguishable.
type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Create(ctx context.Context, owner Owner, name string) (Workspace, error) {
	if err := owner.Validate(); err != nil {
		return Workspace{}, fmt.Errorf("validate owner: %w", err)
	}
	now := time.Now().UTC()
	workspace := Workspace{
		ID:        uuid.New(),
		Name:      name,
		Owner:     owner,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := workspace.Validate(); err != nil {
		return Workspace{}, fmt.Errorf("validate workspace: %w", err)
	}
	if err := s.store.Create(ctx, workspace); err != nil {
		return Workspace{}, fmt.Errorf("create workspace: %w", err)
	}
	return workspace, nil
}

func (s *Service) List(ctx context.Context, owner Owner) ([]Workspace, error) {
	if err := owner.Validate(); err != nil {
		return nil, fmt.Errorf("validate owner: %w", err)
	}
	result, err := s.store.List(ctx, owner)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	return result, nil
}

func (s *Service) Get(ctx context.Context, owner Owner, id string) (Workspace, error) {
	if err := owner.Validate(); err != nil {
		return Workspace{}, fmt.Errorf("validate owner: %w", err)
	}
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.String() != id {
		return Workspace{}, fmt.Errorf("access workspace: %w", ErrForbidden)
	}
	workspace, err := s.store.Get(ctx, owner, parsed)
	if errors.Is(err, ErrNotFound) {
		return Workspace{}, fmt.Errorf("access workspace: %w", ErrForbidden)
	}
	if err != nil {
		return Workspace{}, fmt.Errorf("get workspace: %w", err)
	}
	return workspace, nil
}

func (s *Service) Authorize(ctx context.Context, owner Owner, id string) error {
	if _, err := s.Get(ctx, owner, id); err != nil {
		return fmt.Errorf("authorize workspace access: %w", err)
	}
	return nil
}
