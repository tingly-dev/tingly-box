# Team

Path: `/agent/team` (default team) or `/agent/team/:slug` (additional teams); `/agent/team/keys` (Team Keys overview)

![Team Workspace](../images/team-workspace.png)

**Team** is its own top-level item in the left rail, with its own workspace pages and key management. It is shown by default; the **Team** switch in the [Power-ups menu](./02-scenario-overview.md#power-ups-the-menu-behind-the-gear) hides or shows the rail item (nothing the Teams manage stops).

---

## What a Team Is

Multi-team workspaces give each team its own isolated routing configuration and **Team Keys**, so a single shared Tingly-Box instance can serve several teams — customers, sub-teams, external apps — without their model rules or API keys leaking into each other.

- A **Team Key** (`tb-share-…`) belongs to exactly one Team and can reach only `/tingly/team` and `/tingly/team/v1` — never other Teams, other scenario endpoints (Claude Code, Codex, …), or management APIs.
- Your instance-wide **Global Model Token** is a separate, unscoped credential and keeps full cross-scenario access; it is unaffected by Team boundaries.
- Moving, disabling, or deleting a key or a Team takes effect immediately.

> Older versions called these "Sharing Keys" (and the standalone *API Tokens* / *Sharing* page, `/tingly-box-token`, which now redirects to Team Keys). The UI now says **Team Key** everywhere.

---

## Workspace Navigation

Teams get a **profile-style** block in the sidebar, like Claude Code Profiles:

- Each Team is a nav item named by its **name**, captioned with its slug; the built-in **Default** team (slug `default`) is always first.
- **Add Team** opens an inline popover to name a new Team — the system assigns the slug (`t1`, `t2`, …; a deleted Team's slug number may be reused, but the underlying Team ID never is, so old rules/keys/audit records can't be misattributed).
- After a divider, **Team Keys** (an info icon explains the scope) — an overview across every Team's keys, last because it spans the whole list above it.

---

## Team Workspace Page

The standard agent page ([Claude Code § Page Structure](./03-scenario-claude-code.md#page-structure)) with a Team-shaped header and the Team's keys in view:

**Header**

- **Title** `Team - <name>` with four small icons: **info** (key access scope: keys work only on `/tingly/team` and `/tingly/team/v1`), **?** (the *How Team works* guide, below), **edit** (*Team settings*: rename) and, for non-default Teams, **delete** (move or delete the Team's keys first)
- **Status line** leading with the slug: `default · No requests yet`
- **Enabled** switch: disabling a Team stops its keys from reaching model endpoints, without deleting anything; a warning explains while it is off
- **Requests / Usage / Log**, as on every agent page

**Team Keys card** — part of the header, right below it. Folded it is one line, `Team Keys · 1 key`; click to open (the choice is remembered for every Team on this machine). Open, it lists each key — one per person you hand the Team to:

![Team Keys card, open](../images/team-workspace-keys.png)

- **Create Token** (top right of the card) — dialog *Create Team Key*: a display name, then copy the `tb-share-…` value, **shown only once**. For a disabled Team the button stays visible but disabled, with the reason on hover
- each row: name, masked key with **show** and **copy**, **Last Used**, an enable/disable **switch**, **Move to another team** and **Delete** (immediate, irreversible)

**Connection card** — folds to one `Base URL` line (`/tingly/team`) once the Team has been used. It has **no API Key row**: that would be this instance's own key; Team members connect with their Team Key.

**Plugins card** and **Model Rules** — rules scoped to this Team, independent of every other Team's. **New Model**, **Connect AI**, **Test All** work as elsewhere.

---

## Team Guide Dialog

The **?** icon (also **Help → How Team works**) opens a 4-step walkthrough:

1. **What a Team is for** — an isolated slice of the instance (its own rules, Team Keys, usage); give a group model access without sharing your main setup
2. **How it's separated** — a Team Key belongs to exactly one Team and can only reach `/tingly/team[/v1]`; moving or disabling a key or Team is immediate; your Global model token isn't Team-scoped
3. **Configure a Team Key** — create one from the Team Keys card; copy the `tb-share-…` value (shown only once); move it to another Team anytime without rotating it
4. **How it's used** — point the client at the Team's Base URL with its Team Key as the API key; requests route through the rules below — only models added there are reachable

---

## Team Keys Overview

![Team Keys](../images/team-keys.png)

**Team Keys** in the sidebar (`/agent/team/keys`) lists every Team's keys on one page, grouped into one section per Team (empty Teams included, so each can take its first key), for scanning or moving keys without opening each workspace. An info banner repeats the scope rule. Each section header shows the Team name, slug, a key-count chip, an **Inactive** chip if the Team is disabled, **Open Team**, and its own **Create Token**. The table:

| Column | Description |
|--------|-------------|
| Name | Display name given at creation |
| User | The key's user id (truncated) |
| Token | Masked value with show/hide and copy |
| Status | Enabled/disabled switch |
| Created | Creation timestamp |
| Last Used | Last-used timestamp, or a dash |
| Actions | **Move** (reassign to a different **enabled** Team without rotating the key) and **Delete** |

The same actions are available from each Team's own Team Keys card.

---

## Related Pages

- [Scenario Overview](./02-scenario-overview.md)
- [Custom / Embed](./06-scenario-special.md)
- [Usage Dashboard](./11-dashboard.md) — **Team usage** breaks usage down by Team and account
