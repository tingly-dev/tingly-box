package project

import (
	"context"
	"errors"
	"fmt"
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
	a := &enginetest.Agent{Script: script}
	return newFixtureWith(t, a, Agents{Service: a.Service()})
}

// newFixtureWith is newFixture over the given agents (a is what the fixture
// reports launches of).
func newFixtureWith(t *testing.T, a *enginetest.Agent, agents Agents) *testFixture {
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	layout := Layout{Root: t.TempDir()}
	repo := newRepo(t)
	p := &db.Project{Name: "Demo", Goal: "Ship it", Instructions: "Be careful.", Repos: []db.Repo{{Path: repo, BaseBranch: "main"}}}
	mkProject(t, store, p)
	require.NoError(t, layout.Ensure(p.ID))
	th := NewThreads(store, layout, NewHub(), agents, staticEnv{})
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

func TestThreadReadsSharedProjectUpload(t *testing.T) {
	var uploadPath string
	result := make(chan error, 1)
	f := newFixture(t, func(turn *enginetest.Turn) error {
		if !strings.Contains(turn.Opts.AppendSystemPrompt, filepath.Dir(filepath.Dir(uploadPath))) {
			result <- errors.New("thread was not told the shared library path")
			return nil
		}
		body, err := os.ReadFile(uploadPath)
		if err == nil && string(body) != "shared upload contents" {
			err = errors.New("unexpected shared upload contents")
		}
		result <- err
		return nil
	})
	a, err := f.threads.layout.SaveAttachment(f.project.ID, "shared.txt", strings.NewReader("shared upload contents"), 0)
	require.NoError(t, err)
	uploadPath, err = f.threads.layout.LibraryPath(f.project.ID, a.Path)
	require.NoError(t, err)
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Read upload", Task: "Read the shared upload", Origin: "coordinator"})
	require.NoError(t, err)
	assert.NotEqual(t, f.repo, th.Cwd, "thread runs in a worktree")
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("thread did not read the upload")
	}
}

func TestThreadCommitsBecomeReady(t *testing.T) {
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
	assert.EqualValues(t, 6, got.ContextTokens, "context is the prompt size of the turn's last call")
	assert.NotNil(t, got.LastTurnAt)
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

	_, err = f.threads.PRDiff(context.Background(), th.ID, 12)
	require.Error(t, err, "a PR the thread does not hold has no diff")
	addOpenPR(t, f, th, 12)
	d, err := f.threads.PRDiff(context.Background(), th.ID, 12)
	require.NoError(t, err)
	assert.Equal(t, "local", d.Source)
	require.Len(t, d.Files, 1)
	assert.Equal(t, "feature.txt", d.Files[0].Path)
	assert.Equal(t, "added", d.Files[0].Status)
	fd, err := f.threads.PRDiffFile(context.Background(), th.ID, 12, "feature.txt", "")
	require.NoError(t, err)
	assert.Contains(t, fd.Patch, "+x")
}

func TestMergeNeedsOneOfTwoMethods(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "x"})
	require.NoError(t, err)
	for _, method := range []string{"", "merge", "rebase", "SQUASH"} {
		_, err = f.threads.Merge(context.Background(), th.ID, MergeSpec{Method: method})
		assert.ErrorIs(t, err, ErrBadMergeMethod, method)
	}
}

// mergeFixture is a ready thread with a commit, whose repository has a GitHub
// origin that git pushes to a local bare repository, and a fake GitHub.
func mergeFixture(t *testing.T) (*testFixture, *db.Thread, *fakeGitHub, string) {
	t.Helper()
	return mergeFixtureWith(t, func(turn *enginetest.Turn) error {
		dir := turn.Opts.ProjectPath
		require.NoError(t, os.WriteFile(filepath.Join(dir, "feature.txt"), []byte("x\n"), 0o644))
		run(t, dir, "git", "add", ".")
		run(t, dir, "git", "commit", "-qm", "feature")
		turn.Say("Added feature.txt.", 1, 1)
		return nil
	})
}

