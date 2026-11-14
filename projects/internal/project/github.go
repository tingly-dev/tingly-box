package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// GitHubRepo names a repository on github.com: where a thread's issues and PRs
// go. (GitHub Enterprise hosts are a later step; see .design/github-tools.md.)
type GitHubRepo struct {
	Owner, Name string
}

// Slug is owner/name: what a record stores and the REST paths are built from.
func (r GitHubRepo) Slug() string { return r.Owner + "/" + r.Name }

func (r GitHubRepo) path() string { return "repos/" + r.Owner + "/" + r.Name }

// GitHubItem is an issue or PR as GitHub reports it.
type GitHubItem struct {
	Number int
	URL    string
	Title  string
	// Body is the description, as the item's author wrote it.
	Body string
	// State is open, closed or merged.
	State string
	// IsPR is set by Issue when the number is a PR.
	IsPR bool
	// The rest describe a PR.
	Draft      bool
	Head, Base string
	CI         string
	MergeState string
}

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
	// UpdatePR changes only the fields that are not nil.
	UpdatePR(ctx context.Context, r GitHubRepo, number int, title, body, base *string) (GitHubItem, error)
	// UpdateIssue changes only the fields that are not nil.
	UpdateIssue(ctx context.Context, r GitHubRepo, number int, title, body *string) (GitHubItem, error)
	// PR reads a PR with its CI and mergeability; Issue reads an issue (the
	// issues API serves PRs too, and says so in IsPR).
	PR(ctx context.Context, r GitHubRepo, number int) (GitHubItem, error)
	Issue(ctx context.Context, r GitHubRepo, number int) (GitHubItem, error)
}

// ghCLI is GitHub over the host's own `gh`, signed in as the user. Every call
// is a REST request through `gh api` with a JSON body: no GraphQL (`gh pr edit`
// fails on this repository's retired Projects classic), and no shell quoting.
type ghCLI struct {
	// bin is the gh executable; empty means "gh" on PATH.
	bin string
}

const ghTimeout = 60 * time.Second

// tokenShapes are the strings that must never leave this process: GitHub's
// token formats, bearer headers and credentials inside URLs.
var tokenShapes = []*regexp.Regexp{
	regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})\b`),
	regexp.MustCompile(`(?i)\b(authorization:?\s*(?:bearer|token)\s+)[^\s"']+`),
	regexp.MustCompile(`(?i)\b((?:gh|github)_token\s*[=:]\s*)[^\s"']+`),
	regexp.MustCompile(`(https?://)[^/@\s:]+(?::[^/@\s]*)?@`),
}

// redact removes anything that looks like a credential from text that is about
// to become an error, a log line or a tool result.
func redact(s string) string {
	s = tokenShapes[0].ReplaceAllString(s, "[redacted]")
	s = tokenShapes[1].ReplaceAllString(s, "${1}[redacted]")
	s = tokenShapes[2].ReplaceAllString(s, "${1}[redacted]")
	return tokenShapes[3].ReplaceAllString(s, "${1}")
}

// ErrGitHubUnavailable is gh missing or not signed in: the user's to fix, not
// the model's.
var ErrGitHubUnavailable = errors.New("GitHub is not available: gh is missing or not signed in on this machine (the user can run `gh auth login`)")

// api runs one `gh api` request and decodes the JSON answer into out.
func (g ghCLI) api(ctx context.Context, r GitHubRepo, method, path string, body any, out any) error {
	ctx, cancel := context.WithTimeout(ctx, ghTimeout)
	defer cancel()
	bin := g.bin
	if bin == "" {
		bin = "gh"
	}
	args := []string{"api", "-X", method}
	var in bytes.Buffer
	if body != nil {
		args = append(args, "--input", "-")
		if err := json.NewEncoder(&in).Encode(body); err != nil {
			return err
		}
	}
	args = append(args, path)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = &in
	// gh must never wait for a person.
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "NO_COLOR=1", "GH_NO_UPDATE_NOTIFIER=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
			return ErrGitHubUnavailable
		}
		msg := redact(strings.TrimSpace(stderr.String() + " " + stdout.String()))
		if strings.Contains(msg, "gh auth login") || strings.Contains(msg, "HTTP 401") {
			return ErrGitHubUnavailable
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("GitHub %s %s: %s", method, path, clip(msg, 600))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(stdout.Bytes(), out); err != nil {
		return fmt.Errorf("GitHub %s %s: unreadable answer", method, path)
	}
	return nil
}

type ghIssue struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	State   string `json:"state"`
	// PullRequest is set when the "issue" is a PR (the issues API serves both).
	PullRequest *struct{} `json:"pull_request"`
}

