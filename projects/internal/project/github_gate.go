package project

import (
	"context"
	"slices"
	"strings"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// The groups of GitHub actions a project can let its threads do without a
// card (db.Project.GitHubAuto). Each call names the groups it belongs to; it
// runs on its own only when the project auto-approves all of them, else the
// tool puts it to the user. See .design/thread-permissions.md.
const (
	// GitHubPublish: open issues, comment, open and update the thread's own
	// PRs (with a plain push of its own branches), edit what the thread opened.
	GitHubPublish = "publish"
	// GitHubRewrite: force-push (with a lease) a branch the thread created or
	// opened a PR from.
	GitHubRewrite = "rewrite"
	// GitHubOthers: edit an issue or PR the thread did not open, push another
	// thread's branch.
	GitHubOthers = "others"
	// GitHubClose: close an issue or PR, whoever opened it.
	GitHubClose = "close"
	// GitHubMerge: merge a PR on GitHub.
	GitHubMerge = "merge"
)

// GitHubGroups are the groups, in the order the settings show them.
var GitHubGroups = []string{GitHubPublish, GitHubRewrite, GitHubOthers, GitHubClose, GitHubMerge}

// githubAutoOf is the groups p auto-approves. A project saved before the
// setting existed keeps what thread_github_access gave it (the store migrates
// such rows; this covers a row read before that).
func githubAutoOf(p *db.Project) []string {
	if p.GitHubAuto != nil {
		return p.GitHubAuto
	}
	if p.ThreadGitHubAccess == GitHubAsk {
		return []string{}
	}
	return []string{GitHubPublish}
}

// gateCall is what one tool call asks of the gate: the groups it belongs to,
// and why it is never automatic (a default or protected head, a force on a
// branch the thread does not own), when it is not.
type gateCall struct {
	groups []string
	never  []string
}

type userActsKey struct{}

// byUser marks ctx as an action the user started themselves (the Merge
// dialog): their click is the approval, so no gate asks again.
func byUser(ctx context.Context) context.Context { return context.WithValue(ctx, userActsKey{}, true) }

func isByUser(ctx context.Context) bool { v, _ := ctx.Value(userActsKey{}).(bool); return v }

// gate lets a call run when the project auto-approves all of its groups, or
// puts it to the user on a. A call that is never automatic is always put to
// the user, and "allow for this thread" then remembers only its exact target
// (a.Key as the caller set it); any other remembers its groups.
func (l *Links) gate(ctx context.Context, t *db.Thread, g gateCall, a Approval) error {
	if isByUser(ctx) {
		return nil
	}
	groups := slices.Clone(g.groups)
	slices.Sort(groups)
	groups = slices.Compact(groups)
	if len(g.never) > 0 {
		if a.Reason != "" {
			a.Reason = strings.Join(g.never, "; ") + "; " + a.Reason
		} else {
			a.Reason = strings.Join(g.never, "; ")
		}
		return l.authorize(ctx, t, a)
	}
	p, err := l.store.GetProject(t.ProjectID)
	if err != nil {
		return err
	}
	auto := githubAutoOf(p)
	var off []string
	for _, g := range groups {
		if !slices.Contains(auto, g) {
			off = append(off, g)
		}
	}
	if len(off) == 0 {
		return nil
	}
	a.Key = "github\x00" + strings.Join(groups, "+")
	if a.Reason == "" {
		a.Reason = "the project does not auto-approve " + strings.Join(off, ", ")
	} else {
		a.Reason += "; the project does not auto-approve " + strings.Join(off, ", ")
	}
	return l.authorize(ctx, t, a)
}
