// Package scheduling turns schedules into plugin run requests.
package scheduling

import (
	"context"
	"errors"
	"fmt"
	"log"
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
	cron *cron.Cron
}

// NewConfiguredScheduler initializes configured plugin schedules and starts the scheduler.
// The caller must call Scheduler.Stop when the scheduler is no longer needed.
func NewConfiguredScheduler(ctx context.Context, configs catalog.PluginConfigsByName, runFunc RunPluginFunc, logger *log.Logger) (*Scheduler, error) {
	sch := &Scheduler{
		cron: cron.New(),
	}

	if err := sch.registerSchedules(ctx, configs, runFunc, logger); err != nil {
		return nil, fmt.Errorf("registering schedules: %w", err)
	}

	sch.cron.Start()
	return sch, nil
}

func (s *Scheduler) registerSchedules(ctx context.Context, configs catalog.PluginConfigsByName, runFunc RunPluginFunc, logger *log.Logger) error {
	for plugin, config := range configs {
		expr := config.Schedule
		if plugin == "" {
			return ErrInvalidPlugin
		}
		if expr == catalog.ScheduleManual {
			continue
		}

		_, err := s.cron.AddFunc(expr, func() {
			if _, err := runFunc(ctx, plugin); err != nil {
				if logger != nil {
					logger.Printf("scheduled run for plugin %q was not started: %v", plugin, err)
				}
			}
		})
		if err != nil {
			return fmt.Errorf("registering schedule for plugin %q (%q): %w", plugin, expr, err)
		}
	}
	return nil
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

// LastScheduledTime returns the most recent time, at or before now, at which
// expr would have fired, searching back at most lookback. It returns the
// zero time if no activation is found within that window.
func LastScheduledTime(expr string, now time.Time, lookback time.Duration) (time.Time, error) {
	schedule, err := cron.ParseStandard(expr)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing schedule %q: %w", expr, err)
	}

	cursor := now.Add(-lookback)
	var last time.Time
	for {
		next := schedule.Next(cursor)
		if next.IsZero() || next.After(now) {
			break
		}
		last = next
		cursor = next
	}
	return last, nil
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

func successCompletedAt(op operations.Operation) time.Time {
	if op.EndTime != nil {
		return *op.EndTime
	}
	return op.CreatedAt
}

// ResyncAtStartup triggers an immediate run, via runFunc, for a plugin based
// solely on the state of its most recent operation. The rules:
//
//  1. No operation recorded at all -> bootstrap.
//  2. Last operation is StateInterrupted -> trigger a run.
//  3. Last operation is StateFailed -> do nothing.
//  4. Last operation is StateSucceeded:
//     - ScheduleManual -> do nothing;
//     - otherwise -> trigger a run only if that success predates the most
//     recent time the plugin's schedule should have fired, to catch up
//     on ticks missed while the process was down.
//
// Meant to run once at startup, after ResyncAtStartup and before the
// running plugins by http server and scheduler
func ResyncAtStartup(
	ctx context.Context,
	configs catalog.PluginConfigsByName,
	opsStore operations.Store,
	runFunc RunPluginFunc,
	now time.Time,
	lookback time.Duration,
	logger *log.Logger,
) {
	for plugin, config := range configs {
		last, found, err := lastOperation(ctx, opsStore, plugin)
		if err != nil {
			if logger != nil {
				logger.Printf("startup resync check for plugin %q: listing operations: %v", plugin, err)
			}
			continue
		}

		if !found {
			if logger != nil {
				logger.Printf("plugin %q has never run; triggering initial run", plugin)
			}
			triggerRun(ctx, runFunc, plugin, logger)
			continue
		}

		switch last.State {
		case operations.StateInterrupted:
			if logger != nil {
				logger.Printf("plugin %q's last run was interrupted by a restart; triggering run", plugin)
			}
			triggerRun(ctx, runFunc, plugin, logger)

		case operations.StateFailed:
			// TODO: think if simpilify this, and run failed always. But first write tests for that.
			// what if failed was temporary, and next run would succeed?
			//
			// Ran to completion and genuinely failed - not retried
			// automatically.

		case operations.StateSucceeded:
			if config.Schedule == catalog.ScheduleManual {
				continue
			}

			lastScheduled, err := LastScheduledTime(config.Schedule, now, lookback)
			if err != nil {
				if logger != nil {
					logger.Printf("startup resync check for plugin %q: %v", plugin, err)
				}
				continue
			}

			lastSuccess := successCompletedAt(last)
			if lastScheduled.IsZero() || !lastSuccess.Before(lastScheduled) {
				continue
			}

			if logger != nil {
				logger.Printf("plugin %q last succeeded at %s, before scheduled tick at %s; triggering resync", plugin, lastSuccess, lastScheduled)
			}
			triggerRun(ctx, runFunc, plugin, logger)

		default:
			// StatePending/StateRunning shouldn't be possible here
			if logger != nil {
				logger.Printf("plugin %q's last operation %q is in unexpected state %q at startup; leaving it alone", plugin, last.Name, last.State)
			}
		}
	}
}

func triggerRun(ctx context.Context, runFunc RunPluginFunc, plugin string, logger *log.Logger) {
	if _, err := runFunc(ctx, plugin); err != nil {
		if logger != nil {
			logger.Printf("startup resync for plugin %q was not started: %v", plugin, err)
		}
	}
}
