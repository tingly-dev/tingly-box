# MCP: tools, global switches and profiles

This is the single current design for the MCP area. It replaces the earlier
Tool / MCP / Server Tool split and the per-client grants; those documents were
removed with it.

## Model

There is one kind of thing, the **gateway tool**. It is either a tool from an
MCP connection the user added (any MCP, the standard gateway function) or a
**Server Tool** that Tingly Box provides itself (Advisor, Search and Fetch). Server Tools
differ per vendor and cannot be connected by the user, so the only difference
the user sees is that they have a switch and nothing else.

Two layers of switch, deliberately:

1. **Global switch** per tool. A tool that is off is off for every profile. This
   is the control point for people who manage the gateway; most users never touch it.
2. **Profile.** A profile is one entry point (its own endpoint). It has its own
   tool list, and each tool in it has its own switch.

A tool is available through a profile when all three hold: globally on, in the
profile, on in the profile. Profiles are not tied to a client. Codex and Claude
Code simply pick one.

### Default and custom profiles

- Exactly one profile exists out of the box: `default`. It cannot be deleted or
  renamed. It contains every tool, so a newly connected MCP shows up in it
  immediately with its tools on. Only exceptions are stored (a tool switched off
  in Default); membership is implicit.
- Custom profiles are created by the user. They start empty and contain only the
  tools the user adds. New tools are never added to them automatically, so a
  narrow profile never widens silently. Removing a tool from a profile and
  switching it off are different: off keeps it listed.
- Custom profile IDs are random with a prefix (`prof-3f2a9c1e`), never derived
  from the name (see Identifiers).
- Legacy endpoints (`tb`, `all`, one per connection) are gone. The feature has
  not shipped, so there is no migration.

## Pages

Profiles live in the sidebar, as Claude Code's and Image's do: under Tools there
is **Server Tools**, then **MCP Tools**, then one row per MCP profile and "New MCP Profile".

**Naming.** The `default` profile is shown as **MCP Integration** with the
caption "Default MCP profile" (a bare "Default" in the sidebar did not say what
it was); its id stays `default`. Copy never says just "profile": it is always
"MCP profile", because Claude Code and Image have profiles of their own in the
same sidebar. The page where MCPs are connected stays **MCP Tools**.

- **Server Tools** (`/mcp/server-tools`): the tools Tingly Box runs itself
  (Advisor, Search and Fetch). Their own page because they are a different kind of thing: no MCP
  connection, not in profiles, and they work with the experimental MCP flag off.
  Switch, test, and the consultation model (`?source=` opens the same drawer).
- **MCP Tools** (`/mcp/tools`): connect any MCP, see its tools, global switch,
  test a tool. Server Tool sources are filtered out of this list. Each connection card shows what
  it is configured with (URL or command line, how many headers and env vars are
  set) and flags what is still missing (empty URL, command, env or header value,
  Advisor model) with a chip that opens the connection drawer
  (`?source=<id>`).
- **Profile** (`/mcp/profile/:id`, `/mcp/profile/new` creates one), laid out like
  the Claude Code profile page: the header holds the name with rename and delete
  beside it (custom only), a status line, the profile switch, refresh, Verify and
  the filled primary action (Add tools). Directly under it the connection card
  (the shared `AgentAccessSection`: endpoint URL when folded; open it is two rows,
  the endpoint URL and the whole registration command for Claude Code / Codex /
  OpenCode with the user token already written in: nothing is read from the
  machine or an environment variable), then the tool list with a switch
  per tool (custom ones also remove). Tools that are globally off are shown greyed
  with the reason; both Default and custom profiles carry a one-line explanation of what they contain. The sidebar
  reads the list from `profileStore.ts`, which the page keeps current.

Tool-call arguments are not configuration: they are shown read-only on the tool
card and filled in the Test dialog.

The old Tool / MCP / Server Tool pages and the "client" concept are removed.
`/mcp`, `/mcp/profiles`, `/mcp/clients`, `/mcp/routes` land on Default;
`/mcp/sources` lands on MCP Tools and `/tools/servertool` on Server Tools.

## Tool lists: one row language

MCP Tools, Server Tools and the MCP profile page show tools the same way: one
bordered card per connection (or per profile) and one 40px row per tool, not a
card per tool. Rules (visual tokens as in `projects-visual.md`):

