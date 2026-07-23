# Python SDK (`tingly`) — Pencil Graph

Visual companion to `python-sdk.md`. Where that document argues the *why*
and pins the *facts* (exact endpoints, field names, file:line references),
this one is the flow, end to end — architecture, provisioning vs. inference,
a plugin's request lifecycle, the two connection modes, and how the four
example plugins differ in shape. Read `python-sdk.md` first if a term here
is unfamiliar; nothing here is authoritative on its own.

Contents:

- Architecture — one idea, not three layers
- Layer 1 — provisioning vs. inference (`connect()`)
- Layer 2 — a plugin's anatomy and request lifecycle
- Layer 3 — provider-as-upstream wiring
- Two connection modes — scenario+rule vs. scenario+rule+pin
- `X-Tingly-Pin-Provider` vs. `X-Tingly-Probe-Service`
- `router_plugin.py` — the decide-then-pin flow
- Example plugin shapes — hop-count comparison
- Two-token model — which token opens which surface
- Verified live — what each e2e script actually exercises

## Architecture — one idea, not three layers

> tb is a hub of rules. A rule's upstream can be a plugin. A plugin can
> originate calls against any other rule.

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

Three verbs, one relationship:

```
 connect   a plugin (or experiment) CONSUMES a rule   tingly.connect() / plugin.use(scenario).ask()
 serve     a plugin IS a rule's upstream               tingly.Plugin (Anthropic-primary server)
 register  point a rule's upstream at the plugin       POST /api/v2/plugins (idempotent upsert)
```

"Layer 1/2/3" below = connect / serve / register. An implementation tour of
one idea, not three products.

## Layer 1 — provisioning vs. inference (`connect()`)

Two phases. **Provisioning** happens once (admin token, dashed). **Inference**
happens on every call (model token, solid) and reuses the exact same gateway
pipeline as any other tb client — the SDK adds no new path through the box.

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
   tb.ask("...", model="auto")            ← tries Anthropic first, falls back to OpenAI
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
   tb.usage.this_session()     GET /api/v1/requests           (admin token, read-back)
   tb.guardrails.status()      GET /api/v1/guardrails/config  (admin token, read-back)
   tb.quota.list()             GET /api/v1/provider-quota     (admin token, read-back)
   tb.rules.for_model(...)     GET /api/v1/rules?scenario=    (admin token, read-back)
