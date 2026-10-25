# MCP: tools, global switches and profiles

This replaces the three-page split in `mcp-responsibility-split.md` and the
per-client grants in `mcp-workspace.md`. Those documents describe the
experimental stack this one supersedes; they are merged into this file once the
UI lands.

## Model

There is one kind of thing, the **gateway tool**. It is either a tool from an
MCP connection the user added (any MCP, the standard gateway function) or a
**Server Tool** that Tingly Box provides itself (Advisor today). Server Tools
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
  from the name (see the Identifiers rule in `mcp-workspace.md`).
- Legacy endpoints (`tb`, `all`, one per connection) are gone. The feature has
  not shipped, so there is no migration.

## Pages

- **Tools** (`/mcp/tools`): connect any MCP, see every tool (MCP and Server
  Tool in one list), global switch, test a tool. Advisor's model settings live
  on its own row.
- **Profiles** (`/mcp/profiles`): Default first, then custom profiles. Each shows
  its endpoint with the install command, and its tool list with a switch per tool
  and an add/remove control. Tools that are globally off are shown greyed with the
  reason.

The old Tool / MCP / Server Tool pages and the "client" concept are removed.

## Open (carried, not decided here)

- Ordinary connected tools can currently also be injected into model requests
  (`usage.gateway`). This round keeps that mechanism untouched; whether it stays
  a per-tool flag or becomes a Default-profile concern is a follow-up.
- `usage.client` is no longer an input. Availability to MCP clients is derived:
  every non-virtual tool, subject to the switches above. Virtual tools (Advisor)
  need model context and are never exposed over an MCP endpoint.
