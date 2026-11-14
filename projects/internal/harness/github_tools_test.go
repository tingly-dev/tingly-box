package harness

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/coordtest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/internal/project"
)

// These scenarios guard the issue and PR tools (.design/github-tools.md):
// a thread opens a PR through its tool, the system records it, and the
// coordinator sees it. GitHub is a fake: nothing here reaches the network.

// stubGitHub answers like a repository acme/demo with no PRs yet.
type stubGitHub struct {
	mu    sync.Mutex
	calls []string
	ci    string
}

func (g *stubGitHub) log(s string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls = append(g.calls, s)
}

func (g *stubGitHub) DefaultBranch(context.Context, project.GitHubRepo) (string, error) {
	return "main", nil
}
func (g *stubGitHub) Protected(context.Context, project.GitHubRepo, string) (bool, error) {
	return false, nil
}
func (g *stubGitHub) CreateIssue(_ context.Context, _ project.GitHubRepo, title, _ string) (project.GitHubItem, error) {
	g.log("create-issue " + title)
	return project.GitHubItem{Number: 5, URL: "https://github.com/acme/demo/issues/5", Title: title, State: "open"}, nil
}
func (g *stubGitHub) Comment(context.Context, project.GitHubRepo, int, string) (string, error) {
	return "", nil
}
func (g *stubGitHub) OpenPR(context.Context, project.GitHubRepo, string) (*project.GitHubItem, error) {
	return nil, nil
}
func (g *stubGitHub) CreatePR(_ context.Context, _ project.GitHubRepo, title, _, head, base string, _ bool) (project.GitHubItem, error) {
	g.log(fmt.Sprintf("create-pr %s %s->%s", title, head, base))
	return project.GitHubItem{Number: 7, URL: "https://github.com/acme/demo/pull/7", Title: title, State: "open", Head: head, Base: base}, nil
}
func (g *stubGitHub) UpdatePR(context.Context, project.GitHubRepo, int, *string, *string, *string) (project.GitHubItem, error) {
	return project.GitHubItem{}, nil
}
func (g *stubGitHub) UpdateIssue(context.Context, project.GitHubRepo, int, *string, *string) (project.GitHubItem, error) {
	return project.GitHubItem{}, nil
}
func (g *stubGitHub) PR(_ context.Context, _ project.GitHubRepo, n int) (project.GitHubItem, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return project.GitHubItem{Number: n, URL: "https://github.com/acme/demo/pull/7", Title: "Add the tool", State: "open", CI: g.ci, MergeState: "ready"}, nil
}
func (g *stubGitHub) Issue(_ context.Context, _ project.GitHubRepo, n int) (project.GitHubItem, error) {
	return project.GitHubItem{Number: n, State: "open"}, nil
}

// pointOriginAtGitHub gives the repository a github.com origin whose pushes
// land in a local bare repository.
func pointOriginAtGitHub(t *testing.T, repo string) (bare string) {
	t.Helper()
	bare = t.TempDir()
	git(t, bare, "init", "-q", "--bare", "-b", "main")
	git(t, repo, "remote", "add", "origin", "https://github.com/acme/demo.git")
	git(t, repo, "config", "url."+bare+".insteadOf", "https://github.com/acme/demo.git")
	git(t, repo, "push", "-q", "origin", "main")
	return bare
}

// callTool runs one of the Projects tools the way the thread's session would.
func callTool(a *ActorCtx, name string, args map[string]any) (string, bool) {
	session, err := coordtest.ToolSession(a.Turn.Ctx, a.Turn.Opts.MCPConfigPath)
	if err != nil {
		return err.Error(), true
	}
	defer session.Close()
	out, isErr, err := coordtest.CallText(a.Turn.Ctx, session, name, args)
	if err != nil {
		return err.Error(), true
	}
	return out, isErr
}

