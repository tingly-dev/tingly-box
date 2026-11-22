# MCP tool groups: one group, one switch per level

Companion to `mcp-profiles.md` (the model: global switch per tool, profiles,
Server Tools). This document is about how every MCP tool list is laid out and
switched: as groups with a group switch, not as a flat list of rows.

## Problem (inventory, 2026-11-22)

The data is already a hierarchy, but the UI showed it as one only on one page:

| Level | What it is | Where it is switched |
| --- | --- | --- |
| Connection (source) | an MCP the user connected, or a Server Tool | `enabled`, in the connection drawer only |
| Tool | one tool of a connection | global switch (`tool_policies`), MCP Tools / Server Tools / drawer |
| Profile | an endpoint with its own tool list | profile switch, profile page header |
| Tool in profile | membership + own switch | profile page |

Where MCP UI lives (`frontend/src/pages/mcp/` unless noted):

- **MCP Tools** (`MCPToolsPage` → `MCPToolsPanel`): one card per connection,
  rows via `MCPToolList` → `MCPToolCard` → `MCPToolRow`. A grey bar "N of M
  tools on" plus an **All ▾ → Turn on / Turn off** menu was the only bulk control:
  two clicks, no visible group state, and a menu where a switch is expected.
- **Server Tools** (`MCPServerToolsPage`): the same panel.
- **Connection drawer** (`MCPSourceDrawer` → `MCPSourceWorkspace`): the
  connection switch, settings, then the same tools again as separate
  `MCPToolCard`s in a spaced stack — a different look from the card list, no
  count, no filter, no bulk control.
- **MCP profile** (`MCPProfilesPage`): one **flat** list over all connections,
  each row prefixed `source / tool`. No grouping, no group switch: switching off
  all GitHub tools in a profile meant one click per tool.
- **Add tools dialog** (profile page): grouped by connection with tri-state
  checkboxes — the one place that already had groups and a mixed state.
- **Claude Code profile** (`scenario/components/ClaudeCodeMCPRow`): picks an MCP
  profile from a select; no tool-level UI, out of scope.
- **Access Control** (`system/AccessControl`): the MCP token; no tools, out of scope.
- The Projects package has no MCP management UI (only transcript rendering of
  MCP tool calls).

Inconsistencies: three list shapes (card list, spaced cards, flat prefixed
list), bulk control as a menu in one place and absent elsewhere, the group
state ("some on") visible nowhere except the add dialog.

## Options

Previews: Option A is the implemented page (`pr-assets/pr<NNNN>/mcp-tools.png`,
`mcp-profile.png`); B and C are static mockups (`option-b.png`, `option-c.png`).

### A. Grouped cards (chosen)

One card per group. The card header is the group row: fold chevron, name,
quiet origin, status, settings, **"N of M tools on"** and a **group switch**;
the body is the tool rows, each with its own switch, aligned under the group
switch so the two levels read as a column.

- + Keeps what each connection already shows (target, missing values, errors)
  next to its tools; keeps the nested-card look the project prefers.
- + Same component on every page; the profile page simply groups its rows by
  connection.
- + No routing or data change.
- − Long pages with many connections: mitigated by folding (remembered per
  group) and the existing 5-row fold / 8-row filter inside a group.

### B. One tree table

All connections in one bordered table; a connection is a parent row with a
group switch, tools are indented children.

- + Densest; scans like a file tree.
- − Connection details (URL / command, missing-value chips, errors, settings)
  have no room in a 40px parent row; they move to the drawer and failures
  become harder to see.
- − Diverges from the card language used everywhere else.

### C. Master–detail

Connections in a left list with their group switch; the selected connection's
tools on the right.

- + Scales to dozens of connections; group switches all visible at once.
- − Only one connection's tools are visible at a time (comparing or editing
  across connections takes extra clicks); needs a separate phone layout; the
  biggest refactor.

**Recommendation: A.** It fixes the actual complaint (no group, no group
switch, one click per tool) on every page with one component, without moving
information away from where it is acted on. B is a reasonable later density
mode for the profile page if profiles grow very large; C only pays off with many
connections, which is not the case today.

## Design (A)

**Component.** `MCPToolGroup` (`frontend/src/pages/mcp/MCPToolGroup.tsx`) is
the one group: header + optional summary + rows. `MCPToolRow` stays the one
row. `TriStateSwitch` (`@tingly/ui/components/TriStateSwitch`, with
`triStateOf`) is the group switch: on when all are on, off when none are, a
hollow half-way thumb when some are. Clicking a mixed group turns it all on
(the tri-state checkbox convention). The count text beside it carries the
meaning; the mixed look is only a cue.

**What the group switch writes — always one save, never a new field:**

| Page | Group | Group switch writes |
| --- | --- | --- |
| MCP Tools, Server Tools | connection | `tool_policies.enabled` of every tool (`toolsPolicyPatch`); tools the allow list restricts are skipped and do not count toward the state |
| Connection drawer | the connection's tools | the same patch |
| MCP profile | connection inside the profile | the profile's own switch of each row (`setProfileToolsOn`) |

The group switch is **not** the connection switch. "Connection on" starts or
stops the connection (and lives in the drawer); the group switch is the tools'
policy. Keeping them apart follows ux-principles §4 (one knob, one thing). A
connection that is off shows its group switch off and disabled, with "Off" in
the status.

**Rules.**

- No group switch for a group of one tool (nothing to switch together).
- With a filter active (more than 8 tools), "Matching ▾" turns the matched rows
  on or off; the group switch always means the whole group.
- Fold state is per group and remembered on this machine
  (`mcp-group-folded-<key>`); groups start open. The summary (target, missing
  values, errors) stays visible when folded.
- On a custom profile the group header has a remove button that takes the whole
  connection's tools out of the profile; rows keep their own remove.
- The profile's overall "N of M tools on" moves next to the section heading.

**Not changed:** the Add tools dialog keeps its grouped tri-state checkboxes
(it selects, it does not switch), the Claude Code MCP profile picker, the API
and the stored config.

## Open

- Whether the connection switch should also appear in the group header (it
  would put two switches with different meanings side by side; not done).
- A compact tree mode (option B) for very large profiles.
