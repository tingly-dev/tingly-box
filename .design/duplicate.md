# Duplicate a Team or Claude Code profile

A Team (`team:<id>`) and a Claude Code profile (`claude_code:pN`) are the same
thing seen from routing — a sub-scope of a scenario with its own rules and
flags — so one endpoint duplicates both:

```
POST /api/v1/scenario/{scenario}/{id}/duplicate  {name}  →  {id, name, scenario}
```

`id = default` is the scenario's main scope (Default team; main Claude Code
configuration). Implementation: `Config.Duplicate` in
`internal/server/config/duplicate.go`.

## Decisions

- **Sharing keys are never copied.** They are credentials, not routing; a
  copy must not grant anyone access.
- **Main config → profile takes the current mode's rule set.** The main Claude
  Code config keeps both rule sets and toggles them by the Separate flag; a
  profile has one fixed mode.
- **A base config is inherited, not snapshotted.** `team:<id>` and
  `claude_code:pN` fall back to `team` / `claude_code` config
  (`scenarioConfigLocked`). Copying the Default team or the main config
  therefore copies rules only, so the copy keeps following base edits.
- **Built-in rules keep a UUID identity.** A profile's generated settings look
  each tier up by `builtin:<scenario>:<tier>`, not by request model (the main
  haiku rule routes on `tingly/cc-haiku`). So `builtin:<src>:<x>` becomes
  `builtin:<dst>:<x>`; other rules get fresh UUIDs. Getting this wrong fails
  silently (the profile falls back to an unrouted model name) — the handler
  test asserts the generated `ANTHROPIC_MODEL`.
- **Failure:** validation runs before any write; a failed save is undone in
  memory, and a team whose routing copy fails is deleted again. Unknown
  source → 404, other errors → 400, like the rest of the scenario API.
- **Deleting a team removes its routing** (`Config.DeleteTeam`): duplicates
  make teams with rules common, and a deleted team's `team:<id>` rules would
  otherwise linger unreachable. The baseline migration
  `drop-orphan-team-scopes` sweeps routing of teams that no longer exist.
