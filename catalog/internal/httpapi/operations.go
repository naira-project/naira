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

// StatusErrorResource is an error representation carried by failed operations.
type StatusErrorResource struct {
	Message string `json:"message"`
}

// OperationMetadataResource holds progress information about an in-flight
// or completed operation.
type OperationMetadataResource struct {
	Plugin    string     `json:"plugin"`
	StartTime time.Time  `json:"startTime"`
	EndTime   *time.Time `json:"endTime,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

// RunPluginResponse is the successful result of a plugin run operation.
type RunPluginResponse struct {
	NodesUpserted     int `json:"nodesUpserted"`
	RelationsUpserted int `json:"relationsUpserted"`
}

// OperationResource is the JSON representation of an AIP-151 operation.
// See https://github.com/googleapis/googleapis/blob/0c516dc746bccd2e0f29a44e4b4e72a216bfc82a/google/longrunning/operations.proto#L121
// for documentation of fields.
type OperationResource struct {
	Name     string                    `json:"name"`
	Metadata OperationMetadataResource `json:"metadata"`
	Done     bool                      `json:"done"`
	Error    *StatusErrorResource      `json:"error,omitempty"`
	Response *RunPluginResponse        `json:"response,omitempty"`
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
	allowedFields: map[string]bool{"plugin": true},
}

func operationFromCatalogOperation(op operations.Operation) OperationResource {
	done := op.State == operations.StateSucceeded || op.State == operations.StateFailed

	resource := OperationResource{
		Name: op.Name,
		Done: done,
		Metadata: OperationMetadataResource{
			Plugin:    op.Plugin,
			StartTime: op.StartTime,
			EndTime:   op.EndTime,
			CreatedAt: op.CreatedAt,
		},
	}

	switch {
	case done && op.State == operations.StateSucceeded:
		resource.Response = &RunPluginResponse{
			NodesUpserted:     op.NodesUpserted,
			RelationsUpserted: op.RelationsUpserted,
		}
	case done && op.State == operations.StateFailed:
		message := "operation failed without an error"
		if op.Error != nil {
			message = op.Error.Message
		}
		resource.Error = &StatusErrorResource{Message: message}
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

func matchOperationFilter(operation operations.Operation, filter *equalityFilter) (bool, error) {
	matches, err := filter.matchesResource(map[string]string{
		"plugin": operation.Plugin,
	}, "operation")
	if err != nil {
		return false, fmt.Errorf("matching resource filter: %w", err)
	}

	return matches, nil
}

// newListOperationsHandler lists plugin run operations.
// Supported query params:
// - pageSize
// - pageToken
// - filter: only field="value" equality filters
// Supported operation filter fields: plugin.
func newListOperationsHandler(runner *pluginrun.Runner, logger *log.Logger) http.HandlerFunc {
	return handleWithListOptions(operationListOptionsSpec, func(w http.ResponseWriter, r *http.Request, options listOptions) error {
		listed, err := runner.ListOperations(r.Context(), operations.Filter{})
		if err != nil {
			return fmt.Errorf("listing operations: %w", err)
		}

		result := make([]OperationResource, 0)
		for _, op := range listed {
			matches, err := matchOperationFilter(op, options.filter)
			if err != nil {
				return fmt.Errorf("matching operation filter: %w", err)
			}
			if matches {
				result = append(result, operationFromCatalogOperation(op))
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

// newGetOperationHandler returns a single operation.
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
