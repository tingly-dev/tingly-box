package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestWorkerSettingsAreThreadScopedAndReadableAfterRefresh(t *testing.T) {
	store, c := newServer(t, quietAgents())
	var p db.Project
	require.Equal(t, 201, c.do("POST", "/projects", map[string]any{"name": "P", "thread_model": "project-model"}, &p))
	first := &db.Thread{ProjectID: p.ID, ParentID: p.MainThreadID, Agent: db.AgentClaudeCode, Status: db.ThreadIdle, Model: "last-observed"}
	second := &db.Thread{ProjectID: p.ID, ParentID: p.MainThreadID, Agent: db.AgentClaudeCode, Status: db.ThreadIdle}
	require.NoError(t, store.CreateThread(first))
	require.NoError(t, store.CreateThread(second))
	var out db.Thread
	require.Equal(t, 200, c.do("PUT", "/threads/"+first.ID+"/model", map[string]any{"model": "custom-model"}, &out))
	assert.Equal(t, "custom-model", out.ModelOverride)
	assert.Equal(t, "last-observed", out.Model)
	require.Equal(t, 200, c.do("PUT", "/threads/"+first.ID+"/effort", map[string]any{"effort": "high"}, &out))
	assert.Equal(t, "high", out.EffortOverride)
	assert.Equal(t, 400, c.do("PUT", "/threads/"+first.ID+"/effort", map[string]any{"effort": "invalid"}, &out))
	require.Equal(t, 200, c.do("PUT", "/threads/"+first.ID+"/permission-mode", map[string]any{"permission_mode": "plan"}, &out))
	var detail struct {
		Thread db.Thread `json:"thread"`
	}
	require.Equal(t, 200, c.do("GET", "/threads/"+first.ID, nil, &detail))
	assert.Equal(t, "custom-model", detail.Thread.ModelOverride)
	assert.Equal(t, "plan", detail.Thread.PermissionMode)
	assert.Equal(t, "high", detail.Thread.EffortOverride)
	sibling, err := store.GetThread(second.ID)
	require.NoError(t, err)
	assert.Empty(t, sibling.ModelOverride)
	assert.Empty(t, sibling.EffortOverride)
	assert.Empty(t, sibling.PermissionMode)
	savedProject, err := store.GetProject(p.ID)
	require.NoError(t, err)
	assert.Equal(t, p.ThreadModel, savedProject.ThreadModel)
	assert.Equal(t, p.ThreadPermissionMode, savedProject.ThreadPermissionMode)
	out = db.Thread{}
	require.Equal(t, 200, c.do("PUT", "/threads/"+first.ID+"/model", map[string]any{"model": ""}, &out))
	assert.Empty(t, out.ModelOverride)
	assert.Equal(t, "last-observed", out.Model)
}

func TestProjectRunningLimitIsValidatedAndStored(t *testing.T) {
	store, c := newServer(t, quietAgents())
	var p db.Project
	require.Equal(t, 201, c.do("POST", "/projects", map[string]any{"name": "P"}, &p))
	assert.Equal(t, db.DefaultMaxRunningThreads, p.MaxRunningThreads, "a new project starts at the default")

	for _, n := range []int{2, 100, 1} {
		var out db.Project
		require.Equal(t, 200, c.do("PATCH", "/projects/"+p.ID, map[string]any{"max_running_threads": n}, &out), n)
		assert.Equal(t, n, out.MaxRunningThreads)
		saved, err := store.GetProject(p.ID)
		require.NoError(t, err)
		assert.Equal(t, n, saved.MaxRunningThreads)
	}
	require.Equal(t, 200, c.do("PATCH", "/projects/"+p.ID, map[string]any{"max_running_threads": 5}, nil))
	for _, bad := range []any{0, -1, 101, 1000000, "many", 1.5} {
		assert.Equal(t, 400, c.do("PATCH", "/projects/"+p.ID, map[string]any{"max_running_threads": bad}, nil), bad)
	}
	saved, err := store.GetProject(p.ID)
	require.NoError(t, err)
	assert.Equal(t, 5, saved.MaxRunningThreads, "a rejected value changes nothing")

	var rtime struct {
		Limit int `json:"max_running_threads_limit"`
	}
	require.Equal(t, 200, c.do("GET", "/projects/runtime", nil, &rtime))
	assert.Equal(t, 100, rtime.Limit)
}

func TestFolderHasNoRunningLimitSetting(t *testing.T) {
	_, c := newServer(t, quietAgents())
	var f db.Project
	require.Equal(t, 200, c.do("POST", "/projects", map[string]any{"type": "folder", "path": t.TempDir()}, &f))
	assert.Equal(t, 400, c.do("PATCH", "/projects/"+f.ID, map[string]any{"max_running_threads": 2}, nil))
}
