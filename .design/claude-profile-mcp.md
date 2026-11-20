# Claude Code profiles and Tingly Box's MCP

> Status: **proposal, not built** (2026-11-20). Read-only investigation on `main`
> at `a7c593425`; `file:line` references are from that commit.
> Builds on `mcp-profiles.md` (gateway tools, MCP profiles), `github-mcp.md`,
> `remote-cc-profile.md` (how a Claude Code profile reaches the CLI),
> `projects/.design/mcp-tools.md` and `projects/.design/github-tools.md`
> (the `tingly-projects` server and its default-allow rule, PR #2579).

## 1. Terms

**Tingly Box's MCP** is two different things, and they reach Claude Code in
two different ways:

| Kind | What | Where | How a client reaches it |
|---|---|---|---|
| **Server Tools** | Advisor, Search, Fetch (Tingly Box runs them) | `internal/mcp/runtime/builtin_registry.go`, `internal/mcp/tools/`, source `server-webtools` | Only by **gateway injection** into model requests. Never on an MCP endpoint (`Usage.Client=false`, `runtime/policy.go:11-23`). |
| **MCP tools** | Tools of MCP connections the user added (any MCP, the GitHub preset) | `internal/mcp/runtime`, `typ.MCPSourceConfig` | Through an **MCP profile endpoint** `/api/v1/mcp/{profileId}` (`Usage.Client`), and optionally also by gateway injection when a tool has `usage.gateway`. |

`tingly-projects` (the Projects thread/coordinator server) is **not** Tingly
Box's MCP in this sense: it is Projects' own, per-thread, and already wired
(`projects/internal/project/session_tools_mcp.go`).

**Claude Code profile** is `typ.ProfileMeta` under scenario `claude_code`
(`internal/typ/type.go:383`), id `p1`, `p2`…, plus the main `claude_code`
scenario shown as "Default". Its persistent part is
`ClaudeCodeProfileConfig` (env delta, unset env, `defaultMode`); everything
the CLI sees is a generated, disposable artifact
`~/.tingly-box/claude/<id>--<name>/settings.json`
(`agent.MaterializeCCProfileSettings`, `internal/agent/cc_settings.go:431`)
passed with `--settings`. Every launcher names a profile by id:
`tingly-box profile <id>` / `cc --profile` (`internal/command/cc_command.go:48`),
Projects threads (`Project.ThreadProfile`, `Thread.ProfileOverride`), the
coordinator (`Project.CoordinatorProfile`), folder sessions and IM chats
(`Thread.Profile`; IM chats run as Projects threads since
`remote-into-projects.md`). agentboot has no profile concept; it only takes
`SettingsPath`, `MCPServers`, `MCPConfigPath`.

**MCP profile** (`typ.MCPProfile`, `internal/typ/mcp.go:28`) is the existing
whitelist: `default` (every tool, minus exceptions) or a custom `prof-xxxx`
(only the tools added). Not to be confused with the Claude Code profile.

## 2. Today

### 2.1 Gateway injection: on, global, invisible to Claude Code

- `ProtocolHandler.targetTransforms` adds the MCP chain when
  `serverToolsEnabled()` (`internal/protocolserver/protocol_transform.go:268`,
  `protocol_handler.go:154`); `MCPToolInjectionTransform.Apply`
  (`internal/protocolserver/transform/transform_mcp_tool_injection.go:36`)
  appends `Runtime.ListServerToolsForInjection` (`runtime.go:188`): the
  Server Tools that are on and ready, plus connected tools with
  `usage.gateway` when the experimental `mcp` flag is on (scenario `global`
  or `claude_code`).
- The Anthropic Tool Round Stage (`toolround.decide`) runs the owned calls
  on the server (up to 3 rounds) and strips their `tool_use` blocks from the
  stream, so **Claude Code never sees them and never asks permission**.
- **Scope is global**: every request on every scenario once enabled. Not per
  rule, scenario, Claude Code profile or MCP profile.
- Injected tools are appended after the client's tools; the Advisor also
  edits the system prompt. Toggling a tool or the remote-tool cache TTL
  changes the request prefix (prompt cache). `ListVirtualTools` iterates a
  Go map (`internal/tool/virtual.go`), so order is non-deterministic once
  there is more than one virtual tool.

So every Claude Code that routes through Tingly Box (any profile, any
launcher, Projects included) already gets the Server Tools that are on.

### 2.2 Client-side MCP: nothing is wired automatically

| Path | Writes Tingly Box MCP into Claude Code? | Code |
|---|---|---|
| Quick Config / `agent apply claude` | No. Writes `~/.claude/settings.json` env + hooks and `hasCompletedOnboarding` in `~/.claude.json`. | `internal/config/apply_config_claude.go`, `ai/agent/claude_code.go:112` |
| `tingly-box profile <id>` / `cc` | No. `exec claude --settings <artifact>`, no `--mcp-config`. | `internal/command/cc_command.go:120-125` |
| Projects worker thread, folder session, IM chat | No. Only `--mcp-config <tingly-projects tmp file>`, not strict, so the user's own `~/.claude.json` / `.mcp.json` servers also load. | `projects/internal/project/threads.go:922`, `session_tools_mcp.go:35` |
| Projects coordinator | No. `tingly-projects` only, `--strict-mcp-config`, `AllowedTools: mcp__tingly-projects`. | `projects/internal/project/coordinator_session.go:225` |
| Embedded host `Launcher` | Returns env + settings path only; no MCP in the interface. | `internal/server/module/projects/launch.go:29`, `projects/internal/project/launch.go:19` |
| MCP profile page | **Manual**: shows `claude mcp add --transport http tb "<base>/api/v1/mcp/<id>" --header "Authorization: Bearer <token>"` for the user to run. | `frontend/src/pages/mcp/ProfileConnect.tsx:29-33` |

agentboot can already pass MCP: `CommonOptions.MCPServers` (inline JSON) and
one `MCPConfigPath`, both emitted as `--mcp-config` (variadic), plus
`--strict-mcp-config` (`agentboot/claude/cli_builder.go:137-166`).

### 2.3 Gaps found

1. **No automatic client-side wiring** on any path (2.2).
2. **The endpoint takes the admin token.** `/api/v1/mcp/:id` is under the V1
   group with `UserAuthMiddleware` (`internal/middleware/auth.go:252`): only the
   UI user token is accepted. The copy command therefore writes the admin
   token into `~/.claude.json`, where any process (and a prompt-injected
   model with Bash) can read it and call every management API. This must be
   fixed before Tingly Box writes the endpoint into anything itself.
3. **Endpoint needs the experimental flag** (`mcp` on `global` or
   `claude_code`; `internal/mcp/local/transport.go:38`), otherwise 403.
4. **Doubled namespace.** The endpoint names tools
   `tingly_box_mcp__<source>__<tool>` (`internal/tool/name.go`); Claude Code
   adds `mcp__<server>__`, giving
   `mcp__tb__tingly_box_mcp__src-3f2a9c1e__pull_request_review_write`
   (64 chars) and, for the GitHub subset's
   `add_reply_to_pull_request_comment`, 72 chars: past the 64-char
   tool-name limit that `mcp-profiles.md` sized the source id for. What
   Claude Code does with an over-long name (truncate, drop) is not verified.
5. **Gateway injection cannot be scoped** to a profile (2.1).
6. **No permission story** for Tingly Box MCP tools in Claude Code: in
   `default` they prompt, and in `auto` the classifier may deny an unlisted
   MCP tool (probe in `github-tools.md`).

## 3. How to connect: options

Claude Code facts this relies on (from the official docs, read via search
summaries on 2026-11-20; items marked *unverified* need a probe):
`settings.json` (and so `--settings`) **cannot declare `mcpServers`**; servers
come from `~/.claude.json` (user/local scope), `.mcp.json` (project) or
`--mcp-config` (several files or JSON strings), which add to the others unless
`--strict-mcp-config`. One definition per server name wins, whole entry, no
field merge; where `--mcp-config` sits in that precedence is *unverified*.
Transports: `http` (streamable, recommended), `sse` (deprecated), `stdio`.
`${VAR}` expansion in `url`/`headers` is *unverified* for `--mcp-config`;
`headersHelper` exists. Permission rules: `mcp__<server>` (whole server),
`mcp__<server>__<tool>`, `mcp__<server>__get_*`. Tool search is on by default
(MCP tools deferred, list frozen at the first request, so connecting servers
does not break the cache); `ENABLE_TOOL_SEARCH=false` loads all upfront.

| | A. `~/.claude.json` `mcpServers` (what the copy command does) | B. `--mcp-config` file per launch, generated with the profile | C. Gateway injection (`usage.gateway`) |
|---|---|---|---|
| Scope | Every `claude` on the machine, any profile, also outside Tingly Box | Exactly the launches of that Claude Code profile | Every request through the gateway, every client |
| Follows the profile | No (one file for all) | Yes | No (global today) |
| Permission / approval | Claude Code rules and cards | Claude Code rules and cards; rules can ship in the same generated settings | None: runs server-side, user never sees the call |
| Visible in the transcript | Yes | Yes | No (tool_use stripped) |
| Token | Written into the user's file | Written into a 0600 file Tingly Box owns | None needed |
| Merge with user's MCP | Edits a user-owned file (backup, conflicts) | Additive, user files untouched; strict only where wanted (coordinator) | n/a |
| Prompt cache | Fixed per session | Fixed per session (tool search on: frozen at first request) | Prefix changes when switches, discovery or cache TTL change |
| Name collisions | User picks the name | Fixed server name, collapses with a manual registration of the same name | `tingly_box_mcp__…`, deduped against client tools |
| Works for non-Claude clients | Codex/OpenCode have their own files | Claude Code only | Yes |
| Loop limits | Claude Code's | Claude Code's | 3 rounds; mixed owned+client calls are split across requests |

**Recommendation: B for MCP tools, keep C for Server Tools only.**

- MCP tools are actions on the user's systems (GitHub writes, any connected
  server). They belong where the user can see and approve them: Claude Code's
  own permission system, in its transcript. B is the only option that follows
  the Claude Code profile, which is what the user chose for that launch.
