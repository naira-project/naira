package httpapi

import (
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/naira-project/naira/catalog/internal/auth/keycloak"
	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/pluginrun"
	"github.com/naira-project/naira/catalog/internal/sources"
	"github.com/naira-project/naira/catalog/internal/workspaces"
)

// NewRouter constructs authenticated graph, plugin and workspace management routes.
func NewRouter(catalogService *catalog.Service, runner *pluginrun.Runner, plugins catalog.PluginConfigsByName, logger *log.Logger, kc keycloak.Config, workspaceService *workspaces.Service, sourceService *sources.Service) (http.Handler, error) {
	if workspaceService == nil || sourceService == nil {
		return nil, fmt.Errorf("workspace and source services are required")
	}
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
		registerWorkspaceRoutes(r, workspaceService, sourceService)

		r.Get("/auth/verify", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})

		r.Get("/plugins", newListPluginsHandler(plugins, logger))
		r.Get("/plugins/{plugin}", newGetPluginHandler(plugins))
		r.Post("/plugins:run", newRunAllPluginsHandler(runner))
		r.Post("/plugins/{plugin}:run", newRunPluginHandler(runner))

		r.Get("/operations", newListOperationsHandler(runner, logger))
		r.Get("/operations/{operationId}", newGetOperationHandler(runner))

		r.Get("/nodes", newListNodesHandler(catalogService, logger))
		r.Get("/nodes/{kind}/*", newGetNodeHandler(catalogService))

		r.Get("/relations", newListRelationsHandler(catalogService, logger))
		r.Get("/kinds", newListKindsHandler(catalogService, logger))
	})

	return router, nil
}
