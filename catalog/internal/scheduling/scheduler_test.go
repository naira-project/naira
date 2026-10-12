package scheduling

import (
	"context"
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
