package project

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
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
	return &Links{store: store, gh: gh, changed: changed, now: time.Now}
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
	return GitHubRepo{Host: "github.com", Owner: parts[0], Name: parts[1]}, nil
}

func checkText(what, s string, max int) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("%s is required", what)
	}
	if len([]rune(s)) > max {
		return fmt.Errorf("%s is longer than %d characters", what, max)
	}
	return nil
}

// record writes what GitHub said about an item as the thread's link to it and
// tells open pages.
func (l *Links) record(t *db.Thread, repo GitHubRepo, kind, role string, it GitHubItem) (*db.ThreadLink, error) {
	now := l.now()
	link := &db.ThreadLink{ThreadID: t.ID, ProjectID: t.ProjectID, Kind: kind, Repo: repo.Slug(), Number: it.Number,
		URL: it.URL, Title: it.Title, Role: role, State: it.State, Draft: it.Draft, CI: it.CI, MergeState: it.MergeState,
		RefreshedAt: now, AttemptedAt: now}
	if err := l.store.UpsertThreadLink(link); err != nil {
		return nil, err
	}
	l.publish(t.ID)
	return link, nil
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
	if len([]rune(body)) > bodyMax {
		return "", fmt.Errorf("body is longer than %d characters", bodyMax)
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
	// The issues API serves PRs too: look first, so the record has the right kind.
	it, err := l.gh.Issue(ctx, lc.repo, number)
	if err != nil {
		return "", err
	}
	kind := db.LinkIssue
	if it.IsPR {
		kind = db.LinkPR
		if pr, perr := l.gh.PR(ctx, lc.repo, number); perr == nil {
			it = pr
		}
	}
	commentURL, err := l.gh.Comment(ctx, lc.repo, number, body)
	if err != nil {
		return "", err
	}
	_, rerr := l.record(t, lc.repo, kind, db.LinkCommented, it)
	return fmt.Sprintf("commented on %s #%d %s%s", kind, number, commentURL, saved(rerr)), nil
}

// CreatePR pushes the thread's own branch and opens a PR from it.
func (l *Links) CreatePR(ctx context.Context, t *db.Thread, title, body, base string, draft bool) (string, error) {
	if err := checkText("title", title, titleMax); err != nil {
		return "", err
	}
	if len([]rune(body)) > bodyMax {
		return "", fmt.Errorf("body is longer than %d characters", bodyMax)
	}
	lc, err := l.resolve(ctx, t)
	if err != nil {
		return "", err
	}
	if lc.head == "" {
		return "", errors.New("this thread is not on a branch (detached HEAD or no repository): commit your work to a branch first")
	}
	def, err := l.gh.DefaultBranch(ctx, lc.repo)
	if err != nil {
		return "", err
	}
	base = strings.TrimSpace(base)
	if base == "" {
		base = t.BaseBranch
	}
	if base == "" {
		base = def
	}
	if lc.head == base || lc.head == def {
		return "", fmt.Errorf("branch %s is the base or the repository's default branch: a PR comes from the thread's own branch, and it is never pushed to", lc.head)
	}
	if bad := checkBranchName(ctx, t.Cwd, base); bad != nil {
		return "", bad
	}
	if existing, err := l.gh.OpenPR(ctx, lc.repo, lc.head); err != nil {
		return "", err
	} else if existing != nil {
		_, rerr := l.record(t, lc.repo, db.LinkPR, db.LinkOpened, *existing)
		return fmt.Sprintf("PR #%d is already open for branch %s: %s%s", existing.Number, lc.head, existing.URL, saved(rerr)), nil
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

// UpdatePR changes the title, body or base of a PR this thread opened.
func (l *Links) UpdatePR(ctx context.Context, t *db.Thread, number int, title, body, base *string) (string, error) {
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
	if body != nil && len([]rune(*body)) > bodyMax {
		return "", fmt.Errorf("body is longer than %d characters", bodyMax)
	}
	if base != nil {
		s := strings.TrimSpace(*base)
		if s == "" {
			return "", errors.New("base is empty")
		}
		base = &s
	}
	link, err := l.ownPR(t, number, true)
	if err != nil {
		return "", err
	}
	repo, err := repoOfLink(link)
	if err != nil {
		return "", err
	}
	if link.State != db.LinkStateOpen {
		return "", fmt.Errorf("PR #%d is %s: only an open PR can be edited", link.Number, link.State)
	}
	if base != nil {
		if bad := checkBranchName(ctx, t.Cwd, *base); bad != nil {
			return "", bad
		}
	}
	it, err := l.gh.UpdatePR(ctx, repo, link.Number, title, body, base)
	if err != nil {
		return "", err
	}
	// The edit's answer has no CI; keep the last reading of it.
	it.CI = link.CI
	_, rerr := l.record(t, repo, db.LinkPR, db.LinkOpened, it)
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
	return fmt.Sprintf("updated PR #%d %s (%s)%s", it.Number, it.URL, strings.Join(what, ", "), saved(rerr)), nil
}

// ownPR is the PR link a call means: the numbered one, or the thread's only
// one. With mustOpen set it has to be one the thread opened.
func (l *Links) ownPR(t *db.Thread, number int, mustOpen bool) (*db.ThreadLink, error) {
	if number > 0 {
		link, err := l.store.GetThreadLink(t.ID, db.LinkPR, number)
		if errors.Is(err, db.ErrNotFound) {
			return nil, fmt.Errorf("PR #%d is not one of this thread's PRs", number)
		}
		if err != nil {
			return nil, err
		}
		if mustOpen && link.Role != db.LinkOpened {
			return nil, fmt.Errorf("PR #%d was not opened by this thread, so it cannot be edited here (you may comment on it)", number)
		}
		return link, nil
	}
	links, err := l.store.ThreadLinks(t.ID)
	if err != nil {
		return nil, err
	}
	var prs []db.ThreadLink
	for _, k := range links {
		if k.Kind == db.LinkPR && (!mustOpen || k.Role == db.LinkOpened) {
			prs = append(prs, k)
		}
	}
	switch len(prs) {
	case 0:
		return nil, errors.New("this thread has no PR: open one with create_pr")
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
		nums = append(nums, "#"+strconv.Itoa(k.Number))
	}
	return nil, fmt.Errorf("this thread has several PRs (%s): pass number", strings.Join(nums, ", "))
}

// Status reads a PR of the thread. It asks GitHub unless the record is fresh.
func (l *Links) Status(ctx context.Context, t *db.Thread, number int) (string, error) {
	link, err := l.ownPR(t, number, false)
	if err != nil {
		return "", err
	}
	if age := l.now().Sub(link.RefreshedAt); age >= linkMinGap || (link.State == db.LinkStateOpen && link.CI == "") {
		if rerr := l.refresh(ctx, link); rerr != nil {
			return "", rerr
		}
	}
	return describePR(link), nil
}

// describePR is the answer of get_pr_status.
func describePR(k *db.ThreadLink) string {
	var b strings.Builder
	fmt.Fprintf(&b, "PR #%d %q %s\nstate: %s", k.Number, k.Title, k.URL, k.State)
	if k.Draft {
		b.WriteString(" (draft)")
	}
	if k.State == db.LinkStateOpen {
		fmt.Fprintf(&b, "\nchecks: %s\nmergeable: %s", orUnread(k.CI), orUnread(k.MergeState))
	}
	return b.String()
}

// refresh reads one record from GitHub. A failure keeps the last good reading,
// and counts toward its back-off.
func (l *Links) refresh(ctx context.Context, k *db.ThreadLink) error {
	repo, err := repoOfLink(k)
	if err != nil {
		return err
	}
	var it GitHubItem
	if k.Kind == db.LinkPR {
		it, err = l.gh.PR(ctx, repo, k.Number)
	} else {
		it, err = l.gh.Issue(ctx, repo, k.Number)
	}
	now := l.now()
	k.AttemptedAt = now
	if err != nil {
		k.Failures++
		k.LastError = redact(err.Error())
		if serr := l.store.SaveThreadLinkReading(k); serr != nil {
			logrus.WithError(serr).Warn("projects: cannot save a link's refresh failure")
		}
		if k.Failures == 1 {
			// The card shows "could not refresh" from now on.
			l.publish(k.ThreadID)
		}
		return err
	}
	changed := k.State != it.State || k.Draft != it.Draft || k.CI != it.CI || k.MergeState != it.MergeState || k.Title != it.Title || k.LastError != ""
	k.State, k.Draft, k.CI, k.MergeState, k.Title = it.State, it.Draft, it.CI, it.MergeState, it.Title
	if it.URL != "" {
		k.URL = it.URL
	}
	k.RefreshedAt, k.Failures, k.LastError = now, 0, ""
	if err := l.store.SaveThreadLinkReading(k); err != nil {
		return err
	}
	if changed {
		l.publish(k.ThreadID)
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
	open, err := l.store.OpenThreadLinks()
	if err != nil {
		logrus.WithError(err).Warn("projects: cannot list open links")
		return
	}
	now := l.now()
	done := 0
	for i := range open {
		if ctx.Err() != nil || done >= linkSweepMax {
			return
		}
		if now.Before(due(&open[i])) {
			continue
		}
		done++
		if err := l.refresh(ctx, &open[i]); err != nil {
			logrus.WithField("link", fmt.Sprintf("%s#%d", open[i].Repo, open[i].Number)).Debug("projects: link refresh failed: ", redact(err.Error()))
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
	return GitHubRepo{Host: "github.com", Owner: parts[0], Name: parts[1]}, nil
}

// checkBranchName refuses a base that git could not take as a branch name.
func checkBranchName(ctx context.Context, dir, name string) error {
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("%q is not a valid branch name", name)
	}
	if _, err := git(ctx, dir, "check-ref-format", "refs/heads/"+name); err != nil {
		return fmt.Errorf("%q is not a valid branch name", name)
	}
	return nil
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
		if strings.Contains(msg, "non-fast-forward") || strings.Contains(msg, "rejected") {
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
