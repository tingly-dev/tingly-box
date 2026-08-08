# Python SDK (`tingly`) — design

> Audience: tingly-box contributors touching the SDK seam (`sdk/python/`), the
> `/api/v1/sdk/session` endpoint, or the `experiment` scenario.

Diagrams: `.design/python-sdk.pencil.md`.

## Why

tb is a capable personal-intelligence gateway, but extending or experimenting
on top of it meant either editing the Go backend or hand-rolling HTTP calls
with the right base URL, token and scenario path. There was no fast seam for
"I have an idea, let me try it against my box in ten lines".

## The one idea

> **`tingly` is two independent halves: `tingly.Server` lets you write a
> provider in Python; `tingly.connect()` lets you consume tb from Python.
> Wiring them together happens in the tb UI, the same way you'd add Ollama.**

```
  your Python process                      tingly-box                     real upstreams
 ┌──────────────────────┐            ┌────────────────────────┐         ┌──────────────┐
 │ tingly.Server("rag") │◄── (1) ────┤ provider: rag           │         │ Anthropic    │
 │  @srv.chat           │  /v1/msgs  │  api_base=:8765         │  (3)    │ OpenAI       │
 │  def handle(req):    │            │  no_key, self-hosted    ├────────►│ Ollama …     │
 │     ...              │            │  ← same path as Ollama  │         └──────────────┘
 │     srv.tb.ask(...) ─┼── (2) ────►│ ordinary rule/pipeline  │
 └──────────────────────┘  /tingly/  └────────────────────────┘
                           experiment
  (1) and (2) are separate capabilities.
  (1) alone      = a plain custom provider, as independent as any other.
  (1) + (2)      = a provider that can orchestrate the whole box.
```

Neither half needs the other. A Server that never calls back is a perfectly
good provider; a `connect()` experiment that never serves anything is a
perfectly good script.

## tb-side: there is no new concept

**A thing that answers LLM protocol is a provider. That is the whole model.**
tb already had every piece:

- `AuthType=vmodel` (`ai/provider.go`) is a first-class persisted provider
  type with its own DB columns (`internal/data/db/provider_store.go`) and
  `VModelDetail{Models, LatencyProfile}`. The in-process synthetic models are
  already providers.
- Ollama / LM Studio / LocalAI / Jan / vLLM / SGLang are already
  `region: "self-hosted"` provider templates (`internal/data/providers.json`):
  localhost base URL, `no_key_required`, `supports_models_endpoint`.
- Connect AI already has a **Self-hosted** section and a **Custom endpoint**
  path (`frontend/src/components/ConnectProviderDialog.tsx`).

So a Python process speaking `/v1/messages` + `/v1/chat/completions` is
indistinguishable from Ollama, and needs **no backend mechanism at all**. The
only tb-side change for this feature is one data row:

```jsonc
// internal/data/providers.json
"tingly-python": {
  "region": "self-hosted", "type": "self-hosted",
  "base_url_openai":    "http://localhost:8765/v1",
  "base_url_anthropic": "http://localhost:8765",
  "no_key_required": true, "supports_models_endpoint": true
}
```

Self-hosted cards emit `kind: 'local'` from the picker, which the existing
form hook turns into `noKeyRequired: true` + the pre-filled base URL — so the
entry works with zero new frontend code. `supports_models_endpoint` makes tb's
model-list refresh call the server's `GET /v1/models`, so the model id is
discovered rather than typed.

### Two implementations of one concept

| | `AuthType=vmodel` (in-process) | `tingly.Server` (out-of-process) |
|---|---|---|
| code | compiled into tb, Go, `vmodel/` | your own process, Python |
| tb sees | a provider | a provider |
| added by | seeded builtin | Connect AI → Self-hosted |
| dispatch | short-circuits to the in-process handler | ordinary outbound HTTP |
| can call back into tb | no (same process) | **yes** (`srv.tb`) — the reason it exists |

### What we cut, and why it's worth remembering

Four iterations over-built this seam before it collapsed to the row above.
Each was a reasonable-looking step that added a concept tb did not need:

1. A persisted "plugin provider kind" with its own DB column and a distinct
   registration endpoint.
