// Package harness runs whole Projects workflows in process: the real
// coordinator, thread manager, store, event hub and a throwaway git
// repository, with the two models scripted. The coordinator's model is a
// Brain and a thread's Claude Code is an Actor, so one scenario can swap in a
// different behaviour (diligent, lazy, overreaching) and check what the
// system does about it.
//
// It checks workflows and rules, not wire protocols; see
// .design/projects-harness.md.
package harness

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/internal/project"
	"github.com/tingly-dev/tingly-box/projects/llm"
)

// Options describe the project a World starts with.
type Options struct {
	Name         string
	Goal         string
	Instructions string
	// Files seed the project's repository (path relative to its root).
	Files map[string]string
	Brain BrainFunc
	Actor ActorFunc
}

// World is a running project.
type World struct {
	T       *testing.T
	Store   *db.Store
	Layout  project.Layout
	Threads *project.Threads
	Coord   *project.Coordinator
	Project *db.Project
	Repo    string
	Agent   *enginetest.Agent

	brain *brain

	mu      sync.Mutex
	reports []project.Report
	prompts []string
}

type staticEnv struct{}

func (staticEnv) ThreadEnv(context.Context) ([]string, string) {
	return []string{"ANTHROPIC_BASE_URL=http://gw"}, "default-model"
}

// New starts a world and registers its cleanup and invariant checks with t.
func New(t *testing.T, o Options) *World {
	t.Helper()
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	layout := project.Layout{Root: t.TempDir()}
	repo := newRepo(t, o.Files)

	name := o.Name
	if name == "" {
		name = "Demo"
	}
	p := &db.Project{Name: name, Goal: o.Goal, Instructions: o.Instructions, Repos: []db.Repo{{Path: repo, BaseBranch: "main"}}}
	require.NoError(t, store.CreateProjectWithMain(p, &db.Thread{Origin: "user"}, ""))
	require.NoError(t, layout.Ensure(p.ID))

	w := &World{T: t, Store: store, Layout: layout, Project: p, Repo: repo}
	w.brain = &brain{fn: o.Brain}
	actor := o.Actor
	if actor == nil {
		actor = Diligent("done")
	}
	w.Agent = &enginetest.Agent{Script: func(turn *enginetest.Turn) error {
		w.mu.Lock()
		w.prompts = append(w.prompts, turn.Prompt)
		w.mu.Unlock()
		return actor(&ActorCtx{World: w, Turn: turn, Prompt: turn.Prompt, Dir: turn.Opts.ProjectPath})
	}}
	hub := project.NewHub()
	w.Threads = project.NewThreads(store, layout, hub, project.Agents{Service: w.Agent.Service()}, staticEnv{})
	w.Coord = project.NewCoordinator(store, layout, hub, w.Threads, w.brain)
	prev := w.Threads.OnReport
	w.Threads.OnReport = func(r project.Report) {
		w.mu.Lock()
		w.reports = append(w.reports, r)
		w.mu.Unlock()
		prev(r)
	}
	t.Cleanup(w.settle)
	t.Cleanup(w.CheckInvariants)
	return w
}

// settle waits until nothing is running, so no goroutine writes into the temp
// directories after the test ends.
func (w *World) settle() {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && (w.Coord.Busy(w.Project.ID) || w.anyRunning()) {
		time.Sleep(5 * time.Millisecond)
	}
	w.Threads.Shutdown(context.Background())
	time.Sleep(50 * time.Millisecond)
}

func (w *World) anyRunning() bool {
	ths, _ := w.Store.ListThreads(w.Project.ID)
	for _, t := range ths {
		if w.Threads.Running(t.ID) {
			return true
		}
	}
	return false
}

// Say is the user writing in the project conversation.
func (w *World) Say(text string) {
	w.T.Helper()
	_, err := w.Coord.Post(w.Project.ID, text, nil, project.Source{})
	require.NoError(w.T, err)
}

// Convo is the project conversation: the main thread's events.
func (w *World) Convo() []db.ThreadEvent {
	w.T.Helper()
	p, err := w.Store.GetProject(w.Project.ID)
	require.NoError(w.T, err)
	evs, err := w.Store.ListThreadEvents(p.MainThreadID)
	require.NoError(w.T, err)
	return evs
}

// ThreadList is every thread of the project except the main one.
func (w *World) ThreadList() []db.Thread {
	w.T.Helper()
	all, err := w.Store.ListThreads(w.Project.ID)
	require.NoError(w.T, err)
	var out []db.Thread
	for _, t := range all {
		if t.Agent != db.AgentLLM {
			out = append(out, t)
		}
	}
	return out
}

// Thread is the thread with this title.
func (w *World) Thread(title string) db.Thread {
	w.T.Helper()
	for _, t := range w.ThreadList() {
		if t.Title == title {
			return t
		}
	}
	w.T.Fatalf("no thread titled %q", title)
	return db.Thread{}
}

// Eventually waits for cond, and fails with what and a dump of the
// conversation when it does not come true.
func (w *World) Eventually(what string, cond func() bool) {
	w.T.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	var dump []string
	for _, e := range w.Convo() {
		dump = append(dump, e.Kind+": "+e.Content)
	}
	for _, t := range w.ThreadList() {
		dump = append(dump, "thread "+t.Title+" "+t.Status+" "+t.WaitReason)
	}
	w.T.Fatalf("never became true: %s\n%s", what, strings.Join(dump, "\n"))
}

// ThreadIs waits for the titled thread to reach status.
func (w *World) ThreadIs(title, status string) db.Thread {
	w.T.Helper()
	var got db.Thread
	w.Eventually("thread "+title+" is "+status, func() bool {
		for _, t := range w.ThreadList() {
			if t.Title == title {
				got = t
				return t.Status == status
			}
		}
		return false
	})
	return got
}

// Quiet waits for the coordinator and every thread to go idle.
//
// A thread goes idle a moment before its report reaches the coordinator, so
// idle must hold steadily, not just once.
func (w *World) Quiet() {
	w.T.Helper()
	idleSince := time.Time{}
	w.Eventually("everything idle", func() bool {
		if w.Coord.Busy(w.Project.ID) || w.anyRunning() {
			idleSince = time.Time{}
			return false
		}
		if idleSince.IsZero() {
			idleSince = time.Now()
		}
		return time.Since(idleSince) > 150*time.Millisecond
	})
}

// Requests are the model requests the coordinator made, in order.
func (w *World) Requests() []llm.Request { return w.brain.requests() }

// Prompts are the messages each thread turn started with, in order.
func (w *World) Prompts() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.prompts...)
}

// Launches are what each thread turn was started with.
func (w *World) Launches() []string {
	var out []string
	for _, l := range w.Agent.Launches() {
		out = append(out, l.AppendSystemPrompt)
	}
	return out
}

// Memory returns a memory file's text ("" when absent).
func (w *World) Memory(name string) string {
	b, _ := os.ReadFile(filepath.Join(w.Layout.MemoryDir(w.Project.ID), name))
	return string(b)
}

func newRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	if len(files) == 0 {
		files = map[string]string{"README.md": "demo\n"}
	}
	for name, body := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "init")
	return dir
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
}
