package project

import (
	"context"
	"errors"
	"fmt"
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
	// approve asks the user about a call that is outside what a thread may do
	// on its own (see approval.go); nil means there is nobody to ask.
	approve Approver
	now     func() time.Time
	// sweeping keeps two sweeps from overlapping.
	sweeping sync.Mutex
	// views are the last answers of View, for linkMinGap.
	viewMu sync.Mutex
	views  map[string]viewed
	// reading holds the items a manual refresh is reading right now, so a second
	// request for one joins it instead of asking GitHub again.
	readingMu sync.Mutex
	reading   map[string]bool
	// rate is the last request budget GitHub reported; projectAsked is when
	// each project last had all its open items refreshed on request.
	rateMu       sync.Mutex
	rate         GitHubRate
	projectAsked map[string]time.Time
	// files are the files of the PRs whose diff was asked for, for prFilesCacheFor.
	filesMu sync.Mutex
	files   map[string]prFilesEntry
}

// Refresh timing (see the design doc): an open item is read again after
// linkInterval, a failing one after linkInterval doubled per failure up to
// linkBackoffMax, and a tool-triggered read is skipped under linkMinGap.
const (
	linkInterval   = 5 * time.Minute
	linkBackoffMax = time.Hour
	linkMinGap     = 30 * time.Second
	// linkManualGap is how soon after a read a person's refresh asks GitHub
	// again; linkProjectGap is the same for refreshing a whole project.
	linkManualGap  = 30 * time.Second
	linkProjectGap = time.Minute
	// rateFloor is the request budget (of GitHub's 5000 an hour) under which
	// no read is made until the window resets, the sweep's or a person's:
	// what is left is for the tools that write. rateLimitedFor is how long
	// an error that names the rate limit pauses reads when GitHub gave no reset.
	rateFloor      = 300
	rateLimitedFor = 10 * time.Minute
	linkSweepMax   = 20
	linkSweepEvery = time.Minute

	titleMax = 250
	bodyMax  = 60000
)

// noteRate keeps the request budget of GitHub's latest answer.
func (l *Links) noteRate(r GitHubRate) {
	if r.ResetAt.IsZero() {
		return
	}
	l.rateMu.Lock()
	l.rate = r
	l.rateMu.Unlock()
}

// pausedUntil is when reads may resume, or zero when GitHub's budget is fine.
func (l *Links) pausedUntil() time.Time {
	l.rateMu.Lock()
	defer l.rateMu.Unlock()
	if l.rate.ResetAt.IsZero() || l.rate.Remaining >= rateFloor || !l.now().Before(l.rate.ResetAt) {
		return time.Time{}
	}
	return l.rate.ResetAt
}

// NewLinks makes the tools' backend over gh.
func NewLinks(store *db.Store, gh GitHub, changed func(threadID string)) *Links {
	if gh == nil {
		gh = ghCLI{}
	}
	return &Links{store: store, gh: gh, changed: changed, now: time.Now, views: map[string]viewed{}, reading: map[string]bool{}, projectAsked: map[string]time.Time{}, files: map[string]prFilesEntry{}}
}

// linkContext is where a thread's call lands: decided from the thread, never
// from what the model said.
type linkContext struct {
	t    *db.Thread
	repo GitHubRepo
	// head is the branch the thread's work is on; empty outside a branch.
	head string
	// git is how a push reaches the repository; set by the calls that push.
	git githubRemote
}

// resolve finds the thread's GitHub repository (its origin) and branch.
func (l *Links) resolve(ctx context.Context, t *db.Thread) (*linkContext, error) {
	if t.Cwd == "" {
		return nil, errors.New("this thread has no working directory, so there is no repository to use")
	}
	repo, err := originRepo(ctx, t.Cwd)
	if err != nil {
		return nil, err
	}
	head := t.Branch
	if head == "" {
		head = HeadBranch(ctx, t.Cwd)
	}
	return &linkContext{t: t, repo: repo, head: head}, nil
}

