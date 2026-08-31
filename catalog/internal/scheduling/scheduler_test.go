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

func TestMemoryStoreRoundTrip(t *testing.T) {
	store := NewMemoryStore()
	schedule := Schedule{Plugin: " GitHub ", Expression: "*/5 * * * *", Enabled: true}

	require.NoError(t, store.Upsert(schedule))
	got, err := store.Get("github")
	require.NoError(t, err)
	assert.Equal(t, "github", got.Plugin)
	assert.Equal(t, schedule.Expression, got.Expression)
	assert.False(t, got.UpdatedAt.IsZero())
}

func TestConfiguredSchedulerRejectsInvalidExpression(t *testing.T) {
	scheduler := newScheduler(NewMemoryStore(), &stubStarter{calls: make(chan string)}, nil)

	err := scheduler.configureSchedule(Schedule{Plugin: "github", Expression: "not a cron", Enabled: true})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid schedule expression")
}

func TestSchedulerTriggersConfiguredPlugin(t *testing.T) {
	starter := &stubStarter{calls: make(chan string, 1)}
	scheduler := newScheduler(NewMemoryStore(), starter, log.New(io.Discard, "", 0))
	require.NoError(t, scheduler.configureSchedule(Schedule{Plugin: "github", Expression: "* * * * *", Enabled: true}))
	require.NoError(t, scheduler.start())
	defer scheduler.Stop()

	// Cron schedules are minute based; the callback is verified by exercising
	// the registered cron entry directly through the scheduler's entry table.
	scheduler.mu.Lock()
	entryID := scheduler.entries["github"]
	scheduler.mu.Unlock()
	scheduler.cron.Entry(entryID).Job.Run()

	select {
	case plugin := <-starter.calls:
		assert.Equal(t, "github", plugin)
	case <-time.After(time.Second):
		t.Fatal("scheduled run was not triggered")
	}
}
