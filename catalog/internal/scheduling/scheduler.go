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
func NewConfiguredScheduler(configs catalog.PluginConfigsByName, runFunc RunPluginFunc, logger *log.Logger) (*Scheduler, error) {
	sch := &Scheduler{
		cron: cron.New(),
	}

	if err := sch.registerSchedules(configs, runFunc, logger); err != nil {
		return nil, fmt.Errorf("registering schedules: %w", err)
	}

	sch.cron.Start()
	return sch, nil
}

func (s *Scheduler) registerSchedules(configs catalog.PluginConfigsByName, runFunc RunPluginFunc, logger *log.Logger) error {
	for plugin, config := range configs {
		expr := config.Schedule
		if plugin == "" {
			return ErrInvalidPlugin
		}
		if expr == catalog.ScheduleManual {
			continue
		}

		_, err := s.cron.AddFunc(expr, func() {
			if _, err := runFunc(context.Background(), plugin); err != nil {
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
// expr would have fired, searching back at most lookback. It returns the zero
// time if no activation is found within that window.
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

// ResyncStale triggers an immediate run for every non-manual plugin whose
// latest successful run predates the most recent time its schedule should have
// fired. It is intended to run once during application startup.
func ResyncStale(
	ctx context.Context,
	configs catalog.PluginConfigsByName,
	opsStore operations.Store,
	runFunc RunPluginFunc,
	now time.Time,
	lookback time.Duration,
	logger *log.Logger,
) {
	for plugin, config := range configs {
		if config.Schedule == catalog.ScheduleManual {
			continue
		}

		lastScheduled, err := LastScheduledTime(config.Schedule, now, lookback)
		if err != nil {
			logResync(logger, "resync check for plugin %q: %v", plugin, err)
			continue
		}
		if lastScheduled.IsZero() {
			continue
		}

		lastSuccess, found, err := lastSuccessfulRun(opsStore, plugin)
		if err != nil {
			logResync(logger, "resync check for plugin %q: listing operations: %v", plugin, err)
			continue
		}
		if found && !lastSuccess.Before(lastScheduled) {
			continue
		}

		if found {
			logResync(logger, "plugin %q last succeeded at %s, before scheduled tick at %s; triggering resync", plugin, lastSuccess, lastScheduled)
		} else {
			logResync(logger, "plugin %q has no recorded successful run; triggering resync", plugin)
		}

		if _, err := runFunc(ctx, plugin); err != nil {
			logResync(logger, "resync for plugin %q was not started: %v", plugin, err)
		}
	}
}

func lastSuccessfulRun(store operations.Store, plugin string) (time.Time, bool, error) {
	ops, err := store.List(operations.Filter{Plugin: plugin, State: operations.StateSucceeded})
	if err != nil {
		return time.Time{}, false, err
	}
	if len(ops) == 0 {
		return time.Time{}, false, nil
	}

	latest := ops[0]
	completedAt := latest.CreatedAt
	if latest.EndTime != nil {
		completedAt = *latest.EndTime
	}
	return completedAt, true, nil
}

func logResync(logger *log.Logger, format string, args ...any) {
	if logger != nil {
		logger.Printf(format, args...)
	}
}
