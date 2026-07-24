# Python SDK (`tingly`) — design

> Audience: tingly-box contributors touching the SDK seam (`sdk/python/`), the
> `/api/v1/sdk/session` endpoint, or the `experiment` scenario.

Diagram: `.design/python-sdk.pencil.md` — four simple pencil graphs: the one
idea, a request start to finish, the two ways to pick a provider, and
`router_plugin.py`'s decision flow.

## Why

tb is a capable personal-intelligence gateway, but extending or experimenting
on top of it meant either editing the Go backend or hand-rolling HTTP calls
with the right base URL, token and scenario path. There was no fast seam for
"I have an idea, let me try it against my box in ten lines".

`tingly` is that seam: a pip module where the user writes only their own logic
(prompt, retrieval, agent loop) and **reuses the gateway's power** — provider
routing, tier/fallback, guard rails, quota, logging — for free.

## Scope (current milestone)

The near-term target is deliberately narrow — connect, send a message, and
have a plugin work end-to-end including forwarding to another tb rule and
back. All three are done and verified live (`examples/e2e_run.sh`, real `tb`
binary, no mocks):

1. `tingly.connect()` reaches tb and mints a session (Layer 1).
2. `tb.ask(...)` / `tb.anthropic.messages.create(...)` send a message through
   tb's pipeline and get a real answer back (Layer 1).
3. A `tingly.Plugin` runs, tb routes a model to it, and the plugin's handler
   calls `plugin.llm.ask(...)` / `plugin.use(scenario).ask(...)` to forward
   into any other rule and get the result back before answering (Layer 2 +
   Layer 3).

**Protocol scope, deliberately narrowed:** Anthropic is primary everywhere in
the SDK; OpenAI chat completions is a real secondary path — kept, not
removed, but not what new work defaults to. Concretely:

- `Client.ask()` tries the Anthropic transport first when a scenario supports
  both (flipped from OpenAI-first — see "Two-token model" / Request flow).
- The plugin's own HTTP server answers `POST /v1/messages` (Anthropic,
  primary) and `POST /v1/chat/completions` (OpenAI, secondary) — both real,
  sharing one handler and one normalized `ChatRequest`; only the response
  shaping differs per route.
- New plugins register with `api_style="anthropic"` by default
  (`Plugin(api_style=...)` overrides it, per-plugin); the wire-level default
  at `POST /api/v2/plugins` itself (a caller that omits the field entirely)
  stays `"openai"`, for back-compat with anything hitting the endpoint
  directly.

Out of scope for now, unchanged from before: the tb-side plugin sub-process
supervisor, the `/plugins/<name>/*` reverse-proxy mount, and the lifecycle UI
(see Open follow-ups). None of the three milestone points above need them —
a plugin author starts their own process today, same as any local dev server.

## Architecture (one idea, not three layers)

There is a single concept:

> **tb is a hub of rules. A rule's upstream can be a plugin. A plugin can
> originate calls against any other rule.**

A client request matches a **rule** (as today). That rule's upstream is **plugin
code** instead of a provider — the only new thing. The plugin does its custom
work and, for any LLM work, calls **back into tb against any other rule / model /
provider** you have configured. tb stays the single router; a plugin is just a
graph node that happens to be user code and can also originate edges.

```
                         ┌──────────────────── tingly-box (the hub) ───────────────────┐
   clients               │                                                              │
   ┌─────────────┐  req  │   rule A ──upstream──►  PLUGIN CODE (your logic)             │
   │ Claude Code │──────►│   (model=plugin/x)          │                                │
   │ Cursor      │       │                             │ calls back into tb:            │
   │ tb UI       │       │   rule B ◄──────────────────┤  use("…").ask(model="…")       │
   │ tingly.ask()│       │   (→ Anthropic real)        │                                │
   └─────────────┘       │   rule C ◄──────────────────┤  (another model / provider)    │
                         │   rule D ◄──────────────────┘  (even another plugin)         │
                         │      │                                                       │
                         │      ▼  every edge gets: guard rails · routing/tiers ·       │
                         │         failover · quota · logging                           │
                         └──────────────────────────────────────────────────────────────┘
                                          │
                                          ▼  real upstreams (Anthropic / OpenAI / local …)
```

Everything else in this document is *how* that relationship is implemented with
today's pieces — three verbs for the one rule⇄plugin relationship:

| verb | what it is | SDK surface |
|------|------------|-------------|
| **connect** | a plugin (or experiment) *consumes* a rule | `tingly.connect()` / `plugin.use(scenario).ask(model=…)` |
| **serve**   | a plugin *is* a rule's upstream | `tingly.Plugin` (Anthropic-primary server) |
| **register**| point a rule's upstream at the plugin | `register_with_tb()` → tb provider + rule |

The historical "Layer 1/2/3" headings below map exactly to connect / serve /
register. They are an implementation tour, not three separate products.

### tb-side: a plugin is a normal, tagged provider (implemented)

