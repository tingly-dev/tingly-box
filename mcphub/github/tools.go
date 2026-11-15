// The built-in GitHub MCP toolset: the official github-mcp-server's tool
// names over the host's gh, so a host that filters by that subset (Tingly
// Box's GitHub preset) can drop this server in without new names. Tools take
// owner/repo arguments — host-side scopes read exactly those.

package github

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tingly-dev/tingly-box/mcphub/tool"
)

// Tools is every tool the built-in server offers, over c.
func Tools(c Client) []tool.Definition {
	repoProps := func() map[string]any {
		return map[string]any{
			"owner": tool.StrProp("Repository owner (the GitHub account or organization name)"),
			"repo":  tool.StrProp("Repository name"),
		}
	}
	merge := func(maps ...map[string]any) map[string]any {
		out := map[string]any{}
		for _, m := range maps {
			for k, v := range m {
				out[k] = v
			}
		}
		return out
	}
	jsonOut := func(v any) (string, error) {
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	numberProp := func() map[string]any {
		return map[string]any{"type": "integer", "description": "Issue or pull request number"}
	}
	stateProp := func(def string) map[string]any {
		return map[string]any{"type": "string", "description": "Filter by state: open, closed or all", "default": def,
			"enum": []string{"open", "closed", "all"}}
	}

	type repoArgs struct{ Owner, Repo string }

	return []tool.Definition{
		{
			Name:        "get_me",
			Description: "Get the signed-in GitHub user",
			Strict:      true,
			Handle: tool.Typed(func(ctx context.Context, in struct{}) (string, error) {
				login, err := c.Me(ctx)
				if err != nil {
					return "", err
				}
				return jsonOut(map[string]string{"login": login})
			}),
		},
		{
			Name:        "list_issues",
			Description: "List a repository's issues (pull requests are not included)",
			Properties:  merge(repoProps(), map[string]any{"state": stateProp("open")}),
			Required:    []string{"owner", "repo"},
			Handle: tool.Typed(func(ctx context.Context, in struct {
				Owner, Repo, State string
			}) (string, error) {
				if in.State == "" {
					in.State = "open"
				}
				items, err := c.ListIssues(ctx, Repo{in.Owner, in.Repo}, in.State)
				if err != nil {
					return "", err
				}
				return jsonOut(items)
			}),
		},
		{
			Name:        "issue_read",
			Description: "Get an issue or pull request: title, body, state, and for a PR its CI and mergeability",
			Properties:  merge(repoProps(), map[string]any{"issue_number": numberProp()}),
			Required:    []string{"owner", "repo", "issue_number"},
			Handle: tool.Typed(func(ctx context.Context, in struct {
				Owner, Repo string
				IssueNumber int `json:"issue_number"`
			}) (string, error) {
				item, err := c.Read(ctx, Repo{in.Owner, in.Repo}, in.IssueNumber)
				if err != nil {
					return "", err
				}
				return jsonOut(item)
			}),
		},
		{
			Name:        "issue_write",
			Description: "Create an issue, or update one when issue_number is given (only the fields that are set change; state may be open or closed)",
			Properties: merge(repoProps(), map[string]any{
				"issue_number": numberProp(),
				"title":        tool.StrProp("Issue title (required to create)"),
				"body":         tool.StrProp("Issue body (markdown)"),
				"state":        map[string]any{"type": "string", "description": "New state, to update: open or closed", "enum": []string{"open", "closed"}},
			}),
			Required: []string{"owner", "repo"},
			Handle: tool.Typed(func(ctx context.Context, in struct {
				Owner, Repo, Title, Body, State string
				IssueNumber                     int `json:"issue_number"`
			}) (string, error) {
				r := Repo{in.Owner, in.Repo}
				if in.IssueNumber == 0 {
					if in.Title == "" {
						return "", fmt.Errorf("title is required to create an issue")
					}
					item, err := c.CreateIssue(ctx, r, in.Title, in.Body)
					if err != nil {
						return "", err
					}
					return jsonOut(item)
				}
				title, body := ptrOrNil(in.Title), ptrOrNil(in.Body)
				if in.State != "" && in.State != "open" && in.State != "closed" {
					return "", fmt.Errorf("state must be open or closed")
				}
				// The state rides along in the body's fields: pass it through Update.
				item, err := c.updateIssueState(ctx, r, in.IssueNumber, title, body, in.State)
				if err != nil {
					return "", err
				}
				return jsonOut(item)
			}),
		},
		{
			Name:        "add_issue_comment",
			Description: "Add a comment to an issue or pull request",
			Properties:  merge(repoProps(), map[string]any{"issue_number": numberProp(), "body": tool.StrProp("Comment text (markdown)")}),
			Required:    []string{"owner", "repo", "issue_number", "body"},
			Handle: tool.Typed(func(ctx context.Context, in struct {
				Owner, Repo, Body string
				IssueNumber       int `json:"issue_number"`
			}) (string, error) {
				url, err := c.Comment(ctx, Repo{in.Owner, in.Repo}, in.IssueNumber, in.Body)
				if err != nil {
					return "", err
				}
				return jsonOut(map[string]string{"url": url})
			}),
		},
		{
			Name:        "list_pull_requests",
			Description: "List a repository's pull requests",
			Properties:  merge(repoProps(), map[string]any{"state": stateProp("open")}),
			Required:    []string{"owner", "repo"},
			Handle: tool.Typed(func(ctx context.Context, in struct {
				Owner, Repo, State string
			}) (string, error) {
				if in.State == "" {
					in.State = "open"
				}
				items, err := c.ListPulls(ctx, Repo{in.Owner, in.Repo}, in.State)
				if err != nil {
					return "", err
				}
				return jsonOut(items)
			}),
		},
		{
			Name:        "pull_request_read",
			Description: "Get a pull request: title, body, state, CI checks and mergeability, in one request",
			Properties:  merge(repoProps(), map[string]any{"pull_number": numberProp()}),
			Required:    []string{"owner", "repo", "pull_number"},
			Handle: tool.Typed(func(ctx context.Context, in struct {
				Owner, Repo string
				PullNumber  int `json:"pull_number"`
			}) (string, error) {
				item, err := c.Read(ctx, Repo{in.Owner, in.Repo}, in.PullNumber)
				if err != nil {
					return "", err
				}
				return jsonOut(item)
			}),
		},
		{
			Name:        "create_pull_request",
			Description: "Open a pull request from one of this repository's branches",
			Properties: merge(repoProps(), map[string]any{
				"title": tool.StrProp("Pull request title"),
				"body":  tool.StrProp("Pull request body (markdown)"),
				"head":  tool.StrProp("Branch with the changes"),
				"base":  tool.StrProp("Branch to merge into (the repository's default branch when empty)"),
				"draft": map[string]any{"type": "boolean", "description": "Open as a draft"},
			}),
			Required: []string{"owner", "repo", "title", "head"},
			Handle: tool.Typed(func(ctx context.Context, in struct {
				Owner, Repo, Title, Body, Head, Base string
				Draft                                bool
			}) (string, error) {
				item, err := c.CreatePR(ctx, Repo{in.Owner, in.Repo}, in.Title, in.Body, in.Head, in.Base, in.Draft)
				if err != nil {
					return "", err
				}
				return jsonOut(item)
			}),
		},
		{
			Name:        "update_pull_request",
			Description: "Change a pull request's title, body or base branch (only the fields that are set change)",
			Properties: merge(repoProps(), map[string]any{
				"pull_number": numberProp(),
				"title":       tool.StrProp("New title"),
				"body":        tool.StrProp("New body (markdown)"),
				"base":        tool.StrProp("New base branch"),
			}),
			Required: []string{"owner", "repo", "pull_number"},
			Handle: tool.Typed(func(ctx context.Context, in struct {
				Owner, Repo, Title, Body, Base string
				PullNumber                     int `json:"pull_number"`
			}) (string, error) {
				item, err := c.UpdatePR(ctx, Repo{in.Owner, in.Repo}, in.PullNumber, ptrOrNil(in.Title), ptrOrNil(in.Body), ptrOrNil(in.Base))
				if err != nil {
					return "", err
				}
				return jsonOut(item)
			}),
		},
		{
			Name:        "list_branches",
			Description: "List a repository's branches",
			Properties:  repoProps(),
			Required:    []string{"owner", "repo"},
			Handle: tool.Typed(func(ctx context.Context, in repoArgs) (string, error) {
				names, err := c.ListBranches(ctx, Repo{in.Owner, in.Repo})
				if err != nil {
					return "", err
				}
				return jsonOut(names)
			}),
		},
		{
			Name:        "get_file_contents",
			Description: "Get one file of a repository (content is base64, as GitHub answers it)",
			Properties: merge(repoProps(), map[string]any{
				"path": tool.StrProp("Path to the file in the repository"),
				"ref":  tool.StrProp("Branch or tag or commit to read (the default branch when empty)"),
			}),
			Required: []string{"owner", "repo", "path"},
			Handle: tool.Typed(func(ctx context.Context, in struct {
				Owner, Repo, Path, Ref string
			}) (string, error) {
				f, err := c.FileContents(ctx, Repo{in.Owner, in.Repo}, in.Path, in.Ref)
				if err != nil {
					return "", err
				}
				return jsonOut(f)
			}),
		},
	}
}

// updateIssueState edits an issue's fields and, when asked, its state.
func (c Client) updateIssueState(ctx context.Context, r Repo, number int, title, body *string, state string) (Item, error) {
	fields := map[string]*string{"title": title, "body": body}
	editBody := patchBody(fields)
	if state != "" {
		editBody["state"] = state
	}
	var out ghIssue
	if err := c.api(ctx, r, "PATCH", r.path()+"/issues/"+strconv.Itoa(number), editBody, &out); err != nil {
		return Item{}, err
	}
	return out.item(), nil
}

func ptrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Serve runs the built-in GitHub MCP server over stdio, as the host binary's
// `mcp-github` subcommand. It ends when stdin closes.
func Serve(ctx context.Context) error {
	srv := mcp.NewServer(&mcp.Implementation{Name: "tingly-github", Version: "1.0.0"}, nil)
	for _, d := range Tools(Client{}) {
		if _, err := tool.Register(srv, d); err != nil {
			return err
		}
	}
	return srv.Run(ctx, &mcp.StdioTransport{})
}
