# Federated TUI: Design and Decisions

> Audience: contributors touching `internal/command/tui/*` or the
> `tingly-box tui` / `provider` / `rule` / `agent apply` CLI surface.
> This document records why `tui` became a mode menu (not just a
> wizard), how the bubbletea modes and the flag-driven CLI split
> responsibilities, and the gotchas (model-list fallback, back
> navigation, deprecation of the bufio menus) that aren't obvious
> from the code.

---

## 1. Background

`tingly-box tui` originally ran a single linear quickstart wizard
(`internal/command/tui/quickstart.go`) — credential → provider → model →
rules → agent. Everything else interactive lived in a parallel
**bufio**-based text menu under `tingly-box config interactive` (and
`config provider`, `config rule` with no subcommand).

> **2026 update:** `config provider`/`config rule` and all bufio
> fallbacks described in §11/§12 below are gone. `provider` and `rule`
> are now top-level commands and a pure CI surface — every operation is
> a single, fully-parameterized invocation with no interactive
> fallback of any kind, not even TTY-gated. There is nothing left to
> pick; picking something is `tingly-box tui` or Web UI work. §11/§12
> are kept for historical context on how that surface got there.

That left two divergent interactive surfaces:

- a polished bubbletea wizard that could only do first-time setup, and
- a crude text menu (`1. Add  2. List …  0. Back`) for ongoing CRUD,
  with separate prompt code in `config_provider.go` / `config_rule.go`.

Users wanting to manage providers/rules/agents independently couldn't
do it through the nice UI; they fell back to `1`-`2`-Enter sequences in
the text menu. The wizard's agent step also hard-coded three agent
types and didn't expose any per-agent prefs beyond a multiselect.

The redesign turns `tui` into a true interactive console: a top-level
mode menu, each mode owns its own CRUD loop, and the bufio menus are
absorbed (their entry points now delegate into the TUI).

---

## 2. Goals

1. **One interactive surface.** `tingly-box tui` is the canonical place
   for interactive ops — no parallel bufio menus to keep in sync.
2. **All scenarios visible.** Agent mode iterates
   `agent.ListAgentInfo()`, not a hardcoded list, so every supported
   agent shows up with applied/not-applied status.
3. **TUI ⇄ CLI parity for first-time setup.** Anything you can do
   interactively in `tui` you can also do non-interactively with CLI
   flags, so CI scripts don't need a TTY.
4. **Don't grow a second framework.** Modes reuse the existing
   `prompts.go` primitives (Select / Input / Confirm / MultiSelect /
   WithSpinner) and the wizard runner stays for the linear quickstart
   only — no per-mode state machine.

---

## 3. Surface

```
tingly-box tui                       # mode menu
  ├─ QuickStart   → tui.RunQuickstart (the original wizard, unchanged)
  ├─ Provider     → tui.RunProviderMode  (List/Add/Edit/Delete/Refresh)
  ├─ Rule         → tui.RunRuleMode      (List/Add/Edit/Delete)
  ├─ Agent        → tui.RunAgentMode     → per-agent: Apply / Show / Restore
  └─ Exit

tingly-box quickstart                # hidden alias — skips the mode menu,
                                     # jumps straight to the wizard, kept
                                     # for muscle memory / scripts
```

`config interactive` and the no-subcommand `config provider`/`config
rule` forms this section originally described are gone entirely (see
the 2026 update above) — `provider`/`rule` are top-level commands with
no bare/interactive form at all. The flag-driven subcommands (`provider
add NAME URL TOKEN STYLE`, `rule add --scenario … --request-model …`,
etc.) are the CI path and the only path.

---

## 4. The TUIManager interface

Only the top-level menu and QuickStart need a host abstraction. Rather than
passing `*AppManager` directly (which would couple the TUI package to the
command package), they depend on the two host-owned capabilities that cannot
be provided by `serverconfig.Config` itself:

```go
type TUIManager interface {
    GetGlobalConfig() *serverconfig.Config
    StartServerAt(port int) error
}
```

Provider, Rule, and Agent modes take `*serverconfig.Config` directly and
construct their concrete use-cases from it. They do not depend on server
lifecycle. QuickStart reads the configured port from that config and invokes
one atomic `StartServerAt` operation; setup/start intermediate state is kept
inside the command host. There is deliberately no `QuickstartManager` alias:
the single interface has one meaning and one name.

---

## 5. Shared pickers

To keep modes from re-implementing the same flow, the package owns
three small shared pickers:

- **`pickProvider(mgr, prompt)`** (`provider_mode.go`) — sorted Select
  over `ListProviders()`. Used by Provider mode (edit/delete/refresh),
  Rule mode (add/edit service), and Agent mode (apply).
