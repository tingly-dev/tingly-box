# tingly (Python SDK) — v1 framework

Zero dependencies. See [`.design/python-sdk.md`](../../.design/python-sdk.md)
for the design rationale and scope cuts.

## The few-lines path

Turn a Python function into a tb provider:

```python
import tingly

@tingly.image("qwen-image-2.1")
def generate(prompt):
    return pipe(prompt).images[0]      # bytes, or a PIL image — returned as-is

tingly.serve()                          # http://0.0.0.0:8765/v1
```

| decorator | serves | your function gets | and returns |
|---|---|---|---|
| `@tingly.text(model)` | `/v1/chat/completions` | `messages`, `**rest` | `str` (or a ChatCompletion dict) |
| `@tingly.image(model)` | `/v1/images/generations` | `prompt`, `**rest` | image(s): `bytes`, a PIL image, or a list |
| `@tingly.image_edit(model)` | `/v1/images/edits` | `prompt`, `images` (`list[bytes]`), `**rest` | same as `image` |

`**rest` is the rest of the request body, but only the names your function
declares are passed: `def generate(prompt)` gets just the prompt,
`def generate(prompt, size=None)` also gets `size`, and `**kw` gets
everything. Several decorated models can share one process; they are all
listed on `/v1/models` and routed by the request's `model`.

Register it once in tb: **Connect AI → Self-hosted → Custom endpoint**,
OpenAI, `http://localhost:8765/v1`, no key. For images, point an `imagegen`
rule at that provider and your model name. For text, tb translates
Anthropic- and Responses-speaking clients into Chat for this provider, so
Chat is all you write.

Streaming clients work too (Claude Code always streams): when a request asks
for `stream: true`, your function is still called once, and its complete
reply goes back as that protocol's SSE stream with all the content in one
chunk — so the caller sees the answer arrive at once rather than token by
token. Not yet supported: incremental (token-by-token) streaming, and
serialising calls to a single-GPU pipeline (add a lock yourself if you need
one).

[`examples/image.py`](examples/image.py) is a complete image provider with a
fake model (a stdlib-rendered PNG), so it runs anywhere.

## The raw layer

The sugar above is built on `tingly.Server`, which hands each endpoint's
exact protocol to your handler. Use it when you need the wire shape itself.

- `tingly.Server` — be a tb provider. Register a handler per protocol you
  want to serve — `@srv.chat` for OpenAI Chat Completions
  (`/v1/chat/completions`), `@srv.responses` for OpenAI Responses
  (`/v1/responses`), `@srv.messages` for Anthropic (`/v1/messages`) — and
  plug it into tb like Ollama. tb itself dispatches to an outbound provider
  over any of these three (`.design/openai-endpoint-routing.md`), so a
  `Server` standing in for an arbitrary provider needs all three available.
  **The three are independent; nothing bridges them, and none gets a typed
  *wrapper*.** Every handler gets exactly the raw parsed request body the
  caller sent — content blocks, `system`, tool defs and all. This is a
  prototype: it hands you each protocol as it actually is on the wire, full
  stop — no in-between shape, no fields picked out on your behalf. The
  *type hints* on those bodies do point at something real, though: `openai`'s
  and `anthropic`'s own `TypedDict` request types
  (`CompletionCreateParamsBase`, `ResponseCreateParamsBase`,
  `MessageCreateParamsBase`) — zero runtime cost (`TypedDict` is a plain
  `dict` at runtime, and the imports are `TYPE_CHECKING`-only, so `openai`/
  `anthropic` are never required), just real, officially-maintained types
  instead of nothing.
- Images: `@srv.images` (`/v1/images/generations`, raw JSON body) and
  `@srv.image_edits` (`/v1/images/edits`; the multipart form decoded into a
  dict — text fields as strings, `image`/`image[]` as `list[bytes]`, `mask`
  as `bytes`). Return an `ImagesResponse` dict, or image(s) to be wrapped as
  `b64_json`.
- Streaming: the three text endpoints honour `stream: true` by replaying
  whatever the handler returned — a wrapped `str` or your own dict — as that
  protocol's SSE events, all content in one chunk. Handlers don't change.
- `tingly.Client` — call tb from Python. Point it at a running tb and a
  gateway token, ask it to run any scenario/model.

## Quick start

