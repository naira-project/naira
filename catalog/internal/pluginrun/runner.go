// Package pluginrun orchestrates asynchronous plugin runs: it invokes
// plugins, tracks their progress as operations, and writes their results
// into the catalog graph
package pluginrun

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

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/operations"
	"github.com/naira-project/naira/plugins/pkg/pluginapi"
)

type Plugin = pluginapi.Plugin

var (
	ErrInvalidPluginName    = errors.New("invalid plugin name")
	ErrPluginNotFound       = errors.New("plugin not found")
	ErrPluginAlreadyRunning = errors.New("plugin already has a running operation")
)

type Runner struct {
	store         catalog.Store
	operations    operations.Store
	plugins       map[string]Plugin
	logger        *log.Logger
	wg            sync.WaitGroup
	appCtx        context.Context
	pluginTimeout time.Duration
}

func NewRunner(
	// appCtx is used as the parent context for asynchronous plugin runs; it
	// should be cancelled on application shutdown.
	appCtx context.Context,
	store catalog.Store,
	operationStore operations.Store,
	plugins map[string]Plugin,
	pluginTimeout time.Duration,
	logger *log.Logger,
) *Runner {
	registeredPlugins := make(map[string]Plugin, len(plugins))
	for name, plugin := range plugins {
		if plugin == nil {
			continue
		}
		registeredPlugins[normalizePluginName(name)] = plugin
	}

	return &Runner{
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
//
// NOTE: The check for active operations (hasActiveOperation) and operation creation
// are not atomic. This creates a possible race where two concurrent requests for
// the same plugin could both pass the check before either creates the operation.
// The ErrPluginAlreadyRunning error is therefore "best effort" rather than a strict guarantee.
func (r *Runner) RunPluginAsync(_ context.Context, pluginName string) (operations.Operation, error) {
	pluginName = normalizePluginName(pluginName)
	if pluginName == "" {
		return operations.Operation{}, fmt.Errorf("normalize plugin name: %w", ErrInvalidPluginName)
	}

	if _, ok := r.plugins[pluginName]; !ok {
		return operations.Operation{}, fmt.Errorf("looking up plugin %q: %w", pluginName, ErrPluginNotFound)
	}

	if r.hasActiveOperation(pluginName) {
		return operations.Operation{}, fmt.Errorf("plugin %q: %w", pluginName, ErrPluginAlreadyRunning)
	}

	op := operations.Operation{
		Name:      "plugin-run-" + uuid.NewString(),
		Plugin:    pluginName,
		State:     operations.StatePending,
		CreatedAt: time.Now(),
	}
	if err := r.operations.Create(op); err != nil {
		return operations.Operation{}, fmt.Errorf("creating operation for plugin %q: %w", pluginName, err)
	}

	r.wg.Go(func() {
		r.executePluginRun(op.Name, pluginName)
	})

	return op, nil
}

// RunAllPluginsAsync starts an asynchronous run for every registered plugin
// and returns the operations that track their progress.
//
// NOTE: The ctx parameter is accepted for signature consistency but is not used.
// Plugin runs use the runner's appCtx and live beyond the lifetime of individual
// HTTP requests. Canceling ctx will not interrupt in-progress plugin executions.
func (r *Runner) RunAllPluginsAsync(ctx context.Context) []operations.Operation {
	pluginNames := make([]string, 0, len(r.plugins))
	for name := range r.plugins {
		pluginNames = append(pluginNames, name)
	}
	sort.Strings(pluginNames)

	result := make([]operations.Operation, 0, len(pluginNames))
	for _, pluginName := range pluginNames {
		op, err := r.RunPluginAsync(ctx, pluginName)
		if err != nil {
			r.logf("skipping async run for plugin %q: %v", pluginName, err)
			continue
		}
		result = append(result, op)
	}

	return result
}

func (r *Runner) GetOperation(_ context.Context, name string) (operations.Operation, error) {
	op, err := r.operations.Get(name)
	if err != nil {
		return operations.Operation{}, fmt.Errorf("getting operation %q: %w", name, err)
	}
	return op, nil
}

func (r *Runner) ListPlugins() []string {
	names := make([]string, 0, len(r.plugins))
	for name := range r.plugins {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ListOperations returns all operations, optionally filtered by plugin and
// state, ordered by creation time descending.
func (r *Runner) ListOperations(_ context.Context, filter operations.Filter) ([]operations.Operation, error) {
	result, err := r.operations.List(filter)
	if err != nil {
		return nil, fmt.Errorf("listing operations: %w", err)
	}
	return result, nil
}

// Wait blocks until all in-flight plugin runs have finished.
func (r *Runner) Wait() {
	r.wg.Wait()
}

// executePluginRun runs a single plugin and updates the operation outcome.
func (r *Runner) executePluginRun(operationName, pluginName string) {
	if err := r.operations.UpdateState(operationName, operations.StateRunning, nil, 0, 0); err != nil {
		r.logf("marking operation %q as running: %v", operationName, err)
		return
	}

	plugin, ok := r.plugins[pluginName]
	if !ok {
		r.failOperation(operationName, pluginName, fmt.Errorf("looking up plugin %q: %w", pluginName, ErrPluginNotFound))
		return
	}

	runCtx, cancel := context.WithTimeout(r.appCtx, r.pluginTimeout)
	defer cancel()

	response, err := plugin.Collect(runCtx)
	if err != nil {
		r.failOperation(operationName, pluginName, fmt.Errorf("collecting response from plugin %q: %w", pluginName, err))
		return
	}

	snapshotID := uuid.New()
	upsertedNodes, upsertedRelations, err := r.store.ApplyPluginSnapshot(pluginName, snapshotID, response.Nodes, response.Relations)
	if err != nil {
		r.failOperation(operationName, pluginName, fmt.Errorf("upserting graph from plugin %q: %w", pluginName, err))
		return
	}

	r.logf("plugin %q upserted %d nodes and %d relations", pluginName, upsertedNodes, upsertedRelations)

	if err := r.operations.UpdateState(operationName, operations.StateSucceeded, nil, upsertedNodes, upsertedRelations); err != nil {
		r.logf("marking operation %q as succeeded: %v", operationName, err)
	}
}

func (r *Runner) failOperation(operationName, pluginName string, err error) {
	statusErr := &operations.StatusError{Message: err.Error()}

	r.logf("plugin %q run failed: %v", pluginName, err)

	if updateErr := r.operations.UpdateState(operationName, operations.StateFailed, statusErr, 0, 0); updateErr != nil {
		r.logf("marking operation %q as failed: %v", operationName, updateErr)
	}
}

func (r *Runner) hasActiveOperation(pluginName string) bool {
	ops, err := r.operations.List(operations.Filter{Plugin: pluginName})
	if err != nil {
		r.logf("listing operations for plugin %q: %v", pluginName, err)
		return false
	}

	for _, op := range ops {
		if op.State == operations.StatePending || op.State == operations.StateRunning {
			return true
		}
	}
	return false
}

func normalizePluginName(pluginName string) string {
	return strings.TrimSpace(strings.ToLower(pluginName))
}

func (r *Runner) logf(format string, v ...any) {
	if r.logger != nil {
		r.logger.Printf(format, v...)
	}
}
