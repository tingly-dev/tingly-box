# Duplicate a Claude Code profile

```
POST /api/v1/scenario/{scenario}/profiles/{id}/duplicate  {name?}  →  the new ProfileMeta
```

`Config.DuplicateProfile` (`internal/config/profile_duplicate.go`) creates a new
profile that is a deep copy of an existing one; the source is never touched.
Reimplements the profile half of the old, never-merged PR #1888 (which also
duplicated Teams and the main config) on top of the slot model (TEP-2197).

## Decisions

- **What is copied:** every rule of the profile's scenario (routing, smart
  routing, flags, active state, so split slots stay split), the profile's own
  scenario config if it has one, and its Claude Code overrides (`env`,
  `unset_env`, `default_mode`). Services are deep-copied; their usage stats live
  outside the config and start empty.
- **Rule identity:** built-in rules keep the `builtin:<scenario>:<slot>` shape
  under the new scenario (the generated settings and slot logic look them up by
  UUID); any other rule gets a fresh UUID.
- **Name:** optional. Empty picks `<name>-copy`, `<name>-copy-2`, … — the first
  free valid one (characters a profile name cannot hold become `-`, since a
  name is also a URL alias). An explicit name goes through the same validation
  and uniqueness check as create. The UI prefills the same suggestion.
- **No sharing keys / credentials are involved:** profiles hold routing only.
- **Failure:** validation runs before any write; a failed save is undone in
  memory so a retry is not refused. Unknown source → 404, anything else → 400.
- **Settings file:** the handler materializes the copy's Claude Code settings
  like create does, so it is launchable right away.
- Not covered: duplicating the main Claude Code config into a profile.
