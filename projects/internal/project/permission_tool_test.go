package project

import (
	"context"
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

// request_permission puts a thread's request to the user on a card, records
// the answer and, on a yes, a grant its own Claude Code honours. See
// .design/thread-permissions.md §3.

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
	settings := f.threads.layout.SettingsFile(f.project.ID, th.ID)

	out := askPermission(t, f, th.ID, PermissionRequest{Action: "regenerate the client", Reason: "the API changed", Command: "task codegen"}, Answer{Allow: false, Answer: "not now"})
	assert.Contains(t, out, "denied")
	assert.Contains(t, out, "not now")
	assert.Contains(t, out, "Do not run it")
	got, _ := f.store.GetThread(th.ID)
	assert.Empty(t, got.Grants, "a no grants nothing")
	allow, _ := settingsPerms(t, settings)
	assert.NotContains(t, allow, "Bash(task codegen)", "a no allows nothing")

	out = askPermission(t, f, th.ID, PermissionRequest{Action: "regenerate the client", Reason: "the API changed", Command: "task codegen"}, Answer{Allow: true})
	assert.Contains(t, out, "granted")
	assert.Contains(t, out, "run it now with your Bash tool, exactly as written: task codegen", "the thread is told to run it, not to ask again")
	got, _ = f.store.GetThread(th.ID)
	require.Len(t, got.Grants, 1)
	assert.Equal(t, db.GrantOnce, got.Grants[0].Scope)
	assert.Equal(t, "task codegen", got.Grants[0].Command)
	allow, _ = settingsPerms(t, settings)
	assert.Contains(t, allow, "Bash(task codegen)", "the grant is in the settings the thread's Claude Code runs on")

	events, _, _ := f.store.PageThreadEvents(th.ID, db.EventPage{Limit: 50})
	for _, e := range events {
		if e.Kind == db.EvSystem && strings.HasPrefix(e.Content, "The user ") {
			assert.Contains(t, e.Payload, `"user_answer"`, "the note carries the user's answer as a fact")
		}
	}
	notes := systemNotes(t, f.store, th.ID)
	assert.Contains(t, notes, "The user denied: regenerate the client (not now)")
	assert.Contains(t, notes, "The user granted: regenerate the client (for this turn; request "+got.Grants[0].ID[:8]+")")

	// Allow for this thread on the card widens the request to the thread.
	askPermission(t, f, th.ID, PermissionRequest{Action: "build", Reason: "asked", Rule: "Bash(npm run build *)"}, Answer{Allow: true, Scope: "thread"})
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
		{Action: "x", Reason: "y", Rule: "Bash"},
		{Action: "x", Reason: "y", Rule: "Bash(*)"},
		{Action: "x", Reason: "y", Rule: "Bash(:*)"},
		{Action: "x", Reason: "y", Rule: "Bash(python3 -c *)"}, // auto mode drops it: granted and still blocked
		{Action: "x", Reason: "y", Rule: "mcp__tingly-projects__merge(x)"},
		{Action: "x", Reason: "y", Command: "task codegen", Rule: "Bash(task codegen)"},
		{Action: "x", Reason: "y", Command: "rm build/*.o"}, // a * widens the rule past the card
		{Action: "x", Reason: "y", Command: "gh pr merge 12"},
		{Action: "x", Reason: "y", Command: "make && git push --force"},
		{Action: "x", Reason: "y", Command: "task codegen", Scope: "forever"},
		{Action: "x", Reason: "y", Rule: "Bash(npm run build *)", Scope: db.GrantThread, Minutes: -1},
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
	out := askPermission(t, f, th.ID, PermissionRequest{Action: "run the tool", Reason: "build the release", Rule: "Bash(./bin/tingly-project *)", Scope: db.GrantThread, Minutes: 30}, Answer{Allow: true})
	assert.Contains(t, out, "Bash(./bin/tingly-project *) holds from your next tool call")
	allow, _ := settingsPerms(t, f.threads.layout.SettingsFile(f.project.ID, th.ID))
	assert.Contains(t, allow, "Bash(./bin/tingly-project *)")

	// Past its time the rule is gone the next time the settings are written.
	_, err := f.store.UpdateThreadGrants(th.ID, func(gs []db.ThreadGrant) []db.ThreadGrant {
		past := time.Now().Add(-time.Minute)
		gs[0].ExpiresAt = &past
		return gs
	})
	require.NoError(t, err)
	got, _ := f.store.GetThread(th.ID)
	require.NoError(t, f.threads.writeSettings(f.project, got))
	allow, _ = settingsPerms(t, f.threads.layout.SettingsFile(f.project.ID, th.ID))
	assert.NotContains(t, allow, "Bash(./bin/tingly-project *)")
}

