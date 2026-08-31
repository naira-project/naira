package scheduling

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotFound      = errors.New("schedule not found")
	ErrInvalidPlugin = errors.New("invalid plugin name")
)

type MemoryStore struct {
	mu        sync.RWMutex
	schedules map[string]Schedule
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{schedules: make(map[string]Schedule)}
}

func (s *MemoryStore) Get(plugin string) (Schedule, error) {
	plugin = normalizePlugin(plugin)
	if plugin == "" {
		return Schedule{}, fmt.Errorf("get schedule: %w", ErrInvalidPlugin)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	schedule, ok := s.schedules[plugin]
	if !ok {
		return Schedule{}, fmt.Errorf("schedule for plugin %q: %w", plugin, ErrNotFound)
	}
	return schedule, nil
}

func (s *MemoryStore) List() ([]Schedule, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Schedule, 0, len(s.schedules))
	for _, schedule := range s.schedules {
		result = append(result, schedule)
	}
	return result, nil
}

func (s *MemoryStore) Upsert(schedule Schedule) error {
	schedule.Plugin = normalizePlugin(schedule.Plugin)
	if schedule.Plugin == "" {
		return fmt.Errorf("upsert schedule: %w", ErrInvalidPlugin)
	}
	if schedule.UpdatedAt.IsZero() {
		schedule.UpdatedAt = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.schedules[schedule.Plugin] = schedule
	return nil
}

func normalizePlugin(plugin string) string { return strings.TrimSpace(strings.ToLower(plugin)) }
