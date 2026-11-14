package project

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// Links runs the issue and PR tools and keeps the records they leave current.
// See .design/github-tools.md.
type Links struct {
	store *db.Store
	gh    GitHub
	// changed is told which thread's links changed; it publishes the thread.
	changed func(threadID string)
	now     func() time.Time
	// sweeping keeps two sweeps from overlapping.
	sweeping sync.Mutex
	// views are the last answers of View, for linkMinGap.
	viewMu sync.Mutex
	views  map[string]viewed
}

// Refresh timing (see the design doc): an open item is read again after
// linkInterval, a failing one after linkInterval doubled per failure up to
// linkBackoffMax, and a tool-triggered read is skipped under linkMinGap.
const (
	linkInterval   = 5 * time.Minute
	linkBackoffMax = time.Hour
	linkMinGap     = 30 * time.Second
	linkSweepMax   = 20
	linkSweepEvery = time.Minute

	titleMax = 250
	bodyMax  = 60000
)

// NewLinks makes the tools' backend over gh.
func NewLinks(store *db.Store, gh GitHub, changed func(threadID string)) *Links {
	if gh == nil {
		gh = ghCLI{}
	}
	return &Links{store: store, gh: gh, changed: changed, now: time.Now, views: map[string]viewed{}}
}

// linkContext is where a thread's call lands: decided from the thread, never
// from what the model said.
type linkContext struct {
	t    *db.Thread
	repo GitHubRepo
	// head is the branch the thread's work is on; empty outside a branch.
	head string
}

// resolve finds the thread's GitHub repository (its origin) and branch.
func (l *Links) resolve(ctx context.Context, t *db.Thread) (*linkContext, error) {
	if t.Cwd == "" {
		return nil, errors.New("this thread has no working directory, so there is no repository to use")
	}
	remote, err := git(ctx, t.Cwd, "config", "--get", "remote.origin.url")
	if err != nil {
		return nil, errors.New("this thread's repository has no origin remote")
	}
	repo, err := parseGitHubRemote(remote)
	if err != nil {
		return nil, err
	}
	head := t.Branch
	if head == "" {
		head = HeadBranch(ctx, t.Cwd)
	}
	return &linkContext{t: t, repo: repo, head: head}, nil
}

// parseGitHubRemote reads owner and name from a github.com remote URL in any
// of its spellings. Credentials in the URL are dropped, never kept or shown.
func parseGitHubRemote(remote string) (GitHubRepo, error) {
	remote = strings.TrimSpace(remote)
	var host, path string
	switch {
	case strings.Contains(remote, "://"):
		u, err := url.Parse(remote)
		if err != nil {
			return GitHubRepo{}, errors.New("origin is not a GitHub repository")
		}
		host, path = u.Hostname(), u.Path
	case strings.Contains(remote, "@") && strings.Contains(remote, ":"):
		// scp-like: git@github.com:owner/name.git
		rest := remote[strings.Index(remote, "@")+1:]
		host, path, _ = strings.Cut(rest, ":")
	default:
		return GitHubRepo{}, errors.New("origin is not a GitHub repository")
	}
	parts := strings.Split(strings.Trim(strings.TrimSuffix(strings.Trim(path, "/"), ".git"), "/"), "/")
	if !strings.EqualFold(host, "github.com") || len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return GitHubRepo{}, errors.New("origin is not a github.com repository; the issue and PR tools only work with github.com")
	}
	return GitHubRepo{Owner: parts[0], Name: parts[1]}, nil
}

// checkLength refuses text longer than max characters.
func checkLength(what, s string, max int) error {
	if len([]rune(s)) > max {
		return fmt.Errorf("%s is longer than %d characters", what, max)
	}
	return nil
}

// checkText is checkLength for text that must not be empty.
func checkText(what, s string, max int) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("%s is required", what)
	}
	return checkLength(what, s, max)
}

