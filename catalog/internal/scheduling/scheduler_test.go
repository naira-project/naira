package scheduling

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"
	"time"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/operations"
	"github.com/naira-project/naira/catalog/internal/operations/operationstest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewScheduler_Initialization(t *testing.T) {
	tests := []struct {
		name         string
		schedules    map[string]string
		wantErr      bool
		expectedErr  error
		errContains  string
		expectedJobs int
	}{
		{
			name:        "rejects invalid cron expression",
			schedules:   map[string]string{"github": "not a cron"},
			wantErr:     true,
			errContains: `registering schedule for plugin "github"`,
		},
		{
			name:        "rejects empty plugin name",
			schedules:   map[string]string{"": "* * * * *"},
			wantErr:     true,
			expectedErr: ErrInvalidPlugin,
		},
		{
			name:         "ignores empty expression",
			schedules:    map[string]string{"manual": ""},
			wantErr:      false,
			expectedJobs: 0,
		},
		{
			name:         "registers valid schedule",
			schedules:    map[string]string{"github": "* * * * *"},
			wantErr:      false,
			expectedJobs: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			noopRunFunc := func(_ context.Context, plugin string) (operations.Operation, error) {
				return operations.Operation{Plugin: plugin}, nil
			}

			config := make(catalog.PluginConfigsByName, len(tt.schedules))
			for plugin, schedule := range tt.schedules {
				config[plugin] = catalog.PluginConfig{Address: "test", Schedule: schedule}
			}
			scheduler, err := NewScheduler(t.Context(), config, operationstest.NewMemoryStore(), noopRunFunc, log.New(io.Discard, "", 0))

			if tt.wantErr {
				require.Error(t, err)
				if tt.expectedErr != nil {
					assert.ErrorIs(t, err, tt.expectedErr)
				}
				assert.ErrorContains(t, err, tt.errContains)
				return
			}

			require.NoError(t, err)
			assert.Len(t, scheduler.cron.Entries(), tt.expectedJobs)
		})
	}
}

func TestScheduler_Start_ResyncsInterruptedPlugin(t *testing.T) {
	store := operationstest.NewMemoryStore()
	require.NoError(t, store.Create(t.Context(), operations.Operation{
		Name:      "previous-run",
		Plugin:    "example",
		State:     operations.StateInterrupted,
		CreatedAt: time.Now(),
	}))

	var ran []string
	runFunc := func(_ context.Context, plugin string) (operations.Operation, error) {
		ran = append(ran, plugin)
		return operations.Operation{}, nil
	}

	scheduler, err := NewScheduler(
		t.Context(),
		catalog.PluginConfigsByName{"example": {Address: "test", Schedule: catalog.ScheduleManual}},
		store,
		runFunc,
		log.New(io.Discard, "", 0),
	)
	require.NoError(t, err)

	scheduler.Start()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, scheduler.Stop(ctx))
	})

	assert.Equal(t, []string{"example"}, ran)
}

func TestScheduler_Execution(t *testing.T) {
	calls := make(chan string, 1)

	runFunc := func(_ context.Context, plugin string) (operations.Operation, error) {
		calls <- plugin
		return operations.Operation{Plugin: plugin}, nil
	}

	scheduler, err := NewScheduler(
		t.Context(),
		catalog.PluginConfigsByName{"github": {Address: "test", Schedule: "* * * * *"}},
		operationstest.NewMemoryStore(),
		runFunc,
		log.New(io.Discard, "", 0),
	)
	require.NoError(t, err)

	entries := scheduler.cron.Entries()
	require.Len(t, entries, 1)

	// trigger the job manually, without starting the cron or the startup resync
	entries[0].Job.Run()

	select {
	case plugin := <-calls:
		assert.Equal(t, "github", plugin)
	case <-time.After(time.Second):
		t.Fatal("scheduled run was not triggered")
	}
}

