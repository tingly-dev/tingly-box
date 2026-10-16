// Package github is Tingly Box's GitHub integration: it keeps GitHub
// credentials on the host and hands a coding agent only what it needs.
// See .design/github.md.
//
// This file is the launch-time half (design §6): what every agent process
// is started with so it cannot push or administer GitHub on its own. It
// needs no App and no network, so it applies whether or not the `github`
// feature flag is on.
package github

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// Launch is the GitHub-related part of an agent's launch options.
type Launch struct {
	// Env entries (KEY=VALUE) added to the agent process.
	Env []string
	// DisallowedTools are Claude Code permission rules (`--disallowedTools`).
	DisallowedTools []string
	// AppendSystemPrompt tells the agent how to push and open PRs here.
	AppendSystemPrompt string
	// MCPServers are extra MCP servers (name → config) for `--mcp-config`.
	MCPServers map[string]interface{}
}

// Launcher resolves the Launch for one agent session. A nil Launcher is
// valid for callers that only want the static hardening (see BaseLaunch).
type Launcher interface {
	// Launch is called for every turn start. projectPath is the session's
	// working directory.
	Launch(ctx context.Context, projectPath, sessionID string) Launch
}

// denyRules are the permission rules every agent gets (design §6). They
// are a first layer: `bash -c` and pipes slip past prefix rules, which is
// what the guardrails policy "github-write-commands" is for.
var denyRules = []string{
	"Bash(gh auth:*)",
	"Bash(gh pr merge:*)",
	"Bash(gh repo delete:*)",
	"Bash(gh secret:*)",
	"Bash(gh release:*)",
	"Bash(git push:*)",
	// `gh api` turns into a write as soon as it has a method or a field.
	"Bash(gh api -X:*)",
	"Bash(gh api --method:*)",
	"Bash(gh api -f:*)",
	"Bash(gh api -F:*)",
	"Bash(gh api --field:*)",
	"Bash(gh api --raw-field:*)",
	"Bash(gh api --input:*)",
}

const promptReadOnly = "GitHub: you have no GitHub write access in this environment. " +
	"Commit locally if asked, but do not run `git push`, `gh pr create`, `gh pr merge` or any other " +
	"command that changes GitHub; tell the user what is ready to push instead."

const promptConnected = "GitHub: `gh` here is read-only (issues, PRs, CI logs). " +
	"To publish work use the `tb_push` tool (pushes the current HEAD to a tb/<session> branch) and then " +
	"`tb_pr_create`; use `tb_pr_update` and `tb_pr_comment` for follow-ups. " +
	"Never run `git push` or write through `gh`. Merging is done by the user on GitHub."

// BaseLaunch is the static hardening: env that stops git/gh from prompting,
// the deny rules, and the prompt for the "no write access" case.
func BaseLaunch() Launch {
	return Launch{
		Env: []string{
			"GH_PROMPT_DISABLED=1",
			"GIT_TERMINAL_PROMPT=0",
		},
		DisallowedTools:    slices.Clone(denyRules),
		AppendSystemPrompt: promptReadOnly,
	}
}

// EmptyGHConfigDir returns (creating it) the directory GH_CONFIG_DIR points
// at, so the agent's `gh` does not read the user's own login.
func EmptyGHConfigDir(dataDir string) (string, error) {
	dir := filepath.Join(dataDir, "gh-empty")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// Apply merges l into opts without discarding what the host already set.
func (l Launch) Apply(opts *agentboot.ExecutionOptions) {
	opts.Env = append(opts.Env, l.Env...)
	for _, rule := range l.DisallowedTools {
		if !slices.Contains(opts.DisallowedTools, rule) {
			opts.DisallowedTools = append(opts.DisallowedTools, rule)
		}
	}
	if p := strings.TrimSpace(l.AppendSystemPrompt); p != "" {
		if opts.AppendSystemPrompt != "" {
			opts.AppendSystemPrompt += "\n\n"
		}
		opts.AppendSystemPrompt += p
	}
	if len(l.MCPServers) > 0 {
		if opts.MCPServers == nil {
			opts.MCPServers = map[string]interface{}{}
		}
		for k, v := range l.MCPServers {
			opts.MCPServers[k] = v
		}
	}
}

// Prepare applies launcher's Launch (or BaseLaunch if launcher is nil) to
// opts. It is the one call each launch site makes.
func Prepare(ctx context.Context, launcher Launcher, projectPath, sessionID string, opts *agentboot.ExecutionOptions) {
	if launcher == nil {
		BaseLaunch().Apply(opts)
		return
	}
	launcher.Launch(ctx, projectPath, sessionID).Apply(opts)
}
