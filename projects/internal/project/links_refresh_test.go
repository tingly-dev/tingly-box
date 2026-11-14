package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// openedPR gives the worker an open PR of its own, read just now.
func (f *linksFixture) openedPR(t *testing.T) *db.ThreadLink {
	t.Helper()
	it := f.gh.add(db.LinkPR, "Add a", func(it *GitHubItem) { it.CI, it.MergeState = "pending", "unknown" })
	l, err := f.links.record(f.thread(t), GitHubRepo{Owner: "acme", Name: "widgets"}, db.LinkPR, db.LinkOpened, it)
	require.NoError(t, err)
	f.events = nil
	return l
}

func (f *linksFixture) reads() int {
	n := 0
	for _, c := range f.gh.called() {
		if strings.HasPrefix(c, "pr ") || strings.HasPrefix(c, "issue ") {
			n++
		}
	}
	return n
}

func (f *linksFixture) link(t *testing.T, n int) *db.ThreadLink {
	t.Helper()
	l, err := f.store.FindThreadLink(f.worker.ID, "acme/widgets", n)
	require.NoError(t, err)
	return l
}

func TestSweepReadsOpenItemsOnSchedule(t *testing.T) {
	f := newLinksFixture(t)
	l := f.openedPR(t)
	ctx := context.Background()

	f.links.Sweep(ctx)
	assert.Equal(t, 0, f.reads(), "just read: not due")

	f.advance(linkInterval - linkSweepEvery/4 - time.Second)
	f.links.Sweep(ctx)
	assert.Equal(t, 0, f.reads(), "not yet due")

	// A tick that lands a few seconds before the exact interval still counts.
	f.advance(2 * time.Second)
	f.gh.set(l.Number, func(it *GitHubItem) { it.CI, it.MergeState = "passing", "ready" })
	f.links.Sweep(ctx)
	assert.Equal(t, 1, f.reads())
	got := f.link(t, l.Number)
	assert.Equal(t, "passing", got.CI)
	assert.Equal(t, "ready", got.MergeState)
	assert.Equal(t, []string{f.worker.ID}, f.events, "a changed reading is published")

	// An unchanged reading moves the clock and publishes nothing.
	f.events = nil
	f.advance(linkInterval)
	f.links.Sweep(ctx)
	assert.Equal(t, 2, f.reads())
	assert.Empty(t, f.events)
	assert.Equal(t, f.clock.Unix(), f.link(t, l.Number).RefreshedAt.Unix())
}

func TestSweepNeverReadsAFinishedItemAgain(t *testing.T) {
	f := newLinksFixture(t)
	l := f.openedPR(t)
	f.gh.set(l.Number, func(it *GitHubItem) { it.State = "merged" })
	f.advance(linkInterval)
	f.links.Sweep(context.Background())
	require.Equal(t, 1, f.reads())
	assert.Equal(t, db.LinkStateMerged, f.link(t, l.Number).State)

	f.advance(24 * time.Hour)
	f.links.Sweep(context.Background())
	assert.Equal(t, 1, f.reads(), "merged is final")
}

func TestSweepBacksOffAfterFailuresAndKeepsTheLastReading(t *testing.T) {
	f := newLinksFixture(t)
	l := f.openedPR(t)
	ctx := context.Background()
	f.gh.set(l.Number, func(it *GitHubItem) { it.CI = "failing" })
	f.gh.fail = errors.New("HTTP 403 rate limited")

	// The waits double: 5, 10, 20, 40, 60 (the cap), 60 minutes.
	for i, want := range []time.Duration{5, 10, 20, 40, 60, 60} {
		wait := want * time.Minute
		f.advance(wait - linkSweepEvery/4 - 2*time.Second)
		before := f.reads()
		f.links.Sweep(ctx)
		require.Equal(t, before, f.reads(), "step %d: not read before %s", i, wait)
		f.advance(3 * time.Second)
		f.links.Sweep(ctx)
		require.Equal(t, before+1, f.reads(), "step %d: read once when due", i)
		got := f.link(t, l.Number)
		assert.Equal(t, i+1, got.Failures)
		assert.Contains(t, got.LastError, "rate limited")
		assert.Equal(t, "pending", got.CI, "the last good reading stays")
	}
}

func TestOneSuccessClearsTheBackoff(t *testing.T) {
	f := newLinksFixture(t)
	l := f.openedPR(t)
	ctx := context.Background()
	f.gh.fail = errors.New("boom")
	f.advance(linkInterval)
	f.links.Sweep(ctx)
	require.Equal(t, 1, f.link(t, l.Number).Failures)
	assert.Equal(t, []string{f.worker.ID}, f.events, "the first failure is shown")

	f.gh.fail = nil
	f.gh.set(l.Number, func(it *GitHubItem) { it.CI = "passing" })
	f.advance(2 * linkInterval)
	f.links.Sweep(ctx)
	got := f.link(t, l.Number)
	assert.Equal(t, 0, got.Failures)
	assert.Empty(t, got.LastError)
	assert.Equal(t, "passing", got.CI)
}

