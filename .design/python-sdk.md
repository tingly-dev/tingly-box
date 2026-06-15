# Python SDK (`tingly`) — design

> Audience: tingly-box contributors touching the SDK seam (`sdk/python/`), the
> `/api/v1/sdk/session` endpoint, or the `experiment` scenario.

## Why

tb is a capable personal-intelligence gateway, but extending or experimenting
on top of it meant either editing the Go backend or hand-rolling HTTP calls
with the right base URL, token and scenario path. There was no fast seam for
"I have an idea, let me try it against my box in ten lines".

`tingly` is that seam: a pip module where the user writes only their own logic
(prompt, retrieval, agent loop) and **reuses the gateway's power** — provider
routing, tier/fallback, guard rails, quota, logging — for free.

This is **Layer 1** (client-side library). It deliberately ships before Layer 2
(tb-hosted plugins with a manifest + sub-process supervision) and Layer 3
(plugin-as-virtual-model via `vmodel/virtualserver`), both of which build on
this same module and the same `/sdk/session` provisioning seam.

## Shape

```
sdk/python/
  tingly/
    client.py        # Client + connect()  ← the whole user surface
    discovery.py     # probe gateway + POST /sdk/session
    config.py        # (base_url, admin_token) resolution precedence
    scenarios.py     # scenario + transport constants
    transports/      # build openai.OpenAI / anthropic.Anthropic bound to tb
    helpers/         # usage + guardrails views
    cli.py           # `tingly doctor`
    errors.py        # TinglyError hierarchy
```

## Request flow

```
connect(scenario="experiment")
   │
   ├─ config.resolve()           args → env → ~/.tingly-box/sdk.json → config.json → localhost
   ├─ discovery.probe_version()  GET  /api/v1/info/version   (liveness)
   ├─ discovery.create_session() POST /api/v1/sdk/session     (admin token → model token)
   └─ Client(session, gateway_url, admin_token)
          .openai      → openai.OpenAI(base_url = scenario_root + "/v1")
          .anthropic   → anthropic.Anthropic(base_url = scenario_root)
          .ask()       → picks transport from session.transport
          .usage       → GET /api/v1/requests        (admin token)
          .guardrails  → GET /api/v1/guardrails/config (admin token)
```

## Two-token model

- **Admin token** (tb's `UserToken`): authorizes `POST /sdk/session`. Resolved
  from `TINGLY_BOX_TOKEN` / `sdk.json` / `config.json:UserToken`. Provisioning
  requires admin rights.
- **Model token** (tb's `ModelToken`): returned *by* the session, used as the
  bearer for the actual LLM calls. The OpenAI/Anthropic clients carry this, not
  the admin token.

In v0.1 the session returns the existing long-lived model token (same as
`tbclient.GetConnectionConfig` / `GetClaudeCodeEnv` already do). Short-lived
scoped tokens (`expires_at`) are the obvious follow-up — the response field is
already present and `omitempty`.

## Gateway seam: `POST /api/v1/sdk/session`

Handler: `internal/server/sdk_session.go` (`CreateSDKSession`), registered in
`webui_api.go` under the authenticated `apiV1` group (so it needs the admin
token).

Request `{ scenario, name }` → response
`{ base_url, token, scenario, transport, ready, services, expires_at? }`.

- `base_url` is the scenario root `http://host:port/tingly/<scenario>`. Bind
  host `0.0.0.0`/`::` is rewritten to `127.0.0.1` so it's client-usable.
- `transport` is `openai`|`anthropic`|`both`, collapsed from the scenario
  descriptor's `SupportedTransport`.
- `ready`/`services` report whether an active rule with ≥1 service is bound, so
  `tingly doctor` can tell the user the next action instead of failing opaquely.
- Unknown / non-bindable scenario → 404 with `valid_scenarios` in the body.

No new routes were needed for the LLM calls themselves: `/tingly/:scenario` and
`/tingly/:scenario/v1` are already dynamic, so `experiment` flows through the
existing mixin endpoints (`chat/completions`, `messages`, `responses`, …).

## The `experiment` scenario

Added to `internal/typ/type.go` (`ScenarioExperiment = "experiment"`) and the
descriptor registry (`scenario_registry.go`): OpenAI + Anthropic transports,
rule-bindable, path-usable, profile-capable. It exists so SDK traffic has its
own isolated rule instead of polluting `claude_code` / `openai` rules — and so
users can name parallel experiments via profiles (`experiment:p1`).

## UX-principles alignment

- **No mode picker.** `connect()` is identical in dev and (future) hosted
  contexts; the environment decides discovery, not the user.
- **Smart defaults.** `scenario="experiment"`, `model="auto"`.
- **Concrete values.** `usage.this_session()` returns token counts, not aliases.
- **Diagnostics traverse the real path.** `tingly doctor` runs the actual
  discover → session → live round-trip; green = user code will run.
- **Surface the artifact for the next action.** `ready=false` and
  `GuardrailBlockedError(policy_id, reason)` tell the user exactly what to fix.

## Testing

- Python: `sdk/python/tests/` — config precedence, discovery/session (respx
  mocked gateway), transport URL shaping, client transport routing. Integration
  tests that need a live tb are marked `@needs_tb` and skipped by default.
- Go: `internal/server/sdk_session_test.go` freezes the response JSON field
  names (contract with the SDK) and the transport-label logic;
  `internal/typ/scenario_registry_test.go` pins the experiment descriptor.

## Open follow-ups

1. Scoped short-lived session tokens (`expires_at` + refresh on 401).
2. Dedicated `GET /api/v1/sdk/usage?session=` so usage doesn't scan
   `/api/v1/requests`.
3. Async client (`AsyncClient`, `aask`) — transports already have async builders.
4. Layer 2: `tingly.Plugin`, manifest, sub-process supervision (reuse
   `agentboot/process`), `/plugins/<name>/*` reverse proxy, lifecycle UI.
5. Layer 3: auto-register a plugin tool as a virtual model via
   `vmodel/virtualserver`.
