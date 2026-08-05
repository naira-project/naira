package catalog

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNodeNotFound         = errors.New("node not found")
	ErrInvalidPluginName    = errors.New("invalid plugin name")
	ErrPluginNotFound       = errors.New("plugin not found")
	ErrPluginAlreadyRunning = errors.New("plugin already has a running operation")
)

type Service struct {
	store         Store
	operations    OperationStore
	plugins       map[string]Plugin
	logger        *log.Logger
	wg            sync.WaitGroup
	appCtx        context.Context
	pluginTimeout time.Duration
}

// NewService creates a Service. If no operation store is provided, a default
// in-memory store is created. The appCtx is used as the parent context for
// asynchronous plugin runs; it should be cancelled on application shutdown.
func NewService(appCtx context.Context, store Store, plugins map[string]Plugin, pluginTimeout time.Duration, logger *log.Logger, operationStores ...OperationStore) *Service {
	registeredPlugins := make(map[string]Plugin, len(plugins))
	for name, plugin := range plugins {
		if plugin == nil {
			continue
		}
		registeredPlugins[normalizePluginName(name)] = plugin
	}

	operationStore := OperationStore(NewMemoryOperationStore())
	if len(operationStores) > 0 && operationStores[0] != nil {
		operationStore = operationStores[0]
	}

	return &Service{
		appCtx:        appCtx,
		store:         store,
		operations:    operationStore,
		plugins:       registeredPlugins,
		pluginTimeout: pluginTimeout,
		logger:        logger,
	}
}

func (s *Service) RunPlugin(ctx context.Context, pluginName string) error {
	pluginName = normalizePluginName(pluginName)
	if pluginName == "" {
		return fmt.Errorf("normalize plugin name: %w", ErrInvalidPluginName)
	}

	plugin, ok := s.plugins[pluginName]
	if !ok {
		return fmt.Errorf("looking up plugin %q: %w", pluginName, ErrPluginNotFound)
	}

	response, err := plugin.Collect(ctx)
	if err != nil {
		return fmt.Errorf("collecting response from plugin %q: %w", pluginName, err)
	}

	snapshotID := uuid.New()

	upsertedNodes, upsertedRelations, err := s.store.ApplyPluginSnapshot(pluginName, snapshotID, response.Nodes, response.Relations)
	if err != nil {
		return fmt.Errorf("upserting graph from plugin %q: %w", pluginName, err)
	}

	if s.logger != nil {
		s.logger.Printf("plugin %q upserted %d nodes and %d relations", pluginName, upsertedNodes, upsertedRelations)
	}

	return nil
}

func (s *Service) RunAllPlugins(ctx context.Context) RunPluginsResult {
	pluginNames := make([]string, 0, len(s.plugins))
	for name := range s.plugins {
		pluginNames = append(pluginNames, name)
	}
	sort.Strings(pluginNames)

	response := RunPluginsResult{
		Results: make([]RunPluginResult, 0, len(pluginNames)),
	}

	for _, pluginName := range pluginNames {
		err := s.RunPlugin(ctx, pluginName)
		if err != nil {
			response.Results = append(response.Results, RunPluginResult{Plugin: pluginName, Error: err.Error()})
			continue
		}
		response.Results = append(response.Results, RunPluginResult{Plugin: pluginName})
	}

	return response
}

// RunPluginAsync starts an asynchronous plugin run and returns the operation
// that tracks its progress. It returns ErrPluginAlreadyRunning if the plugin
// already has a PENDING or RUNNING operation.
func (s *Service) RunPluginAsync(ctx context.Context, pluginName string) (Operation, error) {
	pluginName = normalizePluginName(pluginName)
	if pluginName == "" {
		return Operation{}, fmt.Errorf("normalize plugin name: %w", ErrInvalidPluginName)
	}

	if _, ok := s.plugins[pluginName]; !ok {
		return Operation{}, fmt.Errorf("looking up plugin %q: %w", pluginName, ErrPluginNotFound)
	}

	if s.hasActiveOperation(pluginName) {
		return Operation{}, fmt.Errorf("plugin %q: %w", pluginName, ErrPluginAlreadyRunning)
	}

	op := Operation{
		Name:      "plugin-run-" + uuid.NewString(),
		Plugin:    pluginName,
		State:     OperationStatePending,
		CreatedAt: time.Now(),
	}
	if err := s.operations.Create(op); err != nil {
		return Operation{}, fmt.Errorf("creating operation for plugin %q: %w", pluginName, err)
	}

	s.wg.Go(func() {
		s.executePluginRun(ctx, op.Name, pluginName)
	})

	return op, nil
}

