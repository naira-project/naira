// Package scheduling turns schedules into plugin run requests.
package scheduling

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"sort"
	"time"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/operations"
	"github.com/robfig/cron/v3"
)

var (
	ErrInvalidPlugin = errors.New("invalid plugin name")
)

type RunPluginFunc func(ctx context.Context, plugin string) (operations.Operation, error)

type Scheduler struct {
	cron     *cron.Cron
	ctx      context.Context
	configs  catalog.PluginConfigsByName
	opsStore operations.Store
	runFunc  RunPluginFunc
	logger   *log.Logger
}

// NewScheduler registers configured plugin schedules without starting them.
// Call Start to resync missed runs and begin firing schedules, and Stop when
// the scheduler is no longer needed.
func NewScheduler(
	ctx context.Context,
	configs catalog.PluginConfigsByName,
	opsStore operations.Store,
	runFunc RunPluginFunc,
	logger *log.Logger,
) (*Scheduler, error) {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}

	sch := &Scheduler{
		cron:     cron.New(),
		ctx:      ctx,
		configs:  configs,
		opsStore: opsStore,
		runFunc:  runFunc,
		logger:   logger,
	}

	if err := sch.registerSchedules(); err != nil {
		return nil, fmt.Errorf("registering schedules: %w", err)
	}

	return sch, nil
}

func (s *Scheduler) registerSchedules() error {
	for plugin, config := range s.configs {
		expr := config.Schedule
		if plugin == "" {
			return ErrInvalidPlugin
		}
		if expr == catalog.ScheduleManual {
			continue
		}

		_, err := s.cron.AddFunc(expr, func() {
			if _, err := s.runFunc(s.ctx, plugin); err != nil {
				s.logger.Printf("scheduled run for plugin %q was not started: %v", plugin, err)
			}
		})
		if err != nil {
			return fmt.Errorf("registering schedule for plugin %q (%q): %w", plugin, expr, err)
		}
	}
	return nil
}

// Start resyncs plugin runs then begins firing configured schedules.
func (s *Scheduler) Start() {
	resyncAtStartup(s.ctx, s.configs, s.opsStore, s.runFunc, time.Now(), s.logger)
	s.cron.Start()
}

func (s *Scheduler) Stop(ctx context.Context) error {
	cronCtx := s.cron.Stop()
	select {
	case <-cronCtx.Done():
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// lastOperation returns the most recently created operation recorded for
// plugin, or found=false if it has never run
func lastOperation(ctx context.Context, store operations.Store, plugin string) (op operations.Operation, found bool, err error) {
	ops, err := store.List(ctx, operations.Filter{Plugin: plugin})
	if err != nil {
		return operations.Operation{}, false, err
	}
	if len(ops) == 0 {
		return operations.Operation{}, false, nil
	}
	return ops[0], true, nil
}

func operationCompletedAt(op operations.Operation) time.Time {
	if op.EndTime != nil {
		return *op.EndTime
	}
	return op.CreatedAt
}

// needsResync reports why a plugin should run at startup, or an empty reason
// when its current operation state and schedule do not require another run.
func needsResync(last *operations.Operation, scheduleExpr string, now time.Time) (string, error) {
	if last == nil {
		return "plugin has never run; triggering initial run", nil
	}

	switch last.State {
	case operations.StateInterrupted:
		return "last run was interrupted by a restart; triggering run", nil
	case operations.StatePending, operations.StateRunning:
		return "", nil
	case operations.StateSucceeded, operations.StateFailed:
		if scheduleExpr == catalog.ScheduleManual {
			return "", nil
		}

		schedule, err := cron.ParseStandard(scheduleExpr)
		if err != nil {
			return "", fmt.Errorf("parsing schedule %q: %w", scheduleExpr, err)
		}
		completedAt := operationCompletedAt(*last)
		next := schedule.Next(completedAt)
		if next.IsZero() || next.After(now) {
			return "", nil
		}
		return fmt.Sprintf("last attempt completed at %s, before scheduled tick at %s; triggering resync", completedAt, next), nil
	default:
		return "", nil
	}
}

// resyncAtStartup uses persisted operation state and configured schedules to
// start bootstrap, interrupted, or missed scheduled runs. It runs once at
// startup before the scheduler and HTTP server accept plugin runs.
func resyncAtStartup(
	ctx context.Context,
	configs catalog.PluginConfigsByName,
	opsStore operations.Store,
	runFunc RunPluginFunc,
	now time.Time,
	logger *log.Logger,
) {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}

	plugins := make([]string, 0, len(configs))
	for plugin := range configs {
		plugins = append(plugins, plugin)
	}
	sort.Strings(plugins)

	for _, plugin := range plugins {
		config := configs[plugin]
		last, found, err := lastOperation(ctx, opsStore, plugin)
		if err != nil {
			logger.Printf("startup resync check for plugin %q: listing operations: %v", plugin, err)
			continue
		}

		var latest *operations.Operation
		if found {
			latest = &last
		}
		reason, err := needsResync(latest, config.Schedule, now)
		if err != nil {
			logger.Printf("startup resync check for plugin %q: %v", plugin, err)
			continue
		}
		if reason == "" {
			continue
		}

		logger.Printf("plugin %q: %s", plugin, reason)
		triggerRun(ctx, runFunc, plugin, logger)
	}
}

func triggerRun(ctx context.Context, runFunc RunPluginFunc, plugin string, logger *log.Logger) {
	if _, err := runFunc(ctx, plugin); err != nil {
		logger.Printf("startup resync for plugin %q was not started: %v", plugin, err)
	}
}
