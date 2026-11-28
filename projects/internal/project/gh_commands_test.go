package project

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestClassifyGh(t *testing.T) {
	for _, c := range []struct {
		line string
		want ghVerdict
		tool string // what a steer names
	}{
		{"go test ./...", ghNone, ""},
		{"ghostscript a.ps", ghNone, ""},
		{"gh pr view 511", ghSteer, "view tool"},
		{"gh pr view 511 -R tingly-dev/other --json body,comments", ghSteer, "view tool"},
		{"gh issue view 12 --comments", ghSteer, "view tool"},
		{"gh pr checks 7", ghSteer, "view tool"},
		{"cd x && gh pr view 5 | jq .", ghSteer, "view tool"},
		{"GH_PAGER= gh pr view 5", ghSteer, "view tool"},
		{"/usr/bin/gh pr view 5", ghSteer, "view tool"},
		{"gh pr create --fill", ghSteer, "create_pr tool"},
		{"gh pr merge 5 --squash", ghSteer, "merge tool"},
		{"gh pr comment 5 --body hi", ghSteer, "comment tool"},
		{"gh issue comment 5 --body hi", ghSteer, "comment tool"},
		{"gh pr close 5", ghSteer, "close tool"},
		{"gh issue create --title x", ghSteer, "create_issue tool"},
		{"gh run view 123 --log-failed", ghRead, ""},
		{"gh run view 123 --log | tail -50", ghRead, ""},
		{"gh pr list --state open", ghRead, ""},
		{"gh pr diff 5", ghRead, ""},
		{"gh search prs repo:a/b fix", ghRead, ""},
		{"gh api repos/a/b/pulls/5/comments", ghRead, ""},
		{"gh api --method GET search/issues -f q=x", ghRead, ""},
		{"gh --version", ghRead, ""},
		{"gh run view 123 --log > log.txt && rm -rf x", ghAsk, ""},
		{"gh api -X POST repos/a/b/issues/5/comments -f body=x", ghAsk, ""},
		{"gh api -XPATCH repos/a/b/pulls/5", ghAsk, ""},
		{"gh api --method=DELETE repos/a/b/git/refs/heads/x", ghAsk, ""},
		{"gh api repos/a/b/issues/5/comments -f body=x", ghAsk, ""},
		{"gh api graphql --input q.json", ghAsk, ""},
		{"gh pr edit 5 --add-label x", ghAsk, ""},
		{"gh pr review 5 --approve", ghAsk, ""},
		{"gh release create v1", ghAsk, ""},
		{"gh repo delete a/b --yes", ghAsk, ""},
		{"gh -R a/b pr view 5", ghAsk, ""},
		{"gh", ghAsk, ""},
		{"gh pr list && gh pr comment 5 -b x", ghSteer, "comment tool"},
	} {
		got, why := classifyGh(c.line)
		assert.Equal(t, c.want, got, c.line)
		if c.tool != "" {
			assert.Contains(t, why, c.tool, c.line)
		}
	}
}

// A gh command reaches the approval prompt because grantAutoGit asks for all
// of them; the prompt answers the ones a tool does and the reads itself.
func TestThreadPrompterSteersGhToTheToolsWithoutACard(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("done", 1, 1); return nil })
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Fix login", Task: "fix"})
	require.NoError(t, err)
	f.report(t)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)
	p := f.threads.prompter(th.ID)
	bash := func(cmd string) agentboot.ApprovalResponse {
		t.Helper()
		got, err := p.OnApproval(context.Background(), agentboot.ApprovalRequestEvent{ToolName: "Bash", Input: map[string]any{"command": cmd}})
		require.NoError(t, err)
		return got
	}

	got := bash("gh pr view 511")
	assert.False(t, got.Approved)
	assert.Contains(t, got.Reason, "view tool")
	assert.Contains(t, got.Reason, "repo owner/name")
	got = bash("gh pr create --fill")
	assert.False(t, got.Approved)
	assert.Contains(t, got.Reason, "create_pr tool")
	assert.Empty(t, p.requests(), "no card for what a tool does")

	_, err = f.threads.SetPermissionMode(th.ID, "auto")
	require.NoError(t, err)
	assert.True(t, bash("gh run view 123 --log-failed").Approved, "a read no tool does runs in auto mode, as the classifier would let it")
	assert.Empty(t, p.requests())
	assert.False(t, f.threads.isAllowed(th.ID, "gh run view 123 --log-failed"), "nothing is remembered")
}

// Outside auto mode a gh read is not let through here: the mode asks for every
// command, so it is a card like any other.
func TestThreadPrompterLeavesGhReadsToTheModeOutsideAutoMode(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("done", 1, 1); return nil })
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Fix login", Task: "fix"})
	require.NoError(t, err)
	f.report(t)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)
	assert.False(t, f.threads.threadInAutoMode(th.ID))
	_, err = f.threads.SetPermissionMode(th.ID, "auto")
	require.NoError(t, err)
	assert.True(t, f.threads.threadInAutoMode(th.ID))
}
