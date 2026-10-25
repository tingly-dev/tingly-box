package project

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/agentboot/claude/fixture"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

type staticEnv struct{}

func (staticEnv) ThreadEnv(context.Context) ([]string, string) {
	return []string{"ANTHROPIC_BASE_URL=http://gw"}, "default-model"
}

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
}

func newRepo(t *testing.T) string {
	dir := t.TempDir()
	run(t, dir, "git", "init", "-q", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi\n"), 0o644))
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-qm", "init")
	return dir
}

type testFixture struct {
	store   *db.Store
	threads *Threads
	agent   *enginetest.Agent
	project *db.Project
	repo    string
	reports chan Report
}

// newFixture runs every thread turn through script, a stand-in for Claude
// Code (one-shot turns: no pool).
func newFixture(t *testing.T, script func(turn *enginetest.Turn) error) *testFixture {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	layout := Layout{Root: t.TempDir()}
	repo := newRepo(t)
	p := &db.Project{Name: "Demo", Goal: "Ship it", Instructions: "Be careful.", Repos: []db.Repo{{Path: repo, BaseBranch: "main"}}}
	mkProject(t, store, p)
	require.NoError(t, layout.Ensure(p.ID))
	a := &enginetest.Agent{Script: script}
	th := NewThreads(store, layout, NewHub(), Agents{Service: a.Service()}, staticEnv{})
	t.Cleanup(func() { shutdownAndSettle(th) })
	f := &testFixture{store: store, threads: th, agent: a, project: p, repo: repo, reports: make(chan Report, 10)}
	th.OnReport = func(rep Report) { f.reports <- rep }
	return f
}

func (f *testFixture) report(t *testing.T) Report {
	t.Helper()
	select {
	case r := <-f.reports:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("no report")
		return Report{}
	}
}

func waitStatus(t *testing.T, store *db.Store, id, status string) *db.Thread {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		th, err := store.GetThread(id)
		require.NoError(t, err)
		if th.Status == status {
			return th
		}
		time.Sleep(10 * time.Millisecond)
	}
	th, _ := store.GetThread(id)
	t.Fatalf("thread status %q, want %q (%s)", th.Status, status, th.WaitReason)
	return nil
}

func TestThreadCommitsBecomeReadyAndMerge(t *testing.T) {
	var f *testFixture
	f = newFixture(t, func(turn *enginetest.Turn) error {
		dir := turn.Opts.ProjectPath
		turn.Say("Adding the file", 4, 2)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("x\n"), 0o644))
		run(t, dir, "git", "add", ".")
		run(t, dir, "git", "commit", "-qm", "feature")
		turn.Say("Added feature.txt and committed it.", 6, 3)
		return nil
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Add feature", Task: "add feature.txt", Origin: "coordinator"})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(th.Branch, "tp/add-feature-"))

	rep := f.report(t)
	assert.Equal(t, ReportFinished, rep.Kind)
	got := waitStatus(t, f.store, th.ID, db.ThreadReady)
	assert.Equal(t, 1, got.CommitsAhead)
	assert.Equal(t, 1, got.FilesChanged)
	assert.EqualValues(t, 10, got.InputTokens)
	assert.Equal(t, "default-model", got.Model)

	spec := f.agent.Launches()[0]
	assert.False(t, spec.Resume)
	assert.Equal(t, th.SessionID, spec.SessionID)
	assert.Equal(t, th.Cwd, spec.ProjectPath)
	assert.Equal(t, DefaultPermissionMode, spec.PermissionMode)
	assert.Equal(t, "default-model", spec.Model)
	assert.Contains(t, spec.AppendSystemPrompt, "Be careful.")
	assert.Contains(t, spec.AppendSystemPrompt, th.Branch)
	assert.Contains(t, spec.Env, "ANTHROPIC_BASE_URL=http://gw")
	assert.Equal(t, "Added feature.txt and committed it.", rep.Text)
	assert.EqualValues(t, 5, got.OutputTokens)

	d, err := f.threads.Diff(context.Background(), th.ID)
	require.NoError(t, err)
	assert.Contains(t, d.Patch, "feature.txt")

	merged, err := f.threads.Merge(context.Background(), th.ID)
	require.NoError(t, err)
	assert.Equal(t, db.ThreadResolved, merged.Status)
	assert.FileExists(t, filepath.Join(f.repo, "feature.txt"))
}

