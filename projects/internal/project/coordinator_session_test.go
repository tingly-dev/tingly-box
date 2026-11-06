package project

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/llm"
)

type coordinatorSessionTestHost struct {
	staticEnv
	*fakeLLM
}

func (coordinatorSessionTestHost) Models(context.Context) ([]llm.Model, error) { return nil, nil }

func TestCoordinatorSessionResumesAcrossMessages(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	agent := &enginetest.Agent{Script: func(turn *enginetest.Turn) error { turn.Init("default-model"); turn.Say("done", 1, 1); return nil }}
	host := coordinatorSessionTestHost{fakeLLM: &fakeLLM{}}
	svc := NewService(store, Layout{Root: t.TempDir()}, Agents{Service: agent.Service()}, host)
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
	svc := NewService(store, Layout{Root: t.TempDir()}, Agents{Service: agent.Service()}, coordinatorSessionTestHost{fakeLLM: &fakeLLM{}})
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

func TestCoordinatorKeepsTheSubscriptionWindowsClaudeReported(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	five := time.Now().Add(2 * time.Hour).Unix()
	agent := &enginetest.Agent{Script: func(turn *enginetest.Turn) error {
		turn.Init("default-model")
		turn.Say("done", 30, 4)
		turn.RateLimit(0.8, five, 0.3, five+3600)
		return nil
	}}
	svc := NewService(store, Layout{Root: t.TempDir()}, Agents{Service: agent.Service()}, coordinatorSessionTestHost{fakeLLM: &fakeLLM{}})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		svc.Threads.Shutdown(ctx)
		_ = svc.CloseTools(ctx)
	})
	p, err := svc.Create(context.Background(), NewProjectSpec{Name: "Project"})
	require.NoError(t, err)
	_, err = svc.Coordinator.Post(p.ID, "hello", nil, Source{})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		events, _ := store.ListThreadEvents(p.MainThreadID)
		return countKind(events, db.EvAssistant) == 1 && !svc.Coordinator.Busy(p.ID)
	}, 5*time.Second, 10*time.Millisecond)
	main, err := store.GetThread(p.MainThreadID)
	require.NoError(t, err)
	require.NotNil(t, main.RateLimits)
	require.Equal(t, 0.8, main.RateLimits.FiveHour.Utilization)
	require.EqualValues(t, five+3600, main.RateLimits.SevenDay.ResetsAt)
}