```python
from tingly import Server, text_of

srv = Server("relay", tb_base_url="http://localhost:12580", tb_token="...")

@srv.chat
def handle_chat(body):
    # body is the raw OpenAI chat-completion request; here we just relay
    # everything to a different tb model and hand the answer straight back.
    return srv.tb.chat(model="claude-opus-4-8", messages=body["messages"])

@srv.responses
def handle_responses(body):
    # body["input"] is the raw OpenAI Responses request field — a plain
    # string here for a simple text turn (a caller sending structured input
    # items would need its own handling, same caveat as @srv.messages).
    return text_of(srv.tb.chat(model="claude-opus-4-8", messages=[{"role": "user", "content": body["input"]}]))

@srv.messages
def handle_messages(body):
    # body is the raw Anthropic request, untouched. .tb still only speaks
    # OpenAI, so this handler is responsible for whatever translation it
    # needs — plain-string content forwards fine as-is; content blocks or
    # tool defs would need explicit handling here.
    return text_of(srv.tb.chat(model="claude-opus-4-8", messages=body["messages"]))

srv.run(port=8765)
```

Register it with tb as a **dual** provider (same as a service that natively
speaks both protocols, e.g. Vertex): **Connect AI -> Self-hosted -> Dual
endpoint**, OpenAI URL `http://localhost:8765/v1`, Anthropic URL
`http://localhost:8765`, no key required — either URL works with or without
a trailing `/v1`, since `Server` answers both (see below). `@srv.responses`
needs no extra registration step: `/responses` lives under the same OpenAI
URL as `/chat/completions`, just a different path — tb picks between them
per-request based on the provider's declared endpoint mode, not a separate
base URL. From then on tb calls it like any other provider. Register just
the decorator(s) you want to serve — an endpoint with no handler answers 404.

`/v1/messages` requests are always handled as if beta — there is no
`?beta=true` / `anthropic-version` branching, matching the simplification
tb's own vmodel virtual server already makes at its HTTP boundary. The one
deliberate bit of path leniency: every endpoint also answers without the
`/v1` prefix (`/chat/completions`, `/responses`, `/messages`), since which
shape a caller's configured base URL expects isn't worth troubleshooting by
hand.

> **No-key + Anthropic-style provider:** the vendored `anthropic-sdk-go`
> treats a genuinely empty API key as "go discover ambient credentials" and
> errors before ever sending the request — unlike the OpenAI client, which
> sends the empty header as-is. Self-hosted templates let you check "no API
> key" and still type a value in the token field (it stays editable), so
> give it any placeholder (e.g. `not-required`) rather than leaving it
> blank; the server itself never checks incoming auth.

Run the bundled examples directly:

```bash
cd sdk/python
TINGLY_BASE_URL=http://localhost:12580 TINGLY_TOKEN=<gateway token> python examples/relay.py
TINGLY_BASE_URL=http://localhost:12580 TINGLY_TOKEN=<gateway token> python examples/fanout.py
python examples/image.py   # no tb connection needed: a fake image model
```

## Using `Client` standalone

```python
from tingly import Client, text_of

tb = Client(base_url="http://localhost:12580", token="...")
resp = tb.chat(model="gpt-4o", messages=[{"role": "user", "content": "hi"}])
print(text_of(resp))
```

## Quota

`Client` also exposes tb's provider-quota admin API — unlike `.chat()`, its
response shapes are already precisely specified in tb's own `openapi.json`,
so this uses the real generated types instead of hand-rolled dicts:

```bash
task gen:py:quota   # generates tingly/_generated_quota.py; needs pydantic
```

```python
tb = Client(base_url="http://localhost:12580", token="...", admin_token="...")

summary = tb.quota_summary()        # Summary
usages = tb.list_quota()            # ListQuotaResponse
one = tb.get_quota(usages.data[0].provider_uuid)  # ProviderUsage
```

`admin_token` is tb's `UserToken` (the `/api/v1/*` credential), distinct
from the gateway `token` (`.chat()`'s `/tingly/*` credential) — it defaults
to `token` since the two are usually the same secret on a single-operator
box. Install the `quota` extra (`pip install -e '.[quota]'`) to get
`pydantic`, or just `pip install pydantic` — either satisfies the generated
file's only dependency.

## Tests

```bash
cd sdk/python
task gen:py:quota   # from repo root, once — the quota tests need it
python -m unittest discover tests
```

The end-to-end test puts a real tb between a client and the plugins —
registered through tb's admin API as a user would — and checks model
discovery, image generation (and that tb saved the PNG), multipart image
edits, chat, an Anthropic client reaching a Chat-only plugin, and streaming.
It is skipped unless `TINGLY_TB_BIN` points at a tb binary:

```bash
task test:py:e2e    # from repo root: builds tb, then runs tests/test_e2e_tb.py
```
