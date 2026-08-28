package httpapi

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/naira-project/naira/catalog/internal/scheduling"
)

// GET /v1/schedules lists the effective schedules known to the catalog.
func newListSchedulesHandler(scheduler *scheduling.Scheduler) http.HandlerFunc {
	return handle(func(w http.ResponseWriter, r *http.Request) error {
		schedules, err := scheduler.ListSchedules()
		if err != nil {
			return fmt.Errorf("listing schedules: %w", err)
		}
		writeJSON(w, http.StatusOK, map[string][]scheduling.Schedule{"schedules": schedules})
		return nil
	})
}

// GET /v1/{plugin}/schedule returns one plugin's effective schedule.
func newGetScheduleHandler(scheduler *scheduling.Scheduler) http.HandlerFunc {
	return handle(func(w http.ResponseWriter, r *http.Request) error {
		schedule, err := scheduler.GetSchedule(chi.URLParam(r, "plugin"))
		if err != nil {
			return fmt.Errorf("getting schedule: %w", err)
		}
		writeJSON(w, http.StatusOK, schedule)
		return nil
	})
}
