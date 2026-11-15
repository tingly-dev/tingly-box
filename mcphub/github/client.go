// Package github is GitHub over the host's own gh CLI: the client the issue
// and PR tools need, used by both Tingly Box and Projects. Everything goes
// through `gh api` as the signed-in user; no token is managed here.
package github

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

// Repo names a repository on github.com: where a thread's issues and PRs
// go. (GitHub Enterprise hosts are a later step; see Projects'
// .design/github-tools.md.)
type Repo struct {
	Owner, Name string
}

// Slug is owner/name: what a record stores and the REST paths are built from.
func (r Repo) Slug() string { return r.Owner + "/" + r.Name }

func (r Repo) path() string { return "repos/" + r.Owner + "/" + r.Name }

// Item is an issue or PR as GitHub reports it.
type Item struct {
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
	// Checks break an open PR's CI down.
	Checks *Checks
	// Rate is what GitHub said about the caller's request budget in the same
	// answer; zero when the answer did not carry it.
	Rate Rate
}

// Rate is the request budget GitHub reports: how many points are left
// and when the window resets.
type Rate struct {
	Remaining int
	ResetAt   time.Time
}

// Checks is an open PR's checks as GitHub last said: how many of each, and
// the failing ones by name (at most ChecksListed, the rest counted in More).
type Checks struct {
	Failing, Pending, Passing, More int
	Failed                          []Check `json:"failed,omitempty"`
}

// ChecksListed is how many failing checks a record names.
const ChecksListed = 5

// Check is one failing check.
type Check struct {
	Name, URL string
}

// ErrUnavailable is gh missing or not signed in: the user's to fix, not
// the model's.
var ErrUnavailable = errors.New("GitHub is not available: gh is missing or not signed in on this machine (the user can run `gh auth login`)")

// tokenShapes are the strings that must never leave this process: GitHub's
// token formats, bearer headers and credentials inside URLs.
var tokenShapes = []*regexp.Regexp{
	regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})\b`),
	regexp.MustCompile(`(?i)\b(authorization:?\s*(?:bearer|token)\s+)[^\s"']+`),
	regexp.MustCompile(`(?i)\b((?:gh|github)_token\s*[=:]\s*)[^\s"']+`),
	regexp.MustCompile(`(https?://)[^/@\s:]+(?::[^/@\s]*)?@`),
}

// Redact removes anything that looks like a credential from text that is about
// to become an error, a log line or a tool result.
func Redact(s string) string {
	s = tokenShapes[0].ReplaceAllString(s, "[redacted]")
	s = tokenShapes[1].ReplaceAllString(s, "${1}[redacted]")
	s = tokenShapes[2].ReplaceAllString(s, "${1}[redacted]")
	return tokenShapes[3].ReplaceAllString(s, "${1}")
}

// ParseRemote reads owner and name from a github.com remote URL in any
// of its spellings. Credentials in the URL are dropped, never kept or shown.
func ParseRemote(remote string) (Repo, error) {
	remote = strings.TrimSpace(remote)
	var host, path string
	switch {
	case strings.Contains(remote, "://"):
		u, err := url.Parse(remote)
		if err != nil {
			return Repo{}, errors.New("origin is not a GitHub repository")
		}
		host, path = u.Hostname(), u.Path
	case strings.Contains(remote, "@") && strings.Contains(remote, ":"):
		// scp-like: git@github.com:owner/name.git
		rest := remote[strings.Index(remote, "@")+1:]
		host, path, _ = strings.Cut(rest, ":")
	default:
		return Repo{}, errors.New("origin is not a GitHub repository")
	}
	parts := strings.Split(strings.Trim(strings.TrimSuffix(strings.Trim(path, "/"), ".git"), "/"), "/")
	if !strings.EqualFold(host, "github.com") || len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Repo{}, errors.New("origin is not a github.com repository; the issue and PR tools only work with github.com")
	}
	return Repo{Owner: parts[0], Name: parts[1]}, nil
}

// Client is GitHub over the host's own `gh`, signed in as the user. Every call
// is a REST request through `gh api` with a JSON body: no GraphQL (`gh pr edit`
// fails on this repository's retired Projects classic), and no shell quoting.
type Client struct {
	// Bin is the gh executable; empty means "gh" on PATH.
	Bin string
}

