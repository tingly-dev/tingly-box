# Remote @cc Profile Selection

> Audience: contributors touching the remote-control bot (@cc execution) or
> Claude Code profiles. Records how a bot's @cc branch is pointed at a Claude
> Code profile and how remote execution reuses the profile launch logic.

---

## 1. The selection: `default_agent`

Each bot's `default_agent` setting answers "which Claude Code configuration
serves @cc for this bot":

| value                  | meaning                                   |
|------------------------|-------------------------------------------|
| `""` / `claude_code`   | main claude_code scenario (default)       |
| `claude_code:<id>`     | the Claude Code profile `<id>`            |

- The value is the backend's standard profiled-scenario naming
  (`typ.ProfiledScenarioName` / `typ.ParseScenarioProfile`) — no new grammar.
- `internal/server/module/imbot`'s `validateDefaultAgent` rejects unknown
  bases and non-existent profile IDs on create/update (400), so a typo cannot
  silently no-op at execution time.
- Clearing writes the explicit base value `claude_code`, not `""` — a
  concrete value reads clearly in raw settings/logs (ux-principles.md §5,
  "show the concrete value, not the alias").

Selection is **per bot**, matching where the @cc branch is configured (the
Remote page's per-bot card), mirroring the per-bot SmartGuide model config on
the @tb branch. There is no per-chat override — one bot routes @cc through
one profile.

## 2. Execution: `ClaudeCodeExecutor.Execute`

1. Reads the bot setting via `deps.GetBotSetting()` (a live store read on
   every message, not a cached value) and extracts the profile with
   `BotSetting.CCProfileID()` — a profile switch in the web UI applies from
   the chat's next message, no bot restart needed.
2. If a profile is selected, calls
   `TBClient.GetClaudeCodeSettingsPathForProfile(ctx, profileID)`, which
   ensures the profile's settings.json via
   `agent.MaterializeCCProfileSettings(...)` — the same on-disk artifact and
   resolution `tingly-box cc --profile <id>` produces — and returns its path.
   The builder renders the complete document in memory, serializes publishers
   per target path, and atomically replaces the artifact only when its content
   changed. Concurrent launches therefore see either the previous complete
   snapshot or the next one, never the main-settings intermediate state.
   That path becomes `ExecutionOptions.SettingsPath`, which
   `agentboot/claude`'s CLI builder turns into a `--settings <path>` flag —
   the same mechanism the local launch uses.
3. With no profile selected (or if materialization fails), falls back to
   `TBClient.GetClaudeCodeEnv(ctx)` → `ExecutionOptions.Env` (the main
   scenario's routing, injected as process env vars).
4. If the selected profile can no longer be resolved (e.g. deleted after
   selection), the executor warns in-chat and runs that message against the
   main scenario instead of failing or silently misrouting.

`Env` and `SettingsPath` are mutually exclusive per run: a resolved profile
sets only `SettingsPath` (mirroring the local CLI's plain `os.Environ()`
passthrough + `--settings`, no extra env injected); the main-scenario path
sets only `Env`.

### 2.1 Configuration and runtime control are separate

The selected profile is persistent configuration; chat commands are
session-scoped runtime control. They meet only at the Claude Code CLI launch:

| layer | source / scope | CLI representation | owns |
|-------|----------------|--------------------|------|
| Profile configuration | bot `default_agent`; persisted per bot | `--settings <profile/settings.json>` | routing env, models, `defaultMode`, status line |
| Session control | `session.PermissionMode`; per chat + agent + project | `--permission-mode <mode>` when non-empty | temporary permission override |

Claude Code's explicit command-line option takes precedence over
`defaultMode` in the selected settings file:

```text
non-empty session PermissionMode > profile defaultMode > Claude Code default
```

This is an override relationship, not a synchronization relationship:

- `/yolo` on stores `bypassPermissions` on the current session. The executor
  passes `--permission-mode bypassPermissions` and uses its host-side
  auto-approve prompter for permission requests. `AskUserQuestion` still goes
  through the normal prompter.
- `/yolo` off stores the empty string. The CLI builder then emits no
  `--permission-mode` flag, so the currently selected profile's `defaultMode`
  becomes authoritative again. It must **not** store or pass `default`, because
  that would still be an explicit CLI override.
- A new, expired, or closed session starts with an empty permission override.
- Switching profiles does not silently mutate session control. If `/yolo` is
  active it remains the higher-priority override; disabling it immediately
  reveals the newly selected profile's policy.
- Runtime commands never rewrite the generated profile settings file.

In code, `AgentRouter` resolves `session.PermissionMode`,
`ClaudeCodeExecutor` places it in `ExecutionOptions.PermissionMode`, and the
Claude adapter is the sole owner of translating a non-empty value into a CLI
flag. The backend remains the Claude Code CLI; no SDK runtime is introduced.

**Why a profile needs `--settings` rather than env vars.** Claude Code's
`--settings <path>` flag *replaces* `~/.claude/settings.json` rather than
merging with it. The main scenario's routing works as plain env vars because,
with no `--settings` flag, the CLI reads `~/.claude/settings.json` and those
values already match (both derived from the same rules via Quick Config). A
profile's routing/models/overrides live in a separate derived file
(`~/.tingly-box/claude/<id>--<name>/settings.json`); injecting them as env
vars instead of referencing that file has no effect, because the CLI still
loads the main settings file, whose values win. This is also why
`BuildCCProfileSettings` copies the user's main settings.json as a base
before layering the profile's deltas on top — nothing else backs it.

The status line ("⏳ CC: Processing new session... (profile: p1)") and the
execution log both carry the profile ID so runs are attributable.

## 3. Frontend surface

The Remote page's per-bot graph carries a node on the @cc branch:

```
@cc → [Agent: Claude Code] → [Profile: Default | <name>]
```

- `CCProfileNode` shows the resolved profile name ("Default" when none;
  warning styling when the selected profile no longer exists). It is the only
  clickable target on this branch — clicking it opens `CCProfileDialog`
  (Default + all profiles from `ProfileContext`,
  `GET /scenario/claude_code/profiles`), one tap to switch, writing
  `default_agent` via the existing imbot update API.
- The Claude Code `AgentNode` itself is informational, not clickable — same
  as the SmartGuide agent node on the @tb branch. The Profile node is the
  branch's one actionable next step; a second click target on the agent node
  would compete with it.
- `default_agent` was already present in the OpenAPI schema, so no codegen
  was needed for this feature.

### 3.1 AgentNode's hover tooltip

`AgentNode` (both the @tb/SmartGuide and @cc/Claude Code nodes) uses
`NodeTooltip` — the shared MUI `Tooltip` wrapper also used by
`BotModelNode`/`CCProfileNode` — rather than a custom hover popover. Two
properties of this specific component matter here:

- **Hysteresis, not hand-rolled timers.** `NodeTooltip` has built-in
  enter/leave delays and never repositions itself under the pointer, so
  passing quickly over a node or across its edge doesn't cause the tooltip to
  flicker open/closed.
- **Inherited text color.** Every line inside the tooltip content sets
  `color: 'inherit'` explicitly. This app's theme (`theme/base.ts`) bakes a
  fixed color into the `body2`/`caption` typography variants
  (`text.secondary` / `text.disabled`) for normal page backgrounds; left at
  their variant default inside the tooltip's own (differently-colored)
  surface, that reads as unintentionally washed-out. `color: 'inherit'` makes
  every line take the tooltip's own text color instead.

Tooltip copy is kept short (3 feature bullets, one description line, one
config hint) so it fits without needing to reposition.

## 4. Non-goals / future

- Per-chat **profile selection** (e.g. a `/profile` bot command) is deliberately
  out of scope; the bot-level selection covers the "one bot = one working set"
  model. Session-scoped permission controls such as `/yolo` are orthogonal and
  do not change which profile is selected. If needed later, a chat-level profile
  field can shadow the bot default.
- Other agents in `default_agent` (codex etc.) — `validateDefaultAgent`
  currently whitelists only `claude_code[.:<profile>]`; extend it and the
  executor routing when a second remote agent lands.