// remote is how the tools' git reaches the thread's repository: over https
// with the tools' credential, not over origin (see githubRemote).
func (l *Links) remote(ctx context.Context, repo GitHubRepo) (githubRemote, error) {
	auth, ok := l.gh.(GitHubAuth)
	if !ok {
		return githubRemote{}, ErrGitHubUnavailable
	}
	cred, err := auth.Credential(ctx, repo)
	if err != nil {
		return githubRemote{}, err
	}
	gr := githubRemote{repo: repo, cred: cred, url: repo.URL() + ".git"}
	if u, ok := l.gh.(gitURLer); ok {
		gr.url = u.GitURL(repo)
	}
	return gr, nil
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
	k.Checks = it.Checks
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
	kind, it, err := l.read(ctx, lc.repo, number)
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
// another stack branch. The push is a plain fast-forward one unless force is
// set, which is --force-with-lease (see pushBranch). What is beyond the
// thread's own say (another thread's branch, the default or a protected branch,
// a force on a branch it did not create or open a PR from) is put to the user,
// not refused. A force over commits this checkout has not seen is an error: the
// lease is never overridden.
func (l *Links) CreatePR(ctx context.Context, t *db.Thread, title, body, head, base string, draft, force bool) (string, error) {
	r, err := l.createPR(ctx, t, title, body, head, base, draft, true, force)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if r.existed {
		fmt.Fprintf(&b, "PR #%d is already open for branch %s (the branch was pushed again, so it has your latest commits): %s", r.item.Number, r.head, r.item.URL)
		switch {
		case len(r.amended) > 0:
			fmt.Fprintf(&b, "; updated its %s", strings.Join(r.amended, ", "))
		case r.notEdited:
			b.WriteString("; its title, description and base were not edited, because this thread had not recorded it as its own (it is now: call create_pr or update again to change them)")
		}
	} else {
		fmt.Fprintf(&b, "opened PR #%d %s (%s → %s)", r.item.Number, r.item.URL, r.head, r.base)
	}
	b.WriteString(saved(r.recordErr))
	for _, n := range r.closed {
		fmt.Fprintf(&b, "\nPR #%d was closed on GitHub, so it is recorded as closed; this is a new PR for the branch.", n)
	}
	if r.rival != nil {
		fmt.Fprintf(&b, "\nPR #%d (%s → %s) is still open against the same base. If this one replaces it, the user has to close it: no tool here closes a PR.", r.rival.Number, r.rival.Head, r.rival.Base)
	}
	return b.String(), nil
}

// MergeInput says how a thread's branch is merged on GitHub.
type MergeInput struct {
	// Method is MergeSquash or MergeMergeRequest.
	Method string
	// Message is the squash commit's message (its first line is the title);
	// empty leaves it to GitHub's default.
	Message string
	// Title and Body describe the PR when one has to be opened; empty means
	// the first line of Message, else the thread's title, and the thread's last
	// report (or its task).
	Title, Body string
}

// MergeOutcome is what Merge did to the thread's PR.
type MergeOutcome struct {
	Item GitHubItem
	// Existed is set when the PR was already open (its branch was pushed again).
	Existed bool
	// Merged is set when the PR was squash-merged.
	Merged bool
}

// Merge publishes the thread's branch the way in.Method says, on GitHub: it
// pushes the branch and opens the PR (or finds the one that is open), and for
// MergeSquash it then squash-merges that PR. Nothing local changes. The merge
// is irreversible and is only ever made at a user's request: the thread's tool
// asks for approval on every call, and the Review dialog is the user's click.
func (l *Links) Merge(ctx context.Context, t *db.Thread, in MergeInput) (MergeOutcome, error) {
	if in.Method != MergeSquash && in.Method != MergeMergeRequest {
		return MergeOutcome{}, ErrBadMergeMethod
	}
	if err := checkLength("message", in.Message, bodyMax); err != nil {
		return MergeOutcome{}, err
	}
	// A new PR is worded the way `gh pr create --fill` would: from the commits.
	title, body := strings.TrimSpace(in.Title), in.Body
	if title == "" {
		title, _, _ = strings.Cut(strings.TrimSpace(in.Message), "\n")
		title = strings.TrimSpace(title)
	}
	if title == "" {
		var filled string
		title, filled, _ = fillPR(ctx, t)
		if strings.TrimSpace(body) == "" {
			body = filled
		}
	}
	r, err := l.createPR(ctx, t, title, body, "", "", false, false, false)
	if err != nil {
		return MergeOutcome{}, err
	}
	out := MergeOutcome{Item: r.item, Existed: r.existed}
	if in.Method == MergeMergeRequest {
		return out, nil
	}
	sha, err := git(ctx, t.Cwd, "rev-parse", "refs/heads/"+r.head)
	if err != nil {
		return out, err
	}
	msgTitle, msgBody, _ := strings.Cut(strings.TrimSpace(in.Message), "\n")
	msgTitle, msgBody = strings.TrimSpace(msgTitle), strings.TrimSpace(msgBody)
	if msgTitle == "" {
		// As `gh pr merge --squash` words it: the PR's title and (#number), and its description.
		msgTitle, msgBody = fmt.Sprintf("%s (#%d)", r.item.Title, r.item.Number), strings.TrimSpace(r.item.Body)
	}
	if err := l.gh.SquashPR(ctx, r.repo, r.item.Number, sha, msgTitle, msgBody); err != nil {
		return out, fmt.Errorf("PR #%d (%s) is open but was not merged, and stays open: %w", r.item.Number, r.item.URL, err)
	}
	// The record says what GitHub says now; if that read fails the merge still happened.
	it, err := l.gh.Read(ctx, r.repo, r.item.Number)
	if err != nil || it.State != db.LinkStateMerged {
		it = r.item
		it.State, it.MergeState, it.CI = db.LinkStateMerged, "", ""
	}
	l.forgetView(t, it.Number)
	_, rerr := l.record(t, r.repo, db.LinkPR, db.LinkOpened, it)
	if rerr != nil {
		logrus.WithError(rerr).Warn("projects: cannot record a merged PR")
	}
	out.Item, out.Merged = it, true
	return out, nil
}

// prResult is what createPR did: the PR, whether it was already open, and the
// branches it joins.
type prResult struct {
	item       GitHubItem
	existed    bool
	repo       GitHubRepo
	head, base string
	// recordErr is why the thread's link to the PR could not be written.
	recordErr error
	// amended are the fields of an already open PR that were changed to what
	// the call said (title, description, base); notEdited is set when the call
	// asked for edits but the PR is not one this thread has recorded as its own.
	amended   []string
	notEdited bool
	// closed are the thread's records for this branch that GitHub no longer
	// has open; rival is another open PR of the thread against the same base.
	closed []int
	rival  *db.ThreadLink
}

// createPR pushes the branch and opens its PR, or finds the one that is open.
// With amend set, the open PR is put right to what title, body and base say
// (only for a PR the thread has recorded as its own, and only the fields given
// that differ); Merge passes it unset, as its title is only a default.
func (l *Links) createPR(ctx context.Context, t *db.Thread, title, body, head, base string, draft, amend, force bool) (prResult, error) {
	if err := checkText("title", title, titleMax); err != nil {
		return prResult{}, err
	}
	if err := checkLength("body", body, bodyMax); err != nil {
		return prResult{}, err
	}
	lc, err := l.resolve(ctx, t)
	if err != nil {
		return prResult{}, err
	}
	own := lc.head
	if head = strings.TrimSpace(head); head != "" {
		if err := checkBranchName(ctx, t.Cwd, head); err != nil {
			return prResult{}, err
		}
		if !BranchExists(ctx, t.Cwd, head) {
			return prResult{}, fmt.Errorf("there is no local branch %s in this checkout: create_pr pushes a local branch (commit your work to one first)", head)
		}
		lc.head = head
	}
	// needs is why the user has to approve this call, one clause per boundary
	// crossed; empty when the thread may do it on its own.
	var needs []string
	if lc.head != "" && lc.head != own {
		other, err := l.anothersBranch(t, lc)
		if err != nil {
			return prResult{}, err
		}
		if other != nil {
			needs = append(needs, fmt.Sprintf("branch %s belongs to another thread (%q)", lc.head, other.Title))
		}
	}
	if lc.head == "" {
		return prResult{}, errors.New("this thread is not on a branch (detached HEAD or no repository): commit your work to a branch first")
	}
	if lc.git, err = l.remote(ctx, lc.repo); err != nil {
		return prResult{}, err
	}
	def, err := l.gh.DefaultBranch(ctx, lc.repo)
	if err != nil {
		return prResult{}, err
	}
	base = strings.TrimSpace(base)
	asked := base
	if base == "" && (head == "" || head == own) {
		base = t.BaseBranch
	}
	if base == "" {
		base = def
	}
	if lc.head == base {
		return prResult{}, fmt.Errorf("branch %s is the base of its own PR: a PR comes from a branch other than its base", lc.head)
	}
	if lc.head == def {
		needs = append(needs, fmt.Sprintf("branch %s is the repository's default branch", lc.head))
	}
	if protected, err := l.gh.Protected(ctx, lc.repo, lc.head); err != nil {
		return prResult{}, err
	} else if protected {
		needs = append(needs, fmt.Sprintf("branch %s is protected on GitHub", lc.head))
	}
	if bad := checkBranchName(ctx, t.Cwd, base); bad != nil {
		return prResult{}, bad
	}
	if force {
		if ok, err := l.mayForce(t, lc); err != nil {
			return prResult{}, err
		} else if !ok {
			needs = append(needs, fmt.Sprintf("a force push of branch %s, which this thread neither created nor opened a PR from", lc.head))
		}
	}
	// The one gate before the first push: everything that could be refused
	// outright has been checked by now, so the user is asked about a call that
	// would otherwise go through.
	push := &pusher{l: l, t: t, git: lc.git, branch: lc.head, force: force, needs: needs, base: base, title: title}
	existing, err := l.gh.OpenPR(ctx, lc.repo, lc.head)
	if err != nil {
		return prResult{}, err
	}
	if existing != nil {
		return l.openPR(ctx, t, lc, *existing, title, body, asked, amend, push)
	}
	// GitHub has no open PR for the branch: a record that says there is one is
	// out of date (its PR was closed or merged), and would be mistaken for the
	// new PR's twin.
	closed := l.settleClosed(ctx, t, lc.repo, lc.head)
	// Before anything is pushed: a base that GitHub does not have fails the PR
	// after its head is already published, and this call never pushes the base.
	if !onOrigin(ctx, t.Cwd, lc.git, base) {
		return prResult{}, fmt.Errorf("base branch %s is not on GitHub: open its PR with create_pr first (that pushes it), or choose a base that is", base)
	}
	if n, ok := commitsAhead(ctx, t.Cwd, base, lc.head); ok && n == 0 {
		return prResult{}, fmt.Errorf("branch %s has no commit beyond %s: commit your work first", lc.head, base)
	}
	if err := push.run(ctx); err != nil {
		return prResult{}, err
	}
	it, err := l.gh.CreatePR(ctx, lc.repo, strings.TrimSpace(title), body, lc.head, base, draft)
	if err != nil {
		return prResult{}, fmt.Errorf("the branch %s was pushed but the PR was not created: %w", lc.head, err)
	}
	_, rerr := l.record(t, lc.repo, db.LinkPR, db.LinkOpened, it)
	return prResult{item: it, repo: lc.repo, head: lc.head, base: base, recordErr: rerr, closed: closed, rival: l.rivalOf(t, lc.repo, it)}, nil
}

// openPR is createPR when the branch already has an open PR: the branch is
// pushed again (the PR is its branch) and, when the call asks for it, the PR is
// put right: the fields that differ from what the call said (a base only when
// one was named) are edited, all before anything is pushed if one is refused.
func (l *Links) openPR(ctx context.Context, t *db.Thread, lc *linkContext, existing GitHubItem, title, body, base string, amend bool, push *pusher) (prResult, error) {
	r := prResult{item: existing, existed: true, repo: lc.repo, head: lc.head, base: existing.Base}
	own, err := l.store.FindThreadLink(t.ID, lc.repo.Slug(), existing.Number)
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		return prResult{}, err
	}
	mine := own != nil && own.Role == db.LinkOpened
	var newTitle, newBody, newBase *string
	if amend && mine {
		if s := strings.TrimSpace(title); s != existing.Title {
			newTitle = &s
		}
		if body != "" && !sameText(body, existing.Body) {
			newBody = &body
		}
		if base != "" && base != existing.Base {
			if base == lc.head {
				return prResult{}, fmt.Errorf("branch %s cannot be the base of its own PR", base)
			}
			if bad := checkBranchName(ctx, t.Cwd, base); bad != nil {
				return prResult{}, bad
			}
			if !onOrigin(ctx, t.Cwd, lc.git, base) {
				return prResult{}, fmt.Errorf("base branch %s is not on GitHub: open its PR with create_pr first (that pushes it), or choose a base that is", base)
			}
			newBase = &base
		}
	}
	// Publish what the thread has done since: the PR is its branch.
	if err := push.run(ctx); err != nil {
		return prResult{}, err
	}
	if newTitle != nil || newBody != nil || newBase != nil {
		it, err := l.gh.UpdatePR(ctx, lc.repo, existing.Number, newTitle, newBody, newBase)
		if err != nil {
			return prResult{}, fmt.Errorf("the branch %s was pushed but PR #%d was not edited: %w", lc.head, existing.Number, err)
		}
		// The edit's answer has no CI and no settled mergeability: keep the last reading.
		if own != nil {
			it.CI, it.Checks = own.CI, own.Checks
			if it.MergeState == "unknown" || it.MergeState == "" {
				it.MergeState = own.MergeState
			}
		}
		l.forgetView(t, it.Number)
		r.item, r.base = it, it.Base
		for _, f := range []struct {
			set  bool
			name string
		}{{newTitle != nil, "title"}, {newBody != nil, "description"}, {newBase != nil, "base"}} {
			if f.set {
				r.amended = append(r.amended, f.name)
			}
		}
	}
	r.notEdited = amend && !mine
	_, r.recordErr = l.record(t, lc.repo, db.LinkPR, db.LinkOpened, r.item)
	return r, nil
}

