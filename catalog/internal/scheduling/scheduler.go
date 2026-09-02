// Package scheduling turns schedules into plugin run requests.
package scheduling

import (
	"context"
	"fmt"
	"log"

	"github.com/naira-project/naira/catalog/internal/operations"
	"github.com/robfig/cron/v3"
)

type RunStarter interface {
	RunPluginAsync(ctx context.Context, plugin string) (operations.Operation, error)
}

type Scheduler struct {
	store   Store
	starter RunStarter
	cron    *cron.Cron
	entries map[string]cron.EntryID
	logger  *log.Logger
}

// newScheduler creates an unstarted Scheduler instance with empty state.
func newScheduler(store Store, starter RunStarter, logger *log.Logger) *Scheduler {
	return &Scheduler{
		store:   store,
		starter: starter,
		cron:    cron.New(),
		entries: make(map[string]cron.EntryID),
		logger:  logger,
	}
}

// NewConfiguredScheduler initializes default plugin schedules, persists them, and starts the scheduler.
// The caller must call Scheduler.Stop when the scheduler is no longer needed.
// schedules maps plugin name to cron expression; plugins not in the map have no schedule.
func NewConfiguredScheduler(schedules map[string]string, starter RunStarter, logger *log.Logger) (*Scheduler, error) {
	scheduler := newScheduler(NewMemoryStore(), starter, logger)

	for plugin, expression := range schedules {
		schedule := Schedule{
			Plugin:     plugin,
			Expression: expression,
			Enabled:    expression != "",
		}

		if err := validateSchedule(schedule); err != nil {
			return nil, fmt.Errorf("invalid schedule for plugin %q: %w", plugin, err)
		}

		if err := scheduler.store.Upsert(schedule); err != nil {
			return nil, fmt.Errorf("saving schedule for plugin %q: %w", plugin, err)
		}
	}

	if err := scheduler.start(); err != nil {
		return nil, fmt.Errorf("starting scheduler: %w", err)
	}

	return scheduler, nil
}

// start loads enabled schedules from the store, registers them with cron, and starts execution.
func (s *Scheduler) start() error {
	schedules, err := s.store.List()
	if err != nil {
		return fmt.Errorf("listing schedules: %w", err)
	}

	for _, schedule := range schedules {
		if !schedule.Enabled || schedule.Expression == "" {
			continue
		}

		// Local copy to avoid closure variable capture issues in goroutines
		pluginName := schedule.Plugin

		entryID, err := s.cron.AddFunc(schedule.Expression, func() {
			if _, err := s.starter.RunPluginAsync(context.Background(), pluginName); err != nil {
				s.logf("scheduled run for plugin %q was not started: %v", pluginName, err)
			}
		})
		if err != nil {
			return fmt.Errorf("registering cron expression for plugin %q: %w", pluginName, err)
		}

		s.entries[pluginName] = entryID
	}

	s.cron.Start()
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

func (s *Scheduler) GetSchedule(plugin string) (Schedule, error) {
	return s.store.Get(plugin)
}

func (s *Scheduler) ListSchedules() ([]Schedule, error) {
	return s.store.List()
}

func validateSchedule(schedule Schedule) error {
	if schedule.Plugin == "" {
		return fmt.Errorf("schedule plugin: %w", ErrInvalidPlugin)
	}
	if schedule.Expression == "" || !schedule.Enabled {
		return nil
	}
	if _, err := cron.ParseStandard(schedule.Expression); err != nil {
		return fmt.Errorf("invalid schedule expression %q: %w", schedule.Expression, err)
	}
	return nil
}

func (s *Scheduler) logf(format string, args ...any) {
	if s.logger != nil {
		s.logger.Printf(format, args...)
	}
}