func TestMergeRefusesDirtyCheckout(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		dir := turn.Opts.ProjectPath
		require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644))
		run(t, dir, "git", "add", ".")
		run(t, dir, "git", "commit", "-qm", "a")
		turn.Say("done", 1, 1)
		return nil
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "a"})
	require.NoError(t, err)
	f.report(t)
	waitStatus(t, f.store, th.ID, db.ThreadReady)
	require.NoError(t, os.WriteFile(filepath.Join(f.repo, "README.md"), []byte("local edit\n"), 0o644))
	_, err = f.threads.Merge(context.Background(), th.ID)
	var blocked *MergeBlockedError
	require.ErrorAs(t, err, &blocked)
	assert.Contains(t, blocked.Command, "merge --no-ff")
}

func TestApprovalWaitsOnUser(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		resp, err := turn.Approve("req-1", "Bash", map[string]any{"command": "make test"})
		if err != nil {
			return err
		}
		if !resp.Approved {
			turn.Say("Not allowed to run tests.", 1, 1)
			return nil
		}
		// A second Bash call is allowed for the rest of the thread.
		resp2, _ := turn.Approve("req-2", "Bash", map[string]any{"command": "make lint"})
		turn.Say(map[bool]string{true: "Tests and lint pass.", false: "lint blocked"}[resp2.Approved], 1, 1)
		return nil
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "run tests"})
	require.NoError(t, err)
	got := waitStatus(t, f.store, th.ID, db.ThreadWaiting)
	assert.Equal(t, "Approve: Run `make test`", got.WaitReason)

	require.NoError(t, f.threads.Respond(th.ID, "req-1", Answer{Allow: true, Scope: "thread"}))
	rep := f.report(t)
	assert.Equal(t, "Tests and lint pass.", rep.Text)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)

	events, _ := f.store.ListThreadEvents(th.ID)
	var approvals int
	for _, e := range events {
		if e.Kind == db.EvApproval {
			approvals++
			assert.Contains(t, e.Payload, `"state":"allowed"`)
		}
	}
	assert.Equal(t, 1, approvals, "the second call was allowed by the thread-wide answer")
}

func TestNeedsInputAndFollowUpResumes(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		if !turn.Opts.Resume {
			turn.Say("Two options exist.\nNEEDS_INPUT: Should I use Postgres or SQLite?", 1, 1)
			return nil
		}
		turn.Say("Used "+turn.Prompt, 1, 1)
		return nil
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "pick a db"})
	require.NoError(t, err)
	rep := f.report(t)
	assert.Equal(t, ReportNeedsInput, rep.Kind)
	got := waitStatus(t, f.store, th.ID, db.ThreadWaiting)
	assert.Equal(t, "Should I use Postgres or SQLite?", got.WaitReason)

	_, err = f.threads.Send(context.Background(), th.ID, "SQLite", Source{}, nil)
	require.NoError(t, err)
	rep = f.report(t)
	assert.Equal(t, "Used SQLite", rep.Text)
	specs := f.agent.Launches()
	require.Len(t, specs, 2)
	assert.True(t, specs[1].Resume)
	assert.Equal(t, specs[0].SessionID, specs[1].SessionID)
}

func TestStopAndPausedProject(t *testing.T) {
	started := make(chan struct{})
	f := newFixture(t, func(turn *enginetest.Turn) error {
		close(started)
		<-turn.Ctx.Done()
		return turn.Ctx.Err()
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "long"})
	require.NoError(t, err)
	<-started
	f.threads.Stop(th.ID)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)
	select {
	case r := <-f.reports:
		t.Fatalf("a stopped turn must not report, got %+v", r)
	case <-time.After(100 * time.Millisecond):
	}

	f.project.Paused = true
	require.NoError(t, f.store.SaveProject(f.project))
	_, err = f.threads.Send(context.Background(), th.ID, "go on", Source{}, nil)
	assert.ErrorIs(t, err, ErrPaused)
}

func TestMemory(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	require.NoError(t, l.Ensure("p"))
	name, err := l.Remember("p", "Release date", "Release moved to Friday.", "release is Friday")
	require.NoError(t, err)
	assert.Equal(t, "release-date.md", name)
	_, err = l.Remember("p", "Release date", "Release moved to Monday.", "release is Monday")
	require.NoError(t, err)
	idx := l.memoryIndexText("p")
	assert.Equal(t, 1, strings.Count(idx, "release-date.md"), idx)
	assert.Contains(t, idx, "release is Monday")
	require.NoError(t, l.DeleteMemory("p", name))
	assert.NotContains(t, l.memoryIndexText("p"), "release-date.md")
	assert.Error(t, l.DeleteMemory("p", MemoryIndex))
	_, err = l.ReadMemory("p", "../etc.md")
	assert.Error(t, err)
}

