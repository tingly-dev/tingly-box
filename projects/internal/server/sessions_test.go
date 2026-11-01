package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/internal/project"
)

func TestFolderThreadEndToEnd(t *testing.T) {
	a := &enginetest.Agent{Script: func(turn *enginetest.Turn) error {
		turn.Say("Here is the answer.", 5, 2)
		return nil
	}}
	_, c := newServer(t, &project.Agents{Service: a.Service()})
	folder := t.TempDir()

	var e map[string]any
	assert.Equal(t, 400, c.do("POST", "/projects", map[string]any{"type": "folder", "path": "/no/such/dir"}, &e))
	assert.Contains(t, e["error"], "not a folder")
	assert.Equal(t, 400, c.do("POST", "/projects", map[string]any{"type": "tree", "name": "x"}, &e))
	assert.Equal(t, 404, c.do("PUT", "/threads/x/permission-mode", map[string]any{"permission_mode": "plan"}, &e))

	// Opening a folder returns its project; opening it again returns the same one.
	var f db.Project
	require.Equal(t, 200, c.do("POST", "/projects", map[string]any{"type": "folder", "path": folder}, &f))
	assert.Equal(t, db.ProjectFolder, f.Type)
	assert.Equal(t, folder, f.Path)
	assert.Empty(t, f.MainThreadID)
	var again db.Project
	require.Equal(t, 200, c.do("POST", "/projects", map[string]any{"type": "folder", "path": folder + "/"}, &again))
	assert.Equal(t, f.ID, again.ID)

	var s db.Thread
	require.Equal(t, 201, c.do("POST", "/projects/"+f.ID+"/threads", map[string]any{"content": "explain this folder", "permission_mode": "plan", "effort": "high"}, &s))
	assert.Equal(t, f.ID, s.ProjectID)
	assert.Equal(t, folder, s.Cwd)

	var detail struct {
		Thread          db.Thread `json:"thread"`
		Running         bool      `json:"running"`
		BackgroundTasks []any     `json:"background_tasks"`
	}
	require.Eventually(t, func() bool {
		c.do("GET", "/threads/"+s.ID, nil, &detail)
		return detail.Thread.Status == db.ThreadIdle
	}, 5*time.Second, 20*time.Millisecond)
	assert.Equal(t, "plan", detail.Thread.PermissionMode)
	assert.Equal(t, "high", detail.Thread.EffortOverride)
	assert.Equal(t, "high", a.Launches()[0].Effort)
	assert.NotNil(t, detail.BackgroundTasks)
	var evs struct {
		Events []db.ThreadEvent `json:"events"`
	}
	c.do("GET", "/threads/"+s.ID+"/events", nil, &evs)
	assert.NotEmpty(t, evs.Events)

	// The folder is listed with the projects, with its thread counts.
	var all struct {
		Projects []struct {
			db.Project
			Threads map[string]int `json:"threads"`
		} `json:"projects"`
	}
	c.do("GET", "/projects", nil, &all)
	require.Len(t, all.Projects, 1)
	assert.Equal(t, db.ProjectFolder, all.Projects[0].Type)
	assert.Equal(t, 1, all.Projects[0].Threads["idle"])
	var threads struct {
		Threads []db.Thread `json:"threads"`
	}
	c.do("GET", "/projects/"+f.ID+"/threads", nil, &threads)
	require.Len(t, threads.Threads, 1)

	var modes struct {
		Modes []string `json:"modes"`
	}
	c.do("GET", "/threads/permission-modes", nil, &modes)
	assert.Contains(t, modes.Modes, "acceptEdits")

	var out db.Thread
	require.Equal(t, 200, c.do("PUT", "/threads/"+s.ID+"/permission-mode", map[string]any{"permission_mode": "default"}, &out))
	assert.Equal(t, "default", out.PermissionMode)
	assert.Equal(t, 400, c.do("PUT", "/threads/"+s.ID+"/permission-mode", map[string]any{"permission_mode": "yolo"}, &e))
	// The standalone server has no launch profiles; a model is free text.
	assert.Equal(t, 400, c.do("PUT", "/threads/"+s.ID+"/profile", map[string]any{"profile": "work"}, &e))
	require.Equal(t, 200, c.do("PUT", "/threads/"+s.ID+"/model", map[string]any{"model": "haiku"}, &out))
	assert.Equal(t, "haiku", out.Model)
	var handoff struct {
		Command string `json:"command"`
	}
	require.Equal(t, 200, c.do("POST", "/threads/"+s.ID+"/handoff", nil, &handoff))
	assert.Contains(t, handoff.Command, "claude --resume "+s.SessionID)
	assert.Equal(t, 409, c.do("POST", "/threads/"+s.ID+"/tasks/task1/stop", nil, &e))
	assert.Equal(t, 400, c.do("GET", "/threads/"+s.ID+"/tasks/bad.id/output", nil, &e))
	assert.Equal(t, 200, c.do("POST", "/threads/"+s.ID+"/resolve", nil, &out))
	assert.Equal(t, db.ThreadResolved, out.Status)
	assert.Equal(t, 409, c.do("POST", "/threads/"+s.ID+"/messages", map[string]any{"content": "late"}, &e))
	assert.Equal(t, 200, c.do("POST", "/threads/"+s.ID+"/reopen", nil, &out))
	assert.Equal(t, 404, c.do("GET", "/threads/missing", nil, &e))

	// Only folders take threads from the web; a project's are the coordinator's.
	var p db.Project
	require.Equal(t, 201, c.do("POST", "/projects", map[string]any{"name": "P"}, &p))
	assert.Equal(t, 400, c.do("POST", "/projects/"+p.ID+"/threads", map[string]any{"content": "x"}, &e))
	// A main thread is not an agent session: nothing to stop, but it can be read.
	assert.Equal(t, 200, c.do("GET", "/threads/"+p.MainThreadID, nil, &detail))
	assert.Equal(t, db.AgentLLM, detail.Thread.Agent)

	// Archiving a folder is how it goes away; DELETE does the same.
	assert.Equal(t, 200, c.do("DELETE", "/projects/"+f.ID, nil, nil))
	var archived struct {
		Projects []db.Project `json:"projects"`
	}
	c.do("GET", "/projects?archived=true", nil, &archived)
	require.Len(t, archived.Projects, 1)
	assert.Equal(t, f.ID, archived.Projects[0].ID)
	assert.Equal(t, 409, c.do("POST", "/threads/"+s.ID+"/messages", map[string]any{"content": "after archive"}, &e))
}

