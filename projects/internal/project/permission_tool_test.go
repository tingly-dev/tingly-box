package project

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// request_permission puts a thread's request to the user on a card, records
// the answer and, on a yes, a grant. See .design/thread-permissions.md §3.

// askPermission runs RequestPermission and answers its card with ans.
func askPermission(t *testing.T, f *testFixture, threadID string, in PermissionRequest, ans Answer) string {
	t.Helper()
	th, err := f.store.GetThread(threadID)
	require.NoError(t, err)
	type result struct {
		out string
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := f.threads.RequestPermission(context.Background(), th, in)
		done <- result{out, err}
	}()
	var pending []db.ThreadEvent
	require.Eventually(t, func() bool { pending = f.threads.PendingRequests(threadID); return len(pending) == 1 }, 5*time.Second, 5*time.Millisecond)
	assert.Contains(t, pending[0].Content, "The thread asks for permission: "+in.Action)
	select {
	case <-done:
		t.Fatal("the call returned before the user answered")
	case <-time.After(30 * time.Millisecond):
	}
	var p requestPayload
	require.NoError(t, jsonUnmarshal(pending[0].Payload, &p))
	require.NoError(t, f.threads.Respond(threadID, p.RequestID, ans))
	r := <-done
	require.NoError(t, r.err)
	return r.out
}

func startedThread(t *testing.T, f *testFixture) *db.Thread {
	t.Helper()
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Ask", Task: "x"})
	require.NoError(t, err)
	f.report(t)
	return th
}

func TestRequestPermissionRecordsTheAnswerAndAGrant(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("done", 1, 1); return nil })
	th := startedThread(t, f)

	out := askPermission(t, f, th.ID, PermissionRequest{Action: "close #2644", Reason: "superseded by #2666", Group: GitHubClose}, Answer{Allow: false, Answer: "keep it for now"})
	assert.Contains(t, out, "denied")
	assert.Contains(t, out, "keep it for now")
	got, _ := f.store.GetThread(th.ID)
	assert.Empty(t, got.Grants, "a no grants nothing")

	out = askPermission(t, f, th.ID, PermissionRequest{Action: "close #2644", Reason: "superseded", Group: GitHubClose}, Answer{Allow: true})
	assert.Contains(t, out, "granted")
	assert.Contains(t, out, "the next close call runs without its card")
	got, _ = f.store.GetThread(th.ID)
	require.Len(t, got.Grants, 1)
	assert.Equal(t, db.GrantOnce, got.Grants[0].Scope)
	assert.Equal(t, GitHubClose, got.Grants[0].Group)

	var notes []string
	events, _, _ := f.store.PageThreadEvents(th.ID, db.EventPage{Limit: 50})
	for _, e := range events {
		if e.Kind == db.EvSystem {
			notes = append(notes, e.Content)
			if e.Content != "" {
				assert.Contains(t, e.Payload, `"user_answer"`, "the note carries the user's answer as a fact")
			}
		}
	}
	assert.Contains(t, notes, "The user denied: close #2644 (keep it for now)")
	assert.Contains(t, notes, "The user granted: close #2644 (once; request "+got.Grants[0].ID[:8]+")")

	// Allow for this thread on the card widens the request to the thread.
	askPermission(t, f, th.ID, PermissionRequest{Action: "merge the stack", Reason: "asked", Group: GitHubMerge}, Answer{Allow: true, Scope: "thread"})
	got, _ = f.store.GetThread(th.ID)
	require.Len(t, got.Grants, 2)
	assert.Equal(t, db.GrantThread, got.Grants[1].Scope)

	// Closing the thread ends what was granted to it.
	_, err := f.threads.Resolve(th.ID)
	require.NoError(t, err)
	got, _ = f.store.GetThread(th.ID)
	assert.Empty(t, got.Grants)
}

func TestRequestPermissionRefusesWhatACardCannotWeigh(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("done", 1, 1); return nil })
	th := startedThread(t, f)
	for _, in := range []PermissionRequest{
		{Action: "x", Reason: "y", Rule: "Bash", Scope: db.GrantThread},
		{Action: "x", Reason: "y", Rule: "Bash(*)", Scope: db.GrantThread},
		{Action: "x", Reason: "y", Rule: "Bash(:*)", Scope: db.GrantThread},
		{Action: "x", Reason: "y", Rule: "mcp__tingly-projects__merge(x)", Scope: db.GrantThread},
		{Action: "x", Reason: "y", Rule: "Bash(task codegen)"}, // a rule cannot be for one call
		{Action: "x", Reason: "y", Group: GitHubMerge, Rule: "Bash(task codegen)", Scope: db.GrantThread},
		{Action: "x", Reason: "y", Group: "everything"},
		{Action: "x", Reason: "y", Group: GitHubMerge, Scope: db.GrantThread, Minutes: -1},
		{Action: "", Reason: "y"},
	} {
		_, err := f.threads.RequestPermission(context.Background(), th, in)
		assert.Error(t, err, "%+v", in)
	}
	assert.Empty(t, f.threads.PendingRequests(th.ID), "a refused request asks nobody")
}

