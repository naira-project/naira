package httpapi

import (
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/naira-project/naira/catalog/internal/scheduling"
)

type ListSchedulesResponse struct {
	Schedules     []scheduling.Schedule `json:"schedules"`
	NextPageToken string                `json:"nextPageToken,omitempty"`
	TotalSize     int32                 `json:"totalSize"`
}

var scheduleListOptionsSpec = listOptionsSpec{
	scope:         "schedules",
	allowedFields: map[string]bool{},
}

func sortSchedules(schedules []scheduling.Schedule) {
	sortResources(schedules, func(schedule scheduling.Schedule) string {
		return schedule.Plugin
	})
}

// GET /v1/schedules lists effective schedules known to the catalog.
// Supported query params:
// - pageSize
// - pageToken
func newListSchedulesHandler(scheduler *scheduling.Scheduler, logger *log.Logger) http.HandlerFunc {
	return handleWithListOptions(scheduleListOptionsSpec, func(w http.ResponseWriter, r *http.Request, options listOptions) error {
		schedules, err := scheduler.ListSchedules()
		if err != nil {
			return fmt.Errorf("listing schedules: %w", err)
		}

		page, nextPageToken, totalSize, err := paginate(schedules, options.pageSize, options.offset, "schedules", logger)
		if err != nil {
			return fmt.Errorf("paginating schedules: %w", err)
		}

		writeJSON(w, http.StatusOK, ListSchedulesResponse{
			Schedules: page, NextPageToken: nextPageToken, TotalSize: int32FromCount(totalSize, logger),
		})
		return nil
	})
}

// GET /v1/{plugin}/schedule returns a single plugin's effective schedule.
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