const ghTimeout = 60 * time.Second

// clip shortens s to n runes with an ellipsis.
func clip(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}

// api runs one `gh api` request and decodes the JSON answer into out.
func (g Client) api(ctx context.Context, r Repo, method, path string, body any, out any) error {
	ctx, cancel := context.WithTimeout(ctx, ghTimeout)
	defer cancel()
	bin := g.Bin
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
			return ErrUnavailable
		}
		msg := Redact(strings.TrimSpace(stderr.String() + " " + stdout.String()))
		if strings.Contains(msg, "gh auth login") || strings.Contains(msg, "HTTP 401") {
			return ErrUnavailable
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

func (i ghIssue) item() Item {
	return Item{Number: i.Number, URL: i.HTMLURL, Title: i.Title, Body: i.Body, State: i.State, IsPR: i.PullRequest != nil}
}

func (p ghPull) item() Item {
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

// DefaultBranch is the repository's default branch.
func (g Client) DefaultBranch(ctx context.Context, r Repo) (string, error) {
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

// Protected is whether GitHub holds the branch protected; a branch that is
// not on GitHub is not.
func (g Client) Protected(ctx context.Context, r Repo, branch string) (bool, error) {
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

func (g Client) CreateIssue(ctx context.Context, r Repo, title, body string) (Item, error) {
	var out ghIssue
	err := g.api(ctx, r, "POST", r.path()+"/issues", map[string]any{"title": title, "body": body}, &out)
	return out.item(), err
}

// Comment adds a comment to an issue or PR and returns its URL.
func (g Client) Comment(ctx context.Context, r Repo, number int, body string) (string, error) {
	var out struct {
		HTMLURL string `json:"html_url"`
	}
	err := g.api(ctx, r, "POST", fmt.Sprintf("%s/issues/%d/comments", r.path(), number), map[string]any{"body": body}, &out)
	return out.HTMLURL, err
}

// OpenPR is the open PR whose head is branch of this repository, if any.
func (g Client) OpenPR(ctx context.Context, r Repo, branch string) (*Item, error) {
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

func (g Client) CreatePR(ctx context.Context, r Repo, title, body, head, base string, draft bool) (Item, error) {
	var out ghPull
	err := g.api(ctx, r, "POST", r.path()+"/pulls", map[string]any{"title": title, "body": body, "head": head, "base": base, "draft": draft}, &out)
	return out.item(), err
}

// SquashPR squash-merges an open PR on GitHub. sha is the head commit the
// caller means to merge: GitHub refuses when the PR's head has moved. Nothing
// else is touched (the branch is not deleted).
func (g Client) SquashPR(ctx context.Context, r Repo, number int, sha, title, message string) error {
	body := map[string]any{"merge_method": "squash", "sha": sha}
	if title != "" {
		body["commit_title"] = title
		body["commit_message"] = message
	}
	var out struct {
		Merged  bool   `json:"merged"`
		Message string `json:"message"`
	}
	if err := g.api(ctx, r, "PUT", fmt.Sprintf("%s/pulls/%d/merge", r.path(), number), body, &out); err != nil {
		return err
	}
	if !out.Merged {
		return fmt.Errorf("GitHub did not merge PR #%d: %s", number, clip(out.Message, 300))
	}
	return nil
}

// Me is the signed-in user's login.
func (g Client) Me(ctx context.Context) (string, error) {
	var out struct {
		Login string `json:"login"`
	}
	if err := g.api(ctx, Repo{}, "GET", "user", nil, &out); err != nil {
		return "", err
	}
	return out.Login, nil
}

// ListIssues is a repository's issues (and only issues: PRs are skipped) in
// the given state ("open", "closed" or "all").
func (g Client) ListIssues(ctx context.Context, r Repo, state string) ([]Item, error) {
	var out []ghIssue
	q := url.Values{"state": {state}}
	if err := g.api(ctx, r, "GET", r.path()+"/issues?"+q.Encode(), nil, &out); err != nil {
		return nil, err
	}
	var items []Item
	for _, i := range out {
		if i.PullRequest == nil {
			items = append(items, i.item())
		}
	}
	return items, nil
}

// ListPulls is a repository's PRs in the given state.
func (g Client) ListPulls(ctx context.Context, r Repo, state string) ([]Item, error) {
	var out []ghPull
	q := url.Values{"state": {state}}
	if err := g.api(ctx, r, "GET", r.path()+"/pulls?"+q.Encode(), nil, &out); err != nil {
		return nil, err
	}
	var items []Item
	for _, p := range out {
		items = append(items, p.item())
	}
	return items, nil
}

// ListBranches is a repository's branch names, newest first.
func (g Client) ListBranches(ctx context.Context, r Repo) ([]string, error) {
	var out []struct {
		Name string `json:"name"`
	}
	if err := g.api(ctx, r, "GET", r.path()+"/branches", nil, &out); err != nil {
		return nil, err
	}
	names := make([]string, len(out))
	for i, b := range out {
		names[i] = b.Name
	}
	return names, nil
}

// File is one file as GitHub serves it.
type File struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// FileContents is one file of a repository at a ref (empty ref: the default
// branch). Content is base64, as GitHub answers.
func (g Client) FileContents(ctx context.Context, r Repo, path, ref string) (File, error) {
	q := url.Values{}
	if ref != "" {
		q.Set("ref", ref)
	}
	var out struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	p := r.path() + "/contents/" + path
	if len(q) != 0 {
		p += "?" + q.Encode()
	}
	if err := g.api(ctx, r, "GET", p, nil, &out); err != nil {
		return File{}, err
	}
	return File{Path: out.Path, Content: out.Content}, nil
}

func (g Client) UpdateIssue(ctx context.Context, r Repo, number int, title, body *string) (Item, error) {
	var out ghIssue
	err := g.api(ctx, r, "PATCH", fmt.Sprintf("%s/issues/%d", r.path(), number), patchBody(map[string]*string{"title": title, "body": body}), &out)
	return out.item(), err
}

// UpdatePR changes only the fields that are not nil.
func (g Client) UpdatePR(ctx context.Context, r Repo, number int, title, body, base *string) (Item, error) {
	var out ghPull
	err := g.api(ctx, r, "PATCH", fmt.Sprintf("%s/pulls/%d", r.path(), number), patchBody(map[string]*string{"title": title, "body": body, "base": base}), &out)
	return out.item(), err
}

// readQuery is the whole of a read: the item, for a PR its head commit's
// checks, and the request budget, in one request that costs one point. (Three
// REST requests used to do this for a PR, and two for an item of unknown kind.)
const readQuery = `query($owner:String!,$name:String!,$number:Int!){
  rateLimit{remaining resetAt}
  repository(owner:$owner,name:$name){
    issueOrPullRequest(number:$number){
      __typename
      ... on Issue{number url title body state}
      ... on PullRequest{number url title body state isDraft headRefName baseRefName mergeable mergeStateStatus
        commits(last:1){nodes{commit{statusCheckRollup{state contexts(first:100){totalCount nodes{
          __typename
          ... on CheckRun{name status conclusion detailsUrl}
          ... on StatusContext{context state targetUrl}
        }}}}}}
      }
    }
  }
}`

type gqlContext struct {
	Typename   string `json:"__typename"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	DetailsURL string `json:"detailsUrl"`
	Context    string `json:"context"`
	State      string `json:"state"`
	TargetURL  string `json:"targetUrl"`
}

type gqlRead struct {
	Data struct {
		RateLimit struct {
			Remaining int       `json:"remaining"`
			ResetAt   time.Time `json:"resetAt"`
		} `json:"rateLimit"`
		Repository struct {
			Item *struct {
				Typename         string `json:"__typename"`
				Number           int    `json:"number"`
				URL              string `json:"url"`
				Title            string `json:"title"`
				Body             string `json:"body"`
				State            string `json:"state"`
				IsDraft          bool   `json:"isDraft"`
				HeadRefName      string `json:"headRefName"`
				BaseRefName      string `json:"baseRefName"`
				Mergeable        string `json:"mergeable"`
				MergeStateStatus string `json:"mergeStateStatus"`
				Commits          struct {
					Nodes []struct {
						Commit struct {
							Rollup *struct {
								State    string `json:"state"`
								Contexts struct {
									TotalCount int          `json:"totalCount"`
									Nodes      []gqlContext `json:"nodes"`
								} `json:"contexts"`
							} `json:"statusCheckRollup"`
						} `json:"commit"`
					} `json:"nodes"`
				} `json:"commits"`
			} `json:"issueOrPullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

// Read is an issue or PR in one request: for a PR its CI and mergeability
// too (IsPR says which it is), and the request budget in Rate.
func (g Client) Read(ctx context.Context, r Repo, number int) (Item, error) {
	var out gqlRead
	body := map[string]any{"query": readQuery, "variables": map[string]any{"owner": r.Owner, "name": r.Name, "number": number}}
	if err := g.api(ctx, r, "POST", "graphql", body, &out); err != nil {
		return Item{}, err
	}
	n := out.Data.Repository.Item
	if n == nil {
		return Item{}, fmt.Errorf("GitHub has no issue or PR #%d in %s", number, r.Slug())
	}
	it := Item{Number: n.Number, URL: n.URL, Title: n.Title, Body: n.Body, State: strings.ToLower(n.State), IsPR: n.Typename == "PullRequest",
		Rate: Rate{Remaining: out.Data.RateLimit.Remaining, ResetAt: out.Data.RateLimit.ResetAt}}
	if !it.IsPR {
		return it, nil
	}
	it.Draft, it.Head, it.Base = n.IsDraft, n.HeadRefName, n.BaseRefName
	if it.State != "open" {
		return it, nil
	}
	it.MergeState = graphqlMergeState(n.MergeStateStatus, n.Mergeable)
	if len(n.Commits.Nodes) > 0 {
		if roll := n.Commits.Nodes[0].Commit.Rollup; roll != nil {
			it.CI, it.Checks = foldChecks(roll.State, roll.Contexts.Nodes, roll.Contexts.TotalCount)
		}
	}
	if it.CI == "" {
		it.CI = "none"
	}
	return it, nil
}

// graphqlMergeState reduces GitHub's merge status to what the card shows (the
// same words mergeState gives for the REST answer).
func graphqlMergeState(status, mergeable string) string {
	switch status {
	case "DIRTY":
		return "conflicts"
	case "CLEAN", "UNSTABLE", "HAS_HOOKS":
		return "ready"
	case "BLOCKED":
		return "blocked"
	case "BEHIND":
		return "behind"
	}
	if mergeable == "CONFLICTING" {
		return "conflicts"
	}
	return "unknown"
}

// foldChecks folds a head commit's check runs and statuses into one word and
// the breakdown the details show. More contexts than were read (over 100) leave
// the rollup's own word the last say, so a failure past the cut still shows.
func foldChecks(rollup string, nodes []gqlContext, total int) (string, *Checks) {
	var sum Checks
	fail := func(name, url string) {
		sum.Failing++
		if len(sum.Failed) < ChecksListed {
			sum.Failed = append(sum.Failed, Check{Name: name, URL: url})
		} else {
			sum.More++
		}
	}
	for _, c := range nodes {
		if c.Typename == "StatusContext" {
			switch c.State {
			case "FAILURE", "ERROR":
				fail(c.Context, c.TargetURL)
			case "PENDING", "EXPECTED":
				sum.Pending++
			default:
				sum.Passing++
			}
			continue
		}
		switch {
		case c.Status != "COMPLETED":
			sum.Pending++
		case c.Conclusion == "FAILURE" || c.Conclusion == "TIMED_OUT" || c.Conclusion == "CANCELLED" || c.Conclusion == "ACTION_REQUIRED" || c.Conclusion == "STARTUP_FAILURE":
			fail(c.Name, c.DetailsURL)
		default:
			sum.Passing++
		}
	}
	switch {
	case total == 0 && len(nodes) == 0:
		return "none", nil
	case sum.Failing > 0 || (total > len(nodes) && (rollup == "FAILURE" || rollup == "ERROR")):
		return "failing", &sum
	case sum.Pending > 0 || (total > len(nodes) && rollup == "PENDING"):
		return "pending", &sum
	}
	return "passing", &sum
}