func TestFolderSessionArchiveAndResolveAreIndependent(t *testing.T) {
	a := &enginetest.Agent{}
	store, c := newServer(t, &project.Agents{Service: a.Service()})
	f := db.Project{Type: db.ProjectFolder, Path: t.TempDir(), Name: "folder"}
	require.NoError(t, store.CreateProject(&f))
	s := db.Thread{ProjectID: f.ID, Agent: db.AgentClaudeCode, Status: db.ThreadWaiting, WaitReason: "stale request", Cwd: f.Path, SessionID: "test-session"}
	require.NoError(t, store.CreateThread(&s))

	var out db.Thread
	require.Equal(t, 200, c.do("POST", "/threads/"+s.ID+"/archive", nil, &out))
	require.NotNil(t, out.ArchivedAt)
	assert.Equal(t, db.ThreadIdle, out.Status)
	assert.Empty(t, out.WaitReason)
	assert.Nil(t, out.ResolvedAt)
	counts, err := store.ProjectThreadCounts(f.ID)
	require.NoError(t, err)
	assert.Equal(t, db.ThreadCounts{}, counts)
	var failure map[string]any
	assert.Equal(t, 409, c.do("POST", "/threads/"+s.ID+"/messages", map[string]any{"content": "hello"}, &failure))
	var list struct {
		Threads []db.Thread `json:"threads"`
	}
	require.Equal(t, 200, c.do("GET", "/projects/"+f.ID+"/threads", nil, &list))
	require.Len(t, list.Threads, 1)
	assert.NotNil(t, list.Threads[0].ArchivedAt)

	out = db.Thread{}
	require.Equal(t, 200, c.do("POST", "/threads/"+s.ID+"/restore", nil, &out))
	assert.Nil(t, out.ArchivedAt)
	assert.Equal(t, db.ThreadIdle, out.Status)
	require.Equal(t, 200, c.do("POST", "/threads/"+s.ID+"/resolve", nil, &out))
	assert.Equal(t, db.ThreadResolved, out.Status)
	require.Equal(t, 200, c.do("POST", "/threads/"+s.ID+"/archive", nil, &out))
	assert.Equal(t, db.ThreadResolved, out.Status)
	assert.NotNil(t, out.ArchivedAt)
	out = db.Thread{}
	require.Equal(t, 200, c.do("POST", "/threads/"+s.ID+"/restore", nil, &out))
	assert.Nil(t, out.ArchivedAt)
	assert.Equal(t, db.ThreadResolved, out.Status)
	assert.NotNil(t, out.ResolvedAt)
}