```

Key reading: the SDK never talks to providers directly — the rightmost column
is reachable **only** through the gateway box in the middle. Provisioning uses
the *admin* token and the `/api/v1/*` control plane; inference uses the
*model* token and the `/tingly/:scenario` data plane. Different tokens,
different surfaces (detailed further down).

## Layer 2 — a plugin's anatomy and request lifecycle

The loop is a cycle — the plugin's handler calls back *into* tb (steps 4–6),
so tb is both the caller (step 3) and the upstream-for-the-plugin (step 5).

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
     (4) handler runs; calls plugin.llm.ask(...)  ── back INTO tb, Anthropic-first
     (5) tb routes that call to a real upstream, applies guard rails/quota/…
     (6) generated text returns to the handler
     (7) handler's str/iterator → response/SSE shaped for whichever route was hit → back to client
```

One process, two roles: as a *server* the plugin answers tb on `:8765/v1`; as
a *client* (`plugin.llm`) it consumes tb via Layer 1. The author writes only
step 4's body — wire parsing, response/SSE shaping, discovery/session,
routing/guard-rails are the SDK and the gateway. Guard rails apply **twice**,
correctly: once on the inbound call to the plugin (step 3), again on the
plugin's own LLM call (step 5) — neither is wired by the author.

## Layer 3 — provider-as-upstream wiring

A Python plugin is out-of-process, so it's selected as a normal
**provider/upstream**, not the in-process `AuthType=virtual` `vmodel` path
(that needs a Go shim; not worth it here — see `python-sdk.md`).

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

Wiring is four steps, no new gateway hot-path code — it's just a provider:
plugin serves both routes → `POST /api/v2/plugins` creates the provider +
binds the rule/service → `model:"plugin/my-rag"` now resolves through the
same dispatcher as every other model → tier the plugin under a real model and
tb fails over automatically when the plugin is down.

## Two connection modes — scenario+rule vs. scenario+rule+pin

Every call still starts the same way: `(scenario, model)` resolves to a
**rule**. What differs is who picks *which* of the rule's services actually
runs.

```
MODE 1 — scenario + rule                          MODE 2 — scenario + rule + pin
tb DECIDES (Client.ask()'s default)                CALLER decides, but SCOPED

tb.ask(model="X")                                  tb.ask(model="X", pin_provider=B_uuid)
   │                                                   │
   ▼                                                   ▼
(scenario, model) ──resolve──► rule          (scenario, model) ──resolve──► rule   (SAME step)
                                  │                                            │
                                  ▼                                            ▼
                          rule.Services[]                             rule.Services[]
                    ┌───────┬───────┬───────┐                   ┌───────┬───────┬───────┐
                    │ tier0 │ tier0 │ tier1  │                   │ tier0 │ tier0 │ tier1  │
                    │  Aa   │  Ab   │  B     │                   │  Aa   │  Ab   │  B  ◄──┼── pin_provider=B_uuid
                    └───┬───┴───┬───┴───┬────┘                   └───────┴───────┴───┬────┘
                        │       │       │                                            │
              affinity → smart-routing → load-balancer            scoping check: B_uuid ∈ rule.Services ?
                        │                                                            │
                        ▼                                                  yes ──────┴────── no
                 ONE service picked                                         │                 │
                 (tb's choice — may vary                                    ▼                 ▼
                  run to run: tier order,                          SKIP affinity/routing/LB    400 "not an
                  session pin, load)                                 entirely — USE B          active service
                                                                                                 on this rule"
```

`router_plugin.py` is what surfaced the gap mode 2 closes: picking a provider
by quota and then calling `.ask(model=X)` is a **guess**, not a decision, the
moment a rule has more than one active service — nothing stops tb's own
load-balancer from choosing differently. Mode 2 makes the pick binding.

## `X-Tingly-Pin-Provider` vs. `X-Tingly-Probe-Service`

Two headers do structurally the same bypass (`internal/server/routing/simple.go`)
but are not interchangeable — the scoping check is the entire difference:

```
                         X-Tingly-Probe-Service            X-Tingly-Pin-Provider
                         (pre-existing, internal-only)      (this branch, SDK-facing)
 ──────────────────────────────────────────────────────────────────────────────────
 header value            "<provider_uuid>:<model>"          "<provider_uuid>"
 rule resolution         SKIPPED — a synthetic rule is      NORMAL — the real rule is
                          built on the fly                   resolved first, same as mode 1
 valid pin targets       ANY provider on the box             only providers already in
                                                              THIS resolved rule's Services[]
 auth                    none at the header level — any      rides the SAME model-token
                          caller reaching tb's HTTP port      auth already required for
                          can send it (.design/probe.md)      /tingly/:scenario/... — nothing new
 who sends it today      tb's own probe/diagnostics UI       any SDK caller —
                          (internal/probe/e2e.go)             Client.ask(pin_provider=...)
 routing source label    SourceProbePin                      SourceProviderPin
 safe for SDK exposure?  NO — deliberately never exposed     YES — that scoping check is
                          to plugin authors                   exactly what makes it safe
```

## `router_plugin.py` — the decide-then-pin flow

A router *generates nothing* — its entire job is picking the ONE candidate
that gets the real call, then guaranteeing it lands there.

```
handle(req)
   │
   question = req.last_user_text()
   ▼
_pick_candidate()
   │
   ├─ for model in CANDIDATE_MODELS:                     e.g. ["sonnet1", "sonnet2"]
   │     rule = Client.rules.for_model(scenario, model)
   │     │
   │     ├─ rule is None ───────────────────────────► SKIP  (model not configured)
   │     │
   │     └─ len(rule.active_services) != 1 ──────────► SKIP  (0 or >1 services — tb's own
   │                                                     LB would decide; a quota check
   │                                                     on ONE of several means nothing)
   │     │
   │     └─ exactly 1 service ──► ResolvedCandidate(model, provider_uuid=services[0].provider)
   │
   ▼
resolved = [ (sonnet1 → A), (sonnet2 → B), … ]            only single-provider rules survive
   │
   ▼
quotas = Client.quota.batch([c.provider_uuid for c in resolved])   ← ONE control-plane round trip
   │        missing quota data for a candidate → headroom defaults to 100.0
   │        ("unknown" is NOT "starved" — see ProviderQuota.headroom_percent)
   ▼
chosen = max(resolved, key=lambda c: quotas[c.provider_uuid].headroom_percent)
   ▼
plugin.use(scenario).ask(question, model=chosen.model,
                          pin_provider=chosen.provider_uuid)
   │                                    └── MODE 2 (above): the provider that was
   │                                        quota-checked is GUARANTEED to serve this
   ▼
answer ── back to the original caller
```

No candidate resolves → `_pick_candidate()` raises loudly (`RuntimeError`),
rather than silently guessing at an unroutable model.

## Example plugin shapes — hop-count comparison

Four plugins, four different relationships to "how many times does this
handler call back into tb, and how is the final one chosen":

```
rag_plugin.py        client ──► plugin ──► tb ──► real model                      (1 hop, fixed rule)
                                             (generation over retrieved context)

critic_plugin.py     client ──► plugin ──► tb ──► DIFFERENT model                 (1 hop, fixed rule)
                                             (cross-model critique)

fusion_plugin.py      client ──► plugin ──┬─► tb ──► model A  ┐
                                           ├─► tb ──► model B  ├── N hops (panel, concurrent)
                                           └─► tb ──► model C  ┘
                                           panel disagrees? ──► tb ──► judge        (+1 hop)
                                           panel agrees?    ──► skip the judge hop

router_plugin.py      client ──► plugin ──► tb.rules  (control plane — no model call)
                                         ──► tb.quota  (control plane — no model call)
                                         ──► tb ──► ONE chosen model, PINNED        (1 hop)
```

rag/critic/router all cost exactly one *generating* hop; router just spends
two extra *control-plane* round trips (rules, quota) deciding which one.
fusion is the only shape that deliberately spends more than one generating
hop per request — that's the point of asking a panel.

## Two-token model — which token opens which surface

```
                    admin token (tb's UserToken)         model token (tb's ModelToken)
                    ──────────────────────────────       ──────────────────────────────
  authorizes         POST /api/v1/sdk/session             /tingly/:scenario/...
                      GET/POST /api/v1/... :               (chat/completions, messages —
                        requests (.usage)                   the actual LLM calls)
                        guardrails/config (.guardrails)
                        provider-quota[...] (.quota)
                        rules?scenario= (.rules)
  resolved via        args → env → sdk.json →              returned BY the session response
                       config.json:UserToken                (Client holds it, never re-resolved)
  who calls it         connect()'s provisioning step;       Client.ask() / .openai / .anthropic
                        Client.usage/.guardrails/
                        .quota/.rules (read-back views)
  scope                full admin — can inspect any         scoped to inference on the scenario
                        rule/provider/quota on the box       the session was minted for
```

Provisioning (admin token) happens once per `connect()`; inference (model
token) happens on every `.ask()` call. A plugin process typically holds both:
its own registration used the admin token once at startup, and every
`plugin.llm.ask(...)` afterward uses a model token from its own `connect()`.

## Verified live — what each e2e script actually exercises

Both are fixed, repeatable, real-`tb`-binary scripts — no mocks, no network,
no API keys (vmodel providers only) — with hard pass/fail assertions.

```
sdk/python/examples/e2e_run.sh              sdk/python/examples/e2e_run_pin.sh
────────────────────────────────────────    ────────────────────────────────────────
plugin registration                          MODE 1: unpinned call → tier0 selected
  (idempotent upsert-by-name)                MODE 2: pinned call → tier1 selected
round-trip: client → tb → plugin →             (overrides tier order)
  plugin.use(...) → tb → another rule         MODE 2 scoping: pin to an unrelated
  → back through tb → client                    provider → rejected (400)
crash (SIGKILL) → circuit breaker            SDK-level: Client.ask(pin_provider=)
  (no fallback tier ⇒ plain error;              round-trips the same way
   add tier-1 to see failover instead)        router_plugin.py run for real:
re-register → same provider, no duplicate       resolves sonnet1/sonnet2 via
                                                 Client.rules, checks quota, forwards
                                                 with pin_provider — tb's own routing
                                                 log confirms a provider_pin-sourced
                                                 selection for that forwarded call
```

`e2e_run_pin.sh`'s first live run also caught a real bug (`Manager.GetQuota`
re-wrapping `ErrUsageNotFound`, 500ing `POST /provider-quota/batch` for any
provider with no quota data) — fixed alongside it; see `python-sdk.md` for
the full writeup.