**Design history, briefly, because it's instructive.** Three earlier
iterations over-built this: first a persisted "plugin provider kind" with its
own DB column and a distinct registration endpoint; then a full ephemeral
service-discovery layer (in-memory registry, per-instance lease, heartbeat
thread, TTL expiry, a `Config` hook consulted on every provider lookup) built
to avoid leaving a stale DB row behind when a plugin process stopped; then,
even after that was cut down to an idempotent upsert, the handler methods
still lived directly on `*Server` in `internal/server/*.go`, coupling plugin
registration — a self-contained concern whose only dependency is `*config.Config`
— into the same file/struct as every other server concern. All three were
fixed. The circuit-breaker point:

**tb already has liveness detection** — every `(rule, service)`
pair is covered by the existing per-service circuit breaker
(`internal/loadbalance/breaker.go`). A dead plugin's first failed request trips
it exactly like a dead real provider; traffic tier-fails-over automatically
when a fallback tier is configured. Lease/heartbeat/TTL was reinventing that
mechanism — distributed-service-discovery machinery for a problem tb doesn't
have (a personal, single-operator box, not a multi-tenant cluster). See
`git log` on this file's directory for the removed designs if useful as a
cautionary reference.

**What shipped instead — the minimal version, in its own module:**

- Lives in `internal/server/module/plugin/` (`handler.go` / `types.go` /
  `routes.go`), matching the same pattern every other server concern already
  uses (`module/provider`, `module/rule`, `module/providertemplate`, …):
  a `Handler` struct constructed with only the dependencies it needs
  (`NewHandler(cfg *config.Config)` — nothing else, since registration is just
  provider + rule creation), and a `RegisterRoutes(group, handler)` mounted
  from `server_webui_api.go`. Plugin logic no longer lives as methods on the
  giant `*Server` struct.
- A plugin is an ordinary provider (`APIStyle=openai|anthropic`, `api_key`/
  `no_key`) carrying the tag `"plugin"` in the existing, generic
  `Provider.Tags` field. `Provider.IsPlugin()` checks for that tag. No new
  struct, no new DB column — Tags already round-trips through the provider
  store unconditionally.
- **`POST /api/v2/plugins`** is an **idempotent upsert by name**: register once
  at startup (and again on every restart) and it updates the same provider
  instead of duplicating it. When `scenario` is given it also idempotently
  ensures the rule. Request carries `api_style` (`openai`|`anthropic`, empty
  → `openai` at the wire level; the SDK's own default is `anthropic`, see
  Scope above) — this is what tells tb which of the plugin's two routes to
  call. Response: `{provider_uuid, model_id, scenario, rule_uuid, ready, note}`.
- **A real, non-obvious fix underneath this:** `Provider.GetAccessToken()`
  returned `""` for a no-key provider, and the vendored `anthropic-sdk-go`
  treats an empty API key as "go look for ambient credentials" — it does its
  own discovery (env vars, `anthropic auth login` profile, …) and errors
  loudly when none exist, instead of just sending an empty/absent header the
  way the OpenAI client does. This was invisible while every plugin was
  `APIStyle=openai`; it surfaced immediately once Anthropic became the
  default and broke the very first live end-to-end run. Fixed by
  `ai.NoKeySentinelToken` (`ai/provider.go`): when `AuthType=api_key`,
  `Token==""` and `NoKeyRequired=true`, `GetAccessToken()` returns that
  sentinel instead of `""` — a real (if meaningless) value the SDK is happy
  to send as the header, which the plugin's own auth check (`api_key=""` →
  accept anything) ignores. General fix, not plugin-specific: any
  no-key-required Anthropic-style provider benefits.
- **`GET /api/v2/plugins`** lists plugin-tagged providers, deriving the display
  model id from the rule(s) bound to each (no extra field needed).
- **Retiring a plugin** is the same as retiring any other provider: delete it
  in the tb UI. There is no separate lifecycle to reason about.

**Active configuration** (SDK): `tingly.configure(url=, admin_token_env=)` /
`Connection` inject the tb target + credentials at runtime (secrets by env
reference), top-precedence in `config.resolve()` — for containers / CI / remote
where there is no `~/.tingly-box`. This part was cheap and answers a real need,
so it stayed. `Plugin.serve(register=True, scenario=…, tb=Connection(...))`
registers once at startup — no background thread, no lease to manage.

Verified end-to-end (`examples/e2e_run.sh`, real `tb` binary, no network/keys):
the plugin registers once as an `api_style=anthropic` provider; a client's
OpenAI-shaped `chat/completions` call to `model=plugin/rag-demo` routes
through tb, which forwards it to the plugin as `POST /v1/messages?beta=true`
(tb's real Anthropic client — the `?beta=true` query string is why the
server routes on path only, ignoring the query); the plugin's handler calls
`plugin.use("experiment").ask(..., model="echo-model")`, itself now an
Anthropic-transport call, which tb routes to a `vmodel` provider and back;
the composed answer returns to the plugin, which tb reshapes back to
OpenAI `chat.completion` for the original caller. Killing the plugin leaves
its provider listed (same as any provider) and the next request fails with a
plain connection error (add a tier-1 fallback to see failover instead);
restarting the plugin upserts the same provider, no duplicate.