// mergeFixtureWith is mergeFixture with the thread's turns run by script; the
// first turn must commit something.
func mergeFixtureWith(t *testing.T, script func(turn *enginetest.Turn) error) (*testFixture, *db.Thread, *fakeGitHub, string) {
	t.Helper()
	f := newFixture(t, script)
	remote := t.TempDir()
	run(t, remote, "git", "init", "-q", "--bare", "-b", "main")
	run(t, f.repo, "git", "remote", "add", "origin", "https://github.com/acme/widgets.git")
	run(t, f.repo, "git", "config", "url."+remote+".insteadOf", "https://github.com/acme/widgets.git")
	run(t, f.repo, "git", "push", "-q", "origin", "main")
	gh := newFakeGitHub()
	links := NewLinks(f.store, gh, f.threads.republish)
	f.threads.links = func() *Links { return links }
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Add feature", Task: "add feature.txt"})
	require.NoError(t, err)
	f.report(t)
	th = waitStatus(t, f.store, th.ID, db.ThreadReady)
	return f, th, gh, remote
}

func TestSquashMergesThePROnGitHubAndLeavesTheThreadOpen(t *testing.T) {
	f, th, gh, remote := mergeFixture(t)
	head := run(t, f.repo, "git", "rev-parse", "HEAD")
	sha := run(t, f.repo, "git", "rev-parse", th.Branch)

	got, err := f.threads.Merge(context.Background(), th.ID, MergeSpec{Method: MergeSquash, Message: "Add the feature\n\nBody."})
	require.NoError(t, err)
	assert.Contains(t, run(t, remote, "git", "branch", "--list", th.Branch), th.Branch, "the branch was pushed")
	calls := strings.Join(gh.called(), "\n")
	assert.Contains(t, calls, fmt.Sprintf(`create-pr acme/widgets "Add the feature" head=%s base=main draft=false`, th.Branch), "a PR is opened first, titled by the message")
	assert.Contains(t, calls, fmt.Sprintf(`squash-pr acme/widgets #11 sha=%s title="Add the feature" message="Body."`, sha), "then squash-merged at the pushed commit")
	assert.Equal(t, db.ThreadIdle, got.Status, "merged work rests: nothing waits for review, and the user decides whether the thread is done")
	assert.Nil(t, got.ResolvedAt)
	assert.NotNil(t, got.MergedAt)
	require.Len(t, got.Links, 1)
	assert.Equal(t, db.LinkStateMerged, got.Links[0].State)
	assert.Equal(t, th.Branch, got.Links[0].Head)
	assert.Equal(t, head, run(t, f.repo, "git", "rev-parse", "HEAD"), "the local checkout is untouched")
	assert.NoFileExists(t, filepath.Join(f.repo, "feature.txt"))
	assert.NotEmpty(t, run(t, f.repo, "git", "branch", "--list", th.Branch), "the branch is kept")
}

func TestSquashMergesAnAlreadyOpenPR(t *testing.T) {
	f, th, gh, _ := mergeFixture(t)
	existing := gh.add(db.LinkPR, "Already open", func(it *GitHubItem) { it.Head, it.Base = th.Branch, "main" })
	gh.existing = &existing
	got, err := f.threads.Merge(context.Background(), th.ID, MergeSpec{Method: MergeSquash})
	require.NoError(t, err)
	calls := strings.Join(gh.called(), "\n")
	assert.NotContains(t, calls, "create-pr", "no second PR")
	assert.Contains(t, calls, fmt.Sprintf("squash-pr acme/widgets #%d", existing.Number))
	assert.Contains(t, calls, `title="Already open (#11)" message=""`, "an empty message is the PR's own title and number, and its description, as gh pr merge words it")
	assert.Equal(t, db.ThreadIdle, got.Status)
}

func TestSquashRefusedByGitHubLeavesThePROpenAndTheThreadAlone(t *testing.T) {
	f, th, gh, _ := mergeFixture(t)
	gh.mergeFail = errors.New("GitHub PUT pulls/11/merge: Pull Request is not mergeable (HTTP 405)")
	_, err := f.threads.Merge(context.Background(), th.ID, MergeSpec{Method: MergeSquash})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not mergeable")
	assert.Contains(t, err.Error(), "stays open")
	got, err := f.store.GetThread(th.ID)
	require.NoError(t, err)
	assert.Equal(t, db.ThreadReady, got.Status)
	assert.Nil(t, got.MergedAt)
	require.Len(t, got.Links, 1, "the PR it opened is still linked")
	assert.Equal(t, db.LinkStateOpen, got.Links[0].State)
}

