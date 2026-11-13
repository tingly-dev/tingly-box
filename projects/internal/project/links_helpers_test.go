package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// fakeGitHub stands in for GitHub: nothing in these tests reaches the network.
type fakeGitHub struct {
	mu       sync.Mutex
	calls    []string
	items    map[int]GitHubItem
	next     int
	def      string
	fail     error
	existing *GitHubItem
}

func newFakeGitHub() *fakeGitHub {
	return &fakeGitHub{items: map[int]GitHubItem{}, next: 10, def: "main"}
}

func (g *fakeGitHub) log(format string, a ...any) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls = append(g.calls, fmt.Sprintf(format, a...))
	return g.fail
}

func (g *fakeGitHub) called() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.calls...)
}

func (g *fakeGitHub) DefaultBranch(_ context.Context, r GitHubRepo) (string, error) {
	return g.def, g.log("default %s", r.Slug())
}

func (g *fakeGitHub) add(kind, title string, mod func(*GitHubItem)) GitHubItem {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.next++
	it := GitHubItem{Number: g.next, Title: title, State: "open"}
	if kind == db.LinkPR {
		it.URL = fmt.Sprintf("https://github.com/acme/widgets/pull/%d", it.Number)
		it.IsPR = true
	} else {
		it.URL = fmt.Sprintf("https://github.com/acme/widgets/issues/%d", it.Number)
	}
	if mod != nil {
		mod(&it)
	}
	g.items[it.Number] = it
	return it
}

func (g *fakeGitHub) CreateIssue(_ context.Context, r GitHubRepo, title, body string) (GitHubItem, error) {
	if err := g.log("create-issue %s %q %q", r.Slug(), title, body); err != nil {
		return GitHubItem{}, err
	}
	return g.add(db.LinkIssue, title, nil), nil
}

func (g *fakeGitHub) Comment(_ context.Context, r GitHubRepo, n int, body string) (string, error) {
	return fmt.Sprintf("https://github.com/acme/widgets/issues/%d#c1", n), g.log("comment %s #%d %q", r.Slug(), n, body)
}

func (g *fakeGitHub) OpenPR(_ context.Context, r GitHubRepo, branch string) (*GitHubItem, error) {
	err := g.log("open-pr %s %s", r.Slug(), branch)
	return g.existing, err
}

func (g *fakeGitHub) CreatePR(_ context.Context, r GitHubRepo, title, body, head, base string, draft bool) (GitHubItem, error) {
	if err := g.log("create-pr %s %q head=%s base=%s draft=%t", r.Slug(), title, head, base, draft); err != nil {
		return GitHubItem{}, err
	}
	return g.add(db.LinkPR, title, func(it *GitHubItem) { it.Head, it.Base, it.Draft = head, base, draft }), nil
}

func (g *fakeGitHub) UpdatePR(_ context.Context, r GitHubRepo, n int, title, body, base *string) (GitHubItem, error) {
	what := ""
	if title != nil {
		what += " title=" + *title
	}
	if body != nil {
		what += " body=" + *body
	}
	if base != nil {
		what += " base=" + *base
	}
	if err := g.log("update-pr %s #%d%s", r.Slug(), n, what); err != nil {
		return GitHubItem{}, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	it := g.items[n]
	if title != nil {
		it.Title = *title
	}
	if base != nil {
		it.Base = *base
	}
	g.items[n] = it
	return it, nil
}

func (g *fakeGitHub) PR(_ context.Context, r GitHubRepo, n int) (GitHubItem, error) {
	err := g.log("pr %s #%d", r.Slug(), n)
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.items[n], err
}

func (g *fakeGitHub) Issue(_ context.Context, r GitHubRepo, n int) (GitHubItem, error) {
	err := g.log("issue %s #%d", r.Slug(), n)
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.items[n], err
}

func (g *fakeGitHub) set(n int, mod func(*GitHubItem)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	it := g.items[n]
	mod(&it)
	g.items[n] = it
}

// linksFixture is a worker whose worktree is on its own branch, with commits
// beyond main and an origin that GitHub's URL points at but git pushes to a
// local bare repository.
type linksFixture struct {
	*toolsFixture
	gh     *fakeGitHub
	links  *Links
	clock  *time.Time
	remote string
	repo   string
	events []string
}

func newLinksFixture(t *testing.T) *linksFixture {
	t.Helper()
	f := newToolsFixture(t)
	repo := newRepo(t)
	remote := t.TempDir()
	run(t, remote, "git", "init", "-q", "--bare", "-b", "main")
	run(t, repo, "git", "remote", "add", "origin", "https://github.com/acme/widgets.git")
	run(t, repo, "git", "config", "url."+remote+".insteadOf", "https://github.com/acme/widgets.git")
	run(t, repo, "git", "push", "-q", "origin", "main")
	run(t, repo, "git", "checkout", "-q", "-b", "tp/work")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0o644))
	run(t, repo, "git", "add", ".")
	run(t, repo, "git", "commit", "-qm", "work")

	f.worker.Cwd, f.worker.RepoPath, f.worker.Branch, f.worker.BaseBranch = repo, repo, "tp/work", "main"
	require.NoError(t, f.store.SaveThread(f.worker))
	gh := newFakeGitHub()
	lf := &linksFixture{toolsFixture: f, gh: gh, remote: remote, repo: repo}
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	lf.clock = &now
	lf.links = NewLinks(f.store, gh, func(id string) { lf.events = append(lf.events, id) })
	lf.links.now = func() time.Time { return *lf.clock }
	f.svc.Links = lf.links
	return lf
}

func (f *linksFixture) advance(d time.Duration) { *f.clock = f.clock.Add(d) }

func (f *linksFixture) thread(t *testing.T) *db.Thread {
	t.Helper()
	th, err := f.store.GetThread(f.worker.ID)
	require.NoError(t, err)
	return th
}

func (f *linksFixture) remoteHas(t *testing.T, branch string) bool {
	t.Helper()
	cmd := run(t, f.remote, "git", "branch", "--list", branch)
	return cmd != ""
}