// sameText is whether two descriptions read the same: GitHub stores a body
// with CRLF line ends and may trim it, which is not a change.
func sameText(a, b string) bool {
	norm := func(s string) string { return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n")) }
	return norm(a) == norm(b)
}

// settleClosed corrects the thread's records of PRs from branch that say open
// though GitHub has no open PR for it: each is read again (merged or closed as
// GitHub says) and, if GitHub cannot be read, marked closed, as it cannot be
// open. It returns the numbers of the records that changed.
func (l *Links) settleClosed(ctx context.Context, t *db.Thread, repo GitHubRepo, branch string) []int {
	links, err := l.store.ThreadLinks(t.ID)
	if err != nil {
		return nil
	}
	var changed []int
	for _, k := range links {
		if k.Repo != repo.Slug() || k.Kind != db.LinkPR || k.Head != branch || k.State != db.LinkStateOpen {
			continue
		}
		_, it, err := l.read(ctx, repo, k.Number)
		if err != nil || it.State == db.LinkStateOpen {
			// An open reading against OpenPR's answer is GitHub catching up, not a
			// reason to keep the record open: it is closed here and a refresh corrects it.
			it = GitHubItem{Number: k.Number, URL: k.URL, Title: k.Title, Head: k.Head, Base: k.Base, State: db.LinkStateClosed, IsPR: true}
		}
		if _, rerr := l.record(t, repo, db.LinkPR, k.Role, it); rerr != nil {
			logrus.WithError(rerr).Warn("projects: cannot record a closed PR")
			continue
		}
		l.forgetView(t, k.Number)
		changed = append(changed, k.Number)
	}
	return changed
}

// rivalOf is another open PR the thread opened against the same base as the new
// PR it, from a different branch: the usual look of a PR opened from the wrong
// branch and opened again from the right one. A stack's PRs have other bases.
func (l *Links) rivalOf(t *db.Thread, repo GitHubRepo, it GitHubItem) *db.ThreadLink {
	links, err := l.store.ThreadLinks(t.ID)
	if err != nil {
		return nil
	}
	for _, k := range links {
		if k.Repo == repo.Slug() && k.Kind == db.LinkPR && k.Role == db.LinkOpened && k.State == db.LinkStateOpen &&
			k.Number != it.Number && k.Base == it.Base && k.Head != it.Head {
			return &k
		}
	}
	return nil
}

// holdsHead is whether links has a PR from branch head in repo.
func holdsHead(links []db.ThreadLink, repo, head string) bool {
	for _, k := range links {
		if k.Kind == db.LinkPR && k.Repo == repo && k.Head == head {
			return true
		}
	}
	return false
}

// anothersBranch is the thread that owns head when it is not this one, else
// nil: a thread that was started on it, or one that opened a PR from it (a stack
// branch) unless this thread has a PR from it too. Every worktree of a
// repository sees every local branch, so the checkout alone does not tell whose
// a branch is.
func (l *Links) anothersBranch(t *db.Thread, lc *linkContext) (*db.Thread, error) {
	if t.RepoPath != "" {
		other, err := l.store.OtherThreadOnBranch(t.RepoPath, lc.head, t.ID)
		if err == nil {
			return other, nil
		} else if !errors.Is(err, db.ErrNotFound) {
			return nil, err
		}
	}
	mine, err := l.store.ThreadLinks(t.ID)
	if err != nil {
		return nil, err
	}
	if holdsHead(mine, lc.repo.Slug(), lc.head) {
		return nil, nil
	}
	linked, err := l.store.LinkedThreads()
	if err != nil {
		return nil, err
	}
	for _, o := range linked {
		if o.ID != t.ID && holdsHead(o.Links, lc.repo.Slug(), lc.head) {
			return l.store.GetThread(o.ID)
		}
	}
	return nil, nil
}

// mayForce is whether the thread may force-push the head on its own: it created
// it (its own branch) or opened a PR from it. A branch merely found in the
// checkout is not the thread's to rewrite without the user's say.
func (l *Links) mayForce(t *db.Thread, lc *linkContext) (bool, error) {
	if lc.head == t.Branch {
		return true, nil
	}
	mine, err := l.store.ThreadLinks(t.ID)
	if err != nil {
		return false, err
	}
	for _, k := range mine {
		if k.Kind == db.LinkPR && k.Role == db.LinkOpened && k.Repo == lc.repo.Slug() && k.Head == lc.head {
			return true, nil
		}
	}
	return false, nil
}

// pusher is the push of one head for one create_pr call: it asks the user once
// when needs says the call is beyond the thread's own work, then pushes.
type pusher struct {
	l           *Links
	t           *db.Thread
	git         githubRemote
	branch      string
	force       bool
	needs       []string
	base, title string
}

func (p *pusher) run(ctx context.Context) error {
	if len(p.needs) > 0 {
		verb := "Push"
		if p.force {
			verb = "Force-push (--force-with-lease)"
		}
		slug := ""
		if lc, err := p.l.resolve(ctx, p.t); err == nil {
			slug = lc.repo.Slug()
		}
		err := p.l.authorize(ctx, p.t, Approval{
			Tool:    githubToolRule(toolCreatePR),
			Summary: fmt.Sprintf("%s branch %s to GitHub and open or update its PR (base %s)", verb, p.branch, p.base),
			Reason:  strings.Join(p.needs, "; "),
			Input:   map[string]any{"head": p.branch, "base": p.base, "force": p.force, "title": p.title},
			Key:     fmt.Sprintf("%s\x00%s\x00%s\x00force=%t", toolCreatePR, slug, p.branch, p.force),
		})
		if err != nil {
			return err
		}
	}
	if !p.force {
		return pushBranch(ctx, p.t.Cwd, p.git, p.branch, nil)
	}
	seen := trackedOrigin(ctx, p.t.Cwd, p.branch)
	return pushBranch(ctx, p.t.Cwd, p.git, p.branch, &seen)
}

// trackedOrigin is the commit this checkout last saw on origin's branch (its
// remote-tracking ref); empty when it never saw one.
func trackedOrigin(ctx context.Context, dir, branch string) string {
	out, _ := git(ctx, dir, "rev-parse", "--verify", "--quiet", "refs/remotes/origin/"+branch)
	return strings.TrimSpace(out)
}

// lsRemote is what GitHub says about the branch ("<sha>\trefs/heads/<b>"),
// asked the push's way; empty when it has none or cannot be reached.
func lsRemote(ctx context.Context, dir string, gr githubRemote, branch string) string {
	out, _ := gr.run(ctx, dir, time.Minute, "ls-remote", "--heads", gr.url, "refs/heads/"+branch)
	return out
}

// onOrigin is whether GitHub has the branch (asked of the repository itself,
// the way the push goes). A repository that cannot be reached counts as having
// it: the push or GitHub then says what is wrong.
func onOrigin(ctx context.Context, dir string, gr githubRemote, branch string) bool {
	out, err := gr.run(ctx, dir, time.Minute, "ls-remote", "--heads", gr.url, "refs/heads/"+branch)
	return err != nil || out != ""
}

// Update changes the title and description of an issue, or the title,
// description and base of a PR. One the thread opened is edited as asked; any
// other is put to the user first, who sees the change on the card. It cannot
// close or merge anything.
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
	shown := map[string]any{"number": number}
	for k, v := range map[string]*string{"title": title, "body": body, "base": base} {
		if v != nil {
			shown[k] = clip(*v, 500)
		}
	}
	link, role, err := l.editTarget(ctx, t, lc, number, what, shown)
	if err != nil {
		return "", err
	}
	if base != nil && link.Kind != db.LinkPR {
		return "", fmt.Errorf("#%d is an issue: base only applies to a PR", link.Number)
	}
	if link.State != db.LinkStateOpen {
		if link.Kind == db.LinkPR {
			return "", fmt.Errorf("PR #%d is %s: only an open one can be edited. To carry on from its branch, commit and call create_pr: it opens a new PR for the branch", link.Number, link.State)
		}
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
		it.CI, it.Checks = link.CI, link.Checks
		if it.MergeState == "unknown" || it.MergeState == "" {
			it.MergeState = link.MergeState
		}
	}
	if err != nil {
		return "", err
	}
	l.forgetView(t, link.Number)
	_, rerr := l.record(t, lc.repo, link.Kind, role, it)
	return fmt.Sprintf("updated %s #%d %s (%s)%s", link.Kind, it.Number, it.URL, strings.Join(what, ", "), saved(rerr)), nil
}