func TestSquashAfterTheMergeToolOnlyRecordsTheMerge(t *testing.T) {
	f, th, gh, _ := mergeFixture(t)
	_, err := f.threads.Merge(context.Background(), th.ID, MergeSpec{Method: MergeMergeRequest})
	require.NoError(t, err)
	gh.set(11, func(it *GitHubItem) { it.State = "merged" })
	require.NoError(t, f.store.UpdateThreadLinks(th.ID, func(l []db.ThreadLink) []db.ThreadLink { l[0].State = db.LinkStateMerged; return l }))
	before := len(gh.called())

	got, err := f.threads.Merge(context.Background(), th.ID, MergeSpec{Method: MergeSquash})
	require.NoError(t, err)
	assert.Equal(t, db.ThreadIdle, got.Status)
	assert.NotNil(t, got.MergedAt)
	assert.Len(t, gh.called(), before, "GitHub is not asked again")
	_, err = f.threads.Merge(context.Background(), th.ID, MergeSpec{Method: MergeMergeRequest})
	assert.ErrorContains(t, err, "already merged")
}

func TestMergeRequestPushesTheBranchOpensAPRAndLinksIt(t *testing.T) {
	f, th, gh, remote := mergeFixture(t)
	head := run(t, f.repo, "git", "rev-parse", "HEAD")

	got, err := f.threads.Merge(context.Background(), th.ID, MergeSpec{Method: MergeMergeRequest, Title: "Add the feature", Body: "Adds feature.txt."})
	require.NoError(t, err)
	assert.Contains(t, run(t, remote, "git", "branch", "--list", th.Branch), th.Branch, "the branch was pushed")
	assert.Contains(t, gh.called(), fmt.Sprintf(`create-pr acme/widgets "Add the feature" head=%s base=main draft=false`, th.Branch))
	for _, c := range gh.called() {
		assert.NotContains(t, c, "squash-pr", "a merge request is not merged")
	}
	require.Len(t, got.Links, 1)
	assert.Equal(t, db.LinkPR, got.Links[0].Kind)
	assert.Equal(t, th.Branch, got.Links[0].Head)
	assert.Equal(t, "main", got.Links[0].Base)
	assert.NotEmpty(t, got.Links[0].URL)
	assert.Equal(t, db.ThreadReady, got.Status, "the branch is not merged until the PR is")
	assert.Nil(t, got.MergedAt)
	assert.Equal(t, head, run(t, f.repo, "git", "rev-parse", "HEAD"), "the main checkout is untouched")
}

func TestMergeRequestDefaultsTitleAndBodyFromTheCommits(t *testing.T) {
	f, th, gh, _ := mergeFixture(t)
	_, err := f.threads.Merge(context.Background(), th.ID, MergeSpec{Method: MergeMergeRequest})
	require.NoError(t, err)
	// The branch has one commit: its subject is the title, as gh pr create --fill takes it.
	subject := run(t, f.repo, "git", "log", "-1", "--format=%s", th.Branch)
	assert.Contains(t, gh.called(), fmt.Sprintf(`create-pr acme/widgets %q head=%s base=main draft=false`, subject, th.Branch))
}

// A description given without a title is kept; only the title comes from the commits.
func TestMergeRequestKeepsAGivenBodyWithoutATitle(t *testing.T) {
	f, th, gh, _ := mergeFixture(t)
	got, err := f.threads.Merge(context.Background(), th.ID, MergeSpec{Method: MergeMergeRequest, Body: "Written by hand."})
	require.NoError(t, err)
	require.Len(t, got.Links, 1)
	gh.mu.Lock()
	defer gh.mu.Unlock()
	assert.Equal(t, "Written by hand.", gh.items[got.Links[0].Number].Body)
}

func TestMergeRequestLinksAnExistingPRInsteadOfOpeningAnother(t *testing.T) {
	f, th, gh, _ := mergeFixture(t)
	existing := gh.add(db.LinkPR, "Already open", func(it *GitHubItem) { it.Head, it.Base = th.Branch, "main" })
	gh.existing = &existing
	got, err := f.threads.Merge(context.Background(), th.ID, MergeSpec{Method: MergeMergeRequest})
	require.NoError(t, err)
	for _, c := range gh.called() {
		assert.NotContains(t, c, "create-pr")
	}
	require.Len(t, got.Links, 1)
	assert.Equal(t, existing.Number, got.Links[0].Number)
}

