// scheduling turns schedules into plugin run requests.
package scheduling

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/robfig/cron/v3"

	"github.com/naira-project/naira/catalog/internal/operations"
)

type RunStarter interface {
	RunPluginAsync(context.Context, string) (operations.Operation, error)
}

type Scheduler struct {
	mu      sync.Mutex
	store   Store
	starter RunStarter
	cron    *cron.Cron
	entries map[string]cron.EntryID
	logger  *log.Logger
}

func NewScheduler(store Store, starter RunStarter, logger *log.Logger) *Scheduler {
	return &Scheduler{
		store: store, starter: starter, cron: cron.New(), entries: make(map[string]cron.EntryID), logger: logger,
	}
}

// NewConfiguredScheduler creates and starts the initial in-process scheduler.
// The scheduler owns interpretation of plugin defaults and their registration.
func NewConfiguredScheduler(plugins map[string]string, defaults map[string]string, starter RunStarter, logger *log.Logger) (*Scheduler, error) {
	scheduler := NewScheduler(NewMemoryStore(), starter, logger)
	for plugin := range plugins {
		expression, configured := defaults[plugin]
		schedule := Schedule{
			Plugin:     plugin,
			Expression: expression,
			Enabled:    configured,
		}
		if err := scheduler.configureSchedule(schedule); err != nil {
			return nil, fmt.Errorf("configuring schedule for plugin %q: %w", plugin, err)
		}
	}
	if err := scheduler.Start(); err != nil {
		return nil, fmt.Errorf("starting scheduler: %w", err)
	}
	return scheduler, nil
}

func (s *Scheduler) Start() error {
	schedules, err := s.store.List()
	if err != nil {
		return fmt.Errorf("listing schedules: %w", err)
	}
	for _, schedule := range schedules {
		if err := s.reconcile(schedule); err != nil {
			return fmt.Errorf("loading schedule for plugin %q: %w", schedule.Plugin, err)
		}
	}
	s.cron.Start()
	return nil
}

func (s *Scheduler) Stop() context.Context {
	return s.cron.Stop()
}

// configureSchedule persists and activates an initial schedule from configuration.
func (s *Scheduler) configureSchedule(schedule Schedule) error {
	if err := validateSchedule(schedule); err != nil {
		return err
	}
	if err := s.store.Upsert(schedule); err != nil {
		return fmt.Errorf("saving schedule: %w", err)
	}
	if err := s.reconcile(schedule); err != nil {
		return fmt.Errorf("activating schedule: %w", err)
	}
	return nil
}

func (s *Scheduler) GetSchedule(plugin string) (Schedule, error) { return s.store.Get(plugin) }
func (s *Scheduler) ListSchedules() ([]Schedule, error)          { return s.store.List() }

func (s *Scheduler) reconcile(schedule Schedule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entryID, ok := s.entries[schedule.Plugin]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, schedule.Plugin)
	}
	if !schedule.Enabled || schedule.Expression == "" {
		return nil
	}

	entryID, err := s.cron.AddFunc(schedule.Expression, func() {
		if _, err := s.starter.RunPluginAsync(context.Background(), schedule.Plugin); err != nil {
			s.logf("scheduled run for plugin %q was not started: %v", schedule.Plugin, err)
		}
	})
	if err != nil {
		return fmt.Errorf("registering cron expression: %w", err)
	}
	s.entries[schedule.Plugin] = entryID
	return nil
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