// editTarget is the item an edit means and the role the thread's record of it
// gets. One the thread opened needs nothing more. Any other (not recorded, or
// one the thread only commented on) is read, shown to the user with what is to
// change, and edited only if they allow it; it is recorded as touched, not
// opened, so the next edit asks again unless they allowed it for the thread.
func (l *Links) editTarget(ctx context.Context, t *db.Thread, lc *linkContext, number int, what []string, input map[string]any) (*db.ThreadLink, string, error) {
	slug := lc.repo.Slug()
	if number <= 0 {
		link, err := l.ownItem(t, slug, 0, true)
		return link, db.LinkOpened, err
	}
	link, err := l.store.FindThreadLink(t.ID, slug, number)
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		return nil, "", err
	}
	if err == nil && link.Role == db.LinkOpened {
		return link, db.LinkOpened, nil
	}
	kind, it, err := l.read(ctx, lc.repo, number)
	if err != nil {
		return nil, "", err
	}
	if err := l.authorize(ctx, t, Approval{
		Tool:    githubToolRule(toolUpdate),
		Summary: fmt.Sprintf("Edit the %s of %s #%d %q, which this thread did not open", strings.Join(what, ", "), kind, number, clip(it.Title, 80)),
		Reason:  fmt.Sprintf("%s #%d was not opened by this thread", kind, number),
		Input:   input,
		Key:     fmt.Sprintf("%s\x00%s\x00%d", toolUpdate, slug, number),
	}); err != nil {
		return nil, "", err
	}
	fresh := &db.ThreadLink{Kind: kind, Repo: slug, Number: number, Role: db.LinkCommented}
	fillLink(fresh, it, l.now())
	return fresh, db.LinkCommented, nil
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
	kind, it, err := l.read(ctx, lc.repo, number)
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

