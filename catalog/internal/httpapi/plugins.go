package httpapi

import (
	"errors"
	"fmt"
	"log"
	"maps"
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/pluginrun"
)

var errPluginResourceNotFound = errors.New("plugin resource not found")

type PluginResource struct {
	Name     string `json:"name"`
	Schedule string `json:"schedule"`
}

type ListPluginsResponse struct {
	Plugins       []PluginResource `json:"plugins"`
	NextPageToken string           `json:"nextPageToken,omitempty"`
	TotalSize     int32            `json:"totalSize"`
}

var pluginListOptionsSpec = listOptionsSpec{
	scope:         "plugins",
	allowedFields: map[string]bool{},
}

func newPluginResource(name string, config catalog.PluginConfig) PluginResource {
	return PluginResource{Name: name, Schedule: config.Schedule}
}

// newListPluginsHandler lists configured plugin resources and their schedules.
func newListPluginsHandler(configs catalog.PluginConfigsByName, logger *log.Logger) http.HandlerFunc {
	return handleWithListOptions(pluginListOptionsSpec, func(w http.ResponseWriter, r *http.Request, options listOptions) error {
		resources := make([]PluginResource, 0, len(configs))
		for _, name := range slices.Sorted(maps.Keys(configs)) {
			resources = append(resources, newPluginResource(name, configs[name]))
		}

		page, nextPageToken, totalSize, err := paginate(resources, options.pageSize, options.offset, "plugins", logger)
		if err != nil {
			return fmt.Errorf("paginating plugins: %w", err)
		}
		writeJSON(w, http.StatusOK, ListPluginsResponse{
			Plugins:       page,
			NextPageToken: nextPageToken,
			TotalSize:     int32FromCount(totalSize, logger),
		})
		return nil
	})
}

// newGetPluginHandler returns one configured plugin resource.
func newGetPluginHandler(configs catalog.PluginConfigsByName) http.HandlerFunc {
	return handle(func(w http.ResponseWriter, r *http.Request) error {
		plugin := chi.URLParam(r, "plugin")
		config, ok := configs[plugin]
		if !ok {
			return fmt.Errorf("getting plugin %q: %w", plugin, errPluginResourceNotFound)
		}
		writeJSON(w, http.StatusOK, newPluginResource(plugin, config))
		return nil
	})
}

// newRunAllPluginsHandler asynchronously runs all registered plugins and
// returns the tracking operations (AIP-151).
func newRunAllPluginsHandler(runner *pluginrun.Runner) http.HandlerFunc {
	return handle(func(w http.ResponseWriter, r *http.Request) error {
		ops := runner.RunAllPluginsAsync(r.Context())

		writeJSON(w, http.StatusAccepted, RunPluginsResponse{Operations: toOperationResources(ops)})
		return nil
	})
}

// newRunPluginHandler asynchronously runs a single plugin and
// returns the tracking operation (AIP-151).
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
