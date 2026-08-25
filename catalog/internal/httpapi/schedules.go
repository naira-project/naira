package httpapi

import (
	"encoding/json"
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

// PUT /v1/{plugin}/schedule replaces one plugin's schedule.
func newSetScheduleHandler(scheduler *scheduling.Scheduler) http.HandlerFunc {
	return handle(func(w http.ResponseWriter, r *http.Request) error {
		var schedule scheduling.Schedule
		if err := json.NewDecoder(r.Body).Decode(&schedule); err != nil {
			return fmt.Errorf("decoding schedule: %w", err)
		}
		schedule.Plugin = chi.URLParam(r, "plugin")
		if err := scheduler.SetSchedule(schedule); err != nil {
			return fmt.Errorf("setting schedule: %w", err)
		}
		updated, err := scheduler.GetSchedule(schedule.Plugin)
		if err != nil {
			return fmt.Errorf("reading updated schedule: %w", err)
		}
		writeJSON(w, http.StatusOK, updated)
		return nil
	})
}