func TestMergeSaysWhyItCannotReachGitHub(t *testing.T) {
	for _, method := range []string{MergeSquash, MergeMergeRequest} {
		for _, c := range []struct {
			name  string
			setup func(f *testFixture, gh *fakeGitHub)
			want  string
		}{
			{"no remote", func(f *testFixture, _ *fakeGitHub) { run(t, f.repo, "git", "remote", "remove", "origin") }, "no origin remote"},
			{"not GitHub", func(f *testFixture, _ *fakeGitHub) {
				run(t, f.repo, "git", "remote", "set-url", "origin", "https://gitlab.com/acme/widgets.git")
			}, "github.com"},
			{"gh not signed in", func(_ *testFixture, gh *fakeGitHub) { gh.fail = ErrGitHubUnavailable }, "gh is missing or not signed in"},
		} {
			t.Run(method+"/"+c.name, func(t *testing.T) {
				f, th, gh, _ := mergeFixture(t)
				c.setup(f, gh)
				_, err := f.threads.Merge(context.Background(), th.ID, MergeSpec{Method: method})
				require.Error(t, err)
				assert.Contains(t, err.Error(), c.want)
				got, err := f.store.GetThread(th.ID)
				require.NoError(t, err)
				assert.Empty(t, got.Links)
				assert.Equal(t, db.ThreadReady, got.Status)
				assert.Nil(t, got.MergedAt)
			})
		}
	}
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
		// The same command is allowed for the rest of the thread.
		resp2, _ := turn.Approve("req-2", "Bash", map[string]any{"command": "make test"})
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
	f.threads.Stop(th.ID, Source{})
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
	f.threads.Stop(th.ID, Source{})
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

func TestThreadsShareRepositoryWhenWorktreesDisabled(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	f.project.DisableWorktree = true
	first, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "First", Task: "x"})
	require.NoError(t, err)
	second, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Second", Task: "y"})
	require.NoError(t, err)
	assert.Equal(t, f.repo, first.Cwd)
	assert.Equal(t, first.Cwd, second.Cwd)
	assert.Empty(t, first.Branch)
	assert.Empty(t, second.Branch)
	assert.Contains(t, threadSystemPrompt(f.threads.layout, f.project, first), "shared repository checkout")
	_, err = f.threads.Merge(context.Background(), first.ID, MergeSpec{Method: MergeSquash})
	require.Error(t, err)
	_, err = f.threads.PRDiff(context.Background(), first.ID, 1)
	require.Error(t, err)
	f.project.DisableWorktree = false
	third, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Third", Task: "z"})
	require.NoError(t, err)
	assert.NotEqual(t, f.repo, third.Cwd)
	assert.NotEmpty(t, third.Branch)
	saved, err := f.store.GetThread(first.ID)
	require.NoError(t, err)
	assert.Equal(t, f.repo, saved.Cwd)
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

func TestSetTitle(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return errors.New("no turns expected") })
	th := &db.Thread{ProjectID: f.project.ID, Agent: db.AgentClaudeCode, Title: "Before", Task: "t", Status: db.ThreadIdle, Cwd: f.repo}
	require.NoError(t, f.store.CreateThread(th))

	renamed, err := f.threads.SetTitle(th.ID, "  After  ")
	require.NoError(t, err)
	assert.Equal(t, "After", renamed.Title)
	stored, err := f.store.GetThread(th.ID)
	require.NoError(t, err)
	assert.Equal(t, "After", stored.Title)
	assert.True(t, th.UpdatedAt.Equal(stored.UpdatedAt), "a rename must not bump updated_at")

	_, err = f.threads.SetTitle(th.ID, "   ")
	assert.ErrorIs(t, err, ErrEmptyTitle)
}

// addOpenPR records an open PR for the thread's branch, as a refresh would.
func addOpenPR(t *testing.T, f *testFixture, th *db.Thread, number int) {
	t.Helper()
	require.NoError(t, f.store.UpdateThreadLinks(th.ID, func(l []db.ThreadLink) []db.ThreadLink {
		return append(l, db.ThreadLink{Kind: db.LinkPR, Repo: "acme/widgets", Number: number, State: db.LinkStateOpen, Head: th.Branch, Base: th.BaseBranch, Role: db.LinkOpened})
	}))
}
