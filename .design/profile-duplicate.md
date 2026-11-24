# Duplicate a profile, the main Claude Code config, or a Team

Three sources, one mechanism: a routing scope (a scenario name — `claude_code`,
`claude_code:pN`, `team`, `team:<id>`) is a set of rules plus an optional own
scenario config. `Config.cloneScopeLocked` (`internal/config/scope_clone.go`)
deep-copies one scope onto an empty one; the per-type wrappers only deal with
what lives outside the scope (profile metadata, the team row).

```
POST /api/v1/scenario/{scenario}/profiles/{id}/duplicate  {name?}  → ProfileMeta
POST /api/v1/teams/{team_id}/duplicate                    {name?}  → TeamInfo
```

`id = default` on the profile endpoint is the main configuration
(`ProfileMainID`; "default" is a reserved profile name). Reimplements the old,
never-merged PR #1888 on top of the slot model (TEP-2197); that PR used one
`/scenario/{scenario}/{id}/duplicate` for everything and also copied the main
config's "current mode" rule set, which no longer exists.

## Decisions

- **Rule identity:** built-in rules keep the `builtin:<scope>:<slot>` shape under
  the new scope (generated settings and slot logic look rules up by UUID; a
  legacy `built-in-cc-*` rule on main maps to its modern form); any other rule
  gets a fresh UUID. Services and smart routing are deep-copied; usage stats
  live outside the config and start empty.
- **A base config is inherited, not snapshotted.** A profile/team falls back to
  its base scenario's config (`scenarioConfigLocked`), so copying the main
  config or the Default team copies rules only and the copy keeps following the
  base. Copying anything else also copies its own scenario config.
- **Profile copy** also copies the Claude Code overrides (`env`, `unset_env`,
  `default_mode`) and writes the copy's settings file so it launches right away.
  A copy of the main config has no overrides (it inherits from main, like any
  profile).
- **Team copy: sharing keys are never copied.** They are credentials, not
  routing; the copy starts with none and is enabled. The team row (SQLite) and
  its routing (config) cannot be saved atomically, so the row is created first
  and taken back out if the routing copy fails. `db.TeamScenario` is the single
  team → scope mapping, shared with the team scope middleware.
- **Deleting a team removes its routing** (`Config.DeleteTeam`): copies make
  teams with rules common, and a deleted team's `team:<id>` rules would
  otherwise linger unreachable. The store decides first (default team, keys
  left), so a refused delete changes nothing. A one-time migration
  (`20261104-drop-orphan-team-scopes`) sweeps routing of teams that no longer
  exist. It is once-only on purpose: the team store is a different database
  from the config, and a restored or recreated team DB would otherwise make
  every live team's routing look orphaned on each boot.
- **Names.** Profile: optional; empty picks `<name>-copy`, `-copy-2`, … (a
  profile name is also a URL alias, so unsafe characters become `-`). Team:
  free text, unique, ≤128 bytes; empty picks `<name> copy`, `<name> copy 2`, …
  The UI prefills the same suggestion (`frontend/src/utils/copyName.ts`).
- **Failure:** validation runs before any write; a failed save is undone in
  memory so a retry is not refused. Unknown source → 404, anything else → 400
  (Teams: 409 for a taken name).
- **UI:** one `DuplicateDialog` (prefilled free name, Enter confirms, stays open
  on failure); entry points on the profile page, the main Claude Code page
  ("Duplicate as profile") and the Team page. Each lands on the copy.
- Not covered: MCP profiles (a tool list, not rules — frontend-only, Default
  must be materialized into an explicit list) and Image profiles (browser
  storage; copy belongs in the store with its backend).