- A is kept as the manual path for a plain `claude` outside Tingly Box
  (the copy command), not something Tingly Box writes automatically: it is
  machine-wide, edits a user file, and would leak across profiles.
- Server Tools are model capabilities (consult another model, search, fetch),
  not actions, and are not on any endpoint by design. They stay gateway-side.
  `usage.gateway` for ordinary MCP tools stays as it is (the open item in
  `mcp-profiles.md`); B does not depend on it. A tool that is both injected
  and on the profile is offered twice under two names; the profile page should
  say so (a later UI point, not a blocker).

## 4. Design (option B)

### 4.1 What a Claude Code profile declares

One field, a reference to an MCP profile; the tool list stays where it is
already curated (MCP profile page), not copied into the Claude Code profile:

```go
type ClaudeCodeProfileConfig struct {
    ...
    // MCPProfile is the MCP profile this Claude Code profile's launches get:
    // "" = inherit (custom profiles inherit the Default Claude Code profile's
    // choice), "off" = none, otherwise an MCP profile id ("default", "prof-…").
    MCPProfile string `json:"mcp_profile,omitempty" yaml:"mcp_profile,omitempty"`
}
```

The main `claude_code` scenario (Default Claude Code profile) stores its own
value in the scenario's config (same field, same meaning, `""` = automatic).

