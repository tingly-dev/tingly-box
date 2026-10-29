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
}

func TestThreadPromptRules(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	p := &db.Project{ID: "p1", Name: "Demo", Instructions: "use make test"}
	s := threadSystemPrompt(l, p, &db.Thread{Title: "Fix bug"})
	assert.Contains(t, s, "not written by the user")
	assert.Contains(t, s, "Memory is background")
	assert.Contains(t, s, "wins; if the two conflict")
	assert.Contains(t, s, "use make test")
	assert.NotContains(t, s, "pull request", "a thread without a repository has no PR rule")

	s = threadSystemPrompt(l, p, &db.Thread{Title: "Fix bug", Branch: "tb/fix", BaseBranch: "main", Cwd: "/w"})
	assert.Contains(t, s, "push your own branch")
	assert.Contains(t, s, "report the failure")
	assert.NotContains(t, s, "gh")
	assert.Contains(t, s, "Never merge it yourself")
	assert.NotContains(t, s, "Do not push", "a PR may be asked for, so pushing is not banned outright")
}