// record writes what GitHub said about an item as the thread's link to it and
// tells open pages.
func (l *Links) record(t *db.Thread, repo GitHubRepo, kind, role string, it GitHubItem) (*db.ThreadLink, error) {
	link := db.ThreadLink{Kind: kind, Repo: repo.Slug(), Number: it.Number, Role: role}
	fillLink(&link, it, l.now())
	stored, err := l.store.UpsertThreadLink(t.ID, link)
	if err != nil {
		return nil, err
	}
	l.publish(t.ID)
	return &stored, nil
}

// fillLink sets what GitHub said about an item on its record, as read at now:
// everything a refresh brings, and nothing about whose it is.
func fillLink(k *db.ThreadLink, it GitHubItem, now time.Time) {
	k.State, k.Draft, k.CI, k.MergeState, k.Title = it.State, it.Draft, it.CI, it.MergeState, it.Title
	if it.Head != "" {
		k.Head, k.Base = it.Head, it.Base
	}
	if it.URL != "" {
		k.URL = it.URL
	}
	k.RefreshedAt, k.AttemptedAt, k.Failures, k.LastError = now, now, 0, ""
}

func (l *Links) publish(threadID string) {
	if l.changed != nil {
		l.changed(threadID)
	}
}

// saved is the sentence a tool adds when GitHub did the work but the record
// could not be written: the action is not repeated.
func saved(err error) string {
	if err == nil {
		return ""
	}
	logrus.WithError(err).Warn("projects: cannot record a thread's GitHub link")
	return " (it could not be recorded in the thread's links)"
}

// CreateIssue opens an issue in the thread's repository.
func (l *Links) CreateIssue(ctx context.Context, t *db.Thread, title, body string) (string, error) {
	if err := checkText("title", title, titleMax); err != nil {
		return "", err
	}
	if err := checkLength("body", body, bodyMax); err != nil {
		return "", err
	}
	lc, err := l.resolve(ctx, t)
	if err != nil {
		return "", err
	}
	it, err := l.gh.CreateIssue(ctx, lc.repo, strings.TrimSpace(title), body)
	if err != nil {
		return "", err
	}
	_, rerr := l.record(t, lc.repo, db.LinkIssue, db.LinkOpened, it)
	return fmt.Sprintf("opened issue #%d %s%s", it.Number, it.URL, saved(rerr)), nil
}

// Comment adds a comment to an issue or PR of the thread's repository.
func (l *Links) Comment(ctx context.Context, t *db.Thread, number int, body string) (string, error) {
	if number <= 0 {
		return "", errors.New("number is required")
	}
	if err := checkText("body", body, bodyMax); err != nil {
		return "", err
	}
	lc, err := l.resolve(ctx, t)
	if err != nil {
		return "", err
	}
	// The issues API serves PRs too: look first, so the record has the right
	// kind. The comment goes only once this has been read in full, so a record
	// is never written from a partial reading.
	kind, it, err := l.read(ctx, lc.repo, number, "")
	if err != nil {
		return "", err
	}
	commentURL, err := l.gh.Comment(ctx, lc.repo, number, body)
	if err != nil {
		return "", err
	}
	l.forgetView(t, number)
	_, rerr := l.record(t, lc.repo, kind, db.LinkCommented, it)
	return fmt.Sprintf("commented on %s #%d %s%s", kind, number, commentURL, saved(rerr)), nil
}

