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
