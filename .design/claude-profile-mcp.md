# Claude Code profiles and Tingly Box's MCP

> Status: **partly built** (2026-11-20): MCP token, short client tool names, no
> injection of connected MCP, Claude Code profile → `--mcp-config` for
> `tingly-box profile`/`cc` (§4). Projects launches and the Default (no-profile)
> path are next (§5). §2 is the investigation on `main` at `a7c593425`;
> `file:line` references there are from that commit.
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
2. **The copy command does not authenticate.** `/api/v1/mcp/:id` is under
   the V1 group with `UserAuthMiddleware` (`internal/middleware/auth.go:252`):
   only the UI user (admin) token is accepted. The copy command
   (`ProfileConnect.tsx`) writes `api.getToken()`, which is `GET /api/v1/token`
   = the **model** token, so a client registered with it gets 401. The
   only token that works is the admin one, which must not be handed to an
   agent (a prompt-injected model with Bash could read it from
   `~/.claude.json` and call every management API).
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

## 3. Decisions (user, 2026-11-20)

They replaced parts of the first proposal (an automatic default, Tingly Box
writing read-only allow rules, one token per MCP profile); §3a keeps the
comparison of ways to connect that still holds.

1. **Ordinary MCP is a standard MCP gateway, never injected.** Every MCP the
   user connects (anything but the built-in Server Tools) reaches an agent only
   the normal way: the agent is started with the MCP server configured and
   calls it itself. The gateway no longer injects connected MCP tools into
   model requests (`usage.gateway` is gone for them); Server Tools (Advisor,
   Search, Fetch) keep their gateway injection.
2. **The agent decides authorization.** Tingly Box writes no allow rules for
   its MCP server: Claude Code's own permission mode and the user's rules
   decide, as for any MCP server. (Unlike `tingly-projects`, which is Tingly
   Box's own bounded tools; #2579 stays as it is.)
3. **The user configures access.** Which MCP profile an agent gets is the
   user's choice on the Claude Code profile; nothing is connected implicitly.
4. **An MCP token, like the user token and the model token.** One global
   token (`tb-mcp-…`), shown and reset on Access Control, accepted only by the
   MCP endpoints. The copy command and generated configs use it; the admin
   token keeps working there too (it can already do everything).
5. **Order:** token, tool names, access (no injection; Claude Code profile →
   `--mcp-config`) first; Projects launches and the rest after.

## 3a. How to connect: options

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
  Connected MCP tools are no longer injected at all (decision 1, §4.3).

## 4. Built (option B, with the decisions of §3)

### 4.1 MCP token

`Config.MCPToken` (`tb-mcp-` + 64 hex, generated on load like the model
token), `GET /api/v1/auth/mcp-token`, `POST /api/v1/auth/mcp-token/reset`, a
third row on Access Control. The transport routes (`/api/v1/mcp/:id`,
`/stream`) moved from the user-authenticated V1 group to the public group
with `MCPAuthMiddleware` (`internal/middleware/auth.go`): the MCP token, or
whatever the user auth accepts. The MCP token on any other route is 401. The
copy command on the MCP profile page and the probe use it (the probe so it
takes the real path). One global token, not one per MCP profile: the user
asked for "an MCP token, like the user token"; per-profile scoping can come
later if a profile needs to be shared with someone else.

Why not per launch (as `tingly-projects`): a `tingly-box profile` launch
`exec`s Claude Code, so Tingly Box never learns when it ends; resumed sessions
and terminal handoff must keep working. Not the model token: it is shared by
every client and team key, and would turn "can call the model" into "can run
my GitHub writes".

### 4.2 Tool names

The endpoint lists `<source>__<tool>` (`coretool.ClientToolName`) and calls
take that name back (`ToolNameFromClient`); the catalog, profiles and
policies keep `tingly_box_mcp__<source>__<tool>`. The probe accepts both. The
longest GitHub tool is now
`mcp__tb__src-3f2a9c1e__add_reply_to_pull_request_comment` (56).

### 4.3 No injection of connected MCP

`SourceUsage` gives ordinary sources `Client` only and Server Tools (`advisor`
by ID too, `server-webtools`, virtual) `Gateway` only; stored `usage.gateway`,
per-tool usage and `visibility: server` are ignored. `ListServerToolsForInjection`
is the Server Tools only; the remote tool cache and `WithConnectedTools` are
gone. The toolround never claims a connected tool's call, so a model that
names one hands it to the client. Harness: `TestMCPRemoteNotInjected` (24
pairs × stream × transport; the old code fails every case).

### 4.4 Claude Code profile → MCP profile

- `typ.ProfileMeta.MCPProfile` (`mcp_profile`), empty = none, the user's
  choice, never inherited. Set with `PUT /scenario/claude_code/profiles/:id`
  `{"mcp_profile": "<id>" | ""}`, validated against the MCP profiles.
- `agent.MaterializeCCProfileMCPConfig` writes `mcp.json` next to the
  profile's `settings.json`, 0600, only when changed:
  `{"mcpServers": {"tb" | "tb-<slug>": {"type": "http", "url": ".../api/v1/mcp/<id>", "headers": {"Authorization": "Bearer <MCP token>"}}}}`.
  The server name matches the copy command (`MCPServerName` ↔
  `profileServerName`). No permission rules: Claude Code's own decide. MCP
  off, the MCP profile missing or switched off, or no choice: no file (a
  stale one is removed), no flag.
- `runCC` (`tingly-box profile <id>`, `cc --profile`, the `--resume` handoff)
  passes `--mcp-config <file>` **before** `--settings`: the flag is variadic,
  and next to the passthrough it would swallow a prompt (`ccExecArgs`). Not
  strict: the user's own servers keep loading.
- UI: an "MCP" row in the Claude Code profile page's connection card (None /
  MCP Integration / custom MCP profiles, a link to the chosen one, a warning
  when it is gone), shown while MCP is on.

## 5. Next

1. **Projects launches** (worker threads, folder sessions, IM chats): the
   embedded host's `Launcher` hands Projects the profile's `mcp.json` path (an
   optional `LaunchMCP(ctx, profile)`), agentboot takes several
   `--mcp-config` files (`tingly-projects` and `tb`). Coordinator stays off
   (strict config, it delegates). Standalone Projects is unchanged: threads
   get `tingly-projects` plus the user's own `~/.claude.json` servers.
2. **Default (no profile)**: the main `claude_code` scenario has no
   `ProfileMeta`; it needs its own `mcp_profile` (and `tingly-box cc` without
   a profile the same file under `claude/default/`).
3. `openapi.json` and the client SDK: `task codegen` for `/auth/mcp-token`
   and `mcp_profile` (the frontend calls them already; typecheck flags the
   two token paths until then).
4. MCP profile page: "Used by Claude Code profiles: …".
5. Later, on request: Codex/OpenCode equivalents; an "Add to Claude Code"
   button for `~/.claude.json`; per-MCP-profile tokens.

## 6. To probe

- Precedence when `tb` comes from both `~/.claude.json` and `--mcp-config`.
- Whether `--mcp-config` expands `${VAR}` in headers (would let the file hold
  no secret; today the plan is a 0600 file with the token).
- Claude Code's behaviour on a tool name over 64 characters.
- Tool search with tb-routed non-Claude models: are deferred MCP tools found,
  or is `ENABLE_TOOL_SEARCH=false` needed per profile (the coordinator needed it).