## Shape

```
sdk/python/
  tingly/
    client.py        # Layer 1: Client + connect()  ← consume tb
    discovery.py     # probe gateway + POST /sdk/session
    config.py        # (base_url, admin_token) resolution precedence
    scenarios.py     # scenario + transport constants
    transports/      # build openai.OpenAI / anthropic.Anthropic bound to tb
    helpers/         # usage + guardrails + quota + rules views
    plugin/          # Layer 2: be an AI server tb routes to
      core.py        #   Plugin class (@plugin.chat, .llm, .serve, api_style)
      server.py      #   stdlib HTTP server: /v1/messages (primary) + /v1/chat/completions (secondary), + SSE
      types.py       #   ChatRequest / Message (from_anthropic_body / from_openai_body)
      manifest.py    #   tingly.toml read/write
      register.py    #   one-shot, idempotent register with tb
    cli.py           # `tingly doctor` + `tingly plugin {init,run}`
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
          .ask()       → Anthropic first when the scenario supports both, else OpenAI
          .usage       → GET /api/v1/requests        (admin token)
          .guardrails  → GET /api/v1/guardrails/config (admin token)
          .quota       → GET/POST /api/v1/provider-quota[...] (admin token)
          .rules       → GET /api/v1/rules?scenario=       (admin token)
```

## How it works (pencil)

Two phases. **Provisioning** happens once in `connect()` (admin token, dashed
lines). **Inference** happens on every call (model token, solid lines) and
reuses the exact same gateway pipeline as any other tb client — the SDK adds no
new path through the box.

```
        YOUR PYTHON                          tingly-box GATEWAY                      UPSTREAMS
  ┌───────────────────────┐         ┌──────────────────────────────────┐      ┌───────────────┐
  │  import tingly         │         │                                  │      │  Anthropic    │
  │  tb = tingly.connect() │         │   /api/v1/...   (admin auth)      │      │  OpenAI       │
  │                        │         │   /tingly/:scn  (model auth)      │      │  Deepseek     │
  └───────────┬───────────┘         └──────────────────────────────────┘      │  vLLM / local │
              │                                                                └───────▲───────┘
   ── PROVISION (once, admin token) ─────────────────────────────────────────────────┊────────
              │                                                                       ┊
   config.resolve()                                                                   ┊
   args→env→sdk.json→config.json→localhost                                            ┊
              │                                                                       ┊
              │  GET  /api/v1/info/version  (liveness) ┄┄┄┄┄┄┄┄►┐                      ┊
              │  POST /api/v1/sdk/session   {scenario,name} ┄┄┄►│ CreateSDKSession    ┊
              │     Authorization: Bearer <ADMIN/UserToken>     │  · validate scenario in registry
              │                                                 │  · transport = openai|anthropic|both
              │  ◄┄┄┄ {base_url, token=<ModelToken>,            │  · ready/services from active rule
              │        transport, ready, services} ┄┄┄┄┄┄┄┄┄┄┄┄┄┘
              ▼
   Client  ── builds lazily ──►  openai.OpenAI(base_url = root+"/v1",  api_key=ModelToken)
                                 anthropic.Anthropic(base_url = root,  api_key=ModelToken)

   ══ INFERENCE (every call, model token) ═══════════════════════════════════════════════════
              │
   tb.ask("...", model="auto")
   tb.openai.chat.completions.create(...)
   tb.anthropic.messages.create(...)
              │  POST /tingly/experiment/v1/chat/completions     ┌─────────────────────────┐
              │  POST /tingly/experiment/v1/messages             │   the SAME pipeline as   │
              │     Authorization: Bearer <ModelToken> ─────────►│   any other tb client    │
              │                                                  │                          │
              │                                                  │  scenario → rule resolve │
              │                                                  │  guard rails (in/out)    │
              │                                                  │  smart routing / tiers   │
              │                                                  │  circuit-breaker failover│──► pick
              │                                                  │  quota + usage logging   │    upstream
              │                                                  │  protocol transform      │──────────►
              │  ◄──────────── response (+ usage recorded) ──────└─────────────────────────┘   (solid)
              ▼
   tb.usage.this_session()     GET /api/v1/requests          (admin token, read-back)
   tb.guardrails.status()      GET /api/v1/guardrails/config (admin token, read-back)
```

Key reading of the graph:

- The SDK never talks to providers directly — the rightmost column is reachable
  **only** through the gateway box in the middle. That is the whole point: the
  experiment inherits routing/fallback/guard-rails/quota for free.
- Provisioning (dashed) uses the **admin** token and the `/api/v1/*` control
  plane; inference (solid) uses the **model** token and the `/tingly/:scenario`
  data plane. Different tokens, different surfaces.