// G1: a thread opens its PR through the tool; the record and the coordinator's
// view follow, and nothing but the thread's own branch is pushed.
func TestJourneyThreadOpensAPRThroughItsTool(t *testing.T) {
	gh := &stubGitHub{ci: "failing"}
	var prOut string
	var prErr bool
	var sawLinks atomic.Bool
	var bare string
	var w *World
	w = New(t, Options{
		Files: projectsRepo,
		Actor: func(a *ActorCtx) error {
			a.Commit("change.txt", a.Prompt+"\n")
			prOut, prErr = callTool(a, "create_pr", map[string]any{"title": "Add the tool", "body": "It adds the tool."})
			a.Say("Opened the PR: " + prOut)
			return nil
		},
		Brain: func(c *coordtest.Turn) error {
			if c.FromUser() && !c.CheckIn() {
				out, isErr := c.Call("start_thread", map[string]string{"title": endTool, "task": "Add the end-of-turn tool and open a PR."})
				assert.False(t, isErr, out)
				c.Reply("Started.")
				return nil
			}
			// The thread's report arrives: read the thread, with its PR.
			out, isErr := c.Call("get_thread", map[string]string{"thread_id": c.ThreadRef(endTool)})
			assert.False(t, isErr, out)
			assert.Contains(t, out, "issues and PRs")
			assert.Contains(t, out, "/pull/7")
			assert.Contains(t, out, "checks=unread", "a new PR has not been read yet")
			sawLinks.Store(true)
			c.Reply(NoUpdate)
			return nil
		},
	})
	bare = pointOriginAtGitHub(t, w.Repo)
	w.Service.SetGitHub(gh)

	w.Say("Add the end-of-turn tool and open a PR.")
	th := w.ThreadIs(endTool, db.ThreadReady)
	w.Quiet()

	require.False(t, prErr, prOut)
	assert.True(t, sawLinks.Load(), "the coordinator read the thread with its PR")
	assert.Contains(t, prOut, "opened PR #7")
	assert.Equal(t, []string{"create-pr Add the tool " + th.Branch + "->main"}, gh.calls)

	links, err := w.Store.ThreadLinks(th.ID)
	require.NoError(t, err)
	require.Len(t, links, 1)
	assert.Equal(t, 7, links[0].Number)
	assert.Equal(t, db.LinkOpened, links[0].Role)
	assert.Equal(t, "acme/demo", links[0].Repo)

	// Only the thread's own branch reached origin.
	assert.Equal(t, th.Branch, strings.TrimSpace(strings.TrimPrefix(git(t, bare, "branch", "--list", th.Branch), "*")))
	assert.NotContains(t, git(t, bare, "branch", "--list"), "release")
	assert.Equal(t, git(t, w.Repo, "rev-parse", "main"), git(t, bare, "rev-parse", "main"), "main is untouched")
}

// G2: with nothing wired for GitHub the tool says so and records nothing.
func TestJourneyPRToolWithoutAGitHubOriginRefuses(t *testing.T) {
	var out string
	var isErr bool
	w := New(t, Options{
		Files: projectsRepo,
		Actor: func(a *ActorCtx) error {
			a.Commit("change.txt", a.Prompt+"\n")
			out, isErr = callTool(a, "create_pr", map[string]any{"title": "x"})
			a.Say("Could not open the PR: " + out)
			return nil
		},
		Brain: func(c *coordtest.Turn) error {
			if c.CheckIn() || !c.FromUser() {
				c.Reply(NoUpdate)
				return nil
			}
			c.Call("start_thread", map[string]string{"title": endTool, "task": "Add the tool."})
			c.Reply("Started.")
			return nil
		},
	})
	w.Service.SetGitHub(&stubGitHub{})
	w.Say("Add the tool.")
	th := w.ThreadIs(endTool, db.ThreadReady)
	w.Quiet()
	assert.True(t, isErr)
	assert.Contains(t, out, "no origin remote")
	links, _ := w.Store.ThreadLinks(th.ID)
	assert.Empty(t, links)
}

// stackGitHub numbers the PRs it is asked to open and keeps their branches, so
// a base moved by update is read back.
type stackGitHub struct {
	stubGitHub
	prs map[int]project.GitHubItem
}

func (g *stackGitHub) CreatePR(_ context.Context, _ project.GitHubRepo, title, _, head, base string, _ bool) (project.GitHubItem, error) {
	g.log(fmt.Sprintf("create-pr %s %s->%s", title, head, base))
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.prs == nil {
		g.prs = map[int]project.GitHubItem{}
	}
	n := 7 + len(g.prs)
	it := project.GitHubItem{Number: n, URL: fmt.Sprintf("https://github.com/acme/demo/pull/%d", n), Title: title, State: "open", Head: head, Base: base}
	g.prs[n] = it
	return it, nil
}

