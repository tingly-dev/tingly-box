package project

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/coordtest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestConfigurationSaveDefersCoordinatorUntilNextMessage(t *testing.T) {
	var seen prompts
	f, svc := newServiceFixture(t, testHost{}, nil, func(c *coordtest.Turn) error {
		seen.add(c)
		c.Reply("ok")
		return nil
	})
	_, err := svc.Coordinator.Post(f.project.ID, "first", nil, Source{})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(seen.all()) == 1 && !svc.Coordinator.Busy(f.project.ID) }, 5*time.Second, time.Millisecond)

	f.project.Instructions = "First edit."
	require.NoError(t, svc.Save(f.project))
	f.project.Instructions = "Final edit."
	f.project.Repos = nil
	require.NoError(t, svc.Save(f.project))
	pending, err := f.store.PendingCoordinatorInputs(f.project.ID)
	require.NoError(t, err)
	assert.Zero(t, pending, "settings must not be queued as coordinator inputs")
	assert.False(t, svc.Coordinator.Busy(f.project.ID), "settings must not wake the coordinator")
	assert.Len(t, seen.all(), 1)

	_, err = svc.Coordinator.Post(f.project.ID, "next", nil, Source{})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(seen.all()) >= 2 && !svc.Coordinator.Busy(f.project.ID) }, 5*time.Second, time.Millisecond)
	turns := seen.all()
	require.Len(t, turns, 2)
	launches := f.agent.Launches()
	require.Len(t, launches, 2)
	assert.Equal(t, launches[0].SessionID, launches[1].SessionID)
	assert.True(t, launches[1].Resume)
	assert.Contains(t, turns[1], "Final edit.")
	assert.Contains(t, turns[1], "no repositories")
	assert.NotContains(t, turns[1], "First edit.")
}

func TestInactiveAndMetadataChangesDoNotScheduleCoordinator(t *testing.T) {
	var seen prompts
	f, svc := newServiceFixture(t, testHost{}, nil, func(c *coordtest.Turn) error {
		seen.add(c)
		return nil
	})
	f.project.Starred = true
	require.NoError(t, svc.Save(f.project))
	require.NoError(t, svc.Save(f.project))
	assert.Empty(t, convo(t, f.store, f.project.ID))
	require.NoError(t, svc.SetPaused(f.project, true))
	f.project.Instructions = "Updated while paused"
	require.NoError(t, svc.Save(f.project))
	require.NoError(t, svc.SetPaused(f.project, false))
	require.Eventually(t, func() bool { return !svc.Coordinator.Busy(f.project.ID) }, 5*time.Second, time.Millisecond)
	assert.Empty(t, seen.all())
	pending, err := f.store.PendingCoordinatorInputs(f.project.ID)
	require.NoError(t, err)
	assert.Zero(t, pending)
	require.Len(t, convo(t, f.store, f.project.ID), 1)
}

func TestReportUsesLatestClearedSettings(t *testing.T) {
	var seen prompts
	f, svc := newServiceFixture(t, testHost{}, nil, func(c *coordtest.Turn) error {
		seen.add(c)
		c.Reply("ok")
		return nil
	})
	_, err := svc.Coordinator.Post(f.project.ID, "first", nil, Source{})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(seen.all()) == 1 && !svc.Coordinator.Busy(f.project.ID) }, 5*time.Second, time.Millisecond)
	f.project.Goal, f.project.Instructions, f.project.Repos = "", "", nil
	require.NoError(t, svc.Save(f.project))
	svc.Coordinator.onReport(Report{Thread: db.Thread{ID: "worker", ProjectID: f.project.ID, Title: "Work"}, Kind: ReportFinished, Text: "done"})
	require.Eventually(t, func() bool { return len(seen.all()) == 2 && !svc.Coordinator.Busy(f.project.ID) }, 5*time.Second, time.Millisecond)
	assert.Contains(t, seen.all()[1], `Goal: ""`)
	assert.Contains(t, seen.all()[1], `Instructions: ""`)
	assert.Contains(t, seen.all()[1], "no repositories")
	assert.Contains(t, seen.all()[1], `kind="check-in"`)
}

func TestLegacySettingsInputsAreNotDelivered(t *testing.T) {
	var seen prompts
	f, svc := newServiceFixture(t, testHost{}, nil, func(c *coordtest.Turn) error {
		seen.add(c)
		c.Reply("ok")
		return nil
	})
	notice := &db.ThreadEvent{ThreadID: f.project.MainThreadID, Kind: db.EvSystem, Source: db.SourceSystem, Content: "old settings notice", Payload: `{"settings_changed":true}`}
	require.NoError(t, f.store.AddCoordinatorInput(f.project.ID, notice))
	_, err := svc.Coordinator.Post(f.project.ID, "real message", nil, Source{})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(seen.all()) == 1 && !svc.Coordinator.Busy(f.project.ID) }, 5*time.Second, time.Millisecond)
	assert.NotContains(t, seen.all()[0], "old settings notice")
	assert.Contains(t, seen.all()[0], "real message")
	pending, err := f.store.PendingCoordinatorInputs(f.project.ID)
	require.NoError(t, err)
	assert.Zero(t, pending)
}

