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
| **serve**   | a plugin *is* a rule's upstream | `tingly.Plugin` (OpenAI server) |
| **register**| point a rule's upstream at the plugin | `register_with_tb()` → tb provider + rule |

The historical "Layer 1/2/3" headings below map exactly to connect / serve /
register. They are an implementation tour, not three separate products.

### tb-side: a first-class "plugin" provider kind (implemented)

"Register" is now **one step**: `POST /api/v2/plugins` creates a plugin-kind
provider *and* (when a scenario is given) the rule whose upstream is that plugin.

- **`ai.PluginDetail`** + `Provider.PluginDetail` + `Provider.IsPlugin()` mark a
  provider as backed by plugin code. It is **distinct from `AuthTypeVirtual`**
  (in-process `vmodel`): a plugin is an ordinary OpenAI HTTP upstream
  (`APIStyle=openai`, `api_key`/`no_key`), so **routing is unchanged** — the
  marker is metadata for UI grouping + lifecycle discovery only. Persisted via a
  new `plugin_detail` column, reconstructed unconditionally (independent of auth
  type), AutoMigrate-created.
- **`POST /api/v2/plugins`** `{name, endpoint, model_id?, token?, scenario?, tier?}`
  → creates the provider; if `scenario` is bindable, also creates a rule
  (`RequestModel=model_id`, single tier service → the plugin). Returns
  `{provider_uuid, model_id, scenario, rule_uuid, ready}`.
- **`GET /api/v2/plugins`** lists plugin-kind providers for the UI's plugin
  section.
- SDK: `register_with_tb(..., scenario=…)` and `tingly plugin register
  --scenario experiment` do the full one-step wire-in.

This makes "configure this rule with a plugin" literal and eliminates the
provider+rule two-step mode-picker (`ux-principles.md`).

### Plugin as runtime service (dynamic registration, implemented)

A plugin is a **runtime instance**, not a static config entry. It registers at
startup, heartbeats to hold a lease, and is auto-removed when it stops/dies —
nothing persisted. Differs from a standard provider (durable, operator-managed).

- **In-memory `PluginRegistry`** on the Server (process-local, matching tb's
  circuit-breaker stance — no shared store). Stable id from name (UUIDv5) so a
  restart re-registers under the same id; rotating `lease_id` per register.
- **Config hook** `EphemeralProviderResolver`: `GetProviderByUUID` /
  `validateRuleServices` fall back to the registry, so **routing resolves live
  plugins transparently** and an expired one simply isn't found → existing tier
  failover routes to a tier-1 real model. The db layer stays pure persistence.
- **DNS-style layering**: the rule (the durable "name") is ensured idempotently;
  the instance (endpoint + liveness) is ephemeral. No live instance ⇒ failover.
- Endpoints (apiV2): `POST /plugins/register` (leased; ensures rule),
  `POST /plugins/heartbeat`, `POST /plugins/deregister`, `GET /plugins` (live +
  pinned). The persistent `POST /api/v2/plugins` remains as the **pin** path.

**Active configuration** (SDK): `tingly.configure(url=, admin_token_env=)` /
`Connection` inject the tb target + credentials at runtime (secrets by env
reference), top-precedence in `config.resolve()` — for containers / CI / remote
where there is no `~/.tingly-box`. `Plugin.serve(register=True, scenario=…,
ttl_seconds=…, tb=Connection(...))` self-registers, heartbeats on a background
thread, and deregisters on shutdown.

Verified end-to-end (`examples/e2e_run.sh`): the plugin self-registers as a live
`ephemeral` instance, a client call routes through it, and it calls back into tb
— no network/keys. Remaining tb-side: rule-editor UI "plugin" kind (frontend),
process supervisor, scoped inference tokens, fully-ephemeral binding.


## Shape