// The real Claude Code driver, on a scripted CLI: what the CLI prints
// becomes the thread's transcript, its permission request waits in the
// thread, and its last text is the report.
func TestThreadOnScriptedClaudeCLI(t *testing.T) {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	layout := Layout{Root: t.TempDir()}
	p := &db.Project{Name: "Demo"}
	mkProject(t, store, p)
	require.NoError(t, layout.Ensure(p.ID))

	script := fixture.Script{
		fixture.System("sess-1", t.TempDir()),
		fixture.AssistantText("Running the tests."),
		fixture.PermissionRequest("req-1", "Bash", map[string]any{"command": "go test ./..."}),
		fixture.AssistantText("All tests pass."),
		fixture.Result(true),
	}
	svc, err := agentboot.NewAgentService(agentboot.DefaultConfig())
	require.NoError(t, err)
	svc.RegisterAgent(agentboot.AgentTypeClaude, claude.NewAgentWithFactory(claude.Config{}, fixture.Factory(script)))
	th := NewThreads(store, layout, NewHub(), Agents{Service: svc}, staticEnv{})
	t.Cleanup(func() { shutdownAndSettle(th) })
	reports := make(chan Report, 1)
	th.OnReport = func(r Report) { reports <- r }

	thread, err := th.Start(context.Background(), p, StartSpec{Task: "run tests"})
	require.NoError(t, err)
	got := waitStatus(t, store, thread.ID, db.ThreadWaiting)
	assert.Equal(t, "Approve: Run `go test ./...`", got.WaitReason)
	require.NoError(t, th.Respond(thread.ID, "req-1", Answer{Allow: true}))

	select {
	case r := <-reports:
		assert.Equal(t, ReportFinished, r.Kind)
		assert.Equal(t, "All tests pass.", r.Text)
	case <-time.After(5 * time.Second):
		t.Fatal("no report")
	}
	events, _ := store.ListThreadEvents(thread.ID)
	var texts []string
	for _, e := range events {
		if e.Kind == db.EvAssistant {
			texts = append(texts, e.Content)
		}
	}
	assert.Equal(t, []string{"Running the tests.", "All tests pass."}, texts)
}

// A message sent while a turn runs is queued and runs next, resuming the
// same session.
func TestMessageDuringATurnRunsNext(t *testing.T) {
	release := make(chan struct{})
	f := newFixture(t, func(turn *enginetest.Turn) error {
		if !turn.Opts.Resume {
			<-release
		}
		turn.Say("did "+turn.Prompt, 1, 1)
		return nil
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "first"})
	require.NoError(t, err)
	waitStatus(t, f.store, th.ID, db.ThreadWorking)
	_, err = f.threads.Send(context.Background(), th.ID, "second", Source{}, nil)
	require.NoError(t, err)
	close(release)

	assert.Equal(t, "did first", f.report(t).Text)
	assert.Equal(t, "did second", f.report(t).Text)
	launches := f.agent.Launches()
	require.Len(t, launches, 2)
	assert.True(t, launches[1].Resume)
	assert.Eventually(t, func() bool { return !f.threads.Running(th.ID) }, 5*time.Second, 10*time.Millisecond)
}

func TestMessageSentAfterStopRuns(t *testing.T) {
	started, stopping, hold := make(chan struct{}), make(chan struct{}), make(chan struct{})
	f := newFixture(t, func(turn *enginetest.Turn) error {
		if !turn.Opts.Resume {
			close(started)
			<-turn.Ctx.Done()
			close(stopping)
			<-hold // the stopped turn is still ending when the next message arrives
			return turn.Ctx.Err()
		}
		turn.Say("did "+turn.Prompt, 1, 1)
		return nil
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "long"})
	require.NoError(t, err)
	<-started
	f.threads.Stop(th.ID)
	<-stopping
	_, err = f.threads.Send(context.Background(), th.ID, "instead", Source{}, nil)
	require.NoError(t, err)
	close(hold)

	assert.Equal(t, "did instead", f.report(t).Text)
	assert.Eventually(t, func() bool { return !f.threads.Running(th.ID) }, 5*time.Second, 10*time.Millisecond)
}