// CreatePR pushes a branch of the thread's checkout and opens a PR from it. The
// head is the thread's own branch unless one is named: a thread may work on
// several branches (a stack), and base may be any branch on GitHub, including
// another stack branch. The push is the same plain one either way.
func (l *Links) CreatePR(ctx context.Context, t *db.Thread, title, body, head, base string, draft bool) (string, error) {
	if err := checkText("title", title, titleMax); err != nil {
		return "", err
	}
	if err := checkLength("body", body, bodyMax); err != nil {
		return "", err
	}
	lc, err := l.resolve(ctx, t)
	if err != nil {
		return "", err
	}
	own := lc.head
	if head = strings.TrimSpace(head); head != "" {
		if err := checkBranchName(ctx, t.Cwd, head); err != nil {
			return "", err
		}
		if !BranchExists(ctx, t.Cwd, head) {
			return "", fmt.Errorf("there is no local branch %s in this checkout: create_pr pushes a local branch (commit your work to one first)", head)
		}
		lc.head = head
	}
	if lc.head == "" {
		return "", errors.New("this thread is not on a branch (detached HEAD or no repository): commit your work to a branch first")
	}
	def, err := l.gh.DefaultBranch(ctx, lc.repo)
	if err != nil {
		return "", err
	}
	base = strings.TrimSpace(base)
	if base == "" && (head == "" || head == own) {
		base = t.BaseBranch
	}
	if base == "" {
		base = def
	}
	if lc.head == base || lc.head == def {
		return "", fmt.Errorf("branch %s is the base or the repository's default branch: a PR comes from a work branch, and the default branch is never pushed to", lc.head)
	}
	if protected, err := l.gh.Protected(ctx, lc.repo, lc.head); err != nil {
		return "", err
	} else if protected {
		return "", fmt.Errorf("branch %s is protected on GitHub: it is never pushed to", lc.head)
	}
	if bad := checkBranchName(ctx, t.Cwd, base); bad != nil {
		return "", bad
	}
	if existing, err := l.gh.OpenPR(ctx, lc.repo, lc.head); err != nil {
		return "", err
	} else if existing != nil {
		// Publish what the thread has done since: the PR is its branch.
		if err := pushBranch(ctx, t.Cwd, lc.head); err != nil {
			return "", err
		}
		_, rerr := l.record(t, lc.repo, db.LinkPR, db.LinkOpened, *existing)
		return fmt.Sprintf("PR #%d is already open for branch %s (the branch was pushed again, so it has your latest commits): %s%s", existing.Number, lc.head, existing.URL, saved(rerr)), nil
	}
	// Before anything is pushed: a base that GitHub does not have fails the PR
	// after its head is already published, and this call never pushes the base.
	if !onOrigin(ctx, t.Cwd, base) {
		return "", fmt.Errorf("base branch %s is not on GitHub: open its PR with create_pr first (that pushes it), or choose a base that is", base)
	}
	if n, ok := commitsAhead(ctx, t.Cwd, base, lc.head); ok && n == 0 {
		return "", fmt.Errorf("branch %s has no commit beyond %s: commit your work first", lc.head, base)
	}
	if err := pushBranch(ctx, t.Cwd, lc.head); err != nil {
		return "", err
	}
	it, err := l.gh.CreatePR(ctx, lc.repo, strings.TrimSpace(title), body, lc.head, base, draft)
	if err != nil {
		return "", fmt.Errorf("the branch %s was pushed but the PR was not created: %w", lc.head, err)
	}
	_, rerr := l.record(t, lc.repo, db.LinkPR, db.LinkOpened, it)
	return fmt.Sprintf("opened PR #%d %s (%s → %s)%s", it.Number, it.URL, lc.head, base, saved(rerr)), nil
}

// onOrigin is whether origin has the branch (asked of the remote itself, as the
// push would be). A remote that cannot be reached counts as having it: the
// push or GitHub then says what is wrong.
func onOrigin(ctx context.Context, dir, branch string) bool {
	out, err := git(ctx, dir, "ls-remote", "--heads", "origin", "refs/heads/"+branch)
	return err != nil || strings.TrimSpace(out) != ""
}

