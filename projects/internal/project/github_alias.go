package project

import (
	"context"

	gh "github.com/tingly-dev/tingly-box/mcphub/github"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// The GitHub client lives in mcphub (shared with Tingly Box); these aliases
// keep the project package's vocabulary.
type (
	GitHubRepo = gh.Repo
	GitHubItem = gh.Item
	GitHubRate = gh.Rate
)

var (
	redact               = gh.Redact
	ErrGitHubUnavailable = gh.ErrUnavailable
)

// GitHub is what the issue and PR tools need from GitHub. The real one runs
// the host's gh; tests give a fake, so nothing in the tests reaches GitHub.
type GitHub interface {
	// DefaultBranch is the repository's default branch.
	DefaultBranch(ctx context.Context, r GitHubRepo) (string, error)
	// Protected is whether GitHub holds the branch protected; a branch that is
	// not on GitHub is not.
	Protected(ctx context.Context, r GitHubRepo, branch string) (bool, error)
	CreateIssue(ctx context.Context, r GitHubRepo, title, body string) (GitHubItem, error)
	// Comment adds a comment to an issue or PR and returns its URL.
	Comment(ctx context.Context, r GitHubRepo, number int, body string) (string, error)
	// OpenPR is the open PR whose head is branch of this repository, if any.
	OpenPR(ctx context.Context, r GitHubRepo, branch string) (*GitHubItem, error)
	CreatePR(ctx context.Context, r GitHubRepo, title, body, head, base string, draft bool) (GitHubItem, error)
	// SquashPR squash-merges an open PR on GitHub. sha is the head commit the
	// caller means to merge: GitHub refuses when the PR's head has moved. Nothing
	// else is touched (the branch is not deleted).
	SquashPR(ctx context.Context, r GitHubRepo, number int, sha, title, message string) error
	// UpdatePR changes only the fields that are not nil.
	UpdatePR(ctx context.Context, r GitHubRepo, number int, title, body, base *string) (GitHubItem, error)
	// UpdateIssue changes only the fields that are not nil.
	UpdateIssue(ctx context.Context, r GitHubRepo, number int, title, body *string) (GitHubItem, error)
	// Read is an issue or PR in one request: for a PR its CI and mergeability
	// too (IsPR says which it is), and the request budget in Rate.
	Read(ctx context.Context, r GitHubRepo, number int) (GitHubItem, error)
}

// dbChecks copies a reading's checks into the record's shape.
func dbChecks(c *gh.Checks) *db.LinkChecks {
	if c == nil {
		return nil
	}
	out := &db.LinkChecks{Failing: c.Failing, Pending: c.Pending, Passing: c.Passing, More: c.More}
	for _, f := range c.Failed {
		out.Failed = append(out.Failed, db.LinkCheck{Name: f.Name, URL: f.URL})
	}
	return out
}

// ghChecks copies a record's checks back into a reading's shape, for answers
// (an edit's, say) that carry none of their own.
func ghChecks(c *db.LinkChecks) *gh.Checks {
	if c == nil {
		return nil
	}
	out := &gh.Checks{Failing: c.Failing, Pending: c.Pending, Passing: c.Passing, More: c.More}
	for _, f := range c.Failed {
		out.Failed = append(out.Failed, gh.Check{Name: f.Name, URL: f.URL})
	}
	return out
}
