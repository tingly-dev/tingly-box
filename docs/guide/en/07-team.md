# Team

Path: `/agent/team` (default team) or `/agent/team/:slug` (additional teams); `/agent/team/keys` (Team Keys overview)

![Team Workspace](../images/team-workspace.png)

**Team** is its own top-level entry in the left Activity Bar — it is no longer a scenario nested under Agent. It moved out of the Agent sidebar as a dedicated nav group, with its own rail icon, its own workspace pages, and a dedicated Sharing Keys management surface. It is visible in the sidebar by default (the eye-icon toggle on [Scenario Overview](./02-scenario-overview.md) still hides/shows it, since visibility is driven by the same `team` scenario id).

---

## What a Team Is

Multi-team workspaces give each team its own isolated routing configuration and Sharing Keys, so a single shared Tingly-Box instance can serve several teams — customers, sub-teams, external apps — without their model rules or API keys leaking into each other.

- A **Sharing Key** belongs to exactly one Team and can reach only `/tingly/team` and `/tingly/team/v1` — never other Teams, other scenario endpoints (Claude Code, Codex, …), or management APIs.
- Your instance-wide **Global Model Token** is a separate, unscoped credential and keeps full cross-scenario access; it is unaffected by Team boundaries.
- Moving, disabling, or deleting a key or a Team takes effect immediately.

---

## Workspace Navigation

Teams get their own **profile-style** navigation block in the sidebar, mirroring how Claude Code Profiles work:

- Each Team is a separate nav item, subtitled `slug - name`; the built-in **Default** team (stable id, slug `default`) is always first.
- **Add Team** at the bottom of the block opens an inline popover to name and create a new Team — the system assigns the slug (`t1`, `t2`, …; a deleted Team's slug number can be reused, but the underlying Team ID never is, so old rules/keys/audit records can never be misattributed to a new Team reusing the slug).
- A divider, then **Team Keys** — an overview across every Team's Sharing Keys, placed last since it spans the whole list above it.

---

## Team Workspace Page

Same page shape as any other scenario page:

1. **Provider Configuration Card**, titled `Team - <name>`:
   - An **info** icon shows the Sharing Key access-scope tooltip (keys work only against `/tingly/team` and `/tingly/team/v1` — they cannot reach other teams, scenario endpoints, or management APIs)
   - A **How Team works** (help) icon opens the Team Guide dialog (see below)
   - An **edit** icon opens **Team settings** to rename the team; a **delete** icon (non-default teams only) removes it — you must move or delete its Sharing Keys first
   - **Enabled** toggle (top-right): disabling a team blocks its Sharing Keys from reaching model endpoints, without deleting the team or its configuration
   - **Sharing Keys** button opens the per-team key management dialog
   - Same **Plugins** row as other scenarios (Thinking / Smart Compact / Vision Proxy / Record)
2. **Model Rules** (collapsible): routing rules scoped to this team, independent of every other team's

---

## Team Guide Dialog

A 4-step walkthrough, opened from the help icon on the workspace page header — a stepper with Previous/Next controls and a language toggle:

1. **What a Team is for** — an isolated slice of the instance (its own rules, Sharing Keys, usage), for giving a group model access without sharing your main setup
2. **How it's separated** — a Sharing Key belongs to exactly one Team and can only reach `/tingly/team[/v1]`; your Global Model Token is not Team-scoped and keeps full access
3. **Configure a Sharing Key** — how to create one from the Sharing Keys button, and move it between teams without rotating it
4. **How it's used** — point the client at the Team's Base URL using its Sharing Key as the API key; only models added to that Team's rules are reachable

---

## Sharing Keys

Two surfaces manage the same underlying tokens:

- **Sharing Keys dialog**, opened from a Team's workspace page — the keys belonging to that one Team
- **Team Keys page** (`/agent/team/keys`, its own item under the Team nav block) — every Sharing Key across every Team on the instance, grouped into one section per Team (empty Teams included, so each can take its first key), for scanning or moving keys across Teams without opening each workspace individually

![Team Keys](../images/team-keys.png)

Both render the same table:

| Column | Description |
|--------|-------------|
| Name | Display name given at creation |
| User | Creator's user id (truncated) |
| Token | Masked value with show/hide and copy |
| Status | Enabled/disabled switch |
| Created | Creation timestamp |
| Last Used | Last-used timestamp, or a dash |
| Actions | **Move** (reassign to a different **enabled** team, without rotating the key) and **Delete** (immediate, irreversible) |

- **Create Token** is available per Team; the button stays visible but disabled — with the reason on hover — for a disabled Team, so the failure surfaces before the dialog rather than after.
- On the Team Keys page, each Team's section header shows the team name, slug, a key count chip, an **Inactive** chip if the Team is disabled, an **Open Team** shortcut to its workspace page, and its own **Create Token** button.

---

## Related Pages

- [Scenario Overview](./02-scenario-overview.md)
- [Custom / Embed](./06-scenario-special.md)
- [Usage Dashboard](./11-dashboard.md)