- The inference box is *unchanged* tb internals — the SDK contributes the new
  `experiment` scenario and the one provisioning endpoint, nothing in the hot
  path.

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

## ⚠️ Naming collision to resolve before the lifecycle UI

The frontend already has a deliberately-unified name **"Plugins"** for a
completely different concept — per-rule feature flags (`smart_compact`,
`vision_proxy_service`, `clean_header`, `session_affinity`, …), surfaced via
`RulePluginsCard` / `FlagCatalogDialog` / `PluginFeatures` (see
`.design/rule-flags.md` §"统一命名：Plugins"). That unification itself
resolved an earlier "Plugin" / "Rule Extensions" mixed-usage collision — this
exact kind of debt has already been paid down once in this codebase.

The SDK's `tingly.Plugin` / `POST /api/v2/plugins` / the `"plugin"` provider
tag names an unrelated concept — *external code acting as an upstream* — but
reuses the same word. Per `.design/ux-principles.md` §3 ("一个词在产品中只能指
一件事"), this has to be split before it becomes user-visible. It's silent
today only because there's no lifecycle UI yet (see follow-up 4 below) — the
moment that ships, both meanings of "Plugin" appear in the same product, on
adjacent surfaces of the same rule (its flags card vs. its upstream
binding). Rename one side **now**, while the surface is still an API +
Python class and not yet UI copy — cheaper than renaming after users have
`tingly.toml` manifests and muscle memory. This SDK side is the newer,
smaller-footprint concept, so it's the one that should move; candidates:
`upstream plugin`, `connector`, `extension provider`. Needs a product
decision, not a unilateral rename — flagged here rather than acted on.

## Open follow-ups

The Scope milestone above (connect + send + plugin round-trip, Anthropic
primary / OpenAI secondary) is **done**. What's left, roughly in priority
order — 1–3 are backend/SDK-only (no naming exposure, safe to build
regardless of the rename above); 4 is blocked on that decision for its UI
portion specifically (the supervisor + reverse-proxy mount are not):

1. Layer 2 tb-side remainder — Python side is **done** (`tingly.Plugin`,
   manifest, dual-protocol server, `register`); still missing: a sub-process
   supervisor that boots plugins from their manifest (reuse
   `agentboot/process`), a `/plugins/<name>/*` reverse-proxy mount, and the
   install/enable/logs/disable lifecycle UI. The first two are ordinary
   backend work; the UI is the piece that should wait on the naming decision
   above. See the "Layer 2" section below.
2. Scoped short-lived session tokens (`expires_at` + refresh on 401).
3. Dedicated `GET /api/v1/sdk/usage?session=` so usage doesn't scan
   `/api/v1/requests`.
4. Async client (`AsyncClient`, `aask`) — transports already have async builders.

Layer 3 (expose a plugin as a model tb can route to) needed no new work of its
own — it's already fully supported as provider-as-upstream (see "Layer 3"
below) — so it's not listed as a follow-up.

## Layer 2: write an AI server (`tingly.Plugin`)

A plugin is an upstream tb can call two ways — **Anthropic Messages
(primary)** and **OpenAI chat completions (secondary)**, both real, both
always served regardless of what registration advertises. The author writes
one chat handler, and `serve()` runs the HTTP server. The whole surface is
one class.

```python
from tingly import Plugin

plugin = Plugin(name="my-rag")          # model_id defaults to "plugin/my-rag"

@plugin.chat
def handle(req):                        # req: ChatRequest
    docs = retrieve(req.last_user_text())
    return plugin.llm.ask(              # ← calls BACK into tb (Layer 1)
        f"Using {docs}, answer: {req.last_user_text()}", model="auto"
    )

if __name__ == "__main__":
    plugin.serve()                      # http://127.0.0.1:8765/v1
```

### How it works (pencil)

Two things to read here: the **anatomy** of a plugin (left), and the **request
lifecycle** when tb routes a model to it (the numbered loop). Note the loop is a
cycle — the plugin's handler calls back *into* tb (steps 4–6), so tb is both the
caller (step 3) and the upstream-for-the-plugin (step 5).

