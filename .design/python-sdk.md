# Python SDK (`tingly`) — v1 framework

> Audience: contributors touching `sdk/python/`, or deciding what a
> language SDK for tb should even be.

## The one idea

**A Provider is whatever answers the LLM protocol. That's the whole model.**
tb already treats Anthropic, OpenAI, Ollama, vLLM, a vmodel, and a Bedrock
adapter as the same thing: a `Provider` row plus a base URL. A process you
wrote in Python that speaks `/v1/chat/completions` is not a new category —
it's a **self-hosted provider**, wired in exactly like Ollama
(`region: "self-hosted"`, `no_key_required: true`, Connect AI → Self-hosted →
Custom endpoint). tb needs zero new backend concepts for this to work; that
mechanism already ships on `main`.

The interesting part is what such a provider can *be*. Because it's your
code, it can:

- be a pure relay — receive a request, forward it to some other rule/model
  inside tb, return the answer unchanged;
- fan out to several tb models and merge the replies before answering;
- look things up (a Portal, a vector store, a cron result, anything) and
  splice that into the prompt before forwarding.

Wired the other way, the same process is also a **client** of tb: give it
the box's address and a token, and it can ask tb to run any rule/model it
wants, the same way any other caller of `/tingly/*` does.

```
   caller                          tingly-box                              upstreams
 ┌────────┐   any protocol   ┌───────────────────────┐                  ┌─────────────┐
 │ ...    ├─────────────────►│ rule/scenario A        │                  │ Anthropic   │
 └────────┘                  │  → provider: "my-sdk"  │                  │ OpenAI      │
                             │     api_base=:8765/v1  ├── (2) plain ────►│ Ollama ...  │
                             │     no_key, self-hosted │    outbound HTTP│             │
                             └───────────┬────────────┘                  └─────────────┘
                                         │ (1) POST /v1/chat/completions
                                         ▼
                             ┌───────────────────────┐
                             │ your Python process     │
                             │  srv = tingly.Server()   │
                             │  @srv.chat               │
                             │  def handle(body):       │
                             │      ... your logic ...  │
                             │      return srv.tb.chat(  │
                             │        model=...,         │──(3)──► back into tb, a
                             │        scenario=...)      │         DIFFERENT rule/model
                             └───────────────────────┘
```

Step (1) is tb calling your `Server` like any other provider. Step (3) is
your process calling back into tb as a `Client`, hitting a second, unrelated
rule/model. tb never sees the difference between (1)+(3) and a request that
went straight to a real upstream — the loop closes entirely outside tb.

## Prior attempt, and why this doc resets scope

`claude/python-sdk-redesign-wwxkiv` (unmerged) already arrived at this exact
insight — its own design notes describe four discarded iterations of a
"plugin provider" concept (a second DB column, a heartbeat/lease registry,
an idempotent upsert endpoint, a `tingly.toml` manifest + `plugin init/run`
CLI) before collapsing back to "it's just a self-hosted provider row". That
part is correct and does not need to be re-derived.

What grew too large was the *implementation* on top of the correct idea:
`_generated/` models off the full 195-operation OpenAPI spec, a typed
`_api.py` control-plane wrapper, `discovery.py`, `config.py`, `scenarios.py`,
a `transports/` package binding the real `openai`/`anthropic` SDKs, a
`helpers/` package (usage, guardrails, quota views), a CLI, 7 test files, 6
examples, a dedicated CI workflow. That's a product, not a v1.

One piece of that branch's *backend* work was correct and narrow enough to
port outright: `internal/server/module/providerquota` mounted its six
routes straight on the gin router with no swagger annotations, so
`provider-quota` never appeared in `openapi.json` — reachable from the UI,
unreachable from any generated client. That branch's fix (routes.go with
`swagger.WithTags`/`WithResponseModel`, an `available()` guard so routes
register even with a nil quota manager during schema generation) is ported
here unchanged; see `internal/server/module/providerquota/routes.go`.

**v1 ships a framework, not a product.** Two classes — `Server`, `Client` —
enough to stand up the loop above end to end. `Server` does answer both
protocols tb supports (see below), since tb's own **dual provider**
mechanism already exists to carry it — but a handler gets the raw request
body exactly as the caller sent it, on both endpoints, with no typed
wrapper and no normalization across protocols. A prototype hands a handler
the real wire shape and stops there; inventing a shape of our own to sit in
front of it — even a "small" one — is exactly the kind of forward-looking
design this v1 doesn't need yet. Everything else (codegen, discovery,
streaming, helpers) stays a deliberate non-goal until someone has a
concrete need for it.

