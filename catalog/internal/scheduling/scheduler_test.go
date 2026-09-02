package scheduling

import (
	"context"
	"io"
	"log"
	"testing"
	"time"

	"github.com/naira-project/naira/catalog/internal/operations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubStarter struct {
	calls chan string
}

func (s *stubStarter) RunPluginAsync(_ context.Context, plugin string) (operations.Operation, error) {
	s.calls <- plugin
	return operations.Operation{Plugin: plugin}, nil
}

func TestConfiguredSchedulerRejectsInvalidExpression(t *testing.T) {
	schedules := map[string]string{"github": "not a cron"}
	starter := &stubStarter{calls: make(chan string)}

	_, err := NewConfiguredScheduler(schedules, starter, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "registering schedule for plugin")
}

func TestSchedulerTriggersConfiguredPlugin(t *testing.T) {
	starter := &stubStarter{calls: make(chan string, 1)}
	schedules := map[string]string{"github": "* * * * *"}

	scheduler, err := NewConfiguredScheduler(schedules, starter, log.New(io.Discard, "", 0))
	require.NoError(t, err)
	defer scheduler.Stop(context.Background())

	// Fetch registered entries directly from the cron instance and trigger the job manually
	entries := scheduler.cron.Entries()
	require.Len(t, entries, 1)
	entries[0].Job.Run()

	select {
	case plugin := <-starter.calls:
		assert.Equal(t, "github", plugin)
	case <-time.After(time.Second):
		t.Fatal("scheduled run was not triggered")
	}
}
