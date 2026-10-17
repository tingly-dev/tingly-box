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

func TestFolderSessionEndToEnd(t *testing.T) {
	a := &enginetest.Agent{Script: func(turn *enginetest.Turn) error {
		turn.Say("Here is the answer.", 5, 2)
		return nil
	}}
	_, c := newServer(t, &project.Agents{Service: a.Service()})
	folder := t.TempDir()

	var e map[string]any
	assert.Equal(t, 400, c.do("POST", "/projects/sessions", map[string]any{"folder": "/no/such/dir", "content": "hi"}, &e))
	assert.Contains(t, e["error"], "not a folder")
	assert.Equal(t, 404, c.do("PUT", "/projects/sessions/x/permission-mode", map[string]any{"permission_mode": "plan"}, &e))

	var s db.Thread
	require.Equal(t, 200, c.do("POST", "/projects/sessions", map[string]any{"folder": folder, "content": "explain this folder", "permission_mode": "plan"}, &s))
	assert.Equal(t, "", s.ProjectID)

	var detail SessionDetail
	require.Eventually(t, func() bool {
		c.do("GET", "/projects/sessions/"+s.ID, nil, &detail)
		return detail.Session.Status == db.ThreadIdle
	}, 5*time.Second, 20*time.Millisecond)
	assert.Equal(t, "plan", detail.Session.PermissionMode)
	assert.NotEmpty(t, detail.Events)
	assert.NotNil(t, detail.BackgroundTasks)

	var list struct {
		Sessions []db.Thread `json:"sessions"`
	}
	c.do("GET", "/projects/sessions?folder="+folder, nil, &list)
	require.Len(t, list.Sessions, 1)
	c.do("GET", "/projects/sessions?folder=/elsewhere", nil, &list)
	assert.Empty(t, list.Sessions)

	var wd struct {
		Workdirs []struct {
			Path  string `json:"path"`
			Saved bool   `json:"saved"`
		} `json:"workdirs"`
	}
	c.do("GET", "/projects/workdirs", nil, &wd)
	require.Len(t, wd.Workdirs, 1)
	assert.Equal(t, folder, wd.Workdirs[0].Path)
	other := t.TempDir()
	require.Equal(t, 200, c.do("POST", "/projects/workdirs", map[string]any{"path": other}, nil))
	assert.Equal(t, 400, c.do("POST", "/projects/workdirs", map[string]any{"path": other + "/missing"}, &e))
	c.do("GET", "/projects/workdirs", nil, &wd)
	assert.Len(t, wd.Workdirs, 2)
	assert.Equal(t, 204, c.do("DELETE", "/projects/workdirs?path="+other, nil, nil))
	c.do("GET", "/projects/workdirs", nil, &wd)
	assert.Len(t, wd.Workdirs, 1)

	var modes struct {
		Modes []string `json:"modes"`
	}
	c.do("GET", "/projects/permission-modes", nil, &modes)
	assert.Contains(t, modes.Modes, "acceptEdits")

	var out db.Thread
	require.Equal(t, 200, c.do("PUT", "/projects/sessions/"+s.ID+"/permission-mode", map[string]any{"permission_mode": "default"}, &out))
	assert.Equal(t, "default", out.PermissionMode)
	assert.Equal(t, 400, c.do("PUT", "/projects/sessions/"+s.ID+"/permission-mode", map[string]any{"permission_mode": "yolo"}, &e))
	// The standalone server has no launch profiles; a model is free text.
	assert.Equal(t, 400, c.do("PUT", "/projects/sessions/"+s.ID+"/profile", map[string]any{"profile": "work"}, &e))
	require.Equal(t, 200, c.do("PUT", "/projects/sessions/"+s.ID+"/model", map[string]any{"model": "haiku"}, &out))
	assert.Equal(t, "haiku", out.Model)
	var handoff struct {
		Command string `json:"command"`
	}
	require.Equal(t, 200, c.do("POST", "/projects/sessions/"+s.ID+"/handoff", nil, &handoff))
	assert.Contains(t, handoff.Command, "claude --resume "+s.SessionID)
	assert.Equal(t, 409, c.do("POST", "/projects/sessions/"+s.ID+"/tasks/task1/stop", nil, &e))
	assert.Equal(t, 400, c.do("GET", "/projects/sessions/"+s.ID+"/tasks/bad.id/output", nil, &e))
	assert.Equal(t, 200, c.do("POST", "/projects/sessions/"+s.ID+"/archive", nil, &out))
	assert.Equal(t, db.ThreadResolved, out.Status)
	assert.Equal(t, 409, c.do("POST", "/projects/sessions/"+s.ID+"/messages", map[string]any{"content": "late"}, &e))
	assert.Equal(t, 200, c.do("POST", "/projects/sessions/"+s.ID+"/reopen", nil, &out))
	assert.Equal(t, 404, c.do("GET", "/projects/sessions/missing", nil, &e))

	// A session is not reachable through a project's thread routes.
	var p db.Project
	require.Equal(t, 201, c.do("POST", "/projects", map[string]any{"name": "P"}, &p))
	assert.Equal(t, 404, c.do("GET", "/projects/"+p.ID+"/threads/"+s.ID, nil, &e))
}

type SessionDetail struct {
	Session         db.Thread        `json:"session"`
	Events          []db.ThreadEvent `json:"events"`
	BackgroundTasks []any            `json:"background_tasks"`
}
