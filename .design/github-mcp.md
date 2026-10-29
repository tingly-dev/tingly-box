# GitHub MCP: a fixed subset of the official server

Tracking issue: TEP #2308. Builds on `mcp-profiles.md`.

## Decision

GitHub support is the official `github/github-mcp-server`, connected as an
ordinary MCP source (remote `https://api.githubcopilot.com/mcp/` with a PAT as
Bearer). We do not write GitHub tools ourselves. What Tingly Box adds is a
**preset** and two limits, because the upstream server exposes 80+ tools, about
half of them writes, including destructive and account-wide ones.

## The subset is fixed

A source with `preset: "github"` only ever offers the tools in
`internal/mcp/tools/github.go` (`githubSubset`): `get_me`; repository reads
(files, branches, commits, tree, releases, tags); issues (reads, comments,
`issue_write`, sub-issues); pull requests (reads, create/update, review
comments); Actions reads (`actions_get`, `actions_list`, `get_job_logs`).

Not offered, not switchable, not callable: deleting or creating repositories,
fork, file/branch writes (code changes belong to the agent's own checkout),
merge, branch update, workflow triggers, governance, notifications, gists,
Copilot assignment, and all account-wide search tools.

Why fixed rather than a configurable denylist: the user asked to rule out
harmful or mistaken configuration. A list the user can edit can be edited
wrongly. Per-tool switches inside the subset still work like any MCP tool.

Enforcement is in the runtime, not only the tool list:
- `EffectiveToolPolicy` disables any tool outside the subset, so `CallTool`
  refuses it even if a model guesses the name;
- the config API refuses to change a source's `preset` after creation, so the
  subset cannot be switched off by editing the source;
- `DiscoverSource` drops those tools from the catalog, so they never appear in
  the UI or profiles.

## Repository scope per MCP profile

`MCPProfile.github_repos` (`owner/repo`, case-insensitive) limits the GitHub
tools reached through that profile. Empty means unscoped; Default is never
scoped in the UI. `Runtime.CheckToolScope` runs in `MCPRuntimeAdapter.CallTool`
(the profile endpoint call path) and refuses a call whose `owner`/`repo`
arguments are missing or outside the list. Every subset tool except `get_me`
takes both arguments, which is why account-wide search is excluded rather than
rewritten.

Not covered, on purpose: the model-injection path (`usage.gateway`) has no
profile, so it is unscoped. A GitHub source is client-only by default and the
UI never sets gateway usage.

## UI

- MCP Tools, Connect tools: a toggle "Any MCP | GitHub". GitHub asks only for
  a token and shows what is included.
- MCP profile page (custom profiles, when a GitHub source exists): a
  "GitHub repositories" section with chips and an `owner/repo` field.

## Left for later

- Default scope from a Projects project's folders (needs the folder to repo
  mapping from Projects).
- Local stdio mode for GitHub Enterprise (`GITHUB_HOST`); remote only for now.
- OAuth instead of PAT.
