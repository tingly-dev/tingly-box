# Remote @cc Profile Selection

> Audience: contributors touching the remote-control bot (@cc execution) or
> Claude Code profiles. Records how a bot's @cc branch is pointed at a Claude
> Code profile and how remote execution reuses the profile launch logic.

---

## 1. Problem

Claude Code supports named profiles ("claude_code:p1" scenarios with their own
routing rules, unified/separate mode, and settings-env overrides), and the
local launcher already honors them (`tingly-box cc --profile p1`). The remote
agent, however, was hard-wired to the main `claude_code` scenario:
`ClaudeCodeExecutor` always called `TBClient.GetClaudeCodeEnv`, so a user could
not pick a profile to serve @cc duty for a bot.

## 2. The selection: `default_agent`

Bot settings already carried a dormant `default_agent` column (DB → API →
generated frontend types) from the retired RemoteGraph design. It is now the
single field that answers "which agent configuration serves @cc for this bot":

| value                  | meaning                                   |
|------------------------|-------------------------------------------|
| `""` / `claude_code`   | main claude_code scenario (default)       |
| `claude_code:<id>`     | the Claude Code profile `<id>`            |

- The value reuses the backend's profiled-scenario naming
  (`typ.ProfiledScenarioName` / `typ.ParseScenarioProfile`) — no new grammar.
- `internal/server/module/imbot` validates it on create/update: unknown bases
  and non-existent profiles are rejected with 400, so a typo cannot silently
  fall back at execution time.
- Clearing = writing the explicit base value `claude_code` (the settings store
  skips empty strings on partial update, so "" cannot be persisted back).

Selection is **per bot**, matching where the @cc branch is configured (the
Remote page's per-bot card), and mirroring the per-bot SmartGuide model config
on the @tb branch.

## 3. Execution path (reuse, not reimplementation)

**Why env vars alone don't work.** The main scenario's routing has always
been injected as process env vars (`ExecutionOptions.Env`) because Claude
Code CLI, given no `--settings` flag, reads `~/.claude/settings.json` and
those values happen to match (both derived from the same rules via Quick
Config). A profile is different: its routing/models/overrides live in a
*separate* derived file (`~/.tingly-box/claude/<id>--<name>/settings.json`),
and the CLI's `--settings <path>` flag **replaces** `~/.claude/settings.json`
rather than merging with it. So a profile selection only takes effect if that
file is materialized and referenced via `--settings` — injecting its values
as process env instead is silently ignored, because with no `--settings`
flag the CLI still reads the main settings file, whose values win. (This is
also why `BuildCCProfileSettings` copies the user's main settings.json as a
base before layering the profile's deltas on top — nothing else backs it.)

`ClaudeCodeExecutor.Execute`:

1. Reads the bot setting via `deps.GetBotSetting()` (dynamic, straight from the
   store) and extracts the profile with `BotSetting.CCProfileID()` — so a
   profile switch in the web UI applies from the next message, no bot restart.
2. If a profile is selected, calls
   `TBClient.GetClaudeCodeSettingsPathForProfile(ctx, profileID)`, which
   materializes the profile's settings.json via
   `agent.MaterializeCCProfileSettings(...)` — the **same** on-disk artifact
   and resolution `tingly-box cc --profile` produces — and returns its path.
   That path is passed as `ExecutionOptions.SettingsPath`, which
   `agentboot/claude`'s CLI builder turns into `--settings <path>` — the exact
   mechanism the local launch uses.
3. If no profile is selected (or profile materialization fails), falls back to
   `TBClient.GetClaudeCodeEnv(ctx)` → `ExecutionOptions.Env` (the pre-existing
   main-scenario mechanism, unchanged).
4. If the profile cannot be resolved (deleted after selection), the executor
   warns in-chat and falls back to the main scenario for that run instead of
   failing or silently rerouting.

`Env` and `SettingsPath` are mutually exclusive per run: a resolved profile
sets only `SettingsPath` (mirroring the local CLI's `os.Environ()` passthrough
+ `--settings`, no extra env injected); the main-scenario fallback sets only
`Env`.

The status line ("⏳ CC: Processing new session... (profile: p1)") and the
execution log both carry the profile so runs are attributable.

## 4. Frontend surface

The Remote page's per-bot graph grows a node on the @cc branch:

```
@cc → [Agent: Claude Code] → [Profile: Default | <name>]
```

- `CCProfileNode` shows the resolved profile name ("Default" when none;
  warning styling when the selected profile no longer exists).
- Clicking the Profile node opens `CCProfileDialog`: Default + all profiles
  from `ProfileContext` (`GET /scenario/claude_code/profiles`), one tap to
  switch — writes `default_agent` via the existing imbot update API.
- No codegen was needed: `default_agent` was already in the OpenAPI schema.
- The Claude Code `AgentNode` itself is **not** clickable (matches the
  SmartGuide agent node). It used to navigate to `/agent/claude_code`, but
  now that the adjacent Profile node is the actual next action for this
  branch, a second competing click target on the agent node itself would
  fight the same UX principle that motivated the Profile node in the first
  place — so it was removed, and the node's hover tooltip points at the
  Profile node instead of the stale "click to configure" copy.

### 4.1 AgentNode hover tooltip: hysteresis over hand-rolled timers

`AgentNode` (both the @tb/SmartGuide and @cc/Claude Code nodes) originally
built its own hover popover: a manual `open` boolean, a `setTimeout`-based
enter delay, and separate `onMouseEnter`/`onMouseLeave` handlers on both the
anchor and the popover's `Paper` (so moving the pointer from one to the other
didn't close it). Two problems surfaced once the content grew long enough to
occasionally collide with the viewport edge:

1. **Flicker.** MUI's `Popover` repositions itself to stay on-screen; when
   that shift happened under the cursor, the close was instant (no leave
   delay) while re-open waited out the enter timer again, so the popover
   visibly opened/closed in a loop.
2. **Washed-out text.** The popover's `Typography` used the `body2`/`caption`
   variants, whose colors (`text.secondary` / `text.disabled`) are baked into
   this app's theme (`theme/base.ts`) for normal page backgrounds — not for
   sitting on the popover's own dark surface, where they read as
   unintentionally dim.

Fix: replace the hand-rolled popover with `NodeTooltip` (the shared MUI
`Tooltip` wrapper `BotModelNode`/`CCProfileNode` already use), which has
built-in enter/leave hysteresis and doesn't reposition under the pointer, so
it structurally can't fall into that loop. Every line of tooltip content now
sets `color: 'inherit'` so it takes the tooltip's own text color rather than
a page-tuned theme variant color. The copy itself was also trimmed (5 feature
bullets → 3, shorter description) so the tooltip rarely needs to reposition
in the first place.

## 5. Non-goals / future

- Per-chat override (e.g. a `/profile` bot command) is deliberately out of
  scope; the bot-level selection covers the "one bot = one working set" model.
  If needed later, a chat-level field can shadow the bot default.
- Other agents in `default_agent` (codex etc.) — the validation currently
  whitelists only `claude_code[.:<profile>]`; extend `validateDefaultAgent`
  and the executor routing when a second remote agent lands.