```
sdk/python/
  tingly/
    client.py        # Layer 1: Client + connect()  ← consume tb
    discovery.py     # probe gateway + POST /sdk/session
    config.py        # (base_url, admin_token) resolution precedence
    scenarios.py     # scenario + transport constants
    transports/      # build openai.OpenAI / anthropic.Anthropic bound to tb
    helpers/         # usage + guardrails views
    plugin/          # Layer 2: be an AI server tb routes to
      core.py        #   Plugin class (@plugin.chat, .llm, .serve)
      server.py      #   stdlib OpenAI-compatible HTTP server (+ SSE)
      types.py       #   ChatRequest / Message
      manifest.py    #   tingly.toml read/write
      register.py    #   register the plugin as a tb provider (Layer 3)
    cli.py           # `tingly doctor` + `tingly plugin {init,run,register}`
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

## Open follow-ups

1. Scoped short-lived session tokens (`expires_at` + refresh on 401).
2. Dedicated `GET /api/v1/sdk/usage?session=` so usage doesn't scan
   `/api/v1/requests`.
3. Async client (`AsyncClient`, `aask`) — transports already have async builders.
4. Layer 2 — Python side **done** (`tingly.Plugin`, manifest, OpenAI server,
   `register`); remaining tb-side: sub-process supervisor from the manifest
   (reuse `agentboot/process`), `/plugins/<name>/*` reverse proxy, lifecycle UI.
   See the "Layer 2" section below.
5. Layer 3: expose a plugin as a model tb can route to (see "Layer 3" below).

## Layer 2: write an AI server (`tingly.Plugin`)

A plugin is an **OpenAI-compatible upstream**: the author writes one chat
handler, and `serve()` runs the HTTP server. The whole surface is one class.

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
  │        │                           │          │   model=plugin/my-rag        │
  │        │ returns str | iter[str]   │          │                              │
  │        ▼                           │          │  rule: plugin/my-rag → ↑      │
  │  serve()  →  stdlib HTTP server    │          └──────────────┬───────────────┘
  │     POST /v1/chat/completions ◄────┼──── (3) POST /v1/chat ──┘   ▲
  │     GET  /v1/models               │         (model=plugin/my-rag)│ (6) answer
  │     GET  /health                  │                              │
  │     · buffered  → chat.completion │                              │
  │     · stream    → SSE chunks  ────┼──── (7) response ────────────┘
  │        │                          │
  │  plugin.llm  (lazy Layer-1 client)│
  │        │                          │
  └────────┼──────────────────────────┘
           │ (4) plugin.llm.ask("…", model="auto")
           │     = tingly.connect(scenario="experiment") → POST /tingly/experiment/v1/chat
           ▼
   ┌──────────────────────────────────────────────────────────────┐
   │  tingly-box pipeline  (SAME as any client — see Layer 1 graph) │
   │  scenario→rule · guard rails · routing/tiers · failover ·      │
   │  quota · logging · transform ─────────────────────────► (5) real upstream
   └──────────────────────────────────────────────────────────────┘            (Anthropic/
                                                                                 OpenAI/…)

   request lifecycle:
     (1) client sends model="plugin/my-rag" to tb         ── see Layer 3 graph
     (2) tb resolves rule → provider my-rag (api_base = plugin)
     (3) tb POSTs OpenAI /v1/chat/completions to the PLUGIN
     (4) handler runs; calls plugin.llm.ask(...)  ── back INTO tb
     (5) tb routes that call to a real upstream, applies guard rails/quota/…
     (6) generated text returns to the handler
     (7) handler's str/iterator → OpenAI response/SSE back to tb → back to client
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
  (stdlib), so a plugin is one `pip install tingly` away. It serves
  `POST /v1/chat/completions` (buffered **and** real SSE streaming),
  `GET /v1/models`, `GET /health` — exactly what tb needs to treat it as an
  OpenAI upstream.
- **Handler contract is minimal.** Return a `str` (buffered) or an iterator of
  `str` (streamed); the server shapes both into `chat.completion` /
  `chat.completion.chunk`. The author never touches wire format.
- **`plugin.llm` is a lazy Layer-1 client.** The plugin reuses the gateway for
  its own generation instead of hard-coding a provider/key — the recursion in
  the Layer 3 graph.
- **`tingly.toml` manifest** (`manifest.py`) declares name / model_id /
  entrypoint / transport / port, so a future tb-side supervisor can install and
  run the plugin. `tingly plugin init` scaffolds a module + manifest.
- **Optional token auth.** `Plugin(api_key=...)` enforces a bearer token so only
  tb (carrying the matching provider token) can call it.

CLI:

```
tingly plugin init my-rag                 # scaffold my_rag_plugin.py + tingly.toml
tingly plugin run my_rag_plugin.py        # serve it
tingly plugin register my-rag \           # wire it into tb as a provider (Layer 3)
   --url http://127.0.0.1:8765/v1 --model-id plugin/my-rag
```

`register` uses the existing `POST /api/v1/providers` endpoint (admin token,
resolved like `connect()`). Creating the *rule/service* that maps the model into
a scenario is still a user/UI step — the provider is the part the SDK does
idempotently.

**Not yet built (tb-side):** a sub-process supervisor that boots plugins from
their manifest (reuse `agentboot/process`), a `/plugins/<name>/*` reverse-proxy
mount, and the install/enable/logs/disable lifecycle UI. The Python side and the
provider wiring are complete; those are the remaining backend pieces.

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
                          │   provider.api_base = plugin    │   POST     │ POST /v1/chat│    Plugin.serve()
                          └────────────────────────────────┘  /v1/chat  │ /completions │
                                                                        └──────┬───────┘
                                                              ctx.llm.ask() ┄┄┄┘  (plugin may
                                                              back INTO tb for its own LLM calls)
```

Wiring (no new gateway hot-path code — it's just a provider):

1. **Plugin serves** `POST /v1/chat/completions` (Layer 2 `Plugin.serve()`).
2. **Register a provider**: `{name:"my-rag", api_base:"http://127.0.0.1:<port>",
   api_style:"openai", models:["plugin/my-rag"]}` — a *normal* provider, not
   `AuthType=virtual`.
3. **Bind a rule/service**: model `plugin/my-rag` → that provider.
4. Now `model:"plugin/my-rag"` from any client resolves through the same
   dispatcher as every other model. Put the plugin in tier 0 and a real model in
   tier 1 and tb fails over automatically when the plugin is down.

The deeper option — a true in-process `AuthType=virtual` vmodel — means writing
a small Go adapter implementing `openai.VirtualModel` that forwards to the Python
process. Only worth it to bundle the plugin with no separate port;
provider-as-upstream is simpler and already fully supported.