func TestSweepIsBoundedAndSkipsArchivedProjects(t *testing.T) {
	f := newLinksFixture(t)
	for i := 0; i < linkSweepMax+5; i++ {
		f.openedPR(t)
	}
	f.advance(linkInterval)
	f.links.Sweep(context.Background())
	assert.Equal(t, linkSweepMax, f.reads(), "one sweep reads at most the cap")
	f.links.Sweep(context.Background())
	assert.Equal(t, linkSweepMax+5, f.reads(), "the rest follow on the next sweep, the ones just read are not due")

	p, err := f.store.GetProject(f.p.ID)
	require.NoError(t, err)
	now := time.Now()
	p.ArchivedAt = &now
	require.NoError(t, f.store.SaveProject(p))
	f.advance(24 * time.Hour)
	f.links.Sweep(context.Background())
	assert.Equal(t, linkSweepMax+5, f.reads(), "an archived project is left alone")
}

func TestThreadsCarryTheirLinks(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	th := &db.Thread{ProjectID: f.project.ID, Agent: db.AgentClaudeCode, Title: "W"}
	require.NoError(t, f.store.CreateThread(th))
	require.NoError(t, f.store.UpsertThreadLink(&db.ThreadLink{ThreadID: th.ID, ProjectID: f.project.ID, Kind: db.LinkPR, Repo: "acme/widgets", Number: 5, State: db.LinkStateOpen, CI: "failing"}))
	threads := []db.Thread{*th}
	f.threads.Annotate(threads)
	require.Len(t, threads[0].Links, 1)
	assert.Equal(t, 5, threads[0].Links[0].Number)
	assert.Equal(t, " prs=#5(open checks=failing)", linksNote(threads[0].Links))
	one := *th
	f.threads.AnnotateOne(&one)
	assert.Len(t, one.Links, 1)
}

func TestGetThreadShowsLinksToTheCoordinator(t *testing.T) {
	f := newLinksFixture(t)
	l := f.openedPR(t)
	f.gh.set(l.Number, func(it *GitHubItem) { it.CI, it.MergeState = "failing", "conflicts" })
	f.advance(linkInterval)
	f.links.Sweep(context.Background())

	f.svc.Coordinator.threads = &Threads{store: f.store, backgroundCount: func(string) int { return 0 }}
	cs, _ := f.client(t, f.p.MainThreadID)
	out, isErr := f.call(t, cs, "get_thread", map[string]any{"thread_id": f.worker.ID[:8]})
	require.False(t, isErr, out)
	assert.Contains(t, out, "issues and PRs")
	assert.Contains(t, out, "checks=failing")
	assert.Contains(t, out, "merge=conflicts")
	assert.Contains(t, out, " ago")
	list, _ := f.call(t, cs, "list_threads", map[string]any{})
	assert.Contains(t, list, "prs=#")
}

func TestGitHubToolPermissionRules(t *testing.T) {
	rules := func(access, mode string) (allow, ask []string) {
		doc := map[string]any{}
		grantGitHubTools(doc, &db.Project{ThreadGitHubAccess: access}, mode)
		perms := permissionsOf(doc)
		for _, k := range []string{"allow", "ask"} {
			list, _ := perms[k].([]any)
			for _, r := range list {
				if k == "allow" {
					allow = append(allow, r.(string))
				} else {
					ask = append(ask, r.(string))
				}
			}
		}
		return
	}
	status := githubToolRule(toolView)
	var writes []string
	for _, tool := range githubWriteTools {
		writes = append(writes, githubToolRule(tool))
	}

	allow, ask := rules("", "auto")
	assert.Equal(t, []string{status}, allow, "reading is always allowed")
	assert.ElementsMatch(t, writes, ask, "by default, in auto mode, the writing tools ask")

	allow, ask = rules(GitHubAsk, "default")
	assert.Equal(t, []string{status}, allow)
	assert.Empty(t, ask, "the other modes prompt on their own")

	allow, ask = rules(GitHubAllow, "default")
	assert.ElementsMatch(t, append([]string{status}, writes...), allow)
	assert.Empty(t, ask)
	allow, _ = rules(GitHubAllow, "auto")
	assert.ElementsMatch(t, append([]string{status}, writes...), allow)

	// The Bash commands keep asking whatever the project says.
	doc := map[string]any{}
	grantAutoGit(doc, &db.Project{ThreadGitHubAccess: GitHubAllow, ThreadPermissionMode: "auto"}, &db.Thread{Branch: "tp/x", BaseBranch: "main"})
	assert.Contains(t, permissionsOf(doc)["ask"], "Bash(gh pr create *)")
	assert.Contains(t, permissionsOf(doc)["ask"], "Bash(gh pr merge *)")
}