func (g *stackGitHub) UpdatePR(_ context.Context, _ project.GitHubRepo, n int, _, _, base *string) (project.GitHubItem, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	it := g.prs[n]
	if base != nil {
		g.calls = append(g.calls, fmt.Sprintf("update-pr #%d base=%s", n, *base))
		it.Base = *base
	}
	g.prs[n] = it
	return it, nil
}

// G3: a thread works on a stack: it opens a PR for each of its branches, the
// upper one against the lower, then points the upper one back at main once the
// lower one has merged. Both records are kept and the coordinator sees both.
func TestJourneyThreadOpensAStackOfPRs(t *testing.T) {
	gh := &stackGitHub{}
	var outs []string
	var w *World
	var bare string
	w = New(t, Options{
		Files: projectsRepo,
		Actor: func(a *ActorCtx) error {
			bottom := git(t, a.Dir, "branch", "--show-current")
			a.Commit("bottom.txt", "bottom\n")
			out, isErr := callTool(a, "create_pr", map[string]any{"title": "Bottom"})
			require.False(t, isErr, out)
			outs = append(outs, out)

			git(t, a.Dir, "switch", "-q", "-c", "stack/top")
			a.Commit("top.txt", "top\n")
			out, isErr = callTool(a, "create_pr", map[string]any{"title": "Top", "head": "stack/top", "base": bottom})
			require.False(t, isErr, out)
			outs = append(outs, out)

			// The default branch and an unknown branch are not heads.
			out, isErr = callTool(a, "create_pr", map[string]any{"title": "No", "head": "main"})
			assert.True(t, isErr, out)
			out, isErr = callTool(a, "create_pr", map[string]any{"title": "No", "head": "stack/ghost"})
			assert.True(t, isErr, out)

			// The bottom PR has merged: retarget the top one.
			out, isErr = callTool(a, "update", map[string]any{"number": 8, "base": "main"})
			require.False(t, isErr, out)
			outs = append(outs, out)
			a.Say("Opened two PRs: " + strings.Join(outs, "; "))
			return nil
		},
		Brain: func(c *coordtest.Turn) error {
			if c.FromUser() && !c.CheckIn() {
				c.Call("start_thread", map[string]string{"title": endTool, "task": "Add the tool as a stack of two PRs."})
				c.Reply("Started.")
				return nil
			}
			out, isErr := c.Call("get_thread", map[string]string{"thread_id": c.ThreadRef(endTool)})
			assert.False(t, isErr, out)
			assert.Contains(t, out, "/pull/7")
			assert.Contains(t, out, "/pull/8")
			assert.Contains(t, out, "branch=stack/top→main", "the top PR was retargeted at main")
			c.Reply(NoUpdate)
			return nil
		},
	})
	bare = pointOriginAtGitHub(t, w.Repo)
	w.Service.SetGitHub(gh)

	w.Say("Add the tool as a stack of two PRs.")
	th := w.ThreadIs(endTool, db.ThreadReady)
	w.Quiet()

	links, err := w.Store.ThreadLinks(th.ID)
	require.NoError(t, err)
	require.Len(t, links, 2)
	assert.Equal(t, [3]string{th.Branch, "main", "open"}, [3]string{links[0].Head, links[0].Base, links[0].State})
	assert.Equal(t, [3]string{"stack/top", "main", "open"}, [3]string{links[1].Head, links[1].Base, links[1].State})
	assert.Equal(t, []string{
		"create-pr Bottom " + th.Branch + "->main",
		"create-pr Top stack/top->" + th.Branch,
		"update-pr #8 base=main",
	}, gh.calls)

	// Both branches reached origin, plain; main did not move.
	for _, b := range []string{th.Branch, "stack/top"} {
		assert.Equal(t, git(t, w.Repo, "rev-parse", b), git(t, bare, "rev-parse", b))
	}
	assert.Equal(t, git(t, w.Repo, "rev-parse", "main"), git(t, bare, "rev-parse", "main"), "main is untouched")
}