func TestResolveDuringATurnStaysResolved(t *testing.T) {
	started := make(chan struct{})
	f := newFixture(t, func(turn *enginetest.Turn) error {
		close(started)
		<-turn.Ctx.Done()
		return turn.Ctx.Err()
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "long"})
	require.NoError(t, err)
	<-started
	_, err = f.threads.Resolve(th.ID)
	require.NoError(t, err)
	assert.Eventually(t, func() bool { return !f.threads.Running(th.ID) }, 5*time.Second, 10*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	got, err := f.store.GetThread(th.ID)
	require.NoError(t, err)
	assert.Equal(t, db.ThreadResolved, got.Status)
}

func TestAskAnswersReachClaude(t *testing.T) {
	got := make(chan agentboot.AskResponse, 1)
	f := newFixture(t, func(turn *enginetest.Turn) error {
		resp, err := turn.AskQuestions("q-1", map[string]any{"questions": []any{
			map[string]any{"question": "Which database?", "header": "DB", "options": []any{
				map[string]any{"label": "Postgres (Recommended)"}, map[string]any{"label": "SQLite"},
			}},
			map[string]any{"question": "Keep the legacy flag?", "header": "Flag"},
		}})
		if err != nil {
			return err
		}
		got <- resp
		turn.Say("ok", 1, 1)
		return nil
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "pick"})
	require.NoError(t, err)
	w := waitStatus(t, f.store, th.ID, db.ThreadWaiting)
	assert.Equal(t, "Which database?", w.WaitReason)

	require.NoError(t, f.threads.Respond(th.ID, "q-1", Answer{Allow: true, Answers: map[string]string{
		"Which database?": "Postgres (Recommended)", "Keep the legacy flag?": " no ", "Not asked": "x",
	}}))
	resp := <-got
	require.True(t, resp.Approved)
	assert.Equal(t, map[string]string{"Which database?": "Postgres (Recommended)", "Keep the legacy flag?": "no"}, resp.UpdatedInput["answers"])
	assert.NotNil(t, resp.UpdatedInput["questions"], "the questions go back with the answers")
	f.report(t)

	events, _ := f.store.ListThreadEvents(th.ID)
	for _, e := range events {
		if e.Kind == db.EvAsk {
			assert.Contains(t, e.Payload, `"state":"answered"`)
			assert.Contains(t, e.Payload, `"answer":"Which database?: Postgres (Recommended)\nKeep the legacy flag?: no"`)
		}
	}
}

func TestSettleAsk(t *testing.T) {
	one := map[string]any{"questions": []any{map[string]any{"question": "Q?"}}}
	// A typed answer goes to the only question.
	a := settleAsk("Q?", one, Answer{Allow: true, Answer: " use both "})
	assert.Equal(t, map[string]string{"Q?": "use both"}, a.Answers)
	assert.Equal(t, "use both", a.Answer)
	// Declining chooses nothing.
	a = settleAsk("Q?", one, Answer{Allow: false, Answer: "draft"})
	assert.Nil(t, a.Answers)
	assert.Equal(t, "", a.Answer)
	// A question with no structured input is keyed by its text.
	a = settleAsk("Proceed?", nil, Answer{Allow: true, Answer: "yes"})
	assert.Equal(t, map[string]string{"Proceed?": "yes"}, a.Answers)
}

func TestSuggestedAnswers(t *testing.T) {
	q := func(question string, multi bool, labels ...string) map[string]any {
		opts := []any{}
		for _, l := range labels {
			opts = append(opts, map[string]any{"label": l})
		}
		return map[string]any{"question": question, "multiSelect": multi, "options": opts}
	}
	got, ok := suggestedAnswers(map[string]any{"questions": []any{q("DB?", false, "SQLite", "Postgres (Recommended)"), q("Flag?", false, "Keep (推荐)", "Drop")}})
	require.True(t, ok)
	assert.Equal(t, map[string]string{"DB?": "Postgres (Recommended)", "Flag?": "Keep (推荐)"}, got)
	// One question without a suggestion, or with several picks, leaves the whole call to the user.
	_, ok = suggestedAnswers(map[string]any{"questions": []any{q("DB?", false, "Postgres (Recommended)"), q("Flag?", false, "Keep", "Drop")}})
	assert.False(t, ok)
	_, ok = suggestedAnswers(map[string]any{"questions": []any{q("Which?", true, "A (Recommended)", "B")}})
	assert.False(t, ok)
	_, ok = suggestedAnswers(map[string]any{})
	assert.False(t, ok)
}

func TestAskAnswersItselfWithTheSuggestion(t *testing.T) {
	got := make(chan agentboot.AskResponse, 1)
	f := newFixture(t, func(turn *enginetest.Turn) error {
		resp, err := turn.AskQuestions("q-1", map[string]any{"questions": []any{
			map[string]any{"question": "Which database?", "options": []any{
				map[string]any{"label": "Postgres (Recommended)"}, map[string]any{"label": "SQLite"},
			}},
		}})
		if err != nil {
			return err
		}
		got <- resp
		turn.Say("ok", 1, 1)
		return nil
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "pick"})
	require.NoError(t, err)
	waitStatus(t, f.store, th.ID, db.ThreadWaiting)

	// A project thread waits ten minutes by default and says until when.
	events, _ := f.store.ListThreadEvents(th.ID)
	var ask *db.ThreadEvent
	for i := range events {
		if events[i].Kind == db.EvAsk {
			ask = &events[i]
		}
	}
	require.NotNil(t, ask)
	assert.Contains(t, ask.Payload, `"auto_at":`)

	// Switching to "right away" answers the waiting question.
	_, err = f.threads.SetAutoAnswer(th.ID, AutoAnswerNow)
	require.NoError(t, err)
	select {
	case resp := <-got:
		require.True(t, resp.Approved)
		assert.Equal(t, map[string]string{"Which database?": "Postgres (Recommended)"}, resp.UpdatedInput["answers"])
	case <-time.After(5 * time.Second):
		t.Fatal("not answered")
	}
	f.report(t)
	events, _ = f.store.ListThreadEvents(th.ID)
	for _, e := range events {
		if e.Kind == db.EvAsk {
			assert.Contains(t, e.Payload, `"auto":true`)
			assert.Contains(t, e.Payload, `"state":"answered"`)
			assert.NotContains(t, e.Payload, `"auto_at"`)
		}
	}

	_, err = f.threads.SetAutoAnswer(th.ID, "sometimes")
	assert.ErrorIs(t, err, ErrBadAutoAnswer)
}

func TestSessionQuestionsWaitByDefault(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		_, err := turn.AskQuestions("q-1", map[string]any{"questions": []any{
			map[string]any{"question": "Which?", "options": []any{map[string]any{"label": "A (Recommended)"}}},
		}})
		return err
	})
	s, err := f.startFolder(f.repo, FolderThreadSpec{Task: "pick"})
	require.NoError(t, err)
	assert.Equal(t, "main", s.HeadBranch, "a session names the branch its folder is on")
	waitStatus(t, f.store, s.ID, db.ThreadWaiting)
	events, _ := f.store.ListThreadEvents(s.ID)
	for _, e := range events {
		if e.Kind == db.EvAsk {
			assert.NotContains(t, e.Payload, `"auto_at"`)
		}
	}
}