2. A full ephemeral service-discovery layer — in-memory registry, per-instance
   lease, heartbeat thread, TTL expiry, a `Config` hook consulted on every
   provider lookup — built to avoid leaving a stale DB row behind when a
   process stopped. tb already has liveness detection: the per-service circuit
   breaker (`internal/loadbalance/breaker.go`) covers every `(rule, service)`
   pair. This was distributed-systems machinery for a single-operator box.
3. An idempotent `POST /api/v2/plugins` upsert plus a `"plugin"` provider tag
   and `Provider.IsPlugin()` — smaller, but still a second way to create a
   provider, and a second word for one that already had a name.
4. Python-side scaffolding for the above: a `tingly.toml` manifest, a
   `register.py`, and `tingly plugin init|run` CLI subcommands.

All four are gone. The tell was visible before the code was: an earlier
revision of this document carried a section titled *"⚠️ Naming collision to
resolve"*, noting that the frontend already uses **"Plugins"** for an
unrelated concept (per-rule feature flags — see `.design/rule-flags.md`), and
that per `.design/ux-principles.md` §3 one word may only mean one thing. The
resolution turned out not to be a rename. When a new name collides with an
existing one *and* the thing it names already has a name, the second concept
is the bug. Deleting it removed ~1,100 lines and every open follow-up that
depended on it (sub-process supervisor, `/plugins/<name>/*` reverse-proxy
mount, lifecycle UI).

**Kept from that work**, because it fixes a real and general bug:
`ai.NoKeySentinelToken` (`ai/provider.go`). `Provider.GetAccessToken()`
returned `""` for a no-key provider, and the vendored `anthropic-sdk-go`
treats an empty API key as "go look for ambient credentials" — it runs its own
discovery (env vars, `anthropic auth login` profile, …) and errors loudly when
none exist, instead of sending an empty/absent header the way the OpenAI
client does. Any `NoKeyRequired=true` Anthropic-style provider hits this; a
local Python server is simply the one that surfaced it.

## Shape

```
sdk/python/
  tingly/
    _generated/      # ← models.py, generated from openapi.json, NOT committed
    _api.py          # ControlPlane: typed httpx wrapper over /api/v1 + /api/v2
    client.py        # Client + connect()  ← consume tb
    discovery.py     # probe gateway + POST /sdk/session
    config.py        # (base_url, admin_token) resolution precedence
    scenarios.py     # scenario + transport constants
    transports/      # build openai.OpenAI / anthropic.Anthropic bound to tb
    helpers/         # usage, guardrails and quota views
    server/          # ← be a provider
      core.py        #   Server class (@srv.chat, .tb, .use, .run, .connect_hint)
      http.py        #   stdlib HTTP server: /v1/messages + /v1/chat/completions, + SSE
      types.py       #   ChatRequest / Message (from_anthropic_body / from_openai_body)
    cli.py           # `tingly doctor`
    errors.py        # TinglyError hierarchy
```

## The control plane is generated, not hand-written

`openapi.json` describes 150 paths / **195 operations / 276 schemas**. The SDK
hand-wrote four of them, and got them wrong in ways nobody noticed — see below.
So the models come from the spec:

```
openapi.json ──task gen:py──► tingly/_generated/models.py  (pydantic v2, 287 models)
                              └─ consumed by _api.ControlPlane, discovery, helpers
```

- **`task gen:py`** runs `datamodel-code-generator`. It is deliberately *not*
  folded into `task swagger`, and `task codegen` calls it as a last step — a
  backend-only change shouldn't force a `pnpm install`, and a Python-only
  change shouldn't regenerate the frontend client.
- **The output is not committed** (`.gitignore`). It is a pure function of the
  spec, and 2.7k generated lines would drown every SDK diff. CI regenerates it
  before tests and before building the wheel; `pyproject.toml`'s
  `[tool.hatch.build.targets.wheel.force-include]` is what actually ships a
  gitignored file, and `.github/workflows/python-sdk.yml` asserts it made it
  into the wheel — otherwise that breakage would only appear at `pip install`.
- **Requires Python 3.10+** because the generator emits `X | None` unions.
  3.9 reached EOL in October 2025.

### Scope: control plane only

