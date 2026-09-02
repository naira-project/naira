// Package scheduling turns schedules into plugin run requests.
package scheduling

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/naira-project/naira/catalog/internal/operations"
	"github.com/robfig/cron/v3"
)

var (
	ErrInvalidPlugin = errors.New("invalid plugin name")
)

type RunStarter interface {
	RunPluginAsync(ctx context.Context, plugin string) (operations.Operation, error)
}

type Scheduler struct {
	cron *cron.Cron
}

// NewConfiguredScheduler initializes default plugin schedules and starts the scheduler.
// The caller must call Scheduler.Stop when the scheduler is no longer needed.
func NewConfiguredScheduler(schedules map[string]string, starter RunStarter, logger *log.Logger) (*Scheduler, error) {
	sch := &Scheduler{
		cron: cron.New(),
	}

	if err := sch.registerSchedules(schedules, starter, logger); err != nil {
		return nil, fmt.Errorf("registering schedules: %w", err)
	}

	sch.cron.Start()
	return sch, nil
}

func (s *Scheduler) registerSchedules(schedules map[string]string, starter RunStarter, logger *log.Logger) error {
	for plugin, expr := range schedules {
		if plugin == "" {
			return ErrInvalidPlugin
		}
		if expr == "" {
			continue
		}

		_, err := s.cron.AddFunc(expr, func() {
			if _, err := starter.RunPluginAsync(context.Background(), plugin); err != nil {
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