// A pipeline or list is checked by Claude Code part by part, so a command
// grant allows each part as written, and nothing else.
func TestACommandGrantAllowsTheLineAndEachOfItsParts(t *testing.T) {
	assert.Equal(t, []string{"Bash(task codegen)"}, commandRules("task codegen"))
	assert.Equal(t, []string{"Bash(./tool.sh a | tee out.txt)", "Bash(./tool.sh a)", "Bash(tee out.txt)"}, commandRules("./tool.sh a | tee out.txt"))
	assert.Equal(t, []string{"Bash(cd web && pnpm gen:api; echo ok)", "Bash(cd web)", "Bash(pnpm gen:api)", "Bash(echo ok)"}, commandRules("cd web && pnpm gen:api; echo ok"))
}

// One request, one yes: the thread's own Bash call of the granted command
// runs in the same turn, the thread records that it ran and how it ended, and
// a grant for the turn is used up by it.
func TestAGrantedCommandRunsInTheThreadsOwnCallAndIsRecorded(t *testing.T) {
	granted := make(chan struct{})
	turns := 0
	f := newFixture(t, func(turn *enginetest.Turn) error {
		turns++
		if turns == 2 {
			<-granted
			turn.Use("tu-1", "Bash", map[string]any{"command": "task codegen"})
			turn.Result("tu-1", "generated", false)
		}
		turn.Say("done", 1, 1)
		return nil
	})
	th := startedThread(t, f)
	_, err := f.threads.Send(context.Background(), th.ID, "regenerate", Source{}, nil)
	require.NoError(t, err)
	out := askPermission(t, f, th.ID, PermissionRequest{Action: "regenerate the client", Reason: "the API changed", Command: "task codegen"}, Answer{Allow: true})
	require.Contains(t, out, "run it now")
	close(granted)
	f.report(t)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)

	got, _ := f.store.GetThread(th.ID)
	assert.Empty(t, got.Grants, "a grant for the turn is used up by the call that ran it")
	notes := systemNotes(t, f.store, th.ID)
	ran := 0
	for _, n := range notes {
		if strings.HasPrefix(n, "Ran with the user's grant") {
			ran++
			assert.Contains(t, n, "task codegen; it succeeded")
		}
		assert.NotContains(t, n, "was not run")
	}
	assert.Equal(t, 1, ran, "the run is recorded once, under the call")
	assert.Len(t, f.threads.PendingRequests(th.ID), 0, "no second card")
}