```
   A PLUGIN (one Python process)                       tingly-box GATEWAY
  ┌───────────────────────────────────┐          ┌──────────────────────────────┐
  │  Plugin(name="my-rag")             │          │                              │
  │                                    │          │  provider:                   │
  │  @plugin.chat                      │          │   name=my-rag                │
  │  def handle(req): ...              │          │   api_base=http://…:8765/v1  │
  │        │                           │          │   api_style=anthropic (dflt) │
  │        │ returns str | iter[str]   │          │   model=plugin/my-rag        │
  │        ▼                           │          │                              │
  │  serve()  →  stdlib HTTP server    │          │  rule: plugin/my-rag → ↑      │
  │     POST /v1/messages         ◄────┼─ (3) POST /v1/messages?beta=true ──┘  ▲ │
  │       (primary, api_style match)  │         (model=plugin/my-rag)         │ │
  │     POST /v1/chat/completions ◄────┼─ (3') POST /v1/chat/completions ─────┘ │
  │       (secondary, if api_style=openai)                          (6) answer │
  │     GET  /v1/models               │                                        │
  │     GET  /health                  │                                        │
  │     · buffered → message / chat.completion                                 │
  │     · stream    → SSE (message_* events / chat.completion.chunk) ── (7) ───┘
  │        │                          │
  │  plugin.llm  (lazy Layer-1 client)│
  │        │                          │
  └────────┼──────────────────────────┘
           │ (4) plugin.llm.ask("…", model="auto")
           │     = tingly.connect(scenario="experiment") → POST /tingly/experiment/v1/messages
           ▼
   ┌──────────────────────────────────────────────────────────────┐
   │  tingly-box pipeline  (SAME as any client — see Layer 1 graph) │
   │  scenario→rule · guard rails · routing/tiers · failover ·      │
   │  quota · logging · transform ─────────────────────────► (5) real upstream
   └──────────────────────────────────────────────────────────────┘            (Anthropic/
                                                                                 OpenAI/…)

   request lifecycle:
     (1) client sends model="plugin/my-rag" to tb, any protocol  ── see Layer 3 graph
     (2) tb resolves rule → provider my-rag (api_base = plugin, api_style picks the route)
     (3) tb calls the PLUGIN on whichever route matches provider.api_style
         (Anthropic /v1/messages by default; /v1/chat/completions if registered openai)
     (4) handler runs; calls plugin.llm.ask(...)  ── back INTO tb, Anthropic-first
     (5) tb routes that call to a real upstream, applies guard rails/quota/…
     (6) generated text returns to the handler
     (7) handler's str/iterator → response/SSE shaped for whichever route was hit → back to tb → back to client
```

Key reading:

- **One process, two roles.** As a *server* the plugin answers tb on
  `:8765/v1`; as a *client* (`plugin.llm`) it consumes tb via Layer 1. Same
  gateway, both directions.
- **The author writes only step 4's body.** Everything else — wire parsing,
  response/SSE shaping (steps 3 & 7), discovery/session (step 4's connect),
  routing/guard-rails (step 5) — is the SDK and the gateway.
- **Guard rails apply twice, correctly:** once on the inbound call to the
  plugin (step 3, via the provider/rule), and again on the plugin's own LLM call
  (step 5). Neither is wired by the author.

Design choices:

- **No framework dependency.** The server is `http.server.ThreadingHTTPServer`
  (stdlib), so a plugin is one `pip install tingly` away. It always serves
  both `POST /v1/messages` (Anthropic, buffered **and** real SSE) and
  `POST /v1/chat/completions` (OpenAI, same), plus `GET /v1/models`,
  `GET /health` — which route tb actually uses is a registration choice
  (`api_style`), not a server capability limit.
- **Handler contract is minimal and protocol-agnostic.** Return a `str`
  (buffered) or an iterator of `str` (streamed); the server shapes it into
  `message`/SSE `message_*` events for the Anthropic route or
  `chat.completion`/`chat.completion.chunk` for the OpenAI route, whichever
  was hit. The author never touches wire format either way.
  `ChatRequest.from_anthropic_body` folds Anthropic's top-level `system`
  field into a leading `role="system"` message so `req.system_text()` /
  `req.last_user_text()` work the same regardless of which route the caller
  used.
- **`plugin.llm` is a lazy Layer-1 client.** The plugin reuses the gateway for
  its own generation instead of hard-coding a provider/key — the recursion in
  the Layer 3 graph. Its own `ask()` calls try Anthropic first (see Scope).
- **`tingly.toml` manifest** (`manifest.py`) declares name / model_id /
  entrypoint / transport (`anthropic` by default, tracks `Plugin.api_style`) /
  port, so a future tb-side supervisor can install and run the plugin.
  `tingly plugin init` scaffolds a module + manifest.
- **Optional token auth.** `Plugin(api_key=...)` enforces a bearer token so only
  tb (carrying the matching provider token) can call it — checked once,
  ahead of both routes.

CLI:

```
tingly plugin init my-rag                 # scaffold my_rag_plugin.py + tingly.toml
tingly plugin run my_rag_plugin.py        # serve it AND register with tb
```

`run` (via `Plugin.serve()`) registers with `POST /api/v2/plugins` on startup —
an idempotent upsert by name that creates/updates the provider *and* the rule
(when `scenario` is set on the constructor) in one call. There is no separate
`register` command: a one-shot register with nothing keeping it alive would be
meaningless once ephemeral lifecycle was cut (see the "tb-side" section above).

**Not yet built (tb-side):** a sub-process supervisor that boots plugins from
their manifest (reuse `agentboot/process`), a `/plugins/<name>/*` reverse-proxy
mount, and the install/enable/logs/disable lifecycle UI. The Python side and the
provider wiring are complete; those are the remaining backend pieces.

### `Client.quota` — provider usage/limit windows, and a live refresh

Added for `router_plugin.py` below, but attached to `Client` like `.usage` /
`.guardrails` so any caller can use it. Wraps
`GET /api/v1/provider-quota[...]` (`internal/server/module/providerquota/`,
admin token, same `apiV1` auth-middleware group as usage/guardrails):

| SDK call | endpoint | shape |
|---|---|---|
| `quota.list()` | `GET /provider-quota` | `{meta, data:[ProviderUsage]}` |
| `quota.get(uuid)` | `GET /provider-quota/:uuid` | bare `ProviderUsage` (no envelope) |
| `quota.batch(uuids)` | `POST /provider-quota/batch` | `{data: {uuid: ProviderUsage}}` |
| `quota.refresh(uuid?)` | `POST /provider-quota/:uuid?/refresh` | live re-fetch from the upstream account, bypassing tb's cache |

These three response shapes are genuinely different (envelope vs. bare vs.
uuid-keyed map) — not a Python-side inconsistency, that's what the Go handler
(`internal/server/module/providerquota/handler.go:66-177`) actually returns
for each; `QuotaView._from_json`-style parsing per method is intentional, not
an oversight. `provider-quota` isn't in `openapi.json` (no swagger tags on
that module yet), so these shapes were pinned by reading the handler
directly, not generated — worth re-checking if that module ever gets
swagger-annotated.