// Update changes the title and description of an issue, or the title,
// description and base of a PR, that this thread opened. It cannot close or
// merge anything.
func (l *Links) Update(ctx context.Context, t *db.Thread, number int, title, body, base *string) (string, error) {
	if title == nil && body == nil && base == nil {
		return "", errors.New("nothing to change: pass title, body or base")
	}
	if title != nil {
		if err := checkText("title", *title, titleMax); err != nil {
			return "", err
		}
		s := strings.TrimSpace(*title)
		title = &s
	}
	if body != nil {
		if err := checkLength("body", *body, bodyMax); err != nil {
			return "", err
		}
	}
	if base != nil {
		s := strings.TrimSpace(*base)
		if s == "" {
			return "", errors.New("base is empty")
		}
		base = &s
	}
	lc, err := l.resolve(ctx, t)
	if err != nil {
		return "", err
	}
	link, err := l.ownItem(t, lc.repo.Slug(), number, true)
	if err != nil {
		return "", err
	}
	if base != nil && link.Kind != db.LinkPR {
		return "", fmt.Errorf("#%d is an issue: base only applies to a PR", link.Number)
	}
	if link.State != db.LinkStateOpen {
		return "", fmt.Errorf("%s #%d is %s: only an open one can be edited", link.Kind, link.Number, link.State)
	}
	var it GitHubItem
	if link.Kind == db.LinkIssue {
		it, err = l.gh.UpdateIssue(ctx, lc.repo, link.Number, title, body)
	} else {
		if base != nil {
			if bad := checkBranchName(ctx, t.Cwd, *base); bad != nil {
				return "", bad
			}
		}
		it, err = l.gh.UpdatePR(ctx, lc.repo, link.Number, title, body, base)
		// The edit's answer has no CI, and GitHub has not worked out the
		// mergeability yet: keep the last reading of both.
		it.CI = link.CI
		if it.MergeState == "unknown" || it.MergeState == "" {
			it.MergeState = link.MergeState
		}
	}
	if err != nil {
		return "", err
	}
	l.forgetView(t, link.Number)
	_, rerr := l.record(t, lc.repo, link.Kind, db.LinkOpened, it)
	var what []string
	if title != nil {
		what = append(what, "title")
	}
	if body != nil {
		what = append(what, "description")
	}
	if base != nil {
		what = append(what, "base")
	}
	return fmt.Sprintf("updated %s #%d %s (%s)%s", link.Kind, it.Number, it.URL, strings.Join(what, ", "), saved(rerr)), nil
}

// ownItem is the link a call means: the numbered one, or, with no number, the
// thread's only PR. With mustOpen set it has to be one the thread opened.
func (l *Links) ownItem(t *db.Thread, repo string, number int, mustOpen bool) (*db.ThreadLink, error) {
	if number > 0 {
		link, err := l.store.FindThreadLink(t.ID, repo, number)
		if errors.Is(err, db.ErrNotFound) {
			return nil, fmt.Errorf("#%d is not one of this thread's issues or PRs", number)
		}
		if err != nil {
			return nil, err
		}
		if mustOpen && link.Role != db.LinkOpened {
			return nil, fmt.Errorf("#%d was not opened by this thread, so it cannot be edited here (you may comment on it)", number)
		}
		return link, nil
	}
	links, err := l.store.ThreadLinks(t.ID)
	if err != nil {
		return nil, err
	}
	var prs []db.ThreadLink
	for _, k := range links {
		if k.Repo == repo && k.Kind == db.LinkPR && (!mustOpen || k.Role == db.LinkOpened) {
			prs = append(prs, k)
		}
	}
	switch len(prs) {
	case 0:
		return nil, errors.New("this thread has no PR: open one with create_pr, or pass number")
	case 1:
		return &prs[0], nil
	}
	var open []db.ThreadLink
	for _, k := range prs {
		if k.State == db.LinkStateOpen {
			open = append(open, k)
		}
	}
	if len(open) == 1 {
		return &open[0], nil
	}
	var nums []string
	for _, k := range prs {
		n := "#" + strconv.Itoa(k.Number)
		if k.Head != "" {
			n += " (" + k.Head + " → " + k.Base + ")"
		}
		nums = append(nums, n)
	}
	return nil, fmt.Errorf("this thread has several PRs (%s): pass number", strings.Join(nums, ", "))
}

