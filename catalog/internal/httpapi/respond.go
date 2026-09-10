package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"time"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/operations"
	"github.com/naira-project/naira/catalog/internal/pluginrun"
)

type routeHandler func(http.ResponseWriter, *http.Request) error
type listOptionsHandler func(http.ResponseWriter, *http.Request, listOptions) error

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
	case errors.Is(err, catalog.ErrNodeNotFound), errors.Is(err, operations.ErrNotFound), errors.Is(err, errPluginResourceNotFound):
		status = http.StatusNotFound
	case errors.Is(err, pluginrun.ErrPluginAlreadyRunning):
		// AIP-151 parallel operations: reject a run while one is in flight.
		status = http.StatusConflict
	case errors.Is(err, pluginrun.ErrInvalidPluginName), errors.Is(err, pluginrun.ErrPluginNotFound):
		status = http.StatusBadRequest
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

func int32FromCount(value int, logger *log.Logger) int32 {
	if value > math.MaxInt32 {
		if logger != nil {
			logger.Printf("count %d exceeds int32 range", value)
		}
		return math.MaxInt32
	}

	return int32(value)
}
