package clusterregistry

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

var ErrUnauthenticated = errors.New("authenticated identity required")

// Caller identifies a verified OIDC subject. Supply it from trusted authentication,
// never from request bodies.
type Caller struct {
	Issuer  string
	Subject string
}

// Service permits authenticated callers to manage clusters in the shared test workspace.
type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

// CheckAccess requires an authenticated caller and the fixed test workspace.
func (s *Service) CheckAccess(ctx context.Context, caller Caller, workspace Workspace) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("check workspace access: %w", err)
	}
	if strings.TrimSpace(caller.Issuer) == "" || strings.TrimSpace(caller.Subject) == "" {
		return fmt.Errorf("check caller identity: %w", ErrUnauthenticated)
	}
	if workspace.ID != TestWorkspaceID {
		return fmt.Errorf("check workspace: %w", ErrWorkspaceNotFound)
	}
	return nil
}

func (s *Service) Create(ctx context.Context, caller Caller, params CreateParams) (Cluster, error) {
	if err := s.CheckAccess(ctx, caller, params.Workspace); err != nil {
		return Cluster{}, fmt.Errorf("authorize cluster creation: %w", err)
	}
	cluster, err := NewCluster(params)
	if err != nil {
		return Cluster{}, fmt.Errorf("construct cluster: %w", err)
	}
	if err := s.store.Create(ctx, cluster); err != nil {
		return Cluster{}, fmt.Errorf("create cluster: %w", err)
	}
	return cluster, nil
}

func (s *Service) Get(ctx context.Context, caller Caller, workspace Workspace, id uuid.UUID) (Cluster, error) {
	if err := s.CheckAccess(ctx, caller, workspace); err != nil {
		return Cluster{}, fmt.Errorf("authorize cluster read: %w", err)
	}
	cluster, err := s.store.Get(ctx, workspace, id)
	if err != nil {
		return Cluster{}, fmt.Errorf("get cluster: %w", err)
	}
	return cluster, nil
}

func (s *Service) List(ctx context.Context, caller Caller, workspace Workspace) ([]Cluster, error) {
	if err := s.CheckAccess(ctx, caller, workspace); err != nil {
		return nil, fmt.Errorf("authorize cluster list: %w", err)
	}
	clusters, err := s.store.List(ctx, workspace)
	if err != nil {
		return nil, fmt.Errorf("list clusters: %w", err)
	}
	return clusters, nil
}

func (s *Service) Update(ctx context.Context, caller Caller, workspace Workspace, id uuid.UUID, params UpdateParams) error {
	if err := s.CheckAccess(ctx, caller, workspace); err != nil {
		return fmt.Errorf("authorize cluster update: %w", err)
	}
	if err := s.store.Update(ctx, workspace, id, params); err != nil {
		return fmt.Errorf("update cluster: %w", err)
	}
	return nil
}
