package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	failPR   error
	existing *GitHubItem
	// mergeFail is why GitHub refuses to squash-merge a PR.
	mergeFail error
	// protected are the branches GitHub holds protected.
	protected map[string]bool
	// block, when set, holds every read until it is closed. rate is the budget
	// every read reports.
	block chan struct{}
	rate  GitHubRate
	// prFiles are the files of each PR, as GitHub lists them.
	prFiles map[int][]GitHubFile
	// tokenFail is why there is no token to push with.
	tokenFail error
	// gitURL is where the tools' git pushes: a local bare repository.
	gitURL string
}

// fakeToken is the token the fake signs git in with: GitHub's shape, so
// redact would also catch it.
const fakeToken = "ghs_FakeTokenForTheTestsOnly0123456789"

func (g *fakeGitHub) Credential(context.Context, GitHubRepo) (GitHubCredential, error) {
	if g.tokenFail != nil {
		return GitHubCredential{}, g.tokenFail
	}
	return GitHubCredential{Token: fakeToken, Username: "x-access-token", Source: "fake"}, nil
}

// GitURL keeps every test's git off the network: a fixture without a bare
// repository pushes to one that is not there.
func (g *fakeGitHub) GitURL(GitHubRepo) string {
	if g.gitURL == "" {
		return "/nonexistent/fake-github.git"
	}
	return g.gitURL
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

func (g *fakeGitHub) Protected(_ context.Context, r GitHubRepo, branch string) (bool, error) {
	err := g.log("protected %s %s", r.Slug(), branch)
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.protected[branch], err
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
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.existing != nil {
		return g.existing, err
	}
	// Like GitHub: the open PR whose head is the branch, whoever opened it.
	for n := 1; n <= g.next; n++ {
		if it, ok := g.items[n]; ok && it.IsPR && it.State == "open" && it.Head == branch {
			return &it, err
		}
	}
	return nil, err
}

func (g *fakeGitHub) CreatePR(_ context.Context, r GitHubRepo, title, body, head, base string, draft bool) (GitHubItem, error) {
	if err := g.log("create-pr %s %q head=%s base=%s draft=%t", r.Slug(), title, head, base, draft); err != nil {
		return GitHubItem{}, err
	}
	return g.add(db.LinkPR, title, func(it *GitHubItem) { it.Head, it.Base, it.Draft, it.Body = head, base, draft, body }), nil
}

func (g *fakeGitHub) MergePR(_ context.Context, r GitHubRepo, n int, method, sha, title, message string) error {
	if err := g.log("%s-pr %s #%d sha=%s title=%q message=%q", method, r.Slug(), n, sha, title, message); err != nil {
		return err
	}
	if g.mergeFail != nil {
		return g.mergeFail
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	it := g.items[n]
	it.State = "merged"
	g.items[n] = it
	return nil
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
	if body != nil {
		it.Body = *body
	}
	if base != nil {
		it.Base = *base
	}
	g.items[n] = it
	return it, nil
}

func (g *fakeGitHub) UpdateIssue(_ context.Context, r GitHubRepo, n int, title, body *string) (GitHubItem, error) {
	what := ""
	if title != nil {
		what += " title=" + *title
	}
	if body != nil {
		what += " body=" + *body
	}
	if err := g.log("update-issue %s #%d%s", r.Slug(), n, what); err != nil {
		return GitHubItem{}, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	it := g.items[n]
	if title != nil {
		it.Title = *title
	}
	if body != nil {
		it.Body = *body
	}
	g.items[n] = it
	return it, nil
}

func (g *fakeGitHub) Close(_ context.Context, r GitHubRepo, n int) (GitHubItem, error) {
	if err := g.log("close %s #%d", r.Slug(), n); err != nil {
		return GitHubItem{}, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	it := g.items[n]
	it.State = "closed"
	g.items[n] = it
	// Like the issues API: no branches in the answer.
	it.Head, it.Base, it.Draft = "", "", false
	return it, nil
}

func (g *fakeGitHub) Read(_ context.Context, r GitHubRepo, n int) (GitHubItem, error) {
	err := g.log("read %s #%d", r.Slug(), n)
	if g.block != nil {
		<-g.block
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	it, ok := g.items[n]
	switch {
	case err != nil:
	case !ok:
		err = fmt.Errorf("GitHub has no issue or PR #%d in %s", n, r.Slug())
	case it.IsPR:
		err = g.failPR
	}
	it.Rate = g.rate
	return it, err
}

func (g *fakeGitHub) PRFiles(_ context.Context, r GitHubRepo, n int) ([]GitHubFile, error) {
	err := g.log("files %s #%d", r.Slug(), n)
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.prFiles[n], err
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
	// approvals are what the tools asked the user; answer is what the user
	// says (by default no, with no reason).
	approvals []Approval
	answer    func(Approval) (bool, string)
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
	gh.gitURL = remote
	lf := &linksFixture{toolsFixture: f, gh: gh, remote: remote, repo: repo}
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	lf.clock = &now
	lf.links = NewLinks(f.store, gh, func(id string) { lf.events = append(lf.events, id) })
	lf.links.now = func() time.Time { return *lf.clock }
	lf.links.approve = func(_ context.Context, _ string, a Approval) (bool, string, error) {
		lf.approvals = append(lf.approvals, a)
		if lf.answer == nil {
			return false, "", nil
		}
		ok, why := lf.answer(a)
		return ok, why, nil
	}
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

// stackBranch makes a local branch off from with one commit, leaving the
// worktree on the branch it was on: the thread's checkout does not move.
func (f *linksFixture) stackBranch(t *testing.T, name, from string) {
	t.Helper()
	was := run(t, f.repo, "git", "branch", "--show-current")
	run(t, f.repo, "git", "checkout", "-q", "-b", name, from)
	require.NoError(t, os.WriteFile(filepath.Join(f.repo, name[strings.LastIndex(name, "/")+1:]+".txt"), []byte(name+"\n"), 0o644))
	run(t, f.repo, "git", "add", ".")
	run(t, f.repo, "git", "commit", "-qm", name)
	run(t, f.repo, "git", "checkout", "-q", was)
}

// remoteBranches are the branches origin has.
func (f *linksFixture) remoteBranches(t *testing.T) []string {
	t.Helper()
	return strings.Fields(strings.ReplaceAll(run(t, f.remote, "git", "branch", "--format=%(refname:short)"), "*", ""))
}

// allowAll makes the user allow everything the tools ask.
func (f *linksFixture) allowAll() { f.answer = func(Approval) (bool, string) { return true, "" } }