type ghPull struct {
	ghIssue
	Draft          bool   `json:"draft"`
	Merged         bool   `json:"merged"`
	Mergeable      *bool  `json:"mergeable"`
	MergeableState string `json:"mergeable_state"`
	Head           struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

func (i ghIssue) item() GitHubItem {
	return GitHubItem{Number: i.Number, URL: i.HTMLURL, Title: i.Title, Body: i.Body, State: i.State, IsPR: i.PullRequest != nil}
}

func (p ghPull) item() GitHubItem {
	it := p.ghIssue.item()
	if p.Merged {
		it.State = "merged"
	}
	it.Draft, it.Head, it.Base, it.MergeState = p.Draft, p.Head.Ref, p.Base.Ref, mergeState(p)
	return it
}

// patchBody is the fields of an edit that were given.
func patchBody(fields map[string]*string) map[string]any {
	body := map[string]any{}
	for k, v := range fields {
		if v != nil {
			body[k] = *v
		}
	}
	return body
}

// mergeState reduces GitHub's mergeable_state to what the card shows.
func mergeState(p ghPull) string {
	if p.Merged || p.State != "open" {
		return ""
	}
	switch p.MergeableState {
	case "dirty":
		return "conflicts"
	case "clean", "unstable", "has_hooks":
		return "ready"
	case "blocked", "behind":
		return p.MergeableState
	}
	if p.Mergeable != nil && !*p.Mergeable {
		return "conflicts"
	}
	return "unknown"
}

func (g ghCLI) DefaultBranch(ctx context.Context, r GitHubRepo) (string, error) {
	var out struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := g.api(ctx, r, "GET", r.path(), nil, &out); err != nil {
		return "", err
	}
	if out.DefaultBranch == "" {
		return "", errors.New("GitHub did not name the repository's default branch")
	}
	return out.DefaultBranch, nil
}

func (g ghCLI) Protected(ctx context.Context, r GitHubRepo, branch string) (bool, error) {
	var out struct {
		Protected bool `json:"protected"`
	}
	segs := strings.Split(branch, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	err := g.api(ctx, r, "GET", r.path()+"/branches/"+strings.Join(segs, "/"), nil, &out)
	if err != nil && strings.Contains(err.Error(), "HTTP 404") {
		return false, nil
	}
	return out.Protected, err
}

func (g ghCLI) CreateIssue(ctx context.Context, r GitHubRepo, title, body string) (GitHubItem, error) {
	var out ghIssue
	err := g.api(ctx, r, "POST", r.path()+"/issues", map[string]any{"title": title, "body": body}, &out)
	return out.item(), err
}

func (g ghCLI) Comment(ctx context.Context, r GitHubRepo, number int, body string) (string, error) {
	var out struct {
		HTMLURL string `json:"html_url"`
	}
	err := g.api(ctx, r, "POST", fmt.Sprintf("%s/issues/%d/comments", r.path(), number), map[string]any{"body": body}, &out)
	return out.HTMLURL, err
}

func (g ghCLI) OpenPR(ctx context.Context, r GitHubRepo, branch string) (*GitHubItem, error) {
	var out []ghPull
	q := url.Values{"head": {r.Owner + ":" + branch}, "state": {"open"}}
	if err := g.api(ctx, r, "GET", r.path()+"/pulls?"+q.Encode(), nil, &out); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	it := out[0].item()
	return &it, nil
}

func (g ghCLI) CreatePR(ctx context.Context, r GitHubRepo, title, body, head, base string, draft bool) (GitHubItem, error) {
	var out ghPull
	err := g.api(ctx, r, "POST", r.path()+"/pulls", map[string]any{"title": title, "body": body, "head": head, "base": base, "draft": draft}, &out)
	return out.item(), err
}

func (g ghCLI) UpdateIssue(ctx context.Context, r GitHubRepo, number int, title, body *string) (GitHubItem, error) {
	var out ghIssue
	err := g.api(ctx, r, "PATCH", fmt.Sprintf("%s/issues/%d", r.path(), number), patchBody(map[string]*string{"title": title, "body": body}), &out)
	return out.item(), err
}

func (g ghCLI) UpdatePR(ctx context.Context, r GitHubRepo, number int, title, body, base *string) (GitHubItem, error) {
	var out ghPull
	err := g.api(ctx, r, "PATCH", fmt.Sprintf("%s/pulls/%d", r.path(), number), patchBody(map[string]*string{"title": title, "body": body, "base": base}), &out)
	return out.item(), err
}

func (g ghCLI) PR(ctx context.Context, r GitHubRepo, number int) (GitHubItem, error) {
	var pr ghPull
	if err := g.api(ctx, r, "GET", fmt.Sprintf("%s/pulls/%d", r.path(), number), nil, &pr); err != nil {
		return GitHubItem{}, err
	}
	it := pr.item()
	if it.State != "open" {
		return it, nil
	}
	ci, err := g.ci(ctx, r, pr.Head.SHA)
	if err != nil {
		return GitHubItem{}, err
	}
	it.CI = ci
	return it, nil
}

// ci folds the commit's check runs and legacy statuses into one word.
func (g ghCLI) ci(ctx context.Context, r GitHubRepo, sha string) (string, error) {
	var runs struct {
		CheckRuns []struct {
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		} `json:"check_runs"`
	}
	if err := g.api(ctx, r, "GET", fmt.Sprintf("%s/commits/%s/check-runs?per_page=100", r.path(), sha), nil, &runs); err != nil {
		return "", err
	}
	var status struct {
		State      string            `json:"state"`
		Statuses   []json.RawMessage `json:"statuses"`
		TotalCount int               `json:"total_count"`
	}
	if err := g.api(ctx, r, "GET", fmt.Sprintf("%s/commits/%s/status", r.path(), sha), nil, &status); err != nil {
		return "", err
	}
	pending, failing := 0, 0
	for _, c := range runs.CheckRuns {
		switch {
		case c.Status != "completed":
			pending++
		case c.Conclusion == "failure" || c.Conclusion == "timed_out" || c.Conclusion == "cancelled" || c.Conclusion == "action_required":
			failing++
		}
	}
	if len(status.Statuses) > 0 {
		switch status.State {
		case "failure", "error":
			failing++
		case "pending":
			pending++
		}
	}
	switch {
	case len(runs.CheckRuns) == 0 && len(status.Statuses) == 0:
		return "none", nil
	case failing > 0:
		return "failing", nil
	case pending > 0:
		return "pending", nil
	}
	return "passing", nil
}

func (g ghCLI) Issue(ctx context.Context, r GitHubRepo, number int) (GitHubItem, error) {
	var out ghIssue
	err := g.api(ctx, r, "GET", fmt.Sprintf("%s/issues/%d", r.path(), number), nil, &out)
	return out.item(), err
}