- One component (`MCPToolRow`) renders every row; callers only decide what the
  switch means (global on MCP Tools, in-profile on a profile) and pass Remove or
  a caption where needed.
- Row: monospace name, one-line truncated description, switch. Click the row to
  expand its schemas. A tool that is off or restricted is greyed as a whole row.
- Schemas are plain JSON (parameters, output), each with a copy button that
  copies the raw schema. No custom rendering. Each block has a fixed height and
  scrolls inside, so every tool expands to the same size however big its schema.
- Test (and Remove, on a profile) appear on hover or focus, always on touch.
- Status is a dot and a word. Connected, Off and Checking are neutral; amber is
  "needs you" (missing value, unconfigured), red is failed. The connection's
  Settings button is quiet and becomes the filled primary only for Fix connection.
- A header bar says "N of M tools on". With more than 8 tools it adds a filter;
  with more than 5 only the first 5 show until "Show all". All/Matching on/off
  writes one patch for every tool at once; restricted tools are skipped.
- A built-in connection says "Runs inside Tingly Box" instead of its command
  line (the command stays in the connection drawer).

## Identifiers

Source and profile IDs are addressing, not naming. They are never typed by the
user and never derived from the display name: new ones are random with a kind
prefix (`src-3f2a9c1e`, `prof-9b41d07a`). A sequence (`src1`) was rejected
because deleting and re-creating would reuse an ID and silently re-point saved
commands and tool lists at a new object. The suffix is 8 hex characters rather
than a full UUID because the ID is embedded in the normalized tool name
(`tingly_box_mcp__<id>__<tool>`), which models cap at 64 characters. The prefix
also keeps IDs from colliding with management routes under `/mcp/`. Each profile
is registered on the user's machine under its own name (`tb` for Default,
`tb-<slug of the name>` otherwise) so installs do not overwrite each other.

## Where the pieces are

- Model and effective policy: `internal/typ/mcp.go` (`MCPProfile`),
  `internal/mcp/runtime/policy.go` (`ProfileAllows`, `SourceUsage`),
  `internal/mcp/runtime/routing.go` (the routing snapshot both pages read).
- API: `/api/v1/mcp/profiles[/{id}[/probe]]`; the endpoint of a profile is
  `/api/v1/mcp/{id}`.
- UI: `frontend/src/pages/mcp/` (`MCPToolsPage`, `MCPProfilesPage`,
  `profileState.ts`, `profileStore.ts`, shared `useMCPWorkspace`).

## Server Tools work without any MCP connection

Server Tools have gateway-only semantics independent of their implementation.
Advisor is virtual; Search and Fetch use the existing `mcp-builtin` stdio binary
under reserved source ID `server-webtools`. The original `webtools` source remains
an independent MCP connection with independent switches and credentials.

Advisor is on by default but is only offered after a consultation provider is
configured. Server Search and Fetch are **off by default**, with both per-tool
policies explicitly off. Enabling one tool must not enable its sibling. Search
requires an explicitly configured `SERPER_API_KEY` in that source's env (a user
may explicitly supply an environment reference); Fetch uses Jina Reader and
requires no key. Startup refreshes the built-in executable wiring, not the
user's switches, allow list, env or policies.

Disabled server web tools retain static schemas from the same definitions used
by the stdio binary, so their configuration and test actions stay discoverable
without launching a subprocess. Catalog `enabled` remains policy state, not key
readiness. Server versions never appear in MCP profiles or client tool lists.

The experimental MCP flag governs connected MCP tools only. Server Tools can be
injected, called and tested with the flag off once enabled and ready. Injection
continues to support only existing Chat and Anthropic paths; no Responses or
other protocol support is added. Native Search/Fetch definitions are removed only
when the corresponding server replacement was actually injected and remains in
that request. Explicit native named choices retain their native definitions;
Advisor loopback requests, missing search keys, disabled tools and unsupported
paths retain native tools.

## GitHub

The GitHub integration (a fixed subset plus per-profile repository scope) is
described in `github-mcp.md`.

## Open (carried, not decided here)

- Ordinary connected tools can currently also be injected into model requests
  (`usage.gateway`). This round keeps that mechanism untouched; whether it stays
  a per-tool flag or becomes a Default-profile concern is a follow-up.
- `usage.client` is no longer an input. Availability to MCP clients is derived:
  every ordinary MCP tool, subject to the switches above. Server Tools (Advisor,
  Search and Fetch) are gateway-only and never exposed over an MCP endpoint.