func TestUncertainLegacySettingsInputNeedsRecovery(t *testing.T) {
	var seen prompts
	f, svc := newServiceFixture(t, testHost{}, nil, func(c *coordtest.Turn) error {
		seen.add(c)
		c.Reply("ok")
		return nil
	})
	notice := &db.ThreadEvent{ThreadID: f.project.MainThreadID, Kind: db.EvSystem, Source: db.SourceSystem, Payload: `{"settings_changed":true}`}
	require.NoError(t, f.store.AddCoordinatorInput(f.project.ID, notice))
	input, err := f.store.ClaimCoordinatorInput(f.project.ID)
	require.NoError(t, err)
	require.NoError(t, f.store.RecoverCoordinatorInputs())
	require.Error(t, svc.Coordinator.drainSessionInputs(context.Background(), f.project.ID))
	uncertain, err := f.store.UncertainCoordinatorInput(f.project.ID)
	require.NoError(t, err)
	require.Equal(t, input.ID, uncertain)
	assert.Empty(t, seen.all())
	require.NoError(t, f.store.ResolveCoordinatorInput(f.project.ID, input.ID, true))
	require.NoError(t, svc.Coordinator.drainSessionInputs(context.Background(), f.project.ID))
	assert.Empty(t, seen.all(), "retrying a legacy notice must not run a turn")
}

// A settings change during a turn refuses stale tools, but starts no new turn.
func TestConfigurationChangeDuringResponseRejectsStaleToolsAndUsesNewModel(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var mu sync.Mutex
	var stale coordtest.Call
	var models, turnPrompts []string
	var repo string
	f, svc := newServiceFixture(t, testHost{}, func(turn *enginetest.Turn) error { turn.Say("done", 1, 1); return nil }, func(c *coordtest.Turn) error {
		mu.Lock()
		models = append(models, c.Opts.Model)
		turnPrompts = append(turnPrompts, c.Prompt)
		mu.Unlock()
		if c.Opts.Model == "old-model" {
			close(entered)
			<-release
			c.Call("start_thread", map[string]string{"title": "Old plan", "task": "must not run with stale settings", "repo": repo})
			mu.Lock()
			stale = c.Calls()[0]
			mu.Unlock()
			c.Reply("Settings changed; re-planning.")
			return nil
		}
		c.Call("start_thread", map[string]string{"title": "Updated plan", "task": "continue in the replacement repository"})
		c.Reply("Started.")
		return nil
	})
	repo = f.repo
	coord := svc.Coordinator
	// Registered after the fixture's cleanups, so the turn is released first.
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	f.project.CoordinatorModel = "old-model"
	require.NoError(t, f.store.SaveProject(f.project))
	_, err := coord.Post(f.project.ID, "Continue the migration", nil, Source{})
	require.NoError(t, err)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("coordinator did not start on the old model")
	}
	f.project.CoordinatorModel = "new-model"
	f.project.Instructions = "Use the replacement repository."
	f.project.Repos = []db.Repo{{Path: newRepo(t), BaseBranch: "main"}}
	require.NoError(t, svc.Save(f.project))
	once.Do(func() { close(release) })
	require.Eventually(t, func() bool { return !coord.Busy(f.project.ID) }, 5*time.Second, time.Millisecond)
	mu.Lock()
	count := len(models)
	mu.Unlock()
	require.Equal(t, 1, count, "a settings change must not start a new turn")
	_, err = coord.Post(f.project.ID, "Continue with the new settings", nil, Source{})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(models) >= 2 && !coord.Busy(f.project.ID)
	}, 5*time.Second, time.Millisecond)
	threads := threadsOf(t, f)
	require.Len(t, threads, 1, "only the re-evaluated plan can start work")
	assert.Equal(t, "Updated plan", threads[0].Title)
	assert.Equal(t, f.project.Repos[0].Path, threads[0].RepoPath)

	mu.Lock()
	defer mu.Unlock()
	assert.True(t, stale.IsError, "the stale turn's tool was refused")
	assert.Contains(t, stale.Out, "settings changed")
	assert.Equal(t, "new-model", models[1])
	assert.Contains(t, turnPrompts[1], f.project.Repos[0].Path)
	assert.NotContains(t, turnPrompts[1], f.repo+" (base branch")
	assert.Contains(t, turnPrompts[1], f.project.Instructions)
}