func TestARuleGrantGoesIntoTheThreadsSettingsUntilItExpires(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("done", 1, 1); return nil })
	th := startedThread(t, f)
	out := askPermission(t, f, th.ID, PermissionRequest{Action: "run task codegen", Reason: "regenerate the client", Rule: "Bash(task codegen)", Scope: db.GrantThread, Minutes: 30}, Answer{Allow: true})
	assert.Contains(t, out, "Bash(task codegen) is in this thread's settings")
	allow, _ := settingsPerms(t, f.threads.layout.SettingsFile(f.project.ID, th.ID))
	assert.Contains(t, allow, "Bash(task codegen)")

	// Past its time the rule is gone the next time the settings are written.
	got, _ := f.store.GetThread(th.ID)
	_, err := f.store.UpdateThreadGrants(th.ID, func(gs []db.ThreadGrant) []db.ThreadGrant {
		past := time.Now().Add(-time.Minute)
		gs[0].ExpiresAt = &past
		return gs
	})
	require.NoError(t, err)
	got, _ = f.store.GetThread(th.ID)
	require.NoError(t, f.threads.writeSettings(f.project, got))
	allow, _ = settingsPerms(t, f.threads.layout.SettingsFile(f.project.ID, th.ID))
	assert.NotContains(t, allow, "Bash(task codegen)")
	_, err = os.Stat(f.threads.layout.SettingsFile(f.project.ID, th.ID))
	require.NoError(t, err)
}

// The GitHub gate honours a group grant when the switch is off: one for a call
// is used up, one for the thread lasts until it expires.
func TestTheGateHonoursAGroupGrant(t *testing.T) {
	f := newLinksFixture(t)
	f.setAuto(t, GitHubPublish)
	cs, _ := f.client(t, f.worker.ID)
	grant := func(g db.ThreadGrant) {
		_, err := f.store.UpdateThreadGrants(f.worker.ID, func(gs []db.ThreadGrant) []db.ThreadGrant { return append(gs, g) })
		require.NoError(t, err)
	}
	a := f.gh.add(db.LinkIssue, "A", nil)
	b := f.gh.add(db.LinkIssue, "B", nil)

	grant(db.ThreadGrant{ID: "g1", Group: GitHubClose, Scope: db.GrantOnce})
	out, isErr := f.call(t, cs, "close", map[string]any{"number": a.Number})
	require.False(t, isErr, out)
	assert.Empty(t, f.approvals, "the grant stood in for the card")
	out, isErr = f.call(t, cs, "close", map[string]any{"number": b.Number})
	assert.True(t, isErr, out)
	assert.Len(t, f.approvals, 1, "a grant for one call is used up")

	exp := f.clock.Add(10 * time.Minute)
	grant(db.ThreadGrant{ID: "g2", Group: GitHubClose, Scope: db.GrantThread, ExpiresAt: &exp})
	out, isErr = f.call(t, cs, "close", map[string]any{"number": b.Number})
	require.False(t, isErr, out)
	c := f.gh.add(db.LinkIssue, "C", nil)
	f.advance(11 * time.Minute)
	out, isErr = f.call(t, cs, "close", map[string]any{"number": c.Number})
	assert.True(t, isErr, out)
	assert.Len(t, f.approvals, 2, "an expired grant is no grant")

	// What is never automatic is not covered by any grant.
	grant(db.ThreadGrant{ID: "g3", Group: GitHubPublish, Scope: db.GrantThread})
	f.gh.protected = map[string]bool{"release": true}
	f.stackBranch(t, "release", "main")
	out, isErr = f.call(t, cs, "create_pr", map[string]any{"title": "R", "head": "release"})
	assert.True(t, isErr, out)
	assert.Len(t, f.approvals, 3)
}

func TestGrantRulesThatReachPastTheGateOrRunAnythingAreRefused(t *testing.T) {
	for _, r := range []string{"Bash(gh pr merge *)", "Bash(gh:*)", "Bash(git push --force *)", "Bash(git push:*)", "Bash(bash -c *)", "Bash(sh *)", "Bash(env FOO=1 *)", "Bash(xargs *)", "Bash(sudo make *)", "WebFetch(domain:*)", "Bash(* )"} {
		assert.Error(t, checkGrantRule(r), r)
	}
	for _, r := range []string{"Bash(task codegen)", "Bash(git status)", "Bash(npm run build *)", "Bash(./bin/tingly-project *)", "WebFetch(domain:github.com)", "Bash(ghostscript *)", "Bash(shellcheck *)"} {
		assert.NoError(t, checkGrantRule(r), r)
	}
}

// A thread closed while its card waited gets nothing granted.
func TestNoGrantForAThreadClosedWhileTheCardWaited(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("done", 1, 1); return nil })
	th := startedThread(t, f)
	done := make(chan error, 1)
	go func() {
		_, err := f.threads.RequestPermission(context.Background(), th, PermissionRequest{Action: "merge", Reason: "asked", Group: GitHubMerge})
		done <- err
	}()
	var pending []db.ThreadEvent
	require.Eventually(t, func() bool { pending = f.threads.PendingRequests(th.ID); return len(pending) == 1 }, 5*time.Second, 5*time.Millisecond)
	var p requestPayload
	require.NoError(t, jsonUnmarshal(pending[0].Payload, &p))
	_, err := f.store.UpdateThreadGrants(th.ID, func(gs []db.ThreadGrant) []db.ThreadGrant { return gs })
	require.NoError(t, err)
	_, err = f.threads.update(th.ID, func(t *db.Thread) { now := time.Now(); t.ResolvedAt = &now })
	require.NoError(t, err)
	require.NoError(t, f.threads.Respond(th.ID, p.RequestID, Answer{Allow: true}))
	assert.ErrorContains(t, <-done, "closed while the user answered")
	got, _ := f.store.GetThread(th.ID)
	assert.Empty(t, got.Grants)
}