// viewed is a reading View answered with, kept for linkMinGap so a model that
// polls does not become a request to GitHub each time.
type viewed struct {
	at   time.Time
	text string
}

func viewKey(t *db.Thread, number int) string { return t.ID + "#" + strconv.Itoa(number) }

func (l *Links) forgetView(t *db.Thread, number int) {
	l.viewMu.Lock()
	defer l.viewMu.Unlock()
	delete(l.views, viewKey(t, number))
}

// viewBodyMax is how much of an item's description View hands back.
const viewBodyMax = 2000

// View reads an issue or PR of the thread's repository: where it stands, and
// for an open PR its checks and mergeability. An item the thread has a link to
// also has its record brought up to date; any other is only read, never
// recorded. The text is third-party content and comes wrapped as such.
func (l *Links) View(ctx context.Context, t *db.Thread, number int) (string, error) {
	if text, ok := l.viewedRecently(t, number); ok {
		return text, nil
	}
	lc, err := l.resolve(ctx, t)
	if err != nil {
		return "", err
	}
	slug := lc.repo.Slug()
	var link *db.ThreadLink
	if number <= 0 {
		if link, err = l.ownItem(t, slug, 0, false); err != nil {
			return "", err
		}
		number = link.Number
		if text, ok := l.viewedRecently(t, number); ok {
			return text, nil
		}
	} else if link, err = l.store.FindThreadLink(t.ID, slug, number); errors.Is(err, db.ErrNotFound) {
		link = nil // an item the thread has not touched: read, never recorded
	} else if err != nil {
		return "", err
	}
	kind := ""
	if link != nil {
		kind = link.Kind
	}
	kind, it, err := l.read(ctx, lc.repo, number, kind)
	if err != nil {
		return "", err
	}
	if link != nil {
		if err := l.applyReading(t.ID, link, it); err != nil {
			logrus.WithError(err).Warn("projects: cannot save a link's reading")
		}
	}
	text := describeItem(kind, it)
	l.viewMu.Lock()
	if len(l.views) > 200 {
		l.views = map[string]viewed{}
	}
	l.views[viewKey(t, number)] = viewed{at: l.now(), text: text}
	l.viewMu.Unlock()
	return text, nil
}

// viewedRecently is View's last answer for the item when it is under linkMinGap
// old. It is asked before anything is resolved, so a polling model costs no
// process at all.
func (l *Links) viewedRecently(t *db.Thread, number int) (string, bool) {
	if number <= 0 {
		return "", false
	}
	l.viewMu.Lock()
	defer l.viewMu.Unlock()
	c, ok := l.views[viewKey(t, number)]
	return c.text, ok && l.now().Sub(c.at) < linkMinGap
}

// read is an issue or PR in full, and which it is. The issues API answers for
// both and says which; a PR then needs its own read for checks and
// mergeability. A kind the caller already knows (from a link) skips the first.
func (l *Links) read(ctx context.Context, repo GitHubRepo, number int, kind string) (string, GitHubItem, error) {
	if kind == "" {
		it, err := l.gh.Issue(ctx, repo, number)
		if err != nil {
			return "", GitHubItem{}, err
		}
		if !it.IsPR {
			return db.LinkIssue, it, nil
		}
		kind = db.LinkPR
	}
	if kind == db.LinkIssue {
		it, err := l.gh.Issue(ctx, repo, number)
		return kind, it, err
	}
	it, err := l.gh.PR(ctx, repo, number)
	return kind, it, err
}

// describeItem is the answer of view.
func describeItem(kind string, it GitHubItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s #%d %q %s\nstate: %s", kind, it.Number, it.Title, it.URL, it.State)
	if it.Draft {
		b.WriteString(" (draft)")
	}
	if kind == db.LinkPR && it.State == db.LinkStateOpen {
		fmt.Fprintf(&b, "\nchecks: %s\nmergeable: %s", orUnread(it.CI), orUnread(it.MergeState))
	}
	if body := strings.TrimSpace(it.Body); body != "" {
		b.WriteString("\n\n" + clip(body, viewBodyMax))
	}
	return envelope(trustRelay, b.String(), "source", "github", "kind", "view")
}