## Shape

```
sdk/python/
  README.md
  pyproject.toml            # no required deps; `quota` extra pulls in pydantic
  codegen_header.txt         # "GENERATED FILE" banner for the one generated file
  scripts/
    extract_quota_schema.py  # openapi.json -> just the provider-quota schema closure
  tingly/
    __init__.py               # exports Server, Client
    client.py                 # Client — call tb from Python; quota methods
    server.py                 # Server — be a provider from Python
    _generated_quota.py       # pydantic v2, from `task gen:py:quota` — NOT committed
  examples/
    relay.py              # pure forwarder, both protocols, independently
    fanout.py             # ask N tb models, merge the replies
  tests/
    test_framework.py     # Server + Client against a stub HTTP server
```

No CLI, no transports/helpers packages, and — after an earlier draft added
one and rolled it back — no hand-written request/response type module
either. If a handler needs the real `anthropic`/`openai` SDK client, it
constructs one itself, pointed at the URL and token `Client` already
resolved — nothing to wrap yet.

`_generated_quota.py` is the one exception to "no `_generated/`", and it is
deliberately not the same thing as the rolled-back `_generated/` package:
that one modeled the *entire* 195-operation spec speculatively; this one is
scoped, on every regeneration, to exactly the schema closure the
provider-quota endpoints use (10 schemas — see
`scripts/extract_quota_schema.py`) — proportional to what `Client` actually
calls, not the whole backend surface.

## `Client` — call tb

```python
from tingly import Client

tb = Client(base_url="http://localhost:12580", token=os.environ["TINGLY_TOKEN"])
reply = tb.chat(model="gpt-4o", messages=[{"role": "user", "content": "hi"}],
                scenario="custom")
```

`chat()` POSTs to `{base_url}/tingly/{scenario}/v1/chat/completions` with
`Authorization: Bearer {token}` and returns the parsed OpenAI-shaped JSON
response — the same envelope any OpenAI-compatible caller of tb gets. Which
rule/model actually answers is resolved by tb's existing scenario+model
routing; the SDK does not duplicate that logic, only calls into it.

`token` is a gateway token (`ModelToken`, or a scoped multi-tenant API
token) — the same credential you'd hand to Claude Code or any other CLI
pointed at tb, obtained from tb's own settings/token management UI. Colloquially
"the admin API key", but nothing here is `/api/v1`-management-scoped; it is
the model-auth credential the `/tingly/*` gateway already accepts.

`chat()` is OpenAI wire only, no streaming. `.tb` always speaks OpenAI to tb
regardless of which protocol a `Server` request arrived on (below) — one
outbound shape is enough; tb accepts OpenAI-style calls on any scenario no
matter what the original caller used.

`Client` also exposes a small, deliberately narrow slice of tb's admin
plane — quota — covered in its own section below. Every other admin-plane
call (list providers, create rule, …) stays out of scope: a plain
`httpx`/`requests` call against tb's already-public API covers those if a
handler ever needs them, and there is still no generated control-plane
wrapper for the *rest* of the API surface (see Non-goals).

## `Server` — be a provider

```python
from tingly import Server, text_of

srv = Server("my-sdk", tb_base_url="http://localhost:12580", tb_token=os.environ["TINGLY_TOKEN"])

@srv.chat
def handle_chat(body):
    # body is the raw OpenAI chat-completion request, unmodified.
    return srv.tb.chat(model="claude-opus-4-8", messages=body["messages"],
                        scenario="custom")

@srv.messages
def handle_messages(body):
    # body is the raw Anthropic messages request, unmodified. This handler
    # is responsible for its own translation if it needs one; .tb still
    # only speaks OpenAI to tb.
    return text_of(srv.tb.chat(model="claude-opus-4-8", messages=body["messages"],
                                scenario="custom"))

srv.run(port=8765)
```

`run()` starts a stdlib `ThreadingHTTPServer` exposing:

- `GET /v1/models` (and `/models`) — a one-entry model list (`srv.name`), so
  tb's "supports_models_endpoint" refresh can discover it, exactly like
  Ollama.
- `POST /v1/chat/completions` (and `/chat/completions`) — OpenAI. Hands
  `@srv.chat`'s handler **the raw parsed request body, unmodified**. A
  `dict` result is passed through as-is (the common case — it's already an
  OpenAI ChatCompletion, e.g. straight from `srv.tb.chat(...)`); a `str` is
  wrapped into a minimal one-choice ChatCompletion envelope.
