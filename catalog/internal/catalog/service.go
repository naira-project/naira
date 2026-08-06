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

func NewService(
	// The appCtx is used as the parent context for asynchronous plugin runs;
	// it should be cancelled on application shutdown.
	appCtx context.Context,
	store Store,
	operationStore OperationStore,
	plugins map[string]Plugin,
	pluginTimeout time.Duration,
	logger *log.Logger,
) *Service {
	registeredPlugins := make(map[string]Plugin, len(plugins))
	for name, plugin := range plugins {
		if plugin == nil {
			continue
		}
		registeredPlugins[normalizePluginName(name)] = plugin
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

// RunPluginAsync starts an asynchronous plugin run and returns the operation
// that tracks its progress.
func (s *Service) RunPluginAsync(_ context.Context, pluginName string) (Operation, error) {
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
		s.executePluginRun(op.Name, pluginName)
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
			s.logf("skipping async run for plugin %q: %v", pluginName, err)
			continue
		}
		operations = append(operations, op)
	}

	return operations
}

func (s *Service) GetOperation(_ context.Context, name string) (Operation, error) {
	op, err := s.operations.Get(name)
	if err != nil {
		return Operation{}, fmt.Errorf("getting operation %q: %w", name, err)
	}
	return op, nil
}

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
func (s *Service) executePluginRun(operationName, pluginName string) {
	if err := s.operations.UpdateState(operationName, OperationStateRunning, nil, 0, 0); err != nil {
		s.logf("marking operation %q as running: %v", operationName, err)
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

	s.logf("plugin %q upserted %d nodes and %d relations", pluginName, upsertedNodes, upsertedRelations)

	if err := s.operations.UpdateState(operationName, OperationStateSucceeded, nil, upsertedNodes, upsertedRelations); err != nil {
		s.logf("marking operation %q as succeeded: %v", operationName, err)
	}
}

// failOperation marks an operation as FAILED with an AIP-193 style error.
func (s *Service) failOperation(operationName, pluginName string, err error) {
	statusErr := &StatusError{Message: err.Error()}

	s.logf("plugin %q run failed: %v", pluginName, err)

	if updateErr := s.operations.UpdateState(operationName, OperationStateFailed, statusErr, 0, 0); updateErr != nil {
		s.logf("marking operation %q as failed: %v", operationName, updateErr)
	}
}

func (s *Service) hasActiveOperation(pluginName string) bool {
	operations, err := s.operations.List(OperationFilter{Plugin: pluginName})
	if err != nil {
		s.logf("listing operations for plugin %q: %v", pluginName, err)
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

func (s *Service) logf(format string, v ...any) {
	if s.logger != nil {
		s.logger.Printf(format, v...)
	}
}
