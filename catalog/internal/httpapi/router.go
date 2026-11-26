package httpapi

import (
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/naira-project/naira/catalog/internal/auth/keycloak"
	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/clusters"
	"github.com/naira-project/naira/catalog/internal/pluginrun"
)

// NewRouter constructs authenticated graph, plugin and cluster management routes.
func NewRouter(catalogService *catalog.Service, runner *pluginrun.Runner, plugins catalog.PluginConfigsByName, logger *log.Logger, kc keycloak.Config, clusterRegistry *clusters.Registry) (http.Handler, error) {
	if clusterRegistry == nil || clusterRegistry.Registrations == nil || clusterRegistry.ConnectionTests == nil {
		return nil, fmt.Errorf("cluster registry with registration and connection test services is required")
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

		r.Get("/auth/verify", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})

		registerWorkspaceRoutes(r, clusterRegistry)

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