// read is an issue or PR in full, and which it is: one request to GitHub
// whatever the kind. Every answer also says how much of the request budget is
// left; a failure that names the rate limit counts as having none.
func (l *Links) read(ctx context.Context, repo GitHubRepo, number int) (string, GitHubItem, error) {
	it, err := l.gh.Read(ctx, repo, number)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "rate limit") {
			l.noteRate(GitHubRate{Remaining: 0, ResetAt: l.now().Add(rateLimitedFor)})
		}
		return "", GitHubItem{}, err
	}
	l.noteRate(it.Rate)
	if it.IsPR {
		return db.LinkPR, it, nil
	}
	return db.LinkIssue, it, nil
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
	_, it, err := l.read(ctx, repo, k.Number)
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
	if until := l.pausedUntil(); !until.IsZero() {
		logrus.Debug("projects: GitHub's request budget is low; link refresh waits until ", until)
		return
	}
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

// Outcomes of a manual refresh of one item.
const (
	RefreshDone   = "refreshed"
	RefreshRecent = "recent"
	RefreshFailed = "failed"
	// RefreshLimited: GitHub's request budget is nearly used up, so nothing
	// was asked; the record is untouched.
	RefreshLimited = "limited"
)

// LinkRef names one of a thread's links.
type LinkRef struct {
	Repo   string `json:"repo"`
	Kind   string `json:"kind,omitempty"`
	Number int    `json:"number"`
}

