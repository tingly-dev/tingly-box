package project

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/coordtest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestCoordinatorSessionResumesAcrossMessages(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	agent := &enginetest.Agent{Script: func(turn *enginetest.Turn) error { turn.Init("default-model"); turn.Say("done", 1, 1); return nil }}
	svc := NewService(store, Layout{Root: t.TempDir()}, Agents{Service: agent.Service()}, testHost{})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		svc.Threads.Shutdown(ctx)
		_ = svc.CloseTools(ctx)
	})
	p, err := svc.Create(context.Background(), NewProjectSpec{Name: "Project"})
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		_, err := svc.Coordinator.Post(p.ID, "hello", nil, Source{})
		require.NoError(t, err)
		want := i + 1
		require.Eventually(t, func() bool {
			events, _ := store.ListThreadEvents(p.MainThreadID)
			return countKind(events, db.EvAssistant) == want && !svc.Coordinator.Busy(p.ID)
		}, 5*time.Second, 10*time.Millisecond)
	}
	launches := agent.Launches()
	require.Len(t, launches, 2)
	require.False(t, launches[0].Resume)
	require.True(t, launches[1].Resume)
	require.Equal(t, launches[0].SessionID, launches[1].SessionID)
	require.NotEmpty(t, launches[0].MCPConfigPath)
}

func TestCoordinatorTurnsAddUpOnTheMainThread(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	agent := &enginetest.Agent{Script: func(turn *enginetest.Turn) error { turn.Init("default-model"); turn.Say("done", 30, 4); return nil }}
	svc := NewService(store, Layout{Root: t.TempDir()}, Agents{Service: agent.Service()}, testHost{})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		svc.Threads.Shutdown(ctx)
		_ = svc.CloseTools(ctx)
	})
	p, err := svc.Create(context.Background(), NewProjectSpec{Name: "Project"})
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		_, err := svc.Coordinator.Post(p.ID, "hello", nil, Source{})
		require.NoError(t, err)
		want := i + 1
		require.Eventually(t, func() bool {
			events, _ := store.ListThreadEvents(p.MainThreadID)
			return countKind(events, db.EvAssistant) == want && !svc.Coordinator.Busy(p.ID)
		}, 5*time.Second, 10*time.Millisecond)
	}
	main, err := store.GetThread(p.MainThreadID)
	require.NoError(t, err)
	require.Equal(t, 2, main.Turns)
	require.EqualValues(t, 60, main.InputTokens)
	require.EqualValues(t, 8, main.OutputTokens)
	require.EqualValues(t, 30, main.ContextTokens)
	require.NotNil(t, main.LastTurnAt)
	require.Equal(t, "default-model", main.ReportedModel)
}

// A report turn ends with the system's check-in, which carries the reply
// budget the coordinator prompt refers to.
func TestCoordinatorReportTurnCarriesTheReplyBudget(t *testing.T) {
	var seen prompts
	f, svc := newServiceFixture(t, testHost{}, func(turn *enginetest.Turn) error {
		turn.Say("Done.", 1, 1)
		return nil
	}, func(c *coordtest.Turn) error {
		seen.add(c)
		if c.CheckIn() {
			c.Reply(noUpdate)
			return nil
		}
		c.Call("start_thread", map[string]string{"title": "Do it", "task": "do it"})
		c.Reply("Started.")
		return nil
	})
	_, err := svc.Coordinator.Post(f.project.ID, "Please do it.", nil, Source{})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(seen.all()) == 2 && !svc.Coordinator.Busy(f.project.ID) }, 5*time.Second, 10*time.Millisecond)

	turns := seen.all()
	assert.NotContains(t, turns[0], "earlier messages exist", "a short history is shown whole")
	report := strings.TrimSpace(turns[1])
	last := report[strings.LastIndex(report, "<event"):]
	assert.True(t, strings.HasSuffix(report, "</event>"), "the check-in comes last")
	assert.Contains(t, last, `kind="check-in"`)
	assert.Contains(t, last, `trust="system"`)
	assert.Contains(t, last, `replies_since_user="1"`)
	assert.Contains(t, last, `minutes_since_user="0"`)
	assert.Contains(t, report, "Done.", "the report itself precedes it")
}

// A new session is given the latest part of a long conversation and told
// that more can be read with read_conversation.
func TestCoordinatorSessionSaysWhenHistoryIsCut(t *testing.T) {
	var seen prompts
	f, svc := newServiceFixture(t, testHost{}, nil, func(c *coordtest.Turn) error {
		seen.add(c)
		c.Reply("ok")
		return nil
	})
	for i := 0; i < historyLimit+5; i++ {
		require.NoError(t, f.store.AddThreadEvent(&db.ThreadEvent{ThreadID: f.project.MainThreadID, Kind: db.EvUser, Content: fmt.Sprintf("m%03d", i)}))
	}
	_, err := svc.Coordinator.Post(f.project.ID, "And now?", nil, Source{})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(seen.all()) == 1 && !svc.Coordinator.Busy(f.project.ID) }, 5*time.Second, 10*time.Millisecond)

	first := seen.all()[0]
	assert.Contains(t, first, "earlier messages exist")
	assert.Contains(t, first, "read_conversation")
	assert.Contains(t, first, fmt.Sprintf("m%03d", historyLimit+4))
	assert.NotContains(t, first, "m000", "the oldest messages are left to read_conversation")
}