- **All vs whitelist** is answered by which MCP profile is chosen: `default`
  = all tools (newly connected ones included); a custom MCP profile = only its
  tools (never widens silently). No second whitelist on the Claude Code side.
- **Default value** (ux-principles §6: a smart default over a switch):
  automatic = `default` MCP profile **when the experimental MCP flag is on and
  at least one client tool is on**, otherwise nothing. With the flag off (the
  shipped default) nothing changes for anyone. An empty server is never
  written (no tools = no `--mcp-config` entry, no per-turn cost).
- A deleted MCP profile resolves to nothing and the Claude Code profile page
  shows a warning chip (as `remote-cc-profile.md` §1 does for a deleted Claude
  Code profile); a launch never fails because of it.

### 4.2 What is generated

`MaterializeCCProfileSettings` (and the default-profile path in `runCC`) also
writes, next to `settings.json` in the profile artifact dir, `mcp.json`, mode
0600, atomically and only when changed (`writeGeneratedFileIfChanged`):

```json
{"mcpServers": {"tb": {"type": "http",
  "url": "http://localhost:<port>/api/v1/mcp/<mcp-profile-id>",
  "headers": {"Authorization": "Bearer <mcp client token>"}}}}
```

- Base URL: the same loopback base the profile's `ANTHROPIC_BASE_URL` uses.
- Server name: `tb` for the `default` MCP profile and `tb-<slug>` for a custom
  one: the names the copy command already uses (`mcp-profiles.md`
  Identifiers), so a user who also registered it by hand gets one server, not
  the same tools twice. *Probe*: which definition wins on a name clash.