- `POST /v1/messages` (and `/messages`) — Anthropic. Hands `@srv.messages`'s
  handler **the raw parsed request body, unmodified** — content blocks,
  `system`, tool defs and all. A `dict` result is passed through as-is
  (assumed already Anthropic-shaped); a `str` is wrapped into a minimal
  Anthropic message envelope. Always treated as beta-shaped
  unconditionally — no `?beta=true` / `anthropic-version` branching, the
  same simplification tb's own `vmodel` virtual server already makes at its
  HTTP boundary ("accepts both and canonicalizes to the beta superset").

**The two handlers are completely independent — there is no bridge between
them, and neither gets a typed wrapper.** Both receive exactly the parsed
JSON body the caller sent; nothing extracts fields into a dataclass, and
nothing converts a reply from one protocol's shape into the other's. A
provider that wants to serve both protocols registers both decorators and
writes native code for each against the real shape — the framework's job
stops at routing the request to the right handler and applying a
purely-local (never cross-protocol) string-to-envelope wrap when a handler
returns text. This was tried twice, over two revisions, and rolled back
both times: first a shared `ChatRequest` normalizing across protocols
(folding Anthropic's `system` into a message, flattening content blocks to
text, converting an OpenAI dict reply into an Anthropic envelope via
`text_of()`), then — after that was cut — a "small" OpenAI-only
`ChatRequest` dataclass still standing in front of `@srv.chat`'s raw body.
Both were the same mistake at different sizes: planning a shape on the
handler's behalf instead of handing over what the caller actually sent.
Register only `@srv.chat` (or only `@srv.messages`) to serve one protocol;
the other endpoint then answers 404.

Path leniency is the one deliberate bit of cleverness `Server` does apply,
regardless of protocol: every route also answers without the `/v1` prefix,
since which shape a caller's configured base URL expects isn't worth
troubleshooting by hand.

No streaming — still a deliberate cut, since SSE framing and partial-JSON
handler contracts don't matter until a handler exists that needs them.

### The no-key + Anthropic-style footgun (and why it isn't a backend fix)

The vendored `anthropic-sdk-go` treats a genuinely empty API key as "go
discover ambient credentials" and errors before ever sending the request
(the OpenAI SDK client sends an empty header as-is, so that side always
worked). This looked, on the prior branch, like it needed a backend fix
(`ai.NoKeySentinelToken`, never merged). It doesn't: self-hosted templates
already leave the token field editable even with "no API key" checked
(`optionalEditableToken` in `ProviderFormDialog.tsx`), and
`CreateProvider`/`UpdateProvider` only *require* a token when
`no_key_required` is false. So the fix is operational, not code: give the
Anthropic side of the dual registration any non-empty placeholder token
(e.g. `not-required`) — the SDK's credential-chain check is satisfied, the
placeholder goes out over the wire, and `Server` ignores incoming auth
entirely since it never checked it.

### Register it with tb

As a **dual** provider (see `.design/dual-provider.md`) — the "Dual
endpoint" card, not "Custom endpoint": Connect AI → **Self-hosted** →
**Dual endpoint**, OpenAI URL `http://localhost:8765/v1`, Anthropic URL
`http://localhost:8765` (the Anthropic SDK strips a trailing `/v1` if
present, so either works, but the bare host matches how Bedrock/Vertex/
Azure templates present their Anthropic side), same shared token (see
above). tb then dispatches each inbound request to whichever URL matches
the *client's own* protocol — exactly the mechanism dual-provider was built
for, just with a Python process instead of Vertex/Bedrock behind it.

## Quota — the one place this SDK generates types

`Client.list_quota()` / `.get_quota(uuid)` / `.quota_summary()` call tb's
`provider-quota` admin API and return real generated pydantic models
(`task gen:py:quota`), not dicts:

```python
tb = Client(base_url="http://localhost:12580", token="...", admin_token="...")

summary = tb.quota_summary()                       # Summary
usages = tb.list_quota()                            # ListQuotaResponse
one = tb.get_quota(usages.data[0].provider_uuid)     # ProviderUsage
```

This is a deliberate exception to `Server`'s "hand over the raw wire body,
invent nothing" rule, and the two aren't in tension: that rule is about a
protocol the SDK doesn't own (OpenAI's or Anthropic's own wire format, where
any shape this SDK invented would be a guess). Quota is the opposite case —
tb's *own* API, already precisely specified in its own `openapi.json`, with
a generator that produces the exact types for free. Hand-rolling a dict
parse here would be inventing a *worse*, hand-guessed version of a shape
that already exists correctly. "Use what's already generated instead of
guessing" and "don't invent a shape for a protocol you don't own" are the
same principle pointed at two different situations.

