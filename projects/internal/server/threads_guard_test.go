package server

import (
	"bufio"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/internal/project"
)

func quietAgents() *project.Agents {
	a := &enginetest.Agent{Script: func(turn *enginetest.Turn) error { turn.Say("ok", 1, 1); return nil }}
	return &project.Agents{Service: a.Service()}
}

func TestWorkerActionsAreRefusedOnAMainThread(t *testing.T) {
	store, c := newServer(t, quietAgents())
	var p db.Project
	require.Equal(t, 201, c.do("POST", "/projects", map[string]any{"name": "Demo"}, &p))
	require.NotEmpty(t, p.MainThreadID)
	main := "/threads/" + p.MainThreadID

	for _, call := range []struct {
		method, path string
		body         any
	}{
		{"POST", "/archive", nil},
		{"POST", "/reopen", nil},
		{"POST", "/merge", nil},
		{"POST", "/handoff", nil},
		{"POST", "/respond", map[string]any{"request_id": "r", "allow": true}},
		{"PUT", "/model", map[string]any{"model": "m"}},
		{"PUT", "/profile", map[string]any{"profile": "x"}},
		{"PUT", "/permission-mode", map[string]any{"permission_mode": "plan"}},
		{"PUT", "/auto-answer", map[string]any{"auto_answer": "now"}},
	} {
		var e map[string]any
		code := c.do(call.method, main+call.path, call.body, &e)
		assert.Equal(t, http.StatusBadRequest, code, call.path)
		assert.Contains(t, e["error"], "main thread", call.path)
	}
	th, err := store.GetThread(p.MainThreadID)
	require.NoError(t, err)
	assert.Equal(t, db.ThreadIdle, th.Status, "the conversation was not touched")
	assert.Empty(t, th.PermissionMode)
}

// Stop is the exception among the worker actions: on the conversation it
// addresses the coordinator's turn, not the thread, so it succeeds (a no-op
// while nothing runs).
func TestStopWorksOnAMainThread(t *testing.T) {
	store, c := newServer(t, quietAgents())
	var p db.Project
	require.Equal(t, 201, c.do("POST", "/projects", map[string]any{"name": "Demo"}, &p))
	var th db.Thread
	assert.Equal(t, 200, c.do("POST", "/threads/"+p.MainThreadID+"/stop", nil, &th))
	assert.Equal(t, p.MainThreadID, th.ID)
	got, err := store.GetThread(p.MainThreadID)
	require.NoError(t, err)
	assert.Equal(t, db.ThreadIdle, got.Status, "the conversation itself stays")
}

func TestProjectThreadsOfAnUnknownProjectIs404(t *testing.T) {
	_, c := newServer(t, quietAgents())
	var e map[string]any
	assert.Equal(t, http.StatusNotFound, c.do("GET", "/projects/nope/threads", nil, &e))
}

func TestFolderProjectRefusesReposAndThreadModel(t *testing.T) {
	_, c := newServer(t, quietAgents())
	var f db.Project
	require.Equal(t, 200, c.do("POST", "/projects", map[string]any{"type": "folder", "path": t.TempDir()}, &f))
	var e map[string]any
	assert.Equal(t, 400, c.do("PATCH", "/projects/"+f.ID, map[string]any{"repos": []string{"/x"}}, &e))
	assert.Equal(t, 400, c.do("PATCH", "/projects/"+f.ID, map[string]any{"thread_model": "m"}, &e))
	assert.Equal(t, 200, c.do("PATCH", "/projects/"+f.ID, map[string]any{"name": "Renamed", "starred": true}, &f))
	assert.Equal(t, "Renamed", f.Name)
}

func TestPausedFolderRefusesANewThread(t *testing.T) {
	_, c := newServer(t, quietAgents())
	var f db.Project
	require.Equal(t, 200, c.do("POST", "/projects", map[string]any{"type": "folder", "path": t.TempDir()}, &f))
	require.Equal(t, 200, c.do("PATCH", "/projects/"+f.ID, map[string]any{"paused": true}, &f))
	var e map[string]any
	assert.Equal(t, http.StatusConflict, c.do("POST", "/projects/"+f.ID+"/threads", map[string]any{"content": "go"}, &e))
}

// The sidebar's stream carries thread and project changes, not transcript
// entries (tool results and payloads).
func TestAllProjectsStreamOmitsThreadEvents(t *testing.T) {
	store, c := newServer(t, quietAgents())
	_ = store
	resp, err := http.Get(c.base + "/projects/events")
	require.NoError(t, err)
	defer resp.Body.Close()
	lines := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), "data: ") {
				lines <- sc.Text()
			}
		}
		close(lines)
	}()

	var f db.Project
	require.Equal(t, 200, c.do("POST", "/projects", map[string]any{"type": "folder", "path": t.TempDir()}, &f))
	var th db.Thread
	require.Equal(t, 201, c.do("POST", "/projects/"+f.ID+"/threads", map[string]any{"content": "hello"}, &th))

	sawThread := false
	deadline := time.After(5 * time.Second)
	for !sawThread {
		select {
		case l := <-lines:
			assert.NotContains(t, l, `"type":"thread_event"`)
			if strings.Contains(l, `"type":"thread"`) {
				sawThread = true
			}
		case <-deadline:
			t.Fatal("no thread event on the all-projects stream")
		}
	}
	// Let the turn finish so its events (and writes) are done before cleanup.
	require.Eventually(t, func() bool {
		var d struct {
			Thread db.Thread `json:"thread"`
		}
		c.do("GET", "/threads/"+th.ID, nil, &d)
		return d.Thread.Status == db.ThreadIdle
	}, 5*time.Second, 20*time.Millisecond)
drain:
	for {
		select {
		case l := <-lines:
			assert.NotContains(t, l, `"type":"thread_event"`)
		case <-time.After(200 * time.Millisecond):
			break drain
		}
	}
}
