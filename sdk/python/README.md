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
| `@tingly.openai_chat(model)` / `@tingly.chat` | `/v1/chat/completions` | `messages`, `**rest` | `str` (or a ChatCompletion dict) |
| `@tingly.openai_responses(model)` / `@tingly.responses` | `/v1/responses` | `input` (string or item list, as sent), `**rest` | `str` (or a Response dict) |
| `@tingly.anthropic_message(model)` / `@tingly.message` | `/v1/messages` | `messages`, `**rest` (incl. `system`) | `str` (or a Message dict) |
| `@tingly.image(model)` | `/v1/images/generations` | `prompt`, `**rest` | image(s): `bytes`, a PIL image, or a list |
| `@tingly.image_edit(model)` | `/v1/images/edits` | `prompt`, `images` (`list[bytes]`), `**rest` | same as `image` |

Every value is exactly what the caller sent — unpacked from the body, never
converted. `**rest` is the rest of the request body, but only the names your
function declares are passed: `def generate(prompt)` gets just the prompt,
`def generate(prompt, size=None)` also gets `size`, `model` is there if you
declare it, and `**kw` gets everything. Several decorated models can share
one process; they are all listed on `/v1/models` and routed by the
request's `model`.

Register it once in tb: **Connect AI → Self-hosted → Custom endpoint**,
OpenAI, `http://localhost:8765/v1`, no key. For images, point an `imagegen`
rule at that provider and your model name. For text, tb translates
Anthropic- and Responses-speaking clients into Chat for this provider, so
`openai_chat` is usually all you write. The text decorators never convert
between protocols: `anthropic_message` is for a provider registered
Anthropic-style (or Dual), which tb then calls with the Anthropic body
itself, and `openai_responses` for one in Responses mode.

Streaming clients work too (Claude Code always streams): when a request asks
for `stream: true`, your function is still called once, and its complete
reply goes back as that protocol's SSE stream with all the content in one
chunk — so the caller sees the answer arrive at once rather than token by
token. Not yet supported: incremental (token-by-token) streaming, and
serialising calls to a single-GPU pipeline (add a lock yourself if you need
one).

[`examples/image.py`](examples/image.py) is a complete image provider with a
fake model (a stdlib-rendered PNG), so it runs anywhere.

## Your own `Server`

`tingly.openai_chat(...)` & co. and `tingly.serve()` are the same methods on a
default `tingly.Server` — one contract, two ways to reach it. Build your own
`Server` when you need what the default one doesn't have: `.tb` (a `Client`
back into tb), several servers in one process, or isolation in tests.

```python
from tingly import Server, text_of

srv = Server(tb_base_url="http://localhost:12580", tb_token="...")  # or TINGLY_BASE_URL / TINGLY_TOKEN

@srv.openai_chat("relay")
def relay(messages):
    # relay to a different tb model; the ChatCompletion dict goes back as-is
    return srv.tb.chat(model="claude-opus-4-8", messages=messages)

@srv.anthropic_message("relay")
def relay_anthropic(messages, system=None):
    # messages and system exactly as the Anthropic caller sent them; .tb only
    # speaks OpenAI, so content blocks or tool defs would need handling here
    return text_of(srv.tb.chat(model="claude-opus-4-8", messages=messages))

@srv.openai_responses("relay")
def relay_responses(input):
    # input is a string for a simple turn, or the list of input items as sent
    return text_of(srv.tb.chat(model="claude-opus-4-8", messages=[{"role": "user", "content": input}]))

srv.run(port=8765)
```

To have tb call `anthropic_message` with the Anthropic body natively,
register it as a **dual** provider: **Connect AI → Self-hosted → Dual
endpoint**, OpenAI URL `http://localhost:8765/v1`, Anthropic URL
`http://localhost:8765` — either URL works with or without a trailing `/v1`,
since `Server` answers both. `openai_responses` needs no extra step:
`/responses` lives under the same OpenAI URL, and tb picks between it and
`/chat/completions` per the provider's declared endpoint mode. An endpoint
with no function answers 404.

`/v1/messages` is always handled as beta — `?beta=true` and
`anthropic-version` are accepted and not branched on, matching the
simplification tb's own vmodel virtual server makes at its HTTP boundary.

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
