package scheduling

import (
	"context"
	"io"
	"log"
	"testing"
	"time"

	"github.com/naira-project/naira/catalog/internal/catalog"
	"github.com/naira-project/naira/catalog/internal/operations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubStarter struct {
	calls chan string
	err   error
}

func (s *stubStarter) RunPluginAsync(_ context.Context, plugin string) (operations.Operation, error) {
	if s.calls != nil {
		s.calls <- plugin
	}
	return operations.Operation{Plugin: plugin}, s.err
}

func TestNewConfiguredScheduler_Initialization(t *testing.T) {
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
			starter := &stubStarter{calls: make(chan string, 1)}

			config := make(catalog.PluginConfig, len(tt.schedules))
			for plugin, schedule := range tt.schedules {
				config[plugin] = catalog.PluginDefinition{Address: "test", Schedule: schedule}
			}
			scheduler, err := NewConfiguredScheduler(config, starter, log.New(io.Discard, "", 0))

			if tt.wantErr {
				require.Error(t, err)
				if tt.expectedErr != nil {
					assert.ErrorIs(t, err, tt.expectedErr)
				}
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, scheduler.Stop(context.Background()))
			})

			assert.Len(t, scheduler.cron.Entries(), tt.expectedJobs)
		})
	}
}

func TestScheduler_Execution(t *testing.T) {
	starter := &stubStarter{calls: make(chan string, 1)}

	scheduler, err := NewConfiguredScheduler(catalog.PluginConfig{"github": {Address: "test", Schedule: "* * * * *"}}, starter, log.New(io.Discard, "", 0))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, scheduler.Stop(context.Background()))
	})

	entries := scheduler.cron.Entries()
	require.Len(t, entries, 1)

	// trigger the job manually
	entries[0].Job.Run()

	select {
	case plugin := <-starter.calls:
		assert.Equal(t, "github", plugin)
	case <-time.After(time.Second):
		t.Fatal("scheduled run was not triggered")
	}
}
