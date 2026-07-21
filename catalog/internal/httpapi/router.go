package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/naira-project/naira/catalog/internal/catalog"
)

type routeHandler func(http.ResponseWriter, *http.Request) error
type listOptionsHandler func(http.ResponseWriter, *http.Request, listOptions) error

func NewRouter(service *catalog.Service, logger *log.Logger) http.Handler {
	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID)
	router.Use(chimiddleware.Recoverer)
	if logger != nil {
		router.Use(requestLogger(logger))
	}

	router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	router.Mount("/mcp", NewMCPHandler(service, logger))

	router.Route("/v1", func(r chi.Router) {
		r.Post("/plugins:run", handle(func(w http.ResponseWriter, r *http.Request) error {
			response := service.RunAllPlugins(r.Context())

			writeJSON(w, http.StatusAccepted, runPluginsResponseFromResult(response))
			return nil
		}))

		// GET /v1/nodes lists catalog nodes.
		// Supported query params:
		// - pageSize
		// - pageToken
		// - filter: only field="value" equality filters
		// Supported node filter fields: name, kind, path.
		r.Get("/nodes", handleWithListOptions(nodeListOptionsSpec, func(w http.ResponseWriter, r *http.Request, options listOptions) error {
			nodes := make([]Node, 0)
			for _, node := range service.ListNodes(r.Context()) {
				node := nodeFromCatalogNode(node)
				matches, err := matchNodeFilter(node, options.filter)
				if err != nil {
					return fmt.Errorf("matching node filter: %w", err)
				}
				if matches {
					nodes = append(nodes, node)
				}
			}

			sortNodes(nodes)

			page, nextPageToken, totalSize, err := paginate(nodes, options.pageSize, options.offset, "nodes", logger)
			if err != nil {
				return fmt.Errorf("paginating nodes: %w", err)
			}

			writeJSON(w, http.StatusOK, ListNodesResponse{Nodes: page, NextPageToken: nextPageToken, TotalSize: int32FromCount(totalSize, logger)})
			return nil
		}))

		r.Get("/nodes/{kind}/*", handle(func(w http.ResponseWriter, r *http.Request) error {
			node, err := service.GetNode(r.Context(), catalog.NodeID{Kind: chi.URLParam(r, "kind"), Path: chi.URLParam(r, "*")})
			if err != nil {
				return fmt.Errorf("getting node: %w", err)
			}

			writeJSON(w, http.StatusOK, nodeFromCatalogNode(node))
			return nil
		}))

		r.Get("/kinds", handle(func(w http.ResponseWriter, r *http.Request) error {
			writeJSON(w, http.StatusOK, catalogKinds)
			return nil
		}))

		// GET /v1/relations lists catalog relations.
		// Supported query params:
		// - pageSize
		// - pageToken
		// - filter: only field="value" equality filters
		// Supported relation filter fields: name, kind, fromNode, toNode.
		r.Get("/relations", handleWithListOptions(relationListOptionsSpec, func(w http.ResponseWriter, r *http.Request, options listOptions) error {
			relations := make([]Relation, 0)
			for _, relation := range service.ListRelations(r.Context()) {
				resource := relationFromCatalogRelation(relation)
				matches, err := matchRelationFilter(resource, options.filter)
				if err != nil {
					return fmt.Errorf("matching relation filter: %w", err)
				}
				if matches {
					relations = append(relations, resource)
				}
			}

			sortRelations(relations)

			page, nextPageToken, totalSize, err := paginate(relations, options.pageSize, options.offset, "relations", logger)
			if err != nil {
				return fmt.Errorf("paginating relations: %w", err)
			}

			writeJSON(w, http.StatusOK, ListRelationsResponse{Relations: page, NextPageToken: nextPageToken, TotalSize: int32FromCount(totalSize, logger)})
			return nil
		}))
	})

	return router
}

func handle(next routeHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := next(w, r); err != nil {
			writeError(w, err)
		}
	}
}

func handleWithListOptions(spec listOptionsSpec, next listOptionsHandler) http.HandlerFunc {
	return handle(func(w http.ResponseWriter, r *http.Request) error {
		options, err := listOptionsFromRequest(r, spec)
		if err != nil {
			return fmt.Errorf("getting list options from request: %w", err)
		}

		return next(w, r, options)
	})
}

func requestLogger(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			logger.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	buf := &bytes.Buffer{}
	encoder := json.NewEncoder(buf)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := err.Error()
	switch {
	case errors.Is(err, catalog.ErrNodeNotFound):
		status = http.StatusNotFound
	case isClientRequestError(err):
		status = http.StatusBadRequest
	case errors.Is(err, errPageTokenEncoding):
		status = http.StatusInternalServerError
		message = http.StatusText(status)
	}

	writeJSON(w, status, map[string]string{"error": message})
}

func isClientRequestError(err error) bool {
	return errors.Is(err, errInvalidPageToken) ||
		errors.Is(err, errInvalidFilter) ||
		errors.Is(err, errInvalidOrderBy) ||
		errors.Is(err, catalog.ErrInvalidIngestion) ||
		errors.Is(err, errInvalidPageSize)
}
