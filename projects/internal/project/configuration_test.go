package project

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/coordtest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestConfigurationSaveNotifiesCoordinatorWithoutMetadataWakeups(t *testing.T) {
	var seen prompts
	var mu sync.Mutex
	var inputs []string
	f, svc := newServiceFixture(t, testHost{}, func(turn *enginetest.Turn) error { return nil }, func(c *coordtest.Turn) error {
		seen.add(c)
		mu.Lock()
		inputs = append(inputs, c.Input())
		mu.Unlock()
		c.Reply(noUpdate)
		return nil
	})
	coord := svc.Coordinator

	f.project.Starred = true
	require.NoError(t, svc.Save(f.project))
	require.NoError(t, svc.Save(f.project))
	assert.Empty(t, seen.all())
	assert.Empty(t, convo(t, f.store, f.project.ID))

	f.project.Instructions = "Use the new workflow."
	f.project.Repos = nil
	require.NoError(t, svc.Save(f.project))
	require.Eventually(t, func() bool { return !coord.Busy(f.project.ID) }, 5*time.Second, time.Millisecond)
	turns := seen.all()
	require.Len(t, turns, 1)
	assert.Contains(t, turns[0], "Use the new workflow.")
	assert.Contains(t, turns[0], "no repositories")
	mu.Lock()
	assert.Contains(t, inputs[0], "instructions, repositories")
	mu.Unlock()
	assert.Contains(t, turns[0], "Continue previously requested work")
	assert.Contains(t, turns[0], `kind="settings-changed"`)
	events := convo(t, f.store, f.project.ID)
	require.Len(t, events, 1, "NO_UPDATE stays silent")
	assert.Equal(t, db.EvSystem, events[0].Kind)
	assert.Equal(t, db.SourceSystem, events[0].Source)

	require.NoError(t, svc.Save(f.project))
	assert.Len(t, seen.all(), 1, "a no-op edit must not wake the coordinator")
	assert.Len(t, convo(t, f.store, f.project.ID), 1)

	f.project.Paused = true
	f.project.Goal = "A new goal"
	require.NoError(t, svc.Save(f.project))
	assert.False(t, coord.Busy(f.project.ID))
	assert.Len(t, seen.all(), 1, "paused projects record settings without running")
	assert.Len(t, convo(t, f.store, f.project.ID), 2)
}

// A settings change while a turn runs: the turn's tools are refused, and the
// settings-changed turn runs on the new model with the new context.
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
		if c.SettingsChanged() {
			c.Call("start_thread", map[string]string{"title": "Updated plan", "task": "continue in the replacement repository"})
		}
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
