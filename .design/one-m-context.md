# Per-rule 1M context

> Audience: tingly-box contributors touching routing rules, Claude Code /
> Codex config apply, or the Anthropic upstream beta header.
> This document records how the 1M (1,000,000-token) context window became a
> first-class **per-rule** concept, and how that single flag fans out into
> three scenario-specific materializations.

---

## 1. Background

The 1M context window used to live in exactly one place: a `[1m]` suffix that
the Claude Code Quick Config appended to a model-slot env string
(`ANTHROPIC_DEFAULT_SONNET_MODEL=tingly/cc-sonnet[1m]`). It had two problems:

1. **The backend never understood it.** Rule matching is an exact string
   compare (`config.go:MatchRuleByModelAndScenario`). A client sending
   `tingly/cc-sonnet[1m]` matched no rule (`tingly/cc-sonnet`) and 404'd — so
   the suffix was decorative; it never enabled anything upstream.
2. **It was Claude-Code-only and env-only.** There was no equivalent for Codex,
   and no per-rule source of truth — you could not look at a rule and know
   whether it wanted 1M.

This change promotes 1M to a **per-rule flag** (`context_1m`) that:

- shows up as a dedicated, always-visible switch on every rule card,
- drives the Claude Code `[1m]` suffix (association, not a second toggle to
  remember),
- widens the Codex model catalog's `context_window` for that rule's model,
- and is actually honored at routing time: the gateway strips the `[1m]`
  suffix before matching and injects the `context-1m-2025-08-07` anthropic-beta
  flag upstream.

---

## 2. The single source of truth: `RuleFlags.Context1M`

```go
// internal/typ/type.go
type RuleFlags struct {
    // ...
    // Context1M requests the 1M (1,000,000-token) context window for this rule.
    Context1M bool `json:"context_1m,omitempty" yaml:"context_1m,omitempty"`
}
```

