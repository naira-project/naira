package httpapi

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/naira-project/naira/catalog/internal/pluginrun"
)

// GET /v1/plugins lists the names of all registered plugins.
func newListPluginsHandler(runner *pluginrun.Runner) http.HandlerFunc {
	return handle(func(w http.ResponseWriter, r *http.Request) error {
		writeJSON(w, http.StatusOK, map[string][]string{"plugins": runner.ListPlugins()})
		return nil
	})
}

// POST /v1/plugins:run asynchronously runs all registered plugins and
// returns the tracking operations (AIP-151).
func newRunAllPluginsHandler(runner *pluginrun.Runner) http.HandlerFunc {
	return handle(func(w http.ResponseWriter, r *http.Request) error {
		ops := runner.RunAllPluginsAsync(r.Context())

		writeJSON(w, http.StatusAccepted, RunPluginsResponse{Operations: toOperationResources(ops)})
		return nil
	})
}

// POST /v1/{plugin}:run asynchronously runs a single plugin and returns the
// tracking operation (AIP-151).
func newRunPluginHandler(runner *pluginrun.Runner) http.HandlerFunc {
	return handle(func(w http.ResponseWriter, r *http.Request) error {
		plugin := chi.URLParam(r, "plugin")
		op, err := runner.RunPluginAsync(r.Context(), plugin)
		if err != nil {
			return fmt.Errorf("running plugin %q: %w", plugin, err)
		}

		writeJSON(w, http.StatusAccepted, operationFromCatalogOperation(op))
		return nil
	})
}
