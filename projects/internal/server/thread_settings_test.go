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
