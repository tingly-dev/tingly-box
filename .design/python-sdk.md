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
                             │  def handle(req):        │
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

**v1 ships a framework, not a product.** Two classes — `Server`, `Client` —
enough to stand up the loop above end to end. `Server` does speak both
protocols tb supports (see below) — that one was worth doing now, since tb's
own **dual provider** mechanism already exists to carry it and the
alternative (pick one protocol, tell every caller to switch) pushes the
limitation onto users of the provider instead of absorbing it in the SDK.
Everything else (codegen, discovery, streaming, helpers) stays a deliberate
non-goal until someone has a concrete need for it.

## Shape

```
sdk/python/
  README.md
  pyproject.toml         # stdlib only, no runtime deps
  tingly/
    __init__.py           # exports Server, Client
    client.py             # Client — call tb from Python
    server.py             # Server — be a provider from Python
    types.py              # Message, ChatRequest — the shapes the handler sees
  examples/
    relay.py              # pure forwarder: one line of "logic"
    fanout.py             # ask N tb models, merge the replies
  tests/
    test_framework.py     # Server + Client against a stub HTTP server
```

No `_generated/`, no CLI, no transports/helpers packages. If a handler needs
the real `anthropic`/`openai` SDK client, it constructs one itself, pointed
at the URL and token `Client` already resolved — nothing to wrap yet.

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

Only `chat()` ships in v1 — OpenAI wire only. No streaming, no admin-plane
calls (list providers, create rule, …) — a plain `httpx`/`requests` call
against tb's already-public API covers those if a handler ever needs them.
`.tb` always speaks OpenAI to tb regardless of which protocol a `Server`
request arrived on (below) — one outbound shape is enough; tb accepts
OpenAI-style calls on any scenario no matter what the original caller used.

## `Server` — be a provider

```python
from tingly import Server

srv = Server("my-sdk", tb_base_url="http://localhost:12580", tb_token=os.environ["TINGLY_TOKEN"])

@srv.chat
def handle(req):
    return srv.tb.chat(model="claude-opus-4-8", messages=req.as_openai_messages(),
                        scenario="custom")

srv.run(port=8765)
```

`run()` starts a stdlib `ThreadingHTTPServer` exposing:

- `GET /v1/models` — a one-entry model list (`srv.name`), so tb's
  "supports_models_endpoint" refresh can discover it, exactly like Ollama.
- `POST /v1/chat/completions` (OpenAI) and `POST /v1/messages` (Anthropic) —
  **both** parse the body into the same `ChatRequest` shape (`model`,
  `messages`, plus `raw` for anything the two typed fields don't cover;
  Anthropic's `system` field is folded in as a leading system message) and
  call the **same** registered handler — a handler never branches on which
  wire protocol the caller used. The result is then wrapped for whichever
  endpoint was hit:
  - on `/v1/chat/completions`: a `dict` is passed through as-is (the common
    case — it's already an OpenAI ChatCompletion, e.g. straight from
    `srv.tb.chat(...)`); a `str` is wrapped into a minimal one-choice
    ChatCompletion envelope;
  - on `/v1/messages`: a `dict` is reduced back to text (via the same
    extraction `Client`'s `text_of()` does — it's assumed OpenAI-shaped,
    since that's the only shape `.tb` ever hands back) and a `str` is used
    directly; either way the result is wrapped into a minimal Anthropic
    message envelope.

Every `/v1/messages` request is treated as beta-shaped unconditionally —
no `?beta=true` / `anthropic-version` branching. This mirrors what tb's own
`vmodel` virtual server already does at its HTTP boundary ("accepts both and
canonicalizes to the beta superset"): a real caller can't tell the
difference in the response shape, so there is no second code path to
maintain for v1.

No streaming (still a deliberate cut — SSE framing and partial-JSON handler
contracts don't matter until a handler exists that needs them), and no
content blocks / tool calls in a handler's *response* — v1 handlers work
with text on both wire protocols, matching the framework's larger "reduce
to a relay/merge, not full protocol fidelity" scope.

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

## Non-goals (v1)

Deliberately deferred, not forgotten:

- Streaming (`Server` responses, `Client.chat` as a stream).
- Content blocks / tool calls in a handler's response (text only, both
  protocols).
- `Client` speaking Anthropic wire to tb (`.tb` is OpenAI-only; tb accepts
  that on any scenario regardless of the original caller's protocol, so
  there's no loss in staying single-shape outbound).
- A generated control-plane client (`_api.py`-style) — only needed once a
  handler wants to *manage* tb (create providers/rules), not just call
  models through it.
- Discovery / auto-registration (`POST /sdk/session`) — registering the
  provider is a one-time manual step via Connect AI today.
- A CLI, packaging/publishing to PyPI.

## Key files

| File | Role |
|---|---|
| `sdk/python/tingly/client.py` | `Client.chat()` — call any tb scenario/model |
| `sdk/python/tingly/server.py` | `Server` — `@srv.chat`, `.tb`, `.run()`, both wire protocols |
| `sdk/python/tingly/types.py` | `Message`, `ChatRequest`, `as_openai_messages()` |
| `sdk/python/examples/relay.py` | Minimal pure-forwarder provider |
| `sdk/python/examples/fanout.py` | Multi-model ask + merge |
| `internal/data/providers.json` | Self-hosted provider templates (Ollama et al.) — the mechanism this SDK plugs into, unchanged |
| `.design/dual-provider.md` | The tb-side mechanism `Server`'s dual registration plugs into |
| `frontend/src/components/ProviderFormDialog.tsx` | `optionalEditableToken` — why the no-key footgun has a UI-level workaround, not a backend one |
| `internal/server/module/provider/handler.go` | `CreateProvider`'s `!req.NoKeyRequired && req.Token == ""` check — confirms a placeholder token is accepted |
