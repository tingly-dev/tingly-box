package live

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuiltinJourneysLoad(t *testing.T) {
	names := Builtin()
	require.Contains(t, names, "one-thread")
	for _, name := range names {
		j, err := Load(name)
		require.NoError(t, err, name)
		assert.Equal(t, name, j.Name, "a builtin journey is named after its file")
		assert.NotEmpty(t, j.Description, name)
	}
	j, _ := Load("one-thread")
	assert.Equal(t, "one-thread", j.ModelID())
	assert.Equal(t, "approve Add CHANGELOG", j.Steps[1].String())
}

func TestDecodeRejectsBadJourneys(t *testing.T) {
	base := "name: x\nproject: {name: P}\nmodel: {id: m, kind: script}\n"
	for name, doc := range map[string]string{
		"unknown field":  base + "steps: [{say: hi}]\nnope: 1\n",
		"two actions":    base + "steps: [{say: hi, signal: go}]\n",
		"no action":      base + "steps: [{}]\n",
		"not a script":   "name: x\nproject: {name: P}\nmodel: {id: m, kind: mock}\nsteps: [{say: hi}]\n",
		"no steps":       base,
		"bad served":     base + "steps: [{served: coordinator}]\n",
		"bad timeout":    base + "timeout: soon\nsteps: [{say: hi}]\n",
		"message mode":   base + "steps: [{message: {thread: T, content: c, mode: now}}]\n",
		"wait no status": base + "steps: [{wait: {thread: T}}]\n",
	} {
		_, err := Decode([]byte(doc))
		assert.Error(t, err, name)
	}
	_, err := Load("no-such-journey")
	assert.ErrorContains(t, err, "one-thread")
}

// fakeProjects answers the API calls the runner makes, with one thread that
// shows an approval card until it is answered.
type fakeProjects struct {
	mu       sync.Mutex
	answered map[string]any
	posted   []string
	signals  []string
}

func (f *fakeProjects) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	write := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	th := map[string]any{"id": "t1", "title": "Add CHANGELOG", "status": "working", "branch": "tp/add-changelog-1", "commits_ahead": 0}
	if f.answered != nil {
		th["status"], th["commits_ahead"] = "ready", 1
	}
	switch r.Method + " " + r.URL.Path {
	case "GET /api/v1/projects/p1/threads":
		write(map[string]any{"threads": []any{th}})
	case "GET /api/v1/projects/p1":
		write(map[string]any{"threads": []any{th}, "coordinator_busy": false})
	case "GET /api/v1/threads/t1":
		write(map[string]any{"thread": th})
	case "GET /api/v1/threads/t1/requests":
		reqs := []any{}
		if f.answered == nil {
			reqs = append(reqs, map[string]any{"kind": "approval", "content": "Run `git commit`", "payload": `{"request_id":"r1","tool":"Bash"}`})
		}
		write(map[string]any{"requests": reqs})
	case "POST /api/v1/threads/t1/respond":
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.answered = body
		write(th)
	case "POST /api/v1/threads/main/messages":
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.posted = append(f.posted, body["content"])
		write(map[string]any{})
	case "GET /api/v1/threads/main/events":
		write(map[string]any{"events": []any{map[string]any{"kind": "assistant", "content": "It waits for review."}}})
	case "GET /_vmodel/scripts/m/served":
		if r.URL.Query().Get("step") != "coordinator.report" {
			w.WriteHeader(408)
		}
		write(map[string]bool{"served": true})
	case "POST /_vmodel/scripts/m/signals/go":
		f.signals = append(f.signals, "go")
		w.WriteHeader(204)
	default:
		w.WriteHeader(404)
	}
}

func TestActionsAndExpectations(t *testing.T) {
	f := &fakeProjects{}
	srv := httptest.NewServer(f)
	defer srv.Close()
	p := &projects{api: newAPI(srv.URL), project: project{ID: "p1", MainThreadID: "main"}}
	m := &model{api: newAPI(srv.URL), id: "m"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, a := range []Action{
		{Say: "Add a CHANGELOG please."},
		{Approve: &ApprovalAction{Thread: "Add CHANGELOG"}},
		{Wait: &WaitAction{Thread: "Add CHANGELOG", Status: "ready"}},
		{Served: "coordinator.report"},
		{Signal: "go"},
	} {
		_, err := act(ctx, p, m, a, time.Second)
		require.NoError(t, err, a.String())
	}
	assert.Equal(t, []string{"Add a CHANGELOG please."}, f.posted)
	assert.Equal(t, map[string]any{"request_id": "r1", "allow": true, "scope": "once"}, f.answered)
	assert.Equal(t, []string{"go"}, f.signals)

	one, two := 1, 2
	assert.Empty(t, expectations(ctx, p, Expect{Threads: &one,
		Thread:               map[string]ThreadExpect{"Add CHANGELOG": {Status: "ready", CommitsAhead: &one, BranchPrefix: "tp/"}},
		ConversationContains: []string{"waits for review"}}))
	assert.Len(t, expectations(ctx, p, Expect{Threads: &two,
		Thread:               map[string]ThreadExpect{"Add CHANGELOG": {Status: "idle"}, "Other": {}},
		ConversationContains: []string{"merged"}}), 4)

	short, stop := context.WithTimeout(ctx, 300*time.Millisecond)
	defer stop()
	_, err := act(short, p, m, Action{Wait: &WaitAction{Thread: "Missing", Status: "ready"}}, time.Second)
	assert.ErrorContains(t, err, `last seen: threads "Add CHANGELOG"`)
}

func TestDirtyAncestors(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "CLAUDE.md"), nil, 0o644))
	dir := filepath.Join(root, "a", "b")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	assert.Contains(t, dirtyAncestors(dir), filepath.Join(root, "CLAUDE.md"))
}