// refresh reads one record from GitHub. A failure keeps the last good reading,
// and counts toward its back-off.
func (l *Links) refresh(ctx context.Context, threadID string, k *db.ThreadLink) error {
	repo, err := repoOfLink(k)
	if err != nil {
		return err
	}
	_, it, err := l.read(ctx, repo, k.Number, k.Kind)
	if err != nil {
		k.AttemptedAt = l.now()
		k.Failures++
		k.LastError = redact(err.Error())
		if serr := l.store.SaveThreadLinkReading(threadID, *k); serr != nil {
			logrus.WithError(serr).Warn("projects: cannot save a link's refresh failure")
		}
		if k.Failures == 1 {
			// The card shows "could not refresh" from now on.
			l.publish(threadID)
		}
		return err
	}
	return l.applyReading(threadID, k, it)
}

// applyReading stores what GitHub just said about a linked item and tells open
// pages when it changed anything they show.
func (l *Links) applyReading(threadID string, k *db.ThreadLink, it GitHubItem) error {
	changed := k.State != it.State || k.Draft != it.Draft || k.CI != it.CI || k.MergeState != it.MergeState || k.Title != it.Title || (it.Head != "" && (k.Head != it.Head || k.Base != it.Base)) || k.LastError != ""
	fillLink(k, it, l.now())
	if err := l.store.SaveThreadLinkReading(threadID, *k); err != nil {
		return err
	}
	if changed {
		l.publish(threadID)
	}
	return nil
}

// due is when an open record may be read again.
func due(k *db.ThreadLink) time.Time {
	wait := linkInterval
	for i := 0; i < k.Failures && wait < linkBackoffMax; i++ {
		wait *= 2
	}
	if wait > linkBackoffMax {
		wait = linkBackoffMax
	}
	// Ticks come at whole intervals and the last try finished a moment after
	// its tick: a little slack keeps every wait from slipping by one tick.
	return k.AttemptedAt.Add(wait - linkSweepEvery/4)
}

// Sweep refreshes the open records that are due, a few at a time. Closed and
// merged records are never read again.
func (l *Links) Sweep(ctx context.Context) {
	if !l.sweeping.TryLock() {
		return
	}
	defer l.sweeping.Unlock()
	threads, err := l.store.LinkedThreads()
	if err != nil {
		logrus.WithError(err).Warn("projects: cannot list linked threads")
		return
	}
	type pending struct {
		threadID string
		link     db.ThreadLink
	}
	var open []pending
	for _, th := range threads {
		for _, k := range th.Links {
			if k.State == db.LinkStateOpen {
				open = append(open, pending{th.ID, k})
			}
		}
	}
	// The ones read longest ago go first when the cap leaves some for later.
	sort.SliceStable(open, func(i, j int) bool { return open[i].link.RefreshedAt.Before(open[j].link.RefreshedAt) })
	now := l.now()
	done := 0
	for i := range open {
		if ctx.Err() != nil || done >= linkSweepMax {
			return
		}
		if now.Before(due(&open[i].link)) {
			continue
		}
		done++
		if err := l.refresh(ctx, open[i].threadID, &open[i].link); err != nil {
			logrus.WithField("link", fmt.Sprintf("%s#%d", open[i].link.Repo, open[i].link.Number)).Debug("projects: link refresh failed: ", redact(err.Error()))
		}
	}
}

// Run sweeps every minute until ctx ends.
func (l *Links) Run(ctx context.Context) {
	tick := time.NewTicker(linkSweepEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			l.Sweep(ctx)
		}
	}
}

