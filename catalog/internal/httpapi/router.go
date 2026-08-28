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
	"github.com/naira-project/naira/catalog/internal/scheduling"
)

// NewRouter wires up the catalog HTTP API.
// catalogService serves read-only graph queries (nodes/relations);
// runner drives and reports on asynchronous plugin runs (plugins/operations).
func NewRouter(catalogService *catalog.Service, runner *pluginrun.Runner, logger *log.Logger, kc keycloak.Config, scheduler *scheduling.Scheduler) (http.Handler, error) {
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

	router.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	router.Route("/v1", func(r chi.Router) {
		r.Use(authMiddleware)

		r.Post("/plugins:run", newRunAllPluginsHandler(runner))
		r.Get("/plugins", newListPluginsHandler(runner))
		r.Get("/schedules", newListSchedulesHandler(scheduler))
		r.Get("/{plugin}/schedule", newGetScheduleHandler(scheduler))
		r.Post("/{plugin}:run", newRunPluginHandler(runner))

		r.Get("/operations", newListOperationsHandler(runner, logger))
		r.Get("/operations/{operationId}", newGetOperationHandler(runner))

		r.Get("/nodes", newListNodesHandler(catalogService, logger))
		r.Get("/nodes/{kind}/*", newGetNodeHandler(catalogService))

		r.Get("/relations", newListRelationsHandler(catalogService, logger))
	})

	return router, nil
}