Registered in `RuleFlagRegistry()` (so it persists through the generic flag
wire path and the text-flag editor round-trips it), but **rendered as a
promoted switch**, not as a generic plugin chip — see §5. One concept, one
visible home (UX principle #3).

The flag is scenario-agnostic in storage; each scenario materializes it
differently (§3, §4). Whether the routed model can actually do 1M is **not**
gated anywhere — per product decision, "命中即下发,不校验模型": if the flag
(or the `[1m]` suffix) is present, the beta is sent and the upstream is left to
accept or reject it. This mirrors the existing Quick Config philosophy
(`.design/claude-code-config.md` §4.2).

---

## 3. Claude Code / Anthropic materialization

### 3.1 `[1m]` suffix preprocessing (request path)

`typ.StripContext1MSuffix(model) (base string, had bool)` is the one place the
`[1m]` token is recognized. It is applied:

- in `Server.determineRuleWithScenario` — so **any** scenario tolerates a
  trailing `[1m]` in the request model for routing (general gateway behavior,
  requirement #4); and
- in `anthropic.go` right after the model is read off the body, so the handler
  knows whether the suffix was present and can echo a clean `response_model`.

Effective intent for a request:

```
want1m = suffixHad1m  ||  rule.Flags.Context1M
```

When `want1m`, the handler attaches it to the request context
(`typ.WithContext1M`) — the same Type-2 context-passing pattern that
`custom_user_agent` uses (`.design/rule-flags.md`).

### 3.2 Beta injection (transport)

`claudeRoundTripper.RoundTrip` (the Claude Code OAuth transport) reads
`typ.GetContext1M(req.Context())` and, when set, appends
`context-1m-2025-08-07` to the merged `anthropic-beta` header. No model
allow-list check: a gateway shouldn't hard-code which models support a 1M
window — we send the beta whenever the rule asks for it and let the upstream
accept or reject. (The old `supportsContext1M` model allow-list was removed for
this reason.)

Why here: the round tripper is the single choke point that rebuilds the beta
header for OAuth traffic (it clears + re-emits `anthropic-*` headers to
preserve the claude-cli fingerprint). `context-1m-2025-08-07` is already on
`claudeCodeAllowedUpstreamBetas`, i.e. a fingerprint-safe addition.

### 3.3 Quick Config association

`derivePrefsFromRules` (ClaudeCodeQuickConfig.tsx) appends `[1m]` to a model
slot when that slot's rule has `flags.context_1m`. Toggling the rule-level
switch therefore flows straight into the generated `settings.json` env — no
separate "remember to also flip the Quick Config 1M toggle" step. The Quick
Config per-slot `1M` switch still works as an env-only override
(`.design/claude-code-config.md` §5.5 decoupling preserved).

---

## 4. Codex materialization

Codex has no `[1m]` suffix and no beta header; its 1M equivalent is the model
catalog's context window. `RenderCodexModelCatalog(models, oneM)` takes a
`map[string]bool` of which model slugs want 1M and emits:

```
context_window      = 1_000_000   (codex1MContextWindow) when oneM[slug]
max_context_window  = 1_000_000
auto_compact_token_limit / effective_context_window_percent — derived as today
```

The map is built in `collectCodexRuleModels` (now returns the slug list **and**
the 1M set) and threaded through `ApplyCodexConfig` and `CodexParams`. Models
not in the map keep the conservative 200000 default.

---

## 5. Frontend surface

- `RuleFlags.context1m` / `RuleFlagsApi.context_1m` added to
  `RoutingGraphTypes.ts`; the generic snake↔camel flag conversion carries it
  with no per-field code.
- `ModelRequestHeader` gains a `titleExtras` slot; `UnifiedRoutingGraph`
  threads `headerExtras`; `RuleCard` renders a compact **1M** switch there
  (`OneMContextSwitch`). Visible on every rule (requirement #1).
- `context_1m` is filtered out of `RulePluginsCard` and `FlagCatalogDialog`
  so it has exactly one visible home (the promoted switch), not two.

## 6. Toggle lifecycle: a small restart reminder

Flipping the 1M switch on an **agent scenario** opens `OneMConfirmDialog`, whose
only job is to remind the user to **restart that agent** for the change to take
effect — one generic line, the agent name being the only thing that varies. No
per-agent paragraphs, no in-dialog "applied" panel; **Cancel** reverts (the
switch is controlled by the flag, written only on confirm).

`oneMAgentForScenario(scenario)` maps the scenario to the restartable agent
(`codex` → Codex, `claude_code` → Claude Code, `claude_desktop` → Claude
Desktop) or `null` for non-agent scenarios (plain anthropic/openai), which just
toggle the flag directly with no dialog.

On confirm, `RuleCard`:
1. writes the `context_1m` flag, then
2. re-applies the agent's config **only for agents that have a config file** —
   `codex` → `api.applyCodexConfig()`, `claude_code` →
   `applyClaudeCodeFromRules()` (regenerates `settings.json` from the current
   rules via the same `derivePrefsFromRules` the Quick Config uses).
   `claude_desktop` is gateway-only (no config file) so it just saves.
3. surfaces the outcome through the **same `showNotification` snackbar** the rest
   of the card uses (success → "restart {agent}…", failure → the backend
   message) — consistent with every other rule-card action, not a bespoke
   result panel.

Re-applying Claude Code config from the rule card is safe because
`api.applyClaudeConfig` already **replaces** the `settings.json` env block on
every apply and the Quick Config derives that env from rules + tb defaults — so
the rule-card apply produces exactly what a default Quick Config apply would.

## 8. Touch list

Backend: `internal/typ/type.go`, `internal/typ/flag_registry.go`,
`internal/typ/id.go`, `internal/server/anthropic.go`,
`internal/server/handlers.go`, `internal/client/claude_round_tripper.go`,
`internal/server/config/apply_config.go`,
`internal/server/module/configapply/handler.go`, `ai/agent/codex.go`,
`internal/agent/rule_bridge.go`.

Frontend: `RoutingGraphTypes.ts`, `ModelRequestHeader.tsx`,
`UnifiedRoutingGraph.tsx`, `RuleCard.tsx`, `RulePluginsCard.tsx`,
`FlagCatalogDialog.tsx`, `ClaudeCodeQuickConfig.tsx`,
`OneMContextSwitch.tsx`, `OneMConfirmDialog.tsx`.