func TestNeedsResync(t *testing.T) {
	// robfig/cron evaluates schedules in local time, so build test instants in
	// the same location to make schedule boundary cases deterministic.
	now := time.Date(2025, time.January, 15, 12, 0, 0, 0, time.Local)
	todayTick := time.Date(2025, time.January, 15, 0, 0, 0, 0, time.Local)
	previousTick := time.Date(2025, time.January, 14, 0, 0, 0, 0, time.Local)
	lastHour := now.Add(-time.Hour)
	lastDay := now.Add(-24 * time.Hour)
	yearAgo := time.Date(2024, time.January, 2, 0, 0, 0, 0, time.Local)
	invalidSchedule := "not a cron expression"

	tests := []struct {
		name       string
		operation  *operations.Operation
		schedule   string
		wantResync bool
		wantError  bool
	}{
		{
			name:       "no previous operation bootstraps plugin",
			schedule:   catalog.ScheduleManual,
			wantResync: true,
		},
		{
			name:       "interrupted operation is retried even for manual schedule",
			operation:  &operations.Operation{State: operations.StateInterrupted},
			schedule:   catalog.ScheduleManual,
			wantResync: true,
		},
		{
			name:      "pending operation is left alone",
			operation: &operations.Operation{State: operations.StatePending},
			schedule:  "0 0 * * *", // every day at 00:00
		},
		{
			name:      "running operation is left alone",
			operation: &operations.Operation{State: operations.StateRunning},
			schedule:  "0 0 * * *",
		},
		{
			name:      "unknown operation state is left alone",
			operation: &operations.Operation{State: "UNKNOWN"},
			schedule:  "0 0 * * *",
		},
		{
			name:      "successful manual operation is not rerun",
			operation: &operations.Operation{State: operations.StateSucceeded, CreatedAt: lastDay},
			schedule:  catalog.ScheduleManual,
		},
		{
			name:      "failed manual operation is not retried automatically",
			operation: &operations.Operation{State: operations.StateFailed, CreatedAt: lastDay},
			schedule:  catalog.ScheduleManual,
		},
		{
			name:       "missed scheduled success triggers catch-up",
			operation:  &operations.Operation{State: operations.StateSucceeded, CreatedAt: previousTick},
			schedule:   "0 0 * * *",
			wantResync: true,
		},
		{
			name:       "missed scheduled failure triggers catch-up",
			operation:  &operations.Operation{State: operations.StateFailed, CreatedAt: previousTick},
			schedule:   "0 0 * * *",
			wantResync: true,
		},
		{
			name:      "success exactly at latest tick does not trigger catch-up",
			operation: &operations.Operation{State: operations.StateSucceeded, CreatedAt: todayTick},
			schedule:  "0 0 * * *",
		},
		{
			name:      "success after latest tick does not trigger catch-up",
			operation: &operations.Operation{State: operations.StateSucceeded, CreatedAt: lastHour},
			schedule:  "0 0 * * *",
		},
		{
			name:      "completion time takes precedence over creation time",
			operation: &operations.Operation{State: operations.StateSucceeded, CreatedAt: previousTick, EndTime: &lastHour},
			schedule:  "0 0 * * *",
		},
		{
			name:       "creation time is used when completion time is absent",
			operation:  &operations.Operation{State: operations.StateSucceeded, CreatedAt: previousTick},
			schedule:   "0 0 * * *",
			wantResync: true,
		},
		{
			name:       "missed annual tick is not limited by a lookback window",
			operation:  &operations.Operation{State: operations.StateSucceeded, CreatedAt: yearAgo},
			schedule:   "0 0 1 1 *", // every year on January 1st
			wantResync: true,
		},
		{
			name:      "invalid schedule returns an error",
			operation: &operations.Operation{State: operations.StateSucceeded, CreatedAt: lastDay},
			schedule:  invalidSchedule,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, err := needsResync(tt.operation, tt.schedule, now)
			if tt.wantError {
				require.Error(t, err)
				assert.Empty(t, reason)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantResync, reason != "")
		})
	}
}

type startupStore struct {
	*operationstest.MemoryStore
	listErrors map[string]error
}

func (s startupStore) List(ctx context.Context, filter operations.Filter) ([]operations.Operation, error) {
	if err := s.listErrors[filter.Plugin]; err != nil {
		return nil, err
	}
	return s.MemoryStore.List(ctx, filter)
}

func TestResyncAtStartup_SkipsListFailuresContinuesAfterRunFailureAndSortsPlugins(t *testing.T) {
	store := startupStore{
		MemoryStore: operationstest.NewMemoryStore(),
		listErrors:  map[string]error{"a": errors.New("database unavailable")},
	}

	var gotRuns []string
	runFunc := func(_ context.Context, plugin string) (operations.Operation, error) {
		gotRuns = append(gotRuns, plugin)
		if plugin == "b" {
			return operations.Operation{}, errors.New("plugin admission rejected")
		}
		return operations.Operation{}, nil
	}

	resyncAtStartup(
		t.Context(),
		catalog.PluginConfigsByName{
			"c": {Schedule: catalog.ScheduleManual},
			"a": {Schedule: catalog.ScheduleManual},
			"b": {Schedule: catalog.ScheduleManual},
		},
		store,
		runFunc,
		time.Now(),
		nil,
	)

	assert.Equal(t, []string{"b", "c"}, gotRuns)
}