- **`pickProviderModel(mgr, provider, prompt)`** (`rule_mode.go`) —
  Select over the provider's model list, with `Custom…` as the escape
  hatch and free-form Input fallback when no list is available. Wraps
  the model-lookup cascade described in §7. Shared by Rule add/edit
  and Agent apply.
- **`pickScenario(initial)`** (`pickers.go`) — Select over
  `typ.BuiltinScenarios()`. Currently only Rule add uses it, but
  factored out because scenario picking is a stable shape and anything
  else that needs it (future per-scenario tools, custom rule kinds)
  should reuse rather than re-invent.

Add a new shared picker here when a third mode needs the same shape;
don't extract speculatively for one caller.

---

## 6. `Pause` after print-style ops

Bubbletea prompts render inline (no alt-screen), so after a
List / Show / Refresh, the next `Select` re-renders right below and
pushes the printed output up off the visible screen — easy to miss.

Modes call `tui.Pause("")` after every print-then-return path
(List, Show, Refresh, "no records configured", and after every
Add/Edit/Delete/Apply/Restore success line). The Pause renders a
`Press any key to continue…` footer and waits for one keystroke.

Tradeoff: one extra key per op. Worth it — the alternative was users
not seeing what just happened.

---

## 7. Model lookup cascade (the gotcha)

`Config.FetchAndSaveProviderModels(uuid)` (in
`internal/server/config/config.go`) has a documented but *half-done*
fallback:

1. Call the provider's `/v1/models` endpoint via the appropriate
   `client.ModelLister`. On success, persist to the DB-backed
   `ModelListManager` and return nil.
2. If the upstream API has no models endpoint (Anthropic API-key,
   OAuth-only providers, etc.) or the call failed, look up the
   compile-time embedded template via
   `TemplateManager.GetEmbeddedModelsForProvider(provider)`. If found,
   return nil — **but do not persist to the DB**, with the comment
   *"caller uses GetEmbeddedModelsForProvider directly"*.

So a caller that only reads `ModelListManager.GetModels(uuid)` after
calling `FetchAndSaveProviderModels` will see an empty list for any
provider that went through the template fallback — and drop the user
into a free-form Input even though the model list is right there in
the binary.

The TUI handles this in one place: **`availableModels(mgr, provider)`**
in `rule_mode.go`. The order is:

```
DB-cached models (ModelListManager.GetModels)
    ↓ empty?
embedded template (TemplateManager.GetEmbeddedModelsForProvider)
    ↓ empty / no template?
nil  →  free-form Input prompt
```

Every place that needs a model list uses this helper:
`pickProviderModel`, `providerRefreshModels`, and quickstart's
`qsModel`. Do **not** call `mm.GetModels(uuid)` directly anywhere in
the TUI — it will silently miss the template fallback.

> Why not call tb's own `/v1/models` HTTP endpoint? The TUI runs in
> the same process as the config layer; round-tripping through HTTP
> would add complexity and a server-must-be-running requirement.
> `internal/server/openai_models.go` uses the same cascade internally.

---

## 8. Agent apply: config files vs routing rule

`agent apply` does two separable things:

1. Write the agent's config files (e.g. `~/.claude/settings.json`)
   so the agent CLI talks to this box.
2. Create/update the routing rule for the agent's scenario, so tb
   knows which provider/model to dispatch the agent's requests to.

These are independent. The flag form has always supported empty
`--provider` / `--model` → "config files only" (ApplyAgent handles
the empty case by skipping rule sync). The TUI's Agent → Apply
asks up front:

> Also wire a routing rule (pick provider + model)?
> Default Yes; No → skip the picker, just rewrite the config files.

Useful when the user manages routing separately in Rule mode and just
wants to repoint an agent's CLI at tb, or after editing rules
out-of-band.

---

## 9. Back-navigation: deliberate omission

The wizard supports per-step back via `StepBack`. Modes do **not** —
a multi-step form (e.g. Rule add: scenario → request-model → provider
→ model → confirm) treats Esc on any step as "abort and return to the
mode menu". Going from step 4 back to step 2 would require a per-form
state machine like the wizard's, and modes aren't worth that
complexity.

Users get the `(back)` annotation in scrollback (from `prompts.go`'s
`Result.IsBack()` rendering) so it's at least visible what happened.
If this becomes a real pain point, the fix is to lift the wizard's
`Step[S]` machinery into mode forms — not to bolt back logic onto
the current flat structure.

---

## 10. Action audit: known gap, intentionally not closed here

Both the HTTP handlers and the TUI/AppManager mutate via the same
`Config.*` methods, but neither side currently feeds the action-audit
pipeline that backs `/api/v1/actions/history`.

What the pipeline expects:

- `multiLogger.WithSource(obs.LogSourceAction).LogAction(action, details, success, message)`
- routed by `WriteEntry` to the action memory sink (entries must carry
  `source: "action"`), read back by `GetActionHistory`.

