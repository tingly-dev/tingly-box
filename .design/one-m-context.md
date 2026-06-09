# Per-rule 1M context

> Audience: tingly-box contributors touching routing rules, Claude Code /
> Codex config apply, or the Anthropic upstream beta header.

---

## 1. Background

The 1M (1,000,000-token) context window is carried by a `[1m]` suffix on
the rule's `request_model` — e.g. `tingly/cc-sonnet[1m]`. This is the
single source of truth: no separate flag, no per-scenario special-casing.

When a rule's model ends with `[1m]`, the gateway:
- matches the request model (including `[1m]`) directly against the rule,
- strips `[1m]` before forwarding the request upstream,
- injects the `context-1m-2025-08-07` anthropic-beta flag for Claude Code /
  Anthropic traffic,
- widens the Codex model catalog's `context_window` to 1,000,000.

---

## 2. Routing

Rule matching is exact string compare (`MatchRuleByModelAndScenario`).
A client sending `tingly/cc-sonnet[1m]` matches a rule whose
`request_model` is `tingly/cc-sonnet[1m]`. The `[1m]` suffix is NOT
stripped before matching — it is part of the rule identity.

After matching, `typ.StripContext1MSuffix` removes the suffix for the
upstream model field and records the 1M intent.

---

## 3. Claude Code / Anthropic materialization

### 3.1 Request path

In `anthropic.go`, after routing succeeds:
```
requestModel, want1M = typ.StripContext1MSuffix(requestModel)
```
When `want1M`, the handler attaches it to the request context
(`typ.WithContext1M`) — the transport reads this to inject the beta.

### 3.2 Beta injection (transport)

`claudeRoundTripper.RoundTrip` reads `typ.GetContext1M(req.Context())`
and, when set, appends `context-1m-2025-08-07` to the `anthropic-beta`
header. No model allow-list check.

### 3.3 Quick Config / settings.json

`derivePrefsFromRules` reads each rule's `request_model` directly.
If the model already has `[1m]`, it flows into the generated env
(e.g. `ANTHROPIC_DEFAULT_SONNET_MODEL=tingly/cc-sonnet[1m]`).

---

## 4. Codex materialization

`CollectCodexRuleModels` detects `[1m]` in the rule's `request_model`
and builds the `oneM` map. `RenderCodexModelCatalog` widens
`context_window` to 1,000,000 for those models.

---

## 5. Frontend surface

A compact **1M** switch (`OneMContextSwitch`) sits inline next to the
request-model name in every rule card header. Toggling it appends or
removes the `[1m]` suffix on the rule's `request_model` — no separate
flag, no confirm dialog, no auto-re-apply. The user re-applies the
agent config (Claude Code Quick Config / Codex apply) when ready.
