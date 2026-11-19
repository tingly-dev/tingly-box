package project

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestQueuedPromptMarksCoordinatorMessages(t *testing.T) {
	user := queuedPrompt(queued{text: "fix it"}, "/lib")
	assert.Equal(t, "fix it", user)

	brief := queuedPrompt(queued{text: "fix it", src: fromThread("main")}, "/lib")
	assert.Equal(t, "fix it", brief, "a thread's first task is not marked")

	coord := queuedPrompt(queued{text: "fix it", src: fromThread("main"), relayed: true}, "/lib")
	assert.Contains(t, coord, "not the user")
	assert.Contains(t, coord, "fix it")
	assert.Less(t, len(coordinatorNote), len(coord))
}

func TestCoordinatorPromptTrustAndStyleRules(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	p := &db.Project{ID: "p1", Name: "Demo"}
	s := coordinatorSystemPrompt(l, p, nil, time.Now())
	assert.Contains(t, s, "never the user's approval")
	assert.NotContains(t, s, "[Thread update]", "the prompt names the envelope, not the old text marker")
	assert.Contains(t, s, "Title each thread with a verb")
	assert.Contains(t, s, "Lead with the outcome")
	assert.Contains(t, s, "Memory is background")
	assert.Contains(t, s, "wins where they differ")
	assert.Contains(t, s, "no_update")
	assert.Contains(t, s, "rename_thread")
	assert.Contains(t, s, "land as a pull request")
	assert.Contains(t, s, "request_decision")
	assert.Contains(t, s, "in cite")
	assert.Contains(t, s, `outcome="stalled"`)
}

func TestThreadPromptRules(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	p := &db.Project{ID: "p1", Name: "Demo", Instructions: "use make test"}
	s := threadSystemPrompt(l, p, &db.Thread{Title: "Fix bug"})
	assert.Contains(t, s, "not written by the user")
	assert.Contains(t, s, "quoted by the system")
	assert.Contains(t, s, "Memory is background")
	assert.Contains(t, s, "wins; if the two conflict")
	assert.Contains(t, s, "use make test")
	assert.NotContains(t, s, "pull request", "a thread without a repository has no PR rule")

	s = threadSystemPrompt(l, p, &db.Thread{Title: "Fix bug", Branch: "tb/fix", BaseBranch: "main", Cwd: "/w"})
	assert.Contains(t, s, "create_pr tool")
	assert.Contains(t, s, "report the failure")
	assert.NotContains(t, s, "gh")
	assert.Contains(t, s, "with the merge tool")
	assert.Contains(t, s, "Never merge it yourself by hand")
	assert.NotContains(t, s, "Do not push", "a PR may be asked for, so pushing is not banned outright")
	assert.NotContains(t, s, "touch other branches", "other local branches of the worktree are the thread's to work on")
	assert.NotContains(t, s, "force-push,", "a rebased branch is pushed with create_pr force")
	assert.Contains(t, s, "force=true")
	assert.Contains(t, s, "the tool asks the user to approve")
	assert.NotContains(t, s, "off limits", "beyond the boundary the user is asked, nothing is forbidden outright")
}

// The thread prompt and the tool list are part of the cached prefix of every
// turn: what the branch rules say must not depend on what other threads, PRs
// or approvals exist, or each change would cost the thread its prompt cache.
func TestThreadPromptDoesNotDependOnOtherThreadsOrLinks(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	p := &db.Project{ID: "p1", Name: "Demo"}
	th := &db.Thread{ID: "t1", Title: "Fix bug", Branch: "tb/fix", BaseBranch: "main", Cwd: "/w", RepoPath: "/r"}
	before := threadSystemPrompt(l, p, th)

	other := *th
	other.Links = []db.ThreadLink{{Kind: db.LinkPR, Repo: "a/b", Number: 7, Role: db.LinkOpened, Head: "tb/other", Base: "main"}}
	th.Links = other.Links
	th.LastReport, th.Status, th.WaitReason = "report", db.ThreadWaiting, "Approve: push"
	assert.Equal(t, before, threadSystemPrompt(l, p, th))
}