A provider's quota is **not one number** — `ProviderUsage.windows` is a list
(session/daily/weekly/monthly/balance/model/...), each with its own
`used`/`limit`/`used_percent` (`ai/quota/types.go`). `ProviderQuota.headroom_percent`
collapses that to the single most-constrained window's remaining percent —
a deliberately naive heuristic for "which candidate is worse off right now"
in a routing pick, not a replacement for reading `.windows` when the
distinction between e.g. a session limit and a monthly cost budget matters.
tb itself has **no built-in quota-aware routing** (`internal/smart_routing`
and `internal/loadbalance` have zero references to `ai/quota` as of this
writing) — a plugin picking by remaining quota is genuinely new behavior,
not a Python reimplementation of something the gateway already does.

### Two connection modes: scenario+rule, and scenario+rule+pin

Every call this SDK makes goes through `(scenario, model)` → tb resolves a
**rule** → the rule's `Services[]` (possibly several, tiered) → tb's own
affinity/smart-routing/load-balancer picks **which** service actually runs.
That's mode 1 — "let tb decide" — and it's what `.ask()` has always done.

Building `router_plugin.py` (below) surfaced a real gap: a plugin that picks
a provider by quota and then calls `.ask(model=X)` has no guarantee that's
the provider tb's load balancer actually uses when the rule has more than
one active service — the "decision" and the execution are two unrelated
code paths that happen to usually agree. Mode 2 closes that:

