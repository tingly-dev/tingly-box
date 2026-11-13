package harness

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/coordtest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/internal/project"
)

// These scenarios guard the coordinator's MCP tools (.design/mcp-tools.md):
// where a thread's branch starts, what a refused request leaves behind, and
// what Stop and Reopen do to a thread's status. They run through the real
// tools, so they hold whatever the tools are built from.

// releaseBranch adds a branch "release" that is one commit ahead of main and
// returns the file only that commit has.
func releaseBranch(t *testing.T, repo string) string {
	t.Helper()
	const file = "release-notes.txt"
	git(t, repo, "checkout", "-q", "-b", "release")
	require.NoError(t, os.WriteFile(filepath.Join(repo, file), []byte("v2\n"), 0o644))
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-qm", "release notes")
	git(t, repo, "checkout", "-q", "main")
	return file
}

func systemNotes(w *World, threadID string) []string {
	evs, err := w.Store.ListThreadEvents(threadID)
	require.NoError(w.T, err)
	var out []string
	for _, e := range evs {
		if e.Kind == db.EvSystem {
			out = append(out, e.Content)
		}
	}
	return out
}

// M1: a thread started from another base begins from that base's work and
// merges back into it, on the branch name the coordinator chose.
func TestJourneyThreadStartsFromAChosenBaseAndBranch(t *testing.T) {
	var sawBaseWork atomic.Bool
	var releaseFile string
	w := New(t, Options{
		Files: projectsRepo,
		Actor: func(a *ActorCtx) error {
			_, err := os.Stat(filepath.Join(a.Dir, releaseFile))
			sawBaseWork.Store(err == nil)
			a.Commit("change.txt", a.Prompt+"\n")
			a.Say("Done on the release line.")
			return nil
		},
		Brain: func(c *coordtest.Turn) error {
			if c.CheckIn() {
				c.Reply(NoUpdate)
				return nil
			}
			out, isErr := c.Call("start_thread", map[string]string{
				"title": endTool, "task": "Backport the end-of-turn tool.", "base": "release", "branch": "work/end-tool"})
			assert.False(t, isErr, out)
			c.Reply("Started on the release branch.")
			return nil
		},
	})
	releaseFile = releaseBranch(t, w.Repo)

	w.Say("Backport the end-of-turn tool to the release branch.")
	th := w.ThreadIs(endTool, db.ThreadReady)
	w.Quiet()

	assert.Equal(t, "work/end-tool", th.Branch)
	assert.Equal(t, "release", th.BaseBranch)
	assert.Equal(t, 1, th.CommitsAhead, "only the thread's own commit is ahead of its base")
	assert.True(t, sawBaseWork.Load(), "the thread's worktree starts from the base's work, not from main")
	git(t, w.Repo, "merge-base", "--is-ancestor", "release", "work/end-tool")
	assert.Equal(t, "main", w.Project.Repos[0].BaseBranch, "the project's own base branch is unchanged")
}

// M2: a request the tools refuse leaves nothing behind, the coordinator hears
// why, and it can start again without the bad part. A proposal is checked
// when it is made: a card cannot be edited, so one that could never start is
// not shown.
func TestJourneyRefusedBaseOrBranchLeavesNothingBehind(t *testing.T) {
	type answer struct {
		out   string
		isErr bool
	}
	var badStart, badPropose, dupPropose, goodStart answer
	w := New(t, Options{
		Files: projectsRepo,
		Actor: Diligent("done"),
		Brain: func(c *coordtest.Turn) error {
			if c.CheckIn() {
				c.Reply(NoUpdate)
				return nil
			}
			card := func(title, base, branch string) map[string]string {
				return map[string]string{"title": title, "task": "x", "base": base, "branch": branch}
			}
			badStart.out, badStart.isErr = c.Call("start_thread", card(endTool, "nope", ""))
			badPropose.out, badPropose.isErr = c.Call("propose_threads", map[string]any{"threads": []map[string]string{card(renameT, "nope", "")}})
			dupPropose.out, dupPropose.isErr = c.Call("propose_threads", map[string]any{"threads": []map[string]string{
				card(endTool, "", "work/x"), card(renameT, "", "work/x")}})
			goodStart.out, goodStart.isErr = c.Call("start_thread", card(endTool, "", ""))
			c.Reply("Started without the base.")
			return nil
		},
	})

	w.Say("Start the end-of-turn tool from a branch that does not exist.")
	w.ThreadIs(endTool, db.ThreadReady)
	w.Quiet()

	assert.True(t, badStart.isErr)
	assert.Contains(t, badStart.out, `base "nope" is not a local branch`)
	assert.True(t, badPropose.isErr)
	assert.Contains(t, badPropose.out, "thread 1:")
	assert.True(t, dupPropose.isErr)
	assert.Contains(t, dupPropose.out, `branch "work/x" is used by another proposed thread`)
	assert.False(t, goodStart.isErr, goodStart.out)

	assert.Len(t, w.ThreadList(), 1, "the refused starts created no thread")
	for _, e := range w.Convo() {
		assert.NotEqual(t, db.EvSuggestions, e.Kind, "a refused proposal shows no card")
	}
	branches := git(t, w.Repo, "branch", "--format=%(refname:short)")
	assert.NotContains(t, branches, "work/x")
}

