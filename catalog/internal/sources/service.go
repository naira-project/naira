package sources

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/naira-project/naira/catalog/internal/workspaces"
)

// Service restricts registration operations to workspace owners.
// Every operation checks ownership before accessing the source store.
type Service struct {
	store      Store
	workspaces *workspaces.Service
}

func NewService(store Store, workspaceService *workspaces.Service) *Service {
	return &Service{
		store:      store,
		workspaces: workspaceService,
	}
}

func (s *Service) Create(ctx context.Context, owner workspaces.Owner, params CreateParams) (Registration, error) {
	if err := s.workspaces.Authorize(ctx, owner, params.WorkspaceID); err != nil {
		return Registration{}, fmt.Errorf("authorize registration creation: %w", err)
	}
	registration, err := NewRegistration(params)
	if err != nil {
		return Registration{}, fmt.Errorf("construct registration: %w", err)
	}
	if err := s.store.Create(ctx, registration); err != nil {
		return Registration{}, fmt.Errorf("create registration: %w", err)
	}
	return registration, nil
}

func (s *Service) Get(ctx context.Context, owner workspaces.Owner, workspaceID string, id uuid.UUID) (Registration, error) {
	if err := s.workspaces.Authorize(ctx, owner, workspaceID); err != nil {
		return Registration{}, fmt.Errorf("authorize registration read: %w", err)
	}
	registration, err := s.store.Get(ctx, workspaceID, id)
	if err != nil {
		return Registration{}, fmt.Errorf("get registration: %w", err)
	}
	return registration, nil
}

func (s *Service) List(ctx context.Context, owner workspaces.Owner, workspaceID string) ([]Registration, error) {
	if err := s.workspaces.Authorize(ctx, owner, workspaceID); err != nil {
		return nil, fmt.Errorf("authorize registration list: %w", err)
	}
	registrations, err := s.store.List(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list registrations: %w", err)
	}
	return registrations, nil
}

func (s *Service) Update(ctx context.Context, owner workspaces.Owner, workspaceID string, id uuid.UUID, params UpdateParams) error {
	if err := s.workspaces.Authorize(ctx, owner, workspaceID); err != nil {
		return fmt.Errorf("authorize registration update: %w", err)
	}
	if err := s.store.Update(ctx, workspaceID, id, params); err != nil {
		return fmt.Errorf("update registration: %w", err)
	}
	return nil
}
