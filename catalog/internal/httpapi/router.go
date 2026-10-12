package httpapi

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/naira-project/naira/catalog/internal/auth/keycloak"
	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/pluginrun"
)

// catalogReader provides the graph queries required by the HTTP API.
type catalogReader interface {
	GetNode(ctx context.Context, id catalog.NodeID) (catalog.Node, error)
	ListNodes(ctx context.Context) ([]catalog.Node, error)
	ListRelations(ctx context.Context) ([]catalog.Relation, error)
}

// NewRouter wires up the catalog HTTP API.
// catalogReader serves read-only graph queries (nodes/relations);
// runner drives and reports on asynchronous plugin runs (plugins/operations).
func NewRouter(catalogReader catalogReader, runner *pluginrun.Runner, plugins catalog.PluginConfigsByName, logger *log.Logger, kc keycloak.Config) (http.Handler, error) {
	authMiddleware, err := keycloak.NewAuthMiddleware(kc)
	if err != nil {
		return nil, fmt.Errorf("creating auth middleware: %w", err)
	}

	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID)
	router.Use(chimiddleware.Recoverer)
	if logger != nil {
		router.Use(requestLogger(logger))
	}

	router.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	router.Route("/v1", func(r chi.Router) {
		r.Use(authMiddleware)

		r.Get("/plugins", newListPluginsHandler(plugins, logger))
		r.Get("/plugins/{plugin}", newGetPluginHandler(plugins))
		r.Post("/plugins:run", newRunAllPluginsHandler(runner))
		r.Post("/plugins/{plugin}:run", newRunPluginHandler(runner))

		r.Get("/operations", newListOperationsHandler(runner, logger))
		r.Get("/operations/{operationId}", newGetOperationHandler(runner))

		r.Get("/nodes", newListNodesHandler(catalogReader, logger))
		r.Get("/nodes/{kind}/*", newGetNodeHandler(catalogReader))

		r.Get("/relations", newListRelationsHandler(catalogReader, logger))
	})

	return router, nil
}