// M3: Stop says who asked, and Reopen only acts on a resolved thread: it
// leaves a running one alone and brings a resolved one back, saying who
// reopened it.
func TestJourneyStopAndReopenSayWhoActedAndKeepStatusTrue(t *testing.T) {
	turnStarted := make(chan struct{})
	w := New(t, Options{
		Files: projectsRepo,
		Actor: func(a *ActorCtx) error {
			close(turnStarted)
			<-a.Turn.Ctx.Done()
			return a.Turn.Ctx.Err()
		},
		Brain: func(c *coordtest.Turn) error {
			if c.CheckIn() {
				c.Reply(NoUpdate)
				return nil
			}
			startThread(c, endTool, "Add the no_update tool.")
			c.Reply("Started.")
			return nil
		},
	})

	w.Say("Add the end-of-turn tool.")
	th := w.ThreadIs(endTool, db.ThreadWorking)
	select {
	case <-turnStarted:
	case <-time.After(8 * time.Second):
		t.Fatal("the thread's turn never started")
	}
	byCoordinator := project.Source{Kind: db.SourceThread, Ref: w.Project.MainThreadID}

	got, err := w.Threads.Reopen(th.ID, byCoordinator)
	require.NoError(t, err)
	assert.Equal(t, db.ThreadWorking, got.Status, "reopening a running thread leaves it alone")
	assert.Nil(t, got.ResolvedAt)

	w.Threads.Stop(th.ID, project.Source{Kind: db.SourceIM})
	w.ThreadIs(endTool, db.ThreadIdle)
	w.Eventually("the stop is noted with who asked", func() bool {
		return strings.Contains(strings.Join(systemNotes(w, th.ID), "\n"), "Stopped from a chat.")
	})

	_, err = w.Threads.Resolve(th.ID)
	require.NoError(t, err)
	w.ThreadIs(endTool, db.ThreadResolved)
	got, err = w.Threads.Reopen(th.ID, byCoordinator)
	require.NoError(t, err)
	assert.Equal(t, db.ThreadIdle, got.Status)
	assert.Nil(t, got.ResolvedAt)
	notes := strings.Join(systemNotes(w, th.ID), "\n")
	assert.Contains(t, notes, "Reopened by the coordinator.")
	assert.Equal(t, 1, strings.Count(notes, "Reopened"), "the first, refused reopen left no note")
}

// M4: the coordinator's session is offered the coordinator's tools and the
// shared ones; the rest of the table is the surface the design promises.
func TestCoordinatorSessionIsOfferedTheWholeToolTable(t *testing.T) {
	var tools []string
	w := New(t, Options{
		Files: projectsRepo,
		Brain: func(c *coordtest.Turn) error {
			tools = c.Tools()
			c.Reply("ok")
			return nil
		},
	})
	w.Say("hello")
	w.Quiet()
	for _, name := range []string{"start_thread", "propose_threads", "message_thread", "get_thread", "list_threads",
		"resolve_thread", "rename_thread", "request_decision", "no_update", "remember", "forget", "update_instructions",
		"read_conversation", "read_shared_file", "list_shared_files", "send_file"} {
		assert.Contains(t, tools, name)
	}
	assert.Len(t, tools, 16, "a new tool is added here on purpose")
}