- The resolver returns `(settingsPath, mcpConfigPath)`; every launcher adds
  `--mcp-config <mcpConfigPath>` when it is non-empty.

### 4.3 Token: a scoped MCP client token, not the admin token

The endpoint accepts, besides the user token, an **MCP client token** that is
valid only on `/api/v1/mcp/{id}` (the transport routes) and only for the MCP
profile it was issued for. One token per MCP profile, stored in the MCP
runtime config, rotated from the MCP profile page. The copy command switches
to it too (fixes gap 2 for manual registrations).

Why stable per MCP profile rather than per launch (as `tingly-projects` does):
a `tingly-box profile` launch `exec`s Claude Code, so Tingly Box never learns
when it ends; resumed sessions, terminal handoff (`tingly-box cc --resume`)
and Projects' resident processes must keep working across turns and server
restarts. Projects can revoke its URLs because it owns the process lifetime;
Tingly Box does not.

Not reused: the model API token. It is shared by every client and team key,
and would turn "can call the model" into "can run my GitHub writes".

### 4.4 Permissions: Tingly Box decides what runs without a card

PR #2579 allows the whole `mcp__tingly-projects` server because those tools
are Tingly Box's own, bounded in code (no close, no force-push, only what the
thread opened, merge always asks). MCP profile tools are not: they are
whatever the user connected, writes included. So the rule differs:

- **Reads run without a card**: tools whose MCP annotation says
  `readOnlyHint: true`, and for the GitHub preset the read half of
  `githubSubset` (known, fixed list). Written as
  `permissions.allow: ["mcp__tb__<tool>", …]` into the generated
  `settings.json` at materialize time (the list is regenerated every launch,
  so it follows the switches). An unannotated tool is a write.
- **Writes ask** (no rule, so Claude Code's mode decides; in Projects the card
  goes through the thread prompter as for any tool).
- **Opt-in "run without asking"** on the MCP profile (one switch, writes
  `allow: ["mcp__tb"]`, whole server). Off by default. `deny` and the
  user's own `ask` rules still win (deny, ask, allow).
- The Projects prompter fallback that answers `mcp__tingly-projects__*` is
  **not** extended to `mcp__tb__*`: a profile or user `ask` rule on a tb tool
  is the user's decision and should reach them.

### 4.5 Namespace

Claude Code already namespaces by server (`mcp__tb__`), so the endpoint, when
serving a client, should name tools `<source>__<tool>` instead of
`tingly_box_mcp__<source>__<tool>`: 16 characters back, and the 72-char GitHub
name becomes 56. `ParseNormalizedToolName` accepts both on the call path. The
feature has not shipped (`mcp-profiles.md`), so manual registrations made so
far only need re-registering; their permission rules by full name would stop
matching, which is why this goes first.

### 4.6 Per launcher

| Launcher | Change |
|---|---|
| `tingly-box profile <id>` / `cc` (and the Default path) | `execArgs` gets `--mcp-config <mcp.json>`. Not strict: the user's own servers keep loading. |
| Projects worker thread, folder session, IM chat (embedded) | `Launcher` gains the MCP config path (new optional interface, e.g. `LaunchMCP(ctx, profile) (path string, err error)`, so `profile == ""` is covered too: today `""` goes to `ThreadEnv`, not to `LaunchEnv`). agentboot `ExecutionOptions` takes a list (`MCPConfigPaths`) so `tingly-projects` and `tb` are two files. The allow rules arrive with the host's settings file, which `workerSettings` already copies as the base. |
| Projects coordinator | **Off.** Strict config and `AllowedTools: mcp__tingly-projects` by design (it delegates; it does not act). Revisit only on request. |
| `tingly-box cc --resume` handoff | Same as the profile launch (it goes through `runCC`). |
| Quick Config / Apply (plain `claude`) | Unchanged. The MCP profile page's copy command (with the scoped token) is the way; an "Add to Claude Code" button that writes `~/.claude.json` with a backup is a later option, not this design. |

