// Package operations implements a small, domain-agnostic tracker for
// long-running asynchronous work (AIP-151).
package operations

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound      = errors.New("operation not found")
	ErrAlreadyExists = errors.New("operation already exists")
)

type State string

const (
	StatePending     State = "PENDING"
	StateRunning     State = "RUNNING"
	StateSucceeded   State = "SUCCEEDED"
	StateFailed      State = "FAILED"
	StateInterrupted State = "INTERRUPTED"
)

// StatusError is an AIP-193 compliant error representation carried by
// failed operations.
type StatusError struct {
	Message string `json:"message"`
}

// Operation represents a single asynchronous unit of work.
type Operation struct {
	Name      string     `json:"name"`
	Plugin    string     `json:"plugin"`
	State     State      `json:"state"`
	StartTime time.Time  `json:"startTime"`
	EndTime   *time.Time `json:"endTime,omitempty"`

	Error *StatusError `json:"error,omitempty"`

	NodesUpserted     int `json:"nodesUpserted"`
	RelationsUpserted int `json:"relationsUpserted"`

	CreatedAt time.Time `json:"createdAt"`
}

// Filter narrows List results by plugin name and/or state. Zero values are
// treated as "don't filter on this field".
type Filter struct {
	Plugin string
	State  State
}

type Store interface {
	Create(ctx context.Context, op Operation) error
	Get(ctx context.Context, name string) (Operation, error)
	List(ctx context.Context, filter Filter) ([]Operation, error)
	UpdateState(ctx context.Context, name string, state State, err *StatusError, nodesUpserted, relationsUpserted int) error
	MarkInterrupted(ctx context.Context) (int, error)
}
