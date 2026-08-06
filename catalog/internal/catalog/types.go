package catalog

import (
	"time"

	"github.com/naira-project/naira/plugins/pkg/pluginapi"
)

type Plugin = pluginapi.Plugin

type PropertyMap = pluginapi.PropertyMap

type NodeID = pluginapi.NodeID

type CollectResponse = pluginapi.CollectResponse

type NodeClaim = pluginapi.NodeClaim

type RelationClaim = pluginapi.RelationClaim

// OperationState represents the lifecycle state of a plugin run operation.
type OperationState string

const (
	OperationStatePending   OperationState = "PENDING"
	OperationStateRunning   OperationState = "RUNNING"
	OperationStateSucceeded OperationState = "SUCCEEDED"
	OperationStateFailed    OperationState = "FAILED"
)

// StatusError is an AIP-193 compliant error representation carried by failed
// operations. Message is a human-readable description of the problem.
type StatusError struct {
	Message string `json:"message"`
}

// Operation represents a single asynchronous plugin run (AIP-151).
type Operation struct {
	Name      string         `json:"name"`
	Plugin    string         `json:"plugin"`
	State     OperationState `json:"state"`
	StartTime time.Time      `json:"startTime"`
	EndTime   *time.Time     `json:"endTime,omitempty"`

	// Error is populated only when State is FAILED.
	// This is the sole error signal (AIP-193).
	Error *StatusError `json:"error,omitempty"`

	NodesUpserted     int `json:"nodesUpserted"`
	RelationsUpserted int `json:"relationsUpserted"`

	CreatedAt time.Time `json:"createdAt"`
}

// OperationStore is the persistence contract for operations.
type OperationStore interface {
	// Create persists a new operation. The caller is responsible for
	// setting Operation.Name (unique identifier).
	Create(op Operation) error

	// Get retrieves a single operation by name.
	// Returns ErrOperationNotFound if the operation does not exist.
	Get(name string) (Operation, error)

	// List returns all operations, ordered by creation time descending
	// (most recent first). Optional filters narrow the result set.
	List(filter OperationFilter) ([]Operation, error)

	// UpdateState atomically transitions an operation to a new state and
	// applies the associated outcome: the error for FAILED, the upserted
	// result counts for SUCCEEDED, and the start time for RUNNING.
	UpdateState(name string, state OperationState, err *StatusError, nodesUpserted, relationsUpserted int) error
}

// OperationFilter carries optional filter criteria for ListOperations.
type OperationFilter struct {
	Plugin string
	State  OperationState
}

var (
	ErrOperationNotFound      = NewOperationError("operation not found")
	ErrOperationAlreadyExists = NewOperationError("operation already exists")
)

// NewOperationError is a helper for creating operation-related sentinel errors.
func NewOperationError(msg string) error {
	return &operationError{msg: msg}
}

type operationError struct{ msg string }

func (e *operationError) Error() string { return e.msg }
