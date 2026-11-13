package project

import (
	"context"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// The GitHub tools a thread's session has: they act on the thread's own
// repository and branch (never named by the model), and leave a link record.
// .design/github-tools.md has the rules; the permission rules for the writing
// ones are written by grantGitHubTools.
const (
	toolCreateIssue  = "create_issue"
	toolCommentIssue = "comment_issue"
	toolCreatePR     = "create_pr"
	toolUpdatePR     = "update_pr"
	toolPRStatus     = "get_pr_status"
)

// githubWriteTools are the tools that publish something on GitHub.
var githubWriteTools = []string{toolCreateIssue, toolCommentIssue, toolCreatePR, toolUpdatePR}

func (f *sessionToolsMCP) githubTools() []Tool {
	l := f.service.Links
	numberProp := map[string]any{"type": "integer", "description": "Number of the issue or PR."}
	return []Tool{
		{Name: toolCreateIssue, ThreadOnly: true, Strict: true,
			Description: "Open an issue in this thread's GitHub repository (its origin). The system records it as one of this thread's issues. Write the title and body as the repository's issue template asks; nothing is added to them.",
			Properties:  map[string]any{"title": strProp("Issue title."), "body": strProp("Issue description in Markdown.")},
			Required:    []string{"title"},
			Handle: threadTool(f, func(ctx context.Context, t *db.Thread, in struct{ Title, Body string }) (string, error) {
				return l.CreateIssue(ctx, t, in.Title, in.Body)
			})},
		{Name: toolCommentIssue, ThreadOnly: true, Strict: true,
			Description: "Comment on an issue or PR of this thread's GitHub repository.",
			Properties:  map[string]any{"number": numberProp, "body": strProp("Comment in Markdown.")},
			Required:    []string{"number", "body"},
			Handle: threadTool(f, func(ctx context.Context, t *db.Thread, in struct {
				Number int
				Body   string
			}) (string, error) {
				return l.Comment(ctx, t, in.Number, in.Body)
			})},
		{Name: toolCreatePR, ThreadOnly: true, Strict: true,
			Description: "Push this thread's own branch (never forced) and open a pull request from it. The branch and repository are the thread's; if the branch already has an open PR it is returned, not duplicated. Write the title and body as the repository's PR template asks; nothing is added to them.",
			Properties: map[string]any{"title": strProp("PR title."), "body": strProp("PR description in Markdown."),
				"base":  strProp("Optional. Branch to merge into; default is this thread's base branch."),
				"draft": map[string]any{"type": "boolean", "description": "Open as a draft."}},
			Required: []string{"title"},
			Handle: threadTool(f, func(ctx context.Context, t *db.Thread, in struct {
				Title, Body, Base string
				Draft             bool
			}) (string, error) {
				return l.CreatePR(ctx, t, in.Title, in.Body, in.Base, in.Draft)
			})},
		{Name: toolUpdatePR, ThreadOnly: true, Strict: true,
			Description: "Change the title, description or base branch of a PR this thread opened. It cannot close or merge a PR.",
			Properties: map[string]any{"number": map[string]any{"type": "integer", "description": "PR number; optional when this thread has one PR."},
				"title": strProp("New title."), "body": strProp("New description (replaces the old one)."), "base": strProp("New base branch.")},
			Handle: threadTool(f, func(ctx context.Context, t *db.Thread, in struct {
				Number            int
				Title, Body, Base *string
			}) (string, error) {
				return l.UpdatePR(ctx, t, in.Number, in.Title, in.Body, in.Base)
			})},
		{Name: toolPRStatus, ThreadOnly: true, Strict: true,
			Description: "Read a PR of this thread: state, draft, checks and whether it can be merged. Readings under 30 seconds old are reused.",
			Properties:  map[string]any{"number": map[string]any{"type": "integer", "description": "PR number; optional when this thread has one PR."}},
			Handle: threadTool(f, func(ctx context.Context, t *db.Thread, in struct{ Number int }) (string, error) {
				return l.Status(ctx, t, in.Number)
			})},
	}
}