- **`X-Tingly-Pin-Provider: <provider_uuid>`** (`internal/server/routing/simple.go`,
  `SimpleSelector.SelectService`) — forces the resolved rule to use that
  exact service, skipping affinity/smart-routing/load-balancing. The check
  that makes this safe to expose to ordinary clients: the provider **must**
  already be one of the resolved rule's own active `Services[]`, or tb
  rejects the request (400) — this cannot be used to reach an unrelated
  provider elsewhere on the box. It also runs on the *same* authenticated
  data-plane path as every other call (the model token already required to
  reach `/tingly/:scenario/...`), unlike the older `X-Tingly-Probe-Service`
  (`internal/server/routing/simple.go`, `.design/probe.md`), which bypasses
  auth entirely by convention (*"any caller that can reach the TB HTTP port
  can send it"*) and pins to **any** provider — that header is only ever
  injected internally by tb's own probe/diagnostics tooling, deliberately
  never exposed to SDK users. `X-Tingly-Pin-Provider` is the scoped,
  authenticated version of the same underlying mechanic
  (`SourceProviderPin` vs. `SourceProbePin` in `internal/server/routing/result.go`).
- SDK surface: `Client.ask(..., pin_provider=<uuid>)` sets the header
  (merges with any caller-supplied `extra_headers`); `tb.openai` /
  `tb.anthropic` accept it directly too, since both vendor SDKs already
  support `extra_headers=` on `.create()` — no SDK change was even required
  for that path, `ask()`'s kwarg is purely for convenience.
- **`Client.rules`** (`tingly/helpers/rules.py`, wraps `GET /api/v1/rules?scenario=`,
  admin token) is how a caller finds out what's *pinnable*:
  `rules.for_model(scenario, model)` returns the resolved `Rule`, whose
  `.active_services` are the only valid `pin_provider` values for that model.
  A rule with more than one active service has more than one valid pin — use
  quota (or whatever signal) to choose among them; a candidate whose rule
  doesn't resolve to exactly one service isn't safely routable by an
  external quota check at all (`router_plugin.py` skips those, rather than
  guessing).

**Deliberately still two modes, not three.** The obvious next question: what
if there's no rule at all yet for the model you want — do you need a *third*,
rule-free "just connect me straight to provider+model" mode? tb already has
the mechanics for exactly that: `X-Tingly-Probe-Service` builds a synthetic
rule on the fly and skips persisted-rule resolution entirely
(`internal/server/protocol_handler.go`, `determineRuleWithScenario`). We
considered exposing an authenticated version of that to the SDK and rejected
it: it would mean requests that don't show up as a rule in the tb UI, with
nowhere to hang guard rails/quota config, re-litigating the exact reasoning
`python-sdk.md` already gives for why `X-Tingly-Probe-Service` stays
internal-only. "No rule yet" isn't a routing problem, it's a one-time setup
step — `POST /api/v1/rule` with a single service, same as `router_plugin.py`'s
own `sonnet1`/`sonnet2` candidates already do. Cheap, idempotent, and it keeps
every reachable provider visible as a rule, which is the whole point of tb
being "a hub of rules" rather than a raw provider proxy.

Verified live against the real `tb` binary (not just mocked) — a fixed,
repeatable regression script, `sdk/python/examples/e2e_run_pin.sh` (three
vmodel providers, no network/keys, `set -uo pipefail` + explicit pass/fail
assertions, non-zero exit on any failure): a rule with provider A at tier 0
and B at tier 1 — an unpinned call selects A (normal tier order, confirmed
via `X-Tingly-Debug-Routing`); the same call with `X-Tingly-Pin-Provider: <B>`
selects B despite the tier order; a pin to a provider not on that rule is
rejected with 400; the same round-trip through `Client.ask(pin_provider=)`;
and `router_plugin.py` run for real end-to-end, resolving `sonnet1`/`sonnet2`
via `Client.rules`, and forwarding with a confirmed `provider_pin`-sourced
selection in tb's own routing log.

**A real bug this surfaced**, fixed alongside it: `Manager.GetQuota` /
`GetQuotaNoCache` (`ai/quota/manager.go`) re-wrapped a not-found store lookup
into a *new* `fmt.Errorf(...)` instead of returning `quota.ErrUsageNotFound`
itself — silently breaking the `err == quota.ErrUsageNotFound` identity
check every caller (`internal/server/module/providerquota/handler.go`, both
`GetQuota` and `BatchGetQuota`) relies on to treat "no data yet" as a skip.
The practical effect: `POST /provider-quota/batch` 500'd the *entire* batch
the moment it included any provider with no quota fetcher (a vmodel/local
provider, exactly what a no-network test setup uses) instead of just
omitting that one provider from the result — `router_plugin.py`'s very first
live run hit this immediately. Fixed by returning the sentinel unwrapped;
covered by `ai/quota/manager_test.go::TestGetQuota_NotFoundIsUnwrapped` and
`internal/server/module/providerquota/handler_test.go` (new — this module
had no tests before).

### Example plugins (`sdk/python/examples/`)

Four, each a different real-world shape of "plugin composes the box by
calling back into other rules" — not toys picked at random, each maps onto a
pattern already in wide use:

- **`rag_plugin.py`** — one call back into tb for generation over retrieved
  context. The baseline shape.
- **`critic_plugin.py`** (`model="plugin/critic"`) — cross-model critique:
  forwards the artifact-to-review to a *different* rule/model, returns a
  structured `{verdict, issues, suggestion}`. Chosen over self-critique
  deliberately: Huang et al. (ICLR 2024) found LLMs can't reliably
  self-correct without external feedback, so a plugin reviewing with a
  different model is the robust variant, not a stylistic choice. This is the
  pattern behind [Zen MCP](https://github.com/jray2123/zen-mcp-server) and
  [Consult7](https://github.com/szeider/consult7) (both real MCP servers
  coding agents use today to consult another model mid-task) and behind
  aider's architect/editor split. Named "critic" deliberately, not "advisor"
  — tb already has an unrelated, in-process `advisor` MCP tool
  (`internal/mcp/runtime/advisor_virtual.go` + the response-hook machinery in
  `internal/server/servertool/`); reusing that name for an architecturally
  different thing (plugin-as-upstream calling back into the gateway, vs. a
  direct in-process upstream call) would be the same collision already
  flagged above for "Plugin" vs. rule-flag "Plugins" — same fix, applied
  before it started rather than after.
- **`fusion_plugin.py`** (`model="plugin/fusion"`) — multi-model consensus:
  polls a panel of rules/models concurrently (`ThreadPoolExecutor`), skips
  the judge call when the panel already agrees, otherwise a judge call
  synthesizes. Mirrors Consult7's 2026 Fusion feature (a panel of frontier
  models answers in parallel, a judge model merges). The clearest
  illustration of the architecture line at the top of this document — a
  plugin can freely originate calls against *any* number of other rules, not
  just one.
- **`router_plugin.py`** (`model="plugin/router"`) — quota-aware dispatch: a
  different shape from the three above, which all *generate* an answer
  themselves. A router generates nothing — for each candidate model it
  resolves the rule via `Client.rules` (skipping any candidate whose rule
  isn't pinned to exactly one active service — see "Two connection modes"
  above), checks quota for that one provider, picks the candidate with the
  most headroom, and forwards with `pin_provider=` so the provider that was
  quota-checked is *guaranteed* to be the one that serves the request — one
  hop total, by design, not N. Same idea as LiteLLM Router's
  `usage-based-routing` strategy (route to whichever deployment has the most
  remaining rate-limit capacity), implemented as a plugin instead of gateway
  config — deliberately reads cached quota by default and only calls
  `.quota.refresh()` when a caller opts in, since LiteLLM's own docs warn
  that a live usage check on every request adds real per-request latency.

Every example plugin has unit tests (`tests/test_example_plugins.py`,
`tests/test_router_plugin.py`, `tests/test_quota.py`, `tests/test_rules.py`)
that monkeypatch `plugin.use`/`Client.quota`/`Client.rules` to fakes and pin
the decision logic — JSON-verdict formatting and graceful degradation on
non-JSON (critic); judge-skipped-on-agreement vs. judge-called-on-disagreement
(fusion); multi-service rules skipped as non-routable, highest-headroom
candidate selection, and `pin_provider=` forwarding (router) — without needing a
live tb.

## Layer 3: can tb *use* a plugin as a model? (yes — as an upstream)

Layer 1 points the **data-flow into** tb: the plugin is a *consumer*. For tb to
*select* a plugin as a model, the flow inverts — the plugin becomes a
*producer*, an HTTP upstream tb calls out to. That is Layer 2's `Plugin.serve()`.

There are two distinct "virtual model" notions; only one fits an out-of-process
Python plugin:

| | in-process `vmodel` (`AuthType=virtual`) | provider-as-upstream |
|---|---|---|
| what | Go code implementing `openai.VirtualModel` / `anthropic.VirtualModel`, compiled in (`ai/provider.go:IsVirtual` → `virtualModelService`) | a normal provider whose `api_base` is an external HTTP server speaking `/v1/chat/completions` or `/v1/messages` |
| lives | inside tb's process | out-of-process, any language |
| Python plugin fit | ✗ (needs a Go shim forwarding to Python) | ✓ natural route |

So a Python plugin is selected by registering it as a **provider/upstream**, not
via the in-process `vmodel` package.

```
   ANY tb client                tingly-box GATEWAY                      UPSTREAMS
  ┌──────────────┐        ┌──────────────────────────────┐
  │ Claude Code  │        │  HandleOpenAIChatCompletions   │     tier 1 ┌──────────────┐
  │ Cursor       │  model │   scenario → rule resolve      │  ┌───────► │ Anthropic /  │
  │ tb UI        ├───────►│   guard rails (in/out)         │  │ fallback│ OpenAI (real)│
  │ tingly.ask() │ "plugin│   smart routing / TIERS  ──────┼──┤         └──────────────┘
  └──────────────┘ /my-rag"│   circuit-breaker failover    │  │ tier 0  ┌──────────────┐
                          │   quota + usage logging        │  └───────► │ my-rag PLUGIN│ ◄─ Layer 2
                          │   provider.api_base = plugin    │   POST     │ POST /v1/    │    Plugin.serve()
                          │   provider.api_style picks route│  /v1/msgs  │ messages     │
                          └────────────────────────────────┘  (dflt)    └──────┬───────┘
                                                              ctx.llm.ask() ┄┄┄┘  (plugin may
                                                              back INTO tb for its own LLM calls)
```

Wiring (no new gateway hot-path code — it's just a provider):

1. **Plugin serves** both `POST /v1/messages` and `POST /v1/chat/completions`
   (Layer 2 `Plugin.serve()`).
2. **Register**: `POST /api/v2/plugins {name:"my-rag", endpoint:"http://127.0.0.1:<port>/v1",
   model_id:"plugin/my-rag", scenario:"experiment", api_style:"anthropic"}`
   creates a *normal* provider (not `AuthType=virtual`, tagged `"plugin"`,
   `APIStyle` set from `api_style`) — this is exactly what `Plugin.serve()`
   does on startup, with `api_style` defaulting to `"anthropic"`.
3. That same call **binds the rule/service**: model `plugin/my-rag` → that provider.
4. Now `model:"plugin/my-rag"` from any client resolves through the same
   dispatcher as every other model. Put the plugin in tier 0 and a real model in
   tier 1 and tb fails over automatically when the plugin is down.

The deeper option — a true in-process `AuthType=virtual` vmodel — means writing
a small Go adapter implementing `openai.VirtualModel` that forwards to the Python
process. Only worth it to bundle the plugin with no separate port;
provider-as-upstream is simpler and already fully supported.