// A grant for the turn that the turn did not use ends with it, and the thread
// says so.
func TestAnUnusedGrantForTheTurnEndsWithIt(t *testing.T) {
	granted := make(chan struct{})
	turns := 0
	f := newFixture(t, func(turn *enginetest.Turn) error {
		turns++
		if turns == 2 {
			<-granted
		}
		turn.Say("done", 1, 1)
		return nil
	})
	th := startedThread(t, f)
	_, err := f.threads.Send(context.Background(), th.ID, "regenerate", Source{}, nil)
	require.NoError(t, err)
	askPermission(t, f, th.ID, PermissionRequest{Action: "regenerate", Reason: "asked", Command: "task codegen"}, Answer{Allow: true})
	askPermission(t, f, th.ID, PermissionRequest{Action: "fetch", Reason: "asked", Rule: "WebFetch(domain:example.com)", Scope: db.GrantThread}, Answer{Allow: true})
	close(granted)
	f.report(t)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)

	got, _ := f.store.GetThread(th.ID)
	require.Len(t, got.Grants, 1, "only the grant for the thread is left")
	assert.Equal(t, "WebFetch(domain:example.com)", got.Grants[0].Rule)
	found := false
	for _, n := range systemNotes(t, f.store, th.ID) {
		found = found || strings.Contains(n, "The grant for task codegen") && strings.Contains(n, "the command was not run")
	}
	assert.True(t, found)
	require.NoError(t, f.threads.writeSettings(f.project, got))
	allow, _ := settingsPerms(t, f.threads.layout.SettingsFile(f.project.ID, th.ID))
	assert.NotContains(t, allow, "Bash(task codegen)", "the next turn's settings no longer allow it")
}

// A thread under a profile runs on settings built from the profile's: a grant
// is written there at once, not only at the next turn.
func TestAGrantReachesAThreadUnderAProfileAtOnce(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("done", 1, 1); return nil })
	work := filepath.Join(t.TempDir(), "work.json")
	require.NoError(t, os.WriteFile(work, []byte(`{"permissions":{"allow":["Read"]}}`), 0o600))
	f.threads.env = profiles{settings: work}
	f.project.ThreadProfile = "work"
	require.NoError(t, f.store.SaveProject(f.project))
	th := startedThread(t, f)
	out := askPermission(t, f, th.ID, PermissionRequest{Action: "regenerate", Reason: "asked", Command: "task codegen"}, Answer{Allow: true})
	assert.Contains(t, out, "run it now")
	allow, _ := settingsPerms(t, f.threads.layout.SessionSettingsFile(th.ID))
	assert.Contains(t, allow, "Bash(task codegen)")
	assert.Contains(t, allow, "Read", "the profile's own rules stay")
}

// A thread closed while its card waited gets nothing granted.
func TestNoGrantForAThreadClosedWhileTheCardWaited(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("done", 1, 1); return nil })
	th := startedThread(t, f)
	done := make(chan error, 1)
	go func() {
		_, err := f.threads.RequestPermission(context.Background(), th, PermissionRequest{Action: "regenerate", Reason: "asked", Command: "task codegen"})
		done <- err
	}()
	var pending []db.ThreadEvent
	require.Eventually(t, func() bool { pending = f.threads.PendingRequests(th.ID); return len(pending) == 1 }, 5*time.Second, 5*time.Millisecond)
	var p requestPayload
	require.NoError(t, jsonUnmarshal(pending[0].Payload, &p))
	_, err := f.threads.update(th.ID, func(t *db.Thread) { now := time.Now(); t.ResolvedAt = &now })
	require.NoError(t, err)
	require.NoError(t, f.threads.Respond(th.ID, p.RequestID, Answer{Allow: true}))
	assert.ErrorContains(t, <-done, "closed while the user answered")
	got, _ := f.store.GetThread(th.ID)
	assert.Empty(t, got.Grants)
}

func TestGrantRulesThatReachPastTheGateOrRunAnythingAreRefused(t *testing.T) {
	for _, r := range []string{"Bash(gh pr merge *)", "Bash(gh:*)", "Bash(git push --force *)", "Bash(git push:*)", "Bash(bash -c *)", "Bash(sh *)", "Bash(env FOO=1 *)", "Bash(xargs *)", "Bash(sudo make *)", "Bash(node -e *)", "Bash(python3 *)", "WebFetch(domain:*)", "Bash(* )"} {
		assert.Error(t, checkGrantRule(r), r)
	}
	for _, r := range []string{"Bash(task codegen)", "Bash(git status)", "Bash(npm run build *)", "Bash(./bin/tingly-project *)", "WebFetch(domain:github.com)", "Bash(ghostscript *)", "Bash(shellcheck *)", "Bash(nodemon *)"} {
		assert.NoError(t, checkGrantRule(r), r)
	}
}
