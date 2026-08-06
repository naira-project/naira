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

type OperationState string

const (
	OperationStatePending   OperationState = "PENDING"
	OperationStateRunning   OperationState = "RUNNING"
	OperationStateSucceeded OperationState = "SUCCEEDED"
	OperationStateFailed    OperationState = "FAILED"
)

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

	Error *StatusError `json:"error,omitempty"`

	NodesUpserted     int `json:"nodesUpserted"`
	RelationsUpserted int `json:"relationsUpserted"`

	CreatedAt time.Time `json:"createdAt"`
}

type OperationStore interface {
	Create(op Operation) error
	Get(name string) (Operation, error)
	List(filter OperationFilter) ([]Operation, error)
	UpdateState(name string, state OperationState, err *StatusError, nodesUpserted, relationsUpserted int) error
}

type OperationFilter struct {
	Plugin string
	State  OperationState
}