### 4.7 Standalone Projects (tp) vs embedded in Tingly Box

- **Standalone**: no MCP runtime, no `Launcher`. Threads get `tingly-projects`
  plus whatever the user registered in `~/.claude.json` (workers and folder
  sessions; the coordinator is strict). Nothing changes; Projects never learns
  about MCP profiles.
- **Embedded**: the host's `Launcher` hands Projects an opaque config path
  (no import of the root module, `task projects:boundary` stays green).
  Projects passes it through and writes no rule for it; the rules are in the
  host's settings file.
- Both: Projects' own server stays per-thread, capability URL, revoked with
  the thread; tb's stays per MCP profile with a scoped token.

### 4.8 UI

- **Claude Code profile page** (`ClaudeCodeProfilePage.tsx`), one row in the
  profile settings, next to permission mode: "MCP tools: Automatic (MCP
  Integration) / <MCP profile> / Off", showing the concrete result ("MCP
  Integration · 12 tools, 9 run without asking") and a link to that MCP
  profile (ux-principles §5, §11). Hidden while the MCP flag is off.
- **MCP profile page**: the connection card gains "Used by Claude Code
  profiles: Default, p2" and the "Run without asking" switch; the copy command
  uses the scoped token.
- Projects needs no UI: a thread's profile already decides.

### 4.9 Prompt cache and cost

The tool set is fixed per Claude Code session; with tool search on (the
default for Claude models) MCP tools are deferred and frozen at the first
request, so they add little to the prefix. With tool search off (as the
coordinator sets for small/compatible models) every on tool costs tokens on
every turn: the reason the automatic default writes nothing when no tool is
on, and why a narrow custom MCP profile is the answer for a heavy Default.
A change on the MCP profile reaches a running session only on its next
process (Projects resident processes keep the old list until restarted).

## 5. Phases (each its own PR, each shippable)

0. **Scoped MCP client token** (backend + copy command). Endpoint accepts it
   only on the transport routes of its MCP profile; UI token keeps working.
   Fixes gap 2 on its own, even if nothing else lands.
1. **Short tool names on the client endpoint** (`<source>__<tool>`), both
   names accepted on call. Golden test on the tool list of a profile.
2. **Materialize `mcp.json`** with the profile artifact, `mcp_profile` field
   with automatic default, `tingly-box profile` / `cc` pass `--mcp-config`.
   Read-only allow rules into the generated settings. Probe with the real CLI
   (isolated HOME, scripted model, `real-run-projects-with-scripted-model`
   method): server connects, a read runs without a card, a write asks, in
   `default`, `auto`, `acceptEdits`.
3. **Projects embedded**: `LaunchMCP` on the host, `MCPConfigPaths` in
   agentboot, worker/folder/IM threads get `tb`. Coordinator stays off.
   Boundary check green; `coordtest` unaffected.
4. **UI**: MCP row on the Claude Code profile page, "Used by" and "Run without
   asking" on the MCP profile page. Screenshots (`ui-screenshots`).
5. Later, on request: per-profile gateway injection (scope `usage.gateway`
   and Server Tools by Claude Code profile), an "Add to Claude Code" button for
   `~/.claude.json`, Codex/OpenCode equivalents of phase 2.

## 6. Open decisions (the user's)

1. Default: automatic (`default` MCP profile when the MCP flag is on and a
   tool is on), or off until chosen per Claude Code profile?
2. Default approval: reads without a card, writes ask (proposed), or the
   whole server without a card like `tingly-projects` (#2579)?
3. Coordinator: keep tb MCP off (proposed)?
4. Short tool names (phase 1): accept that existing manual registrations must
   re-register?
5. Token: one per MCP profile (proposed), or one per Claude Code profile?

## 7. To probe before building

- Precedence when `tb` comes from both `~/.claude.json` and `--mcp-config`.
- Whether `--mcp-config` expands `${VAR}` in headers (would let the file hold
  no secret; today the plan is a 0600 file with the token).
- Claude Code's behaviour on a tool name over 64 characters.
- Tool search with tb-routed non-Claude models: are deferred MCP tools found,
  or is `ENABLE_TOOL_SEARCH=false` needed per profile (the coordinator needed it).