### Generation is scoped to what's used, not the whole spec

`task gen:py:quota` (`Taskfile.yml`) runs `scripts/extract_quota_schema.py`
first: it walks `openapi.json`'s `provider-quota` paths and the transitive
`$ref` closure of schemas they use (10, as of this writing —
`MetaData`, `UsageWindow`, `UsageCost`, `UsageAccount`, `UsageBreakdown`,
`Summary`, `BatchGetQuotaRequest`, `ProviderUsage`, `BatchGetQuotaResponse`,
`ListQuotaResponse`) into a minimal spec, then hands *that* to
`datamodel-code-generator` (pydantic v2, mirroring the prior branch's
`gen:py` invocation) — not the full ~280-schema spec. The output
(`tingly/_generated_quota.py`, 105 lines) is not committed (pure function of
`openapi.json`, see `.gitignore`); regenerate it after any provider-quota
API change, and before running the SDK's tests.

### Two credentials, not one

`.chat()` calls `/tingly/*` with the gateway token (tb's `ModelToken`, or a
scoped multi-tenant API token). Quota calls `/api/v1/*`, which
`getUserAuthMiddleware()` checks against tb's `UserToken` — a genuinely
different credential. `Client.admin_token` defaults to `token` (the two are
usually the same secret on a single-operator box) but can be set separately
when they aren't. This is the first place in the SDK where "the admin API
key" (as the earliest draft of this doc's motivating conversation put it)
means something concrete — everything before this only ever touched
`ModelToken`.

## Non-goals (v1)

Deliberately deferred, not forgotten:

- Streaming (`Server` responses, `Client.chat` as a stream).
- Any bridging between `@srv.chat` and `@srv.messages` — a shared request
  shape, content-block flattening, `system`-folding, or converting one
  protocol's reply into the other's. Tried, rolled back (see above); revisit
  only once a concrete handler needs to serve both protocols from one piece
  of logic, not preemptively.
- `Client` speaking Anthropic wire to tb (`.tb` is OpenAI-only; tb accepts
  that on any scenario regardless of the original caller's protocol, so
  there's no loss in staying single-shape outbound).
- Quota *mutation* (`refresh`, `batch`) — `Client` only exposes the three
  read calls; the two POST endpoints stay unused (their types are generated
  as part of the same schema closure regardless, so adding the methods
  later is a `client.py` change, not a codegen change).
- A generated control-plane client for the rest of the admin API (create
  providers/rules, etc.) — quota is the one surface with a concrete need
  today; everything else stays a plain `httpx`/`requests` call against tb's
  already-public API if a handler ever needs it.
- Discovery / auto-registration (`POST /sdk/session`) — registering the
  provider is a one-time manual step via Connect AI today.
- A CLI, packaging/publishing to PyPI.

## Key files

| File | Role |
|---|---|
| `sdk/python/tingly/client.py` | `Client.chat()` — call any tb scenario/model; quota methods |
| `sdk/python/tingly/server.py` | `Server` — `@srv.chat` / `@srv.messages`, `.tb`, `.run()`, path leniency, both raw-body |
| `sdk/python/tingly/_generated_quota.py` | Generated (`task gen:py:quota`), not committed |
| `sdk/python/scripts/extract_quota_schema.py` | `openapi.json` → provider-quota schema closure, for the generator |
| `Taskfile.yml` (`gen:py:quota`) | The generation task itself |
| `sdk/python/examples/relay.py` | Pure forwarder, both protocols, independently |
| `sdk/python/examples/fanout.py` | Multi-model ask + merge (OpenAI only) |
| `internal/data/providers.json` | Self-hosted provider templates (Ollama et al.) — the mechanism this SDK plugs into, unchanged |
| `.design/dual-provider.md` | The tb-side mechanism `Server`'s dual registration plugs into |
| `frontend/src/components/ProviderFormDialog.tsx` | `optionalEditableToken` — why the no-key footgun has a UI-level workaround, not a backend one |
| `internal/server/module/provider/handler.go` | `CreateProvider`'s `!req.NoKeyRequired && req.Token == ""` check — confirms a placeholder token is accepted |
| `internal/server/module/providerquota/routes.go` | Swagger declarations for provider-quota — ported from the prior branch, the one piece of its backend work worth keeping |
| `internal/server/swagger.go`, `internal/server/server_control.go` | Register provider-quota routes unconditionally (nil-manager-safe) so they always reach `openapi.json` |
| `internal/middleware/auth.go` | `UserAuthMiddleware` vs `ModelAuthMiddleware` — why quota needs `admin_token` and `.chat()` doesn't |