// RunAllPluginsAsync starts an asynchronous run for every registered plugin
// and returns the operations that track their progress.
func (s *Service) RunAllPluginsAsync(ctx context.Context) []Operation {
	pluginNames := make([]string, 0, len(s.plugins))
	for name := range s.plugins {
		pluginNames = append(pluginNames, name)
	}
	sort.Strings(pluginNames)

	operations := make([]Operation, 0, len(pluginNames))
	for _, pluginName := range pluginNames {
		op, err := s.RunPluginAsync(ctx, pluginName)
		if err != nil {
			if s.logger != nil {
				s.logger.Printf("skipping async run for plugin %q: %v", pluginName, err)
			}
			continue
		}
		operations = append(operations, op)
	}

	return operations
}

// GetOperation retrieves a single operation by name.
func (s *Service) GetOperation(_ context.Context, name string) (Operation, error) {
	op, err := s.operations.Get(name)
	if err != nil {
		return Operation{}, fmt.Errorf("getting operation %q: %w", name, err)
	}
	return op, nil
}

// ListPlugins returns the names of all registered plugins.
func (s *Service) ListPlugins() []string {
	names := make([]string, 0, len(s.plugins))
	for name := range s.plugins {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ListOperations returns all operations, optionally filtered by plugin and
// state, ordered by creation time descending.
func (s *Service) ListOperations(_ context.Context, filter OperationFilter) ([]Operation, error) {
	operations, err := s.operations.List(filter)
	if err != nil {
		return nil, fmt.Errorf("listing operations: %w", err)
	}
	return operations, nil
}

// Wait blocks until all in-flight plugin runs have finished.
func (s *Service) Wait() {
	s.wg.Wait()
}

// executePluginRun runs a single plugin and updates the operation outcome.
// The ctx parameter from the caller is intentionally unused — async operations
// derive their lifecycle from s.appCtx (application shutdown) and
// s.pluginTimeout (per-plugin deadline), not from the triggering request.
func (s *Service) executePluginRun(_ context.Context, operationName, pluginName string) {
	if err := s.operations.UpdateState(operationName, OperationStateRunning, nil, 0, 0); err != nil {
		if s.logger != nil {
			s.logger.Printf("marking operation %q as running: %v", operationName, err)
		}
		return
	}

	plugin, ok := s.plugins[pluginName]
	if !ok {
		s.failOperation(operationName, pluginName, fmt.Errorf("looking up plugin %q: %w", pluginName, ErrPluginNotFound))
		return
	}

	runCtx, cancel := context.WithTimeout(s.appCtx, s.pluginTimeout)
	defer cancel()

	response, err := plugin.Collect(runCtx)
	if err != nil {
		s.failOperation(operationName, pluginName, fmt.Errorf("collecting response from plugin %q: %w", pluginName, err))
		return
	}

	snapshotID := uuid.New()
	upsertedNodes, upsertedRelations, err := s.store.ApplyPluginSnapshot(pluginName, snapshotID, response.Nodes, response.Relations)
	if err != nil {
		s.failOperation(operationName, pluginName, fmt.Errorf("upserting graph from plugin %q: %w", pluginName, err))
		return
	}

	if s.logger != nil {
		s.logger.Printf("plugin %q upserted %d nodes and %d relations", pluginName, upsertedNodes, upsertedRelations)
	}

	if err := s.operations.UpdateState(operationName, OperationStateSucceeded, nil, upsertedNodes, upsertedRelations); err != nil {
		if s.logger != nil {
			s.logger.Printf("marking operation %q as succeeded: %v", operationName, err)
		}
	}
}

// failOperation marks an operation as FAILED with an AIP-193 style error.
func (s *Service) failOperation(operationName, pluginName string, err error) {
	statusErr := &StatusError{Message: err.Error()}

	if s.logger != nil {
		s.logger.Printf("plugin %q run failed: %v", pluginName, err)
	}

	if updateErr := s.operations.UpdateState(operationName, OperationStateFailed, statusErr, 0, 0); updateErr != nil {
		if s.logger != nil {
			s.logger.Printf("marking operation %q as failed: %v", operationName, updateErr)
		}
	}
}

// hasActiveOperation reports whether the plugin has a PENDING or RUNNING
// operation (i.e. a run already in flight for that plugin).
func (s *Service) hasActiveOperation(pluginName string) bool {
	operations, err := s.operations.List(OperationFilter{Plugin: pluginName})
	if err != nil {
		if s.logger != nil {
			s.logger.Printf("listing operations for plugin %q: %v", pluginName, err)
		}
		return false
	}

	for _, op := range operations {
		if op.State == OperationStatePending || op.State == OperationStateRunning {
			return true
		}
	}
	return false
}

func (s *Service) GetNode(_ context.Context, id NodeID) (Node, error) {
	return s.store.GetNode(id)
}

func (s *Service) ListNodes(_ context.Context) []Node {
	return s.store.ListNodes()
}

func (s *Service) ListRelations(_ context.Context) []Relation {
	return s.store.ListRelations()
}

func normalizePluginName(pluginName string) string {
	return strings.TrimSpace(strings.ToLower(pluginName))
}
