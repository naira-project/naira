package httpapi

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/naira-project/naira/catalog/internal/operations"
	"github.com/naira-project/naira/catalog/internal/pluginrun"
)

// StatusErrorResource is an AIP-193 compliant error representation carried
// by failed operations.
type StatusErrorResource struct {
	Message string `json:"message"`
}

// OperationMetadataResource holds progress information about an in-flight
// or completed operation. It never carries the operation's result.
type OperationMetadataResource struct {
	Plugin    string     `json:"plugin"`
	State     string     `json:"state"`
	StartTime time.Time  `json:"startTime"`
	EndTime   *time.Time `json:"endTime,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

// RunPluginResult is the successful result of a plugin run operation.
type RunPluginResult struct {
	NodesUpserted     int `json:"nodesUpserted"`
	RelationsUpserted int `json:"relationsUpserted"`
}

// OperationResource is the JSON representation of an AIP-151 operation.
type OperationResource struct {
	Name     string                    `json:"name"`
	Done     bool                      `json:"done"`
	Metadata OperationMetadataResource `json:"metadata"`
	Response *RunPluginResult          `json:"response,omitempty"`
	Error    *StatusErrorResource      `json:"error,omitempty"`
}

type ListOperationsResponse struct {
	Operations    []OperationResource `json:"operations"`
	NextPageToken string              `json:"nextPageToken,omitempty"`
	TotalSize     int32               `json:"totalSize"`
}

type RunPluginsResponse struct {
	Operations []OperationResource `json:"operations"`
}

var operationListOptionsSpec = listOptionsSpec{
	scope:         "operations",
	allowedFields: map[string]bool{"plugin": true, "state": true},
}

func operationFromCatalogOperation(op operations.Operation) OperationResource {
	done := op.State == operations.StateSucceeded || op.State == operations.StateFailed

	resource := OperationResource{
		Name: op.Name,
		Done: done,
		Metadata: OperationMetadataResource{
			Plugin:    op.Plugin,
			State:     string(op.State),
			StartTime: op.StartTime,
			EndTime:   op.EndTime,
			CreatedAt: op.CreatedAt,
		},
	}

	switch {
	case done && op.State == operations.StateSucceeded:
		resource.Response = &RunPluginResult{
			NodesUpserted:     op.NodesUpserted,
			RelationsUpserted: op.RelationsUpserted,
		}
	case done && op.Error != nil:
		resource.Error = &StatusErrorResource{Message: op.Error.Message}
	}

	return resource
}

func toOperationResources(ops []operations.Operation) []OperationResource {
	result := make([]OperationResource, 0, len(ops))
	for _, op := range ops {
		result = append(result, operationFromCatalogOperation(op))
	}
	return result
}

func matchOperationFilter(operation OperationResource, filter *equalityFilter) (bool, error) {
	matches, err := filter.matchesResource(map[string]string{
		"plugin": operation.Metadata.Plugin,
		"state":  operation.Metadata.State,
	}, "operation")
	if err != nil {
		return false, fmt.Errorf("matching resource filter: %w", err)
	}

	return matches, nil
}

// GET /v1/operations lists plugin run operations.
// Supported query params:
// - pageSize
// - pageToken
// - filter: only field="value" equality filters
// Supported operation filter fields: plugin, state.
func newListOperationsHandler(runner *pluginrun.Runner, logger *log.Logger) http.HandlerFunc {
	return handleWithListOptions(operationListOptionsSpec, func(w http.ResponseWriter, r *http.Request, options listOptions) error {
		listed, err := runner.ListOperations(r.Context(), operations.Filter{})
		if err != nil {
			return fmt.Errorf("listing operations: %w", err)
		}

		result := make([]OperationResource, 0)
		for _, op := range listed {
			resource := operationFromCatalogOperation(op)
			matches, err := matchOperationFilter(resource, options.filter)
			if err != nil {
				return fmt.Errorf("matching operation filter: %w", err)
			}
			if matches {
				result = append(result, resource)
			}
		}

		page, nextPageToken, totalSize, err := paginate(result, options.pageSize, options.offset, "operations", logger)
		if err != nil {
			return fmt.Errorf("paginating operations: %w", err)
		}

		writeJSON(w, http.StatusOK, ListOperationsResponse{Operations: page, NextPageToken: nextPageToken, TotalSize: int32FromCount(totalSize, logger)})
		return nil
	})
}

// GET /v1/operations/{operationId} returns a single operation.
func newGetOperationHandler(runner *pluginrun.Runner) http.HandlerFunc {
	return handle(func(w http.ResponseWriter, r *http.Request) error {
		op, err := runner.GetOperation(r.Context(), chi.URLParam(r, "operationId"))
		if err != nil {
			return fmt.Errorf("getting operation: %w", err)
		}

		writeJSON(w, http.StatusOK, operationFromCatalogOperation(op))
		return nil
	})
}