func TestThreadSettingsFileCarriesTheGitHubRules(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	p, err := f.store.GetProject(f.project.ID)
	require.NoError(t, err)
	p.ThreadGitHubAccess = GitHubAllow
	th := &db.Thread{ID: "t1", ProjectID: p.ID}
	require.NoError(t, f.threads.writeSettings(p, th))
	b, err := os.ReadFile(f.threads.layout.SettingsFile(p.ID, th.ID))
	require.NoError(t, err)
	assert.Contains(t, string(b), "mcp__tingly-projects__create_pr")
	assert.Contains(t, string(b), "mcp__tingly-projects__view")
}

func TestRedact(t *testing.T) {
	for in, want := range map[string]string{
		"token ghp_abcdefghijklmnopqrstuvwxyz0123456789 end": "token [redacted] end",
		"github_pat_11ABCDEFG0abcdefghijklmnopqrstuvwxyz":    "[redacted]",
		"Authorization: Bearer abc.def-ghi":                  "Authorization: Bearer [redacted]",
		"GH_TOKEN=abc123 gh":                                 "GH_TOKEN=[redacted] gh",
		"fatal: https://user:p4ss@github.com/a/b.git":        "fatal: https://github.com/a/b.git",
		"https://x-access-token:ghs_abc@github.com/a/b":      "https://github.com/a/b",
		"nothing secret here":                                "nothing secret here",
	} {
		assert.Equal(t, want, redact(in), in)
	}
}

// fakeGH writes a gh executable that records its arguments and body and
// answers from a canned file, so ghCLI is exercised with no network.
func fakeGH(t *testing.T, answer string, exit int) (bin, logFile string) {
	t.Helper()
	dir := t.TempDir()
	logFile = filepath.Join(dir, "log")
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> " + logFile + "\n" +
		"cat >> " + logFile + "\n" +
		"if [ " + string(rune('0'+exit)) + " -ne 0 ]; then echo 'gh: HTTP 422 Unprocessable ghp_abcdefghijklmnopqrstuvwxyz0123456789' >&2; exit " + string(rune('0'+exit)) + "; fi\n" +
		"cat <<'EOF'\n" + answer + "\nEOF\n"
	bin = filepath.Join(dir, "gh")
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	return bin, logFile
}

func TestGhCLIUsesRESTWithAJSONBody(t *testing.T) {
	bin, logFile := fakeGH(t, `{"number":7,"html_url":"https://github.com/acme/widgets/pull/7","title":"T","state":"open","head":{"ref":"tp/x","sha":"abc"},"base":{"ref":"main"},"mergeable_state":"dirty"}`, 0)
	g := ghCLI{bin: bin}
	repo := GitHubRepo{Owner: "acme", Name: "widgets"}

	title, body := "New \"title\"", "line 1\nline 2 $(rm -rf /) `x`"
	it, err := g.UpdatePR(context.Background(), repo, 7, &title, &body, nil)
	require.NoError(t, err)
	assert.Equal(t, 7, it.Number)
	assert.Equal(t, "conflicts", it.MergeState)
	b, err := os.ReadFile(logFile)
	require.NoError(t, err)
	got := string(b)
	assert.Contains(t, got, "api -X PATCH --input - repos/acme/widgets/pulls/7", "REST, not `gh pr edit`")
	assert.Contains(t, got, `"title":"New \"title\""`)
	assert.Contains(t, got, `$(rm -rf /)`, "the body travels as data, never through a shell")
	assert.NotContains(t, got, `"base"`, "an unset field is not sent")

	// An issue is edited through the issues endpoint, with only the fields given.
	_, err = g.UpdateIssue(context.Background(), repo, 9, &title, nil)
	require.NoError(t, err)
	b, _ = os.ReadFile(logFile)
	assert.Contains(t, string(b), "api -X PATCH --input - repos/acme/widgets/issues/9")

	_, err = g.CreatePR(context.Background(), repo, "T", "B", "tp/x", "main", true)
	require.NoError(t, err)
	b, _ = os.ReadFile(logFile)
	assert.Contains(t, string(b), "api -X POST --input - repos/acme/widgets/pulls")
	assert.Contains(t, string(b), `"draft":true`)
}

func TestGhCLIErrorsAreRedactedAndGhMissingIsSaid(t *testing.T) {
	bin, _ := fakeGH(t, "", 1)
	_, err := ghCLI{bin: bin}.CreateIssue(context.Background(), GitHubRepo{Owner: "a", Name: "b"}, "t", "b")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 422")
	assert.NotContains(t, err.Error(), "ghp_abcdef")

	_, err = ghCLI{bin: filepath.Join(t.TempDir(), "no-gh")}.DefaultBranch(context.Background(), GitHubRepo{Owner: "a", Name: "b"})
	assert.ErrorIs(t, err, ErrGitHubUnavailable)
}