// RefreshResult is what a manual refresh did for one item. The reading itself
// reaches clients on the thread event, not here.
type RefreshResult struct {
	Repo   string `json:"repo"`
	Kind   string `json:"kind"`
	Number int    `json:"number"`
	// Status is refreshed, recent (read under linkManualGap ago or being read
	// now: nothing was asked of GitHub), failed or limited.
	Status string `json:"status" enum:"refreshed,recent,failed,limited"`
	Error  string `json:"error,omitempty"`
}

// RefreshLinksResponse is the answer of a manual refresh.
type RefreshLinksResponse struct {
	Results []RefreshResult `json:"results"`
}

// RefreshThread reads a thread's links from GitHub now: the one named, or all
// of them. Unlike the sweep it reads finished items too. An item the thread
// does not have is db.ErrNotFound.
func (l *Links) RefreshThread(ctx context.Context, threadID string, only *LinkRef) ([]RefreshResult, error) {
	links, err := l.store.ThreadLinks(threadID)
	if err != nil {
		return nil, err
	}
	out := []RefreshResult{}
	found := only == nil
	for _, k := range links {
		if only != nil && !(k.Repo == only.Repo && k.Number == only.Number && (only.Kind == "" || k.Kind == only.Kind)) {
			continue
		}
		found = true
		out = append(out, l.refreshNow(ctx, threadID, k))
	}
	if !found {
		return nil, db.ErrNotFound
	}
	return out, nil
}

