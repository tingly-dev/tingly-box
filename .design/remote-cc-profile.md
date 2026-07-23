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

`ClaudeCodeExecutor.Execute`:

1. Reads the bot setting via `deps.GetBotSetting()` (dynamic, straight from the
   store) and extracts the profile with `BotSetting.CCProfileID()` — so a
   profile switch in the web UI applies from the next message, no bot restart.
2. Calls `TBClient.GetClaudeCodeEnvForProfile(ctx, profileID)`:
   - `profileID == ""` → the existing `GetClaudeCodeEnv` (main scenario).
   - otherwise → `agent.ResolveCCProfileSettings(...)` with the profiled
     scenario path — the **same** resolution `tingly-box cc --profile` uses, so
     remote and local launches produce identical env: `ANTHROPIC_BASE_URL`
     pointing at `/tingly/claude_code:<id>`, tier models from the profile's
     builtin rules, the profile's unified/separate mode, and its persisted env
     overrides.
3. If the profile cannot be resolved (deleted after selection), the executor
   warns in-chat and falls back to the main scenario for that run instead of
   failing or silently rerouting.

The status line ("⏳ CC: Processing new session... (profile: p1)") and the
execution log both carry the profile so runs are attributable.

## 4. Frontend surface

The Remote page's per-bot graph grows a node on the @cc branch:

```
@cc → [Agent: Claude Code] → [Profile: Default | <name>]
```

- `CCProfileNode` shows the resolved profile name ("Default" when none;
  warning styling when the selected profile no longer exists).
- Clicking opens `CCProfileDialog`: Default + all profiles from
  `ProfileContext` (`GET /scenario/claude_code/profiles`), one tap to switch —
  writes `default_agent` via the existing imbot update API.
- No codegen was needed: `default_agent` was already in the OpenAPI schema.

## 5. Non-goals / future

- Per-chat override (e.g. a `/profile` bot command) is deliberately out of
  scope; the bot-level selection covers the "one bot = one working set" model.
  If needed later, a chat-level field can shadow the bot default.
- Other agents in `default_agent` (codex etc.) — the validation currently
  whitelists only `claude_code[.:<profile>]`; extend `validateDefaultAgent`
  and the executor routing when a second remote agent lands.
