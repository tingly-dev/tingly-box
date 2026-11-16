package tools

import "strings"

// GitHubPreset marks a source as the GitHub integration (the official
// github-mcp-server, remote or local). The integration is a fixed, curated
// subset: only the tools below are ever offered or callable, whatever the
// upstream server exposes and however the source is configured.
const GitHubPreset = "github"

// GitHubRemoteEndpoint is the hosted github-mcp-server.
const GitHubRemoteEndpoint = "https://api.githubcopilot.com/mcp/"

// githubSubset is every upstream tool the integration allows. Anything not
// listed is dropped from catalogs and refused at call time. Nothing here
// deletes, merges, creates repositories, writes files or branches, triggers
// workflows or searches across the whole account; code changes belong to the
// agent's own checkout.
var githubSubset = map[string]bool{
	"get_me": true,
	// repositories, read only
	"get_file_contents": true, "list_branches": true, "list_commits": true,
	"get_commit": true, "get_repository_tree": true, "get_latest_release": true,
	"get_release_by_tag": true, "list_releases": true, "list_tags": true, "get_tag": true,
	// issues
	"issue_read": true, "list_issues": true, "get_label": true,
	"add_issue_comment": true, "issue_write": true, "update_issue_comment": true, "sub_issue_write": true,
	// pull requests
	"pull_request_read": true, "list_pull_requests": true, "create_pull_request": true,
	"update_pull_request": true, "pull_request_review_write": true,
	"add_comment_to_pending_review": true, "add_reply_to_pull_request_comment": true,
	// actions, read only
	"actions_get": true, "actions_list": true, "get_job_logs": true,
}

// GitHubToolAllowed reports whether an upstream tool belongs to the subset.
func GitHubToolAllowed(name string) bool { return githubSubset[name] }

// GitHubToolNeedsRepo reports whether a subset tool acts on one repository and
// so carries owner/repo arguments. Only get_me does not.
func GitHubToolNeedsRepo(name string) bool { return name != "get_me" }

// GitHubRepoAllowed reports whether owner/repo is in a scope list. Matching is
// case-insensitive, as GitHub's is.
func GitHubRepoAllowed(scope []string, owner, repo string) bool {
	full := strings.ToLower(strings.TrimSpace(owner)) + "/" + strings.ToLower(strings.TrimSpace(repo))
	for _, s := range scope {
		if strings.ToLower(strings.TrimSpace(s)) == full {
			return true
		}
	}
	return false
}