What's actually happening:

- `internal/server/provider_handler.go` and friends log with global
  `logrus.WithFields("action": ...).Info(...)`. Those entries land in
  `LogSourceSystem` (no `source: "action"` field), so the action sink
  stays empty and `/api/v1/actions/history` is effectively unused.
- The TUI/AppManager doesn't log actions at all.

So there is no real audit parity we'd be breaking. We considered
adding `LogAction` calls to AppManager so TUI changes show up in the
action history, but two things kept it from being worth doing in this
PR:

1. **Asymmetric fix**. Wiring only the TUI side would make TUI log
   everything while HTTP still logs nothing — strictly worse than the
   current symmetric "nobody logs" state.
2. **Process-local memory.** The CLI binary and the running server
   are separate processes with separate `MultiLogger` instances and
   separate in-memory action sinks. Even if TUI logged correctly,
   the entries would live in the CLI process's memory and disappear
   when the command exits — they would never reach the server's
   `/api/v1/actions/history`. A real audit trail needs a persistent
   action store (DB-backed) that both processes append to and the
   HTTP endpoint reads back.

The proper fix is therefore out of scope for the TUI work:

- (a) Persist action records to the DB (an `action_log` table or
  reusing the existing log JSON file on disk), and have
  `GetActionHistory` read from there instead of in-memory.
- (b) Once (a) exists, fill in `LogAction` calls at the shared
  `Config.*` layer so both HTTP and TUI/AppManager paths produce
  records automatically — no per-caller work.

Until then, treat action history as not-yet-wired across the board.
TUI users who need an audit trail can rely on system logs (every
mutation eventually shows up via the global logrus hook in
`LogSourceSystem`).

---

## 11. CLI / CI mode (current design)

`provider` and `rule` are top-level commands (`internal/command/provider_cmd.go`,
`rule_cmd.go`) and, together with `agent apply`, are the entire
non-interactive surface. TUI (`tingly-box tui`) and the Web UI are the
entire interactive surface. There is no overlap and no fallback between
them: `provider`/`rule` never prompt, never check for a TTY, and never
redirect into the TUI — every invocation is a single, fully-parameterized
operation, or it fails with a clear error naming the missing flag/arg.
Picking something interactively is `tui`/Web UI work, full stop.

The full provisioning flow fits in three lines, no TTY required:

```bash
tingly-box provider add openai https://api.openai.com $TOKEN openai
tingly-box rule add --scenario openai \
  --request-model gpt-4o --provider openai --model gpt-4o
tingly-box agent apply claude-code --provider openai --model gpt-4o -y
```

Rules for these commands:

1. **`provider add`** takes all four positional args (name, base-url,
   token, api-style) or none; anything partial is a clear error, never a
   partial-prompt merge.
2. **`provider update <uuid>`** / **`rule update <uuid>`** take a
   required UUID plus one or more flags; update only touches the fields
   explicitly passed, and errors if none were.
3. **`provider delete <uuid>`** / **`rule delete <uuid>`** require
   `-y`/`--yes`, checked before any DB lookup — a delete without `-y`
   never even resolves whether the UUID exists.
4. **`provider get <uuid>`** / **`rule export <uuid>`** always require
   the UUID; there is no bare/picker form.

`--provider` (on `rule add`/`update`) accepts a UUID or an exact
provider name. Ambiguous names (multiple providers with the same name)
are rejected with the matching UUIDs printed, so the user can pick by
UUID.

---

## 12. History: from bufio menus to TUI redirect to pure CI

This surface went through two intermediate designs before landing on
the one in §11. Kept here for archaeology, not as current behavior.

**Bufio era.** `config interactive`, `config provider`/`config rule`
(no subcommand), and per-op fallbacks (`runProviderUpdateInteractive`,
`runRuleAddInteractive`, `pickServiceInteractive`, etc.) implemented
their own numbered-menu prompts directly in `internal/command`,
duplicating what the TUI's Provider/Rule modes already did better.

**TUI-redirect era.** All of that bufio code was deleted and
`config provider`/`config rule` (bare, or on update/delete with no
args) instead called `tui.RunProviderMode`/`tui.RunRuleMode` directly,
so there was exactly one interactive implementation. `config provider
get`/`config rule export` kept a minimal bufio picker since TUI had no
equivalent view to redirect to.

**Current era (§11).** Even the TUI-redirect fallback was cut:
`config` was renamed away entirely, `provider`/`rule` became top-level
commands, and every subcommand now requires its full arguments — no
bare form, no partial-arg prompt, no redirect into TUI. The reasoning:
nobody manages providers/rules by hand through a CLI menu once a
mature Web UI exists; the CLI's only remaining value is scriptable,
argument-complete CI usage, so that's the only mode it supports.
Anything interactive is `tui`/Web UI's job now, unconditionally.