// RefreshProject reads the open links of a project's threads from GitHub now,
// least recently read first and at most linkSweepMax of them, as the sweep
// does.
func (l *Links) RefreshProject(ctx context.Context, projectID string) ([]RefreshResult, error) {
	threads, err := l.store.ListThreads(projectID)
	if err != nil {
		return nil, err
	}
	l.readingMu.Lock()
	again := l.now().Sub(l.projectAsked[projectID]) < linkProjectGap
	if !again {
		l.projectAsked[projectID] = l.now()
	}
	l.readingMu.Unlock()
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
	sort.SliceStable(open, func(i, j int) bool { return open[i].link.RefreshedAt.Before(open[j].link.RefreshedAt) })
	if len(open) > linkSweepMax {
		open = open[:linkSweepMax]
	}
	out := make([]RefreshResult, 0, len(open))
	for _, p := range open {
		if ctx.Err() != nil {
			break
		}
		if again {
			out = append(out, RefreshResult{Repo: p.link.Repo, Kind: p.link.Kind, Number: p.link.Number, Status: RefreshRecent})
			continue
		}
		out = append(out, l.refreshNow(ctx, p.threadID, p.link))
	}
	return out, nil
}

// refreshNow is one item's manual read. A failure keeps the last good reading
// and says why, but does not count toward the sweep's back-off: a person
// pressing refresh while gh is signed out must not push the sweep's wait to an
// hour. A reading always publishes, even an unchanged one, since "read just
// now" is what was asked for.
func (l *Links) refreshNow(ctx context.Context, threadID string, k db.ThreadLink) RefreshResult {
	res := RefreshResult{Repo: k.Repo, Kind: k.Kind, Number: k.Number}
	// The stored record, not the caller's copy: it may have been read since.
	if cur, err := l.store.FindThreadLink(threadID, k.Repo, k.Number); err == nil {
		k = *cur
	}
	key := threadID + "|" + k.Repo + "#" + strconv.Itoa(k.Number)
	l.readingMu.Lock()
	if l.reading[key] || l.now().Sub(k.AttemptedAt) < linkManualGap {
		l.readingMu.Unlock()
		res.Status = RefreshRecent
		return res
	}
	if until := l.pausedUntil(); !until.IsZero() {
		l.readingMu.Unlock()
		res.Status = RefreshLimited
		res.Error = fmt.Sprintf("GitHub's request budget is nearly used up; reads resume at %s", until.Local().Format("15:04"))
		return res
	}
	l.reading[key] = true
	l.readingMu.Unlock()
	defer func() {
		l.readingMu.Lock()
		delete(l.reading, key)
		l.readingMu.Unlock()
	}()

	fail := func(err error) RefreshResult {
		k.AttemptedAt = l.now()
		k.LastError = redact(err.Error())
		if serr := l.store.SaveThreadLinkReading(threadID, k); serr != nil {
			logrus.WithError(serr).Warn("projects: cannot save a link's refresh failure")
		}
		l.publish(threadID)
		res.Status, res.Error = RefreshFailed, k.LastError
		return res
	}
	repo, err := repoOfLink(&k)
	if err != nil {
		return fail(err)
	}
	_, it, err := l.read(ctx, repo, k.Number)
	if err != nil {
		return fail(err)
	}
	fillLink(&k, it, l.now())
	if err := l.store.SaveThreadLinkReading(threadID, k); err != nil {
		return fail(err)
	}
	l.publish(threadID)
	res.Status = RefreshDone
	return res
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

// pushBranch publishes one branch of the thread to GitHub the tools' way (gr:
// https, the tools' credential, none of the user's git setup), not over
// origin; origin's remote-tracking branch is then moved as a push to origin
// would. The refspec names only that branch's commit, so it can neither touch
// another branch nor delete one. With lease nil it is a plain push, which can
// only fast-forward. Otherwise it is --force-with-lease on the commit *lease
// names (empty: the branch must not be on GitHub), so it overwrites that commit
// and whatever the thread pushed before it, and nothing a person or another
// thread added since.
func pushBranch(ctx context.Context, dir string, gr githubRemote, branch string, lease *string) error {
	sha, err := git(ctx, dir, "rev-parse", "--verify", "refs/heads/"+branch+"^{commit}")
	if err != nil {
		return fmt.Errorf("pushing %s failed: %w", branch, err)
	}
	args := []string{"push", "--quiet"}
	if lease != nil {
		args = append(args, "--force-with-lease=refs/heads/"+branch+":"+*lease)
	}
	if msg, err := gr.run(ctx, dir, 2*time.Minute, append(args, gr.url, sha+":refs/heads/"+branch)...); err != nil {
		if msg == "" {
			msg = err.Error()
		}
		switch {
		case lease != nil && (strings.Contains(msg, "stale info") || strings.Contains(msg, "[rejected]")):
			seen := "never saw the branch"
			if *lease != "" {
				seen = "last saw it at " + (*lease)[:min(len(*lease), 10)]
			}
			now := ""
			if f := strings.Fields(lsRemote(ctx, dir, gr, branch)); len(f) > 0 {
				now = f[0][:min(len(f[0]), 10)]
			}
			if now == "" && *lease != "" {
				// The branch is gone from GitHub (deleted after a merge, say) but this
				// checkout still has its tracking ref.
				return fmt.Errorf("force-pushing %s was refused: GitHub no longer has branch %s, but this checkout still remembers it at %s. Nothing was pushed. Run `git fetch --prune origin` and call create_pr again (without force, a new branch needs none). git said: %s", branch, branch, (*lease)[:min(len(*lease), 10)], clip(msg, 400))
			}
			if now == "" {
				now = "unknown"
			}
			return fmt.Errorf("force-pushing %s was refused to protect commits on GitHub: origin/%s is at %s, this checkout %s, so a force would overwrite commits you have not seen. Nothing was pushed. Run `git fetch origin`, look at `git log %s..origin/%s`, rebase onto origin/%s and call create_pr with force=true again. git said: %s", branch, branch, now, seen, branch, branch, branch, clip(msg, 400))
		// Only a push that lost to newer commits earns the rebase advice; a
		// hook or a protected branch rejects for other reasons.
		case strings.Contains(msg, "non-fast-forward") || strings.Contains(msg, "fetch first"):
			return fmt.Errorf("pushing %s was rejected because GitHub has commits it does not: rebase onto origin/%s and try again (after a rebase, create_pr with force=true): %s", branch, branch, clip(msg, 400))
		case strings.Contains(msg, "Authentication failed") || strings.Contains(msg, "could not read Username") || strings.Contains(msg, "returned error: 403"):
			return fmt.Errorf("pushing %s failed: GitHub did not accept the %s sign-in for it (the user can run `gh auth login`, or `gh auth refresh -s repo`): %s", branch, orGH(gr.cred.Source), clip(msg, 400))
		}
		return fmt.Errorf("pushing %s failed: %s", branch, clip(msg, 600))
	}
	if _, err := git(ctx, dir, "update-ref", "refs/remotes/origin/"+branch, sha); err != nil {
		logrus.WithError(err).Warn("projects: cannot move origin's tracking branch after a push")
	}
	return nil
}

func orGH(source string) string {
	if source == "" {
		return "gh"
	}
	return source
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