Only tb's **control plane** (`/api/v1`, `/api/v2`) is in `openapi.json`. The
LLM data plane (`/tingly/<scenario>/...`) is dynamic pass-through and appears
nowhere in the spec — those calls go through the vendored `openai` /
`anthropic` SDKs, which are themselves generated, by their own vendors, and
stay that way. So generating here buys **type coverage and no drift**, not
throughput: the control plane is not a hot path.

### What generating from the spec actually caught

Three real defects, none of which any test had noticed:

1. **`Client.usage` could only ever report zeros.** It read
   `GET /api/v1/requests`, took `payload["data"]` — that endpoint sends
   `{"total", "requests"}` — and summed `rec["input_tokens"]` off
   `ModelRequestSummary`, which has no token fields at all. All inside a bare
   `except` returning an empty summary. It now reads
   `GET /api/v1/usage/stats` (`AggregatedStat`, filtered to the session's
   scenario), which is where token counts actually live — and the fields are
   `total_input_tokens` / `total_output_tokens`, which is itself a name the
   hand-written version would have got wrong a second time.
2. **`openapi.json` emitted two case-colliding schemas**, `ErrorDetail`
   (onboarding) and `errorDetail` (probe's local envelope). Generators
   normalize schema keys, so `openapi-python-client` silently *dropped*
   `E2EResponse` and `LightweightResponse` — both of probe's response types.
   Fixed by renaming probe's to `ProbeErrorDetail`; the wire JSON is unchanged.
   This affected every language's generator, not just Python.
3. **`POST /api/v1/sdk/session` lied about its own shape.** The route declares
   `WithResponseModel(SDKSessionResponse{})` but the handler wrapped the
   payload in `{success, data}`, so the spec described a body the endpoint
   never sent. Now returned bare, matching the declaration and matching the
   three sibling endpoints the SDK reads. `TestCreateSDKSessionWritesTheDeclaredShape` drives the real handler and decodes with `DisallowUnknownFields`, so
   a re-wrap fails as an unknown `data` key — the previous test marshalled the
   struct and could not see the envelope at all.

### Failure modes are loud on purpose

`ControlPlane` raises rather than degrading:

| condition | error |
|---|---|
| body doesn't match the generated model | `SchemaMismatchError`, naming `task gen:py` as the fix |
| non-2xx | `APIStatusError`, carrying the decoded body (so a 404's `valid_scenarios` survives without a second request) |
| 401 | `AuthError` |
| connection refused | `GatewayUnreachableError` |

A shape mismatch means the models are stale relative to the running gateway.
Returning a default there would hide precisely the drift that generating from
the spec exists to make impossible to miss — which is how defect 1 above
survived. The views layered on top still degrade where a *state* is legitimately
empty (no usage store → 503 → zero rows), but never where a *shape* is wrong.

## Consume: request flow

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
```

The SDK never talks to providers directly — upstreams are reachable **only**
through the gateway. That is the point: the experiment inherits
routing/fallback/guard-rails/quota for free. Provisioning uses the **admin**
token and the `/api/v1/*` control plane; inference uses the **model** token and
the `/tingly/:scenario` data plane. The inference path is unchanged tb
internals; the SDK contributes the `experiment` scenario and one provisioning
endpoint, nothing in the hot path.

### Two-token model

- **Admin token** (tb's `UserToken`): authorizes `POST /sdk/session`. Resolved
  from `TINGLY_BOX_TOKEN` / `sdk.json` / `config.json:UserToken`.
- **Model token** (tb's `ModelToken`): returned *by* the session, and used as
  the bearer for the actual LLM calls.

In v0.1 the session returns the existing long-lived model token (same as
`tbclient.GetConnectionConfig` / `GetClaudeCodeEnv` already do). Short-lived
scoped tokens (`expires_at`) are the obvious follow-up — the response field is
already present and `omitempty`.

### Gateway seam: `POST /api/v1/sdk/session`

Handler: `internal/server/sdk_session.go` (`CreateSDKSession`), registered in
`server_webui_api.go` under the authenticated `apiV1` group.

Request `{ scenario, name }` → response
`{ base_url, token, scenario, transport, ready, services, expires_at? }`.

- `base_url` is the scenario root `http://host:port/tingly/<scenario>`. Bind
  host `0.0.0.0`/`::` is rewritten to `127.0.0.1` so it's client-usable.
- `transport` is `openai`|`anthropic`|`both`, collapsed from the scenario
  descriptor's `SupportedTransport`.
- `ready`/`services` report whether an active rule with ≥1 service is bound, so
  `tingly doctor` can name the next action instead of failing opaquely.
- Unknown / non-bindable scenario → 404 with `valid_scenarios` in the body.

No new routes were needed for the LLM calls themselves: `/tingly/:scenario` and
`/tingly/:scenario/v1` are already dynamic.

### The `experiment` scenario

Added to `internal/typ/type.go` (`ScenarioExperiment`) and the descriptor
registry (`scenario_registry.go`): OpenAI + Anthropic transports,
rule-bindable, path-usable, profile-capable. It exists so SDK traffic has its
own isolated rule instead of polluting `claude_code` / `openai` rules — and so
users can name parallel experiments via profiles (`experiment:p1`).

## Serve: `tingly.Server`

```python
from tingly import Server

srv = Server(name="router", scenario="openai")   # model id: router

@srv.chat
def handle(req):                     # req: ChatRequest
    question = req.last_user_text()
    target = "claude-opus-4-6" if len(question) > 4000 else "claude-haiku-4-5"
    return srv.use("openai").ask(question, model=target)

if __name__ == "__main__":
    srv.run()                        # http://127.0.0.1:8765
```

Design choices:

- **No framework dependency.** `http.server.ThreadingHTTPServer`, so a
  provider is one `pip install tingly` away. It always serves **both**
  `POST /v1/messages` and `POST /v1/chat/completions` (buffered *and* real
  SSE), plus `GET /v1/models` and `GET /health`.
- **No `api_style` knob.** Both routes are always live, so which one tb calls
  is entirely the provider's `api_style` on the tb side. There was briefly a
  server-side setting mirroring it; two knobs for one fact is exactly the
  "one knob controlling two things" smell of `ux-principles` §4/§6.
- **Handler contract is minimal and protocol-agnostic.** Return a `str`
  (buffered) or an iterator of `str` (streamed); the server shapes it into
  `message`/SSE `message_*` for the Anthropic route or
  `chat.completion`/`chat.completion.chunk` for the OpenAI route.
  `ChatRequest.from_anthropic_body` folds Anthropic's top-level `system` field
  into a leading `role="system"` message so `req.system_text()` /
  `req.last_user_text()` work identically on both routes.
- **Model id is the name, unprefixed.** The provider is already the namespace.
- **Path-only routing.** tb calls Anthropic upstreams as
  `POST /v1/messages?beta=true`; the server routes on the path and ignores the
  query.
- **`srv.tb` is a lazy Layer-1 client**, `srv.use(scenario)` targets another
  rule-set. This is the recursion in the graph above, and the only reason an
  out-of-process provider beats an in-process vmodel.
- **`srv.tb.rules(scenario)` lets it dispatch to what exists.** A rule is tb's
  `(scenario, request_model) -> services` binding, so "pick a rule" is "pick a
  scenario + model" — and listing them means a provider routes to what *this*
  box has rather than to model ids the author hoped were configured. The
  examples default their targets to the `openai` scenario for the same reason:
  it is the one a stock install already populates.
- **Optional token auth.** `Server(api_key=...)` enforces a bearer token so
  only tb (carrying the matching provider token) can call it — checked once,
  ahead of both routes.
- **`connect_hint()` prints literal, pasteable values** on startup (base URLs,
  key, model). `ux-principles` §11: hand over the artifact for the next
  action; §5: show the concrete value.

### No CLI beyond `doctor`

A `Server` is a Python process you start with `python my_server.py`, and it is
added to tb through the ordinary Connect AI flow. Neither step wants a
bespoke command. `tingly doctor` remains because diagnosis genuinely needs to
traverse the real path (`ux-principles` §7); it ends by printing the
Connect AI values for the serving half.

## UX-principles alignment

- **§3 one word, one meaning.** "Plugin" no longer names two things; the
  frontend's rule-flag "Plugins" owns the word.
- **§2 no mode picker.** `connect()` is identical in dev and hosted contexts;
  the environment decides discovery. `Server` has no transport mode to choose.
- **§6 smart defaults.** `scenario="experiment"`, `model="auto"`, port 8765
  matching the provider template.
- **§5 / §11 concrete values, handed over.** `connect_hint()` and
  `tingly doctor` print pasteable base URLs; `ready=false` and
  `GuardrailBlockedError(policy_id, reason)` name what to fix.
- **§7 diagnostics traverse the real path.** `tingly doctor` runs the actual
  discover → session → live round-trip; `e2e_run.sh` deliberately creates the
  provider via the ordinary `POST /api/v1/providers`, proving no second path
  exists.

## Testing

- Python: `sdk/python/tests/` — config precedence, discovery/session (respx
  mocked gateway), the typed control-plane layer and its failure modes, the
  usage/guardrails views (including a test that pins non-zero token totals, so
  a regression to the always-zero shape cannot pass), transport URL shaping,
  client routing, the dual-protocol server over real HTTP, and the example
  handlers with `use()` faked. Integration tests needing a live tb are marked
  `@needs_tb`, skipped by default. CI (`.github/workflows/python-sdk.yml`) runs
  the whole generate → install → test → package chain on 3.10 and 3.13.
- Go: `internal/server/sdk_session_test.go` freezes the response JSON field
  names (contract with the SDK) and the transport-label logic;
  `internal/typ/scenario_registry_test.go` pins the experiment descriptor;
  `internal/data/provider_template_test.go` validates every embedded template
  including `tingly-python`.
- End-to-end: `sdk/python/examples/e2e_run.sh` — real `tb` binary, no network,
  no keys. See its header for the assertions.

## Examples: one model id in, many rules out

Every example under `sdk/python/examples/` is the same shape — a provider that
connects back to tb and dispatches each request to a *different rule* — and
they differ only in policy: `router_server.py` classifies and picks;
`critic_server.py` always routes away from the caller (self-critique is
unreliable, so a different model is the point); `fusion_server.py` fans out to
several at once and then judges; `quota_router_server.py` picks by *remaining
quota*, which is the case that motivated exposing quota to the SDK at all —
several Codex accounts, one model id, always land on the one with room left.
`rag_experiment.py` is the other half, a script that only consumes.

Two things make dispatch decidable without any new tb mechanism, and both are
plain reads through the generated client:

- **`Client.rules(scenario)`** — a rule is tb's
  `(scenario, request_model) -> services` binding, so "pick a rule" is "pick a
  scenario + model", and listing them means an example routes to what *this*
  box has rather than to ids its author hoped were configured.
- **`Client.quota`** — reduces each provider to its tightest countable window,
  mirroring `ai/quota/semantic.go`. The subtle part is what counts as
  countable: a window that is unknown, unlimited, or uncapped is not 0% used,
  it is an absence of data, and scoring it as 0% would make the account we know
  least about look emptiest and win every comparison. Those report `None`.

The join between them is `service.provider`: quota identifies an account by
uuid, tb addresses upstreams by rule, so a provider is only reachable if some
rule points at it. `quota_router_server.py` does that lookup and says so
plainly when it fails, because that failure is a configuration one.

This is worth stating as the canonical shape because it is where the two halves
compose into something neither has alone: **one model id in your editor fans
out across every rule you have configured**, and because each hop is an
ordinary tb rule, guard rails / quota / logging / tier-failover apply to the
inbound request *and* to every call the provider originates.

`e2e_run.sh` proves the dispatch rather than just the plumbing: its two target
rules are backed by different virtual models, so a short prompt and a long one
must come back from demonstrably different upstreams.

> A trap worth recording, found writing that assertion: Go's `encoding/json`
> HTML-escapes `>` to `\u003e`, so grepping a raw response body for a marker
> like `fast->fast-model` silently never matches even when routing is correct.
> Assert on the decoded value, not on its transport encoding.

## Open follow-ups

1. Scoped short-lived session tokens (`expires_at` + refresh on 401).
2. Dedicated `GET /api/v1/sdk/usage?session=` so usage doesn't scan
   `/api/v1/requests`.
3. Async client (`AsyncClient`, `aask`) — transports already have async
   builders.