func TestFailedTurnMarksThreadFailedNotWaiting(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		return errors.New("boom")
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "break"})
	require.NoError(t, err)
	got := waitStatus(t, f.store, th.ID, db.ThreadFailed)
	assert.Contains(t, got.WaitReason, "Failed: ")
	counts, err := f.store.ProjectThreadCounts(f.project.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, counts.Failed)
	assert.Equal(t, 0, counts.Waiting)
}

// A thread's worktree lives in the repository's .claude/worktrees, hidden
// from git status, and its branch keeps the tp/ name.
func TestThreadWorktreeIsInTheRepository(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Add feature", Task: "x", Origin: "coordinator"})
	require.NoError(t, err)
	assert.Equal(t, RepoWorktreeDir(f.repo, "add-feature", th.ID), th.Cwd)
	assert.True(t, strings.HasPrefix(th.Branch, "tp/add-feature-"))
	assert.DirExists(t, th.Cwd)
	out := run(t, f.repo, "git", "status", "--porcelain")
	assert.Empty(t, strings.TrimSpace(out), "the worktree directory is not reported as untracked")
	th2, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Second", Task: "y", Origin: "coordinator"})
	require.NoError(t, err)
	b, err := os.ReadFile(filepath.Join(f.repo, ".git", "info", "exclude"))
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(b), "/.claude/worktrees/"), "excluded once, however many threads")
	assert.NotEqual(t, th.Cwd, th2.Cwd)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)
}

// When the repository cannot hold the worktree, the thread still starts, in
// the data directory, and the thread says so.
func TestThreadWorktreeFallsBackToDataDir(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	// A file where the directory should go makes git unable to create it.
	require.NoError(t, os.WriteFile(filepath.Join(f.repo, ".claude"), []byte("x"), 0o644))
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Add feature", Task: "x", Origin: "coordinator"})
	require.NoError(t, err)
	assert.Equal(t, f.threads.layout.ThreadDir(f.project.ID, th.ID), th.Cwd)
	evs, err := f.store.ListThreadEvents(th.ID)
	require.NoError(t, err)
	var said bool
	for _, e := range evs {
		said = said || (e.Kind == db.EvSystem && strings.Contains(e.Content, "Could not create the worktree"))
	}
	assert.True(t, said, "status %s: %s", th.Status, th.WaitReason)
}
