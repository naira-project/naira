package scheduling

import "time"

// Schedule describes the effective periodic trigger for a plugin.
// An empty Expression means that the plugin is manual-only.
type Schedule struct {
	Plugin     string    `json:"plugin"`
	Expression string    `json:"expression,omitempty"`
	Enabled    bool      `json:"enabled"`
	Source     string    `json:"source"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type Store interface {
	Get(plugin string) (Schedule, error)
	List() ([]Schedule, error)
	Upsert(schedule Schedule) error
}
