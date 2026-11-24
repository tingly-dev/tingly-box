package harness

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/coordtest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/internal/project"
)

// G-merge: a thread merges its PR through its tool. The coordinator is told
// what landed, once, as a relayed fact from GitHub, and the thread's status
// is not moved by it (github.never-moves-status holds at the end).
func TestJourneyAMergedPRIsToldToTheCoordinatorAsAFact(t *testing.T) {
	gh := &stubGitHub{}
	var told []string
	var mu sync.Mutex
	w := New(t, Options{
		Files: projectsRepo,
		Actor: func(a *ActorCtx) error {
			a.Commit("change.txt", a.Prompt+"\n")
			out, isErr := callTool(a, "merge", map[string]any{"method": "squash"})
			assert.False(t, isErr, out)
			a.Say("Merged: " + out)
			return nil
		},
		Brain: func(c *coordtest.Turn) error {
			if c.FromUser() && !c.CheckIn() {
				c.Call("start_thread", map[string]string{"title": endTool, "task": "Add the tool and merge it."})
				c.Reply("Started.")
				return nil
			}
			if in := c.InputEnvelope(); strings.Contains(in, `source="github"`) {
				mu.Lock()
				told = append(told, in+"\n"+c.Input())
				mu.Unlock()
				c.Reply("The tool landed in main.")
				return nil
			}
			c.Reply(NoUpdate)
			return nil
		},
	})
	gh.bare = pointOriginAtGitHub(t, w.Repo)
	w.Service.SetGitHub(gh)
	// The user lets threads merge on their own (a new project asks first).
	w.UpdateProject(func(p *db.Project) { p.GitHubAuto = []string{project.GitHubPublish, project.GitHubMerge} })

	w.Say("Add the tool and merge it.")
	th := w.ThreadIs(endTool, db.ThreadReady)
	w.Quiet()

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, told, 1)
	assert.Contains(t, told[0], `kind="pr_merged"`)
	assert.Contains(t, told[0], `trust="relay"`)
	assert.Contains(t, told[0], "Thread now: status=")
	got, err := w.Store.GetThread(th.ID)
	require.NoError(t, err)
	assert.Nil(t, got.ResolvedAt, "a merge never closes a thread")
}

// G-follow-up: the thread's PR is merged on GitHub while the thread is still
// on it, and the first to see it is the thread's own comment, not a refresh.
// The coordinator is told once, asks the thread to check what is left, and the
// thread answers in a turn of its own. The thread's note says the coordinator
// was told, and no one closes the thread.
func TestJourneyAMergeOnGitHubIsFollowedUpThroughTheCoordinator(t *testing.T) {
	gh := &stubGitHub{}
	var told []string
	var mu sync.Mutex
	w := New(t, Options{
		Files: projectsRepo,
		Actor: func(a *ActorCtx) error {
			if strings.Contains(a.Prompt, "From GitHub") {
				a.Say("PR #7 is merged; nothing is left.")
				return nil
			}
			a.Commit("change.txt", a.Prompt+"\n")
			out, isErr := callTool(a, "create_pr", map[string]any{"title": "Add the tool"})
			assert.False(t, isErr, out)
			// The user merges it on GitHub, then the thread comments on it.
			gh.mu.Lock()
			gh.state = db.LinkStateMerged
			gh.mu.Unlock()
			out, isErr = callTool(a, "comment", map[string]any{"number": 7, "body": "Ready."})
			assert.False(t, isErr, out)
			a.Say("Opened PR #7.")
			return nil
		},
		Brain: func(c *coordtest.Turn) error {
			if c.FromUser() && !c.CheckIn() {
				c.Call("start_thread", map[string]string{"title": endTool, "task": "Add the tool and open its PR."})
				c.Reply("Started.")
				return nil
			}
			if in := c.InputEnvelope(); strings.Contains(in, `source="github"`) {
				mu.Lock()
				told = append(told, in)
				mu.Unlock()
				out, isErr := c.Call("message_thread", map[string]any{"thread_id": c.ThreadRef(endTool), "message": "From GitHub, not the user: PR #7 was merged. Check what is left and report."})
				assert.False(t, isErr, out)
				c.Reply("PR #7 landed; I asked the thread to check what is left.")
				return nil
			}
			c.Reply(NoUpdate)
			return nil
		},
	})
	gh.bare = pointOriginAtGitHub(t, w.Repo)
	w.Service.SetGitHub(gh)
	// Comments and PRs go without a card (the default asks).
	w.UpdateProject(func(p *db.Project) { p.GitHubAuto = []string{project.GitHubPublish} })

	w.Say("Add the tool and open its PR.")
	w.Eventually("the thread answers the follow-up", func() bool {
		for _, p := range w.Prompts() {
			if strings.Contains(p, "From GitHub") {
				return true
			}
		}
		return false
	})
	w.Quiet()
	th := w.Thread(endTool)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, told, 1)
	assert.Contains(t, told[0], `kind="pr_merged"`)
	evs, err := w.Store.ListThreadEvents(th.ID)
	require.NoError(t, err)
	var notes []db.ThreadEvent
	for _, e := range evs {
		if e.Kind == db.EvSystem && strings.Contains(e.Payload, `"github"`) {
			notes = append(notes, e)
		}
	}
	require.Len(t, notes, 1)
	assert.Contains(t, notes[0].Payload, `"coordinator_event"`)
	assert.Nil(t, th.ResolvedAt, "a merge never closes a thread")
}
