package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/coordtest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestCoordinatorToolsDoNotActOnTheMainThread(t *testing.T) {
	var out = map[string]string{}
	var errs = map[string]bool{}
	var mainID string
	f, svc := newServiceFixture(t, testHost{}, idleScript, func(c *coordtest.Turn) error {
		if c.CheckIn() {
			c.Reply(noUpdate)
			return nil
		}
		for _, name := range []string{"resolve_thread", "rename_thread", "message_thread"} {
			args := map[string]any{"thread_id": mainID, "title": "New title", "message": "hi"}
			out[name], errs[name] = c.Call(name, args)
		}
		c.Reply("done")
		return nil
	})
	mainID = f.project.MainThreadID
	_, err := svc.Coordinator.Post(f.project.ID, "go", nil, Source{})
	require.NoError(t, err)
	waitMessages(t, f.store, f.project.ID, func(m []db.ThreadEvent) bool { return countKind(m, db.EvAssistant) >= 1 })
	require.Len(t, out, 3, "the scripted turn ran its calls")
	t.Logf("%v", out)
	main, err := f.store.GetThread(mainID)
	require.NoError(t, err)
	for _, name := range []string{"resolve_thread", "rename_thread", "message_thread"} {
		assert.True(t, errs[name], "%s on the main thread must fail, got %q", name, out[name])
	}
	assert.NotEqual(t, db.ThreadResolved, main.Status)
	assert.NotEqual(t, "New title", main.Title)
}