func repoOfLink(k *db.ThreadLink) (GitHubRepo, error) {
	parts := strings.Split(k.Repo, "/")
	if len(parts) != 2 {
		return GitHubRepo{}, fmt.Errorf("unsupported repository %q", k.Repo)
	}
	return GitHubRepo{Owner: parts[0], Name: parts[1]}, nil
}

// commitsAhead counts the commits head has beyond base, trying the local base
// branch and then origin's. ok is false when neither could be read.
func commitsAhead(ctx context.Context, dir, base, head string) (int, bool) {
	for _, ref := range []string{base, "origin/" + base} {
		out, err := git(ctx, dir, "rev-list", "--count", ref+".."+head)
		if err != nil {
			continue
		}
		if n, err := strconv.Atoi(out); err == nil {
			return n, true
		}
	}
	return 0, false
}

// pushBranch publishes the thread's own branch. The refspec names only that
// branch and has no "+", so it can neither touch another branch nor force.
func pushBranch(ctx context.Context, dir, branch string) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	spec := "refs/heads/" + branch + ":refs/heads/" + branch
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "push", "origin", spec)
	// A push may never wait for a password.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C", "LANGUAGE=C")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		msg := redact(strings.TrimSpace(out.String()))
		// Only a push that lost to newer commits earns the rebase advice; a
		// hook or a protected branch rejects for other reasons.
		if strings.Contains(msg, "non-fast-forward") || strings.Contains(msg, "fetch first") {
			return fmt.Errorf("pushing %s was rejected because origin has commits it does not: rebase onto origin/%s and try again (a force push is not available): %s", branch, branch, clip(msg, 400))
		}
		return fmt.Errorf("pushing %s failed: %s", branch, clip(msg, 600))
	}
	return nil
}

// linksNote is the short form of a thread's links for a line in a list:
// ` prs=#12(open checks=failing) issues=#3(open)`; empty when there are none.
func linksNote(links []db.ThreadLink) string {
	var prs, issues []string
	for _, k := range links {
		s := fmt.Sprintf("#%d(%s", k.Number, k.State)
		if k.Draft {
			s += " draft"
		}
		if k.State == db.LinkStateOpen && k.Kind == db.LinkPR {
			if k.CI != "" && k.CI != "none" {
				s += " checks=" + k.CI
			}
			if k.MergeState != "" && k.MergeState != "unknown" {
				s += " merge=" + k.MergeState
			}
		}
		s += ")"
		if k.Kind == db.LinkPR {
			prs = append(prs, s)
		} else {
			issues = append(issues, s)
		}
	}
	var out string
	if len(prs) > 0 {
		out += " prs=" + strings.Join(prs, ",")
	}
	if len(issues) > 0 {
		out += " issues=" + strings.Join(issues, ",")
	}
	return out
}

// orUnread names a PR field GitHub has not been asked about yet.
func orUnread(s string) string {
	if s == "" {
		return "unread"
	}
	return s
}

// linksDetail lists a thread's links one per line, with how old each reading
// is, for get_thread.
func linksDetail(links []db.ThreadLink, now time.Time) string {
	var b strings.Builder
	for _, k := range links {
		fmt.Fprintf(&b, "- %s #%d %q %s state=%s role=%s", k.Kind, k.Number, clip(k.Title, 120), k.URL, k.State, k.Role)
		if k.Head != "" {
			fmt.Fprintf(&b, " branch=%s→%s", k.Head, k.Base)
		}
		if k.Kind == db.LinkPR && k.State == db.LinkStateOpen {
			fmt.Fprintf(&b, " draft=%t checks=%s merge=%s", k.Draft, orUnread(k.CI), orUnread(k.MergeState))
		}
		fmt.Fprintf(&b, " read %s ago", now.Sub(k.RefreshedAt).Round(time.Minute))
		if k.LastError != "" {
			fmt.Fprintf(&b, " (the last refresh failed: %s)", clip(k.LastError, 120))
		}
		b.WriteString("\n")
	}
	return b.String()
}
