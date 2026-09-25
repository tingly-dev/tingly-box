# Python SDK (`tingly`) — v1 framework

> Audience: contributors touching `sdk/python/`, or deciding what a
> language SDK for tb should be.

## The one idea

**A provider is whatever answers the LLM protocol.** A Python process that
speaks `/v1/chat/completions` is a **self-hosted provider**, wired in exactly
like Ollama (Connect AI → Self-hosted → Custom endpoint, `no_key_required`).
tb needs no new backend concept for this.

Because it's your code, such a provider can relay to another tb model, fan
out to several and merge, or look things up and splice them into the prompt.
The same process can also be a **client** of tb, calling any rule/model via
`/tingly/*`:

```
caller ──► tb rule ──(1)──► your Server (Python) ──(3) srv.tb.chat()──► tb, another rule/model
```

tb can't tell (1)+(3) apart from a request to a real upstream; the loop
closes outside tb.

**Scope: a framework, not a product.** Two classes, `Server` and `Client`,
stdlib only. An earlier branch (`claude/python-sdk-redesign-wwxkiv`,
unmerged) reached the same idea but built a product on it (a full
generated API client, discovery, transports, a CLI…); none of that is
carried over. One backend fix from it was: `providerquota` routes now carry
swagger annotations and register even with a nil quota manager, so
provider-quota reaches `openapi.json` (`internal/server/module/providerquota/routes.go`).

## Shape

```
sdk/python/
  pyproject.toml             # no required deps; `quota` extra pulls in pydantic
  codegen_header.txt         # banner for the one generated file
  scripts/extract_quota_schema.py
  tingly/
    server.py                # Server — the one provider contract
    default.py               # tingly.openai_chat / ... / serve — the same methods on a default Server
    client.py                # Client — call tb; quota methods
    _generated_quota.py      # from `task gen:py:quota`; not committed
  examples/  relay.py  fanout.py  image.py
  tests/     test_framework.py  test_default.py  test_e2e_tb.py  helpers.py
```

## `Server` — be a provider

**One contract, reachable two ways.** One `Server` method per endpoint
registers a plain function under a model name; `run()` serves them. The
module-level `tingly.openai_chat(...)` & co. and `tingly.serve()` are the
same methods on a default `Server`, so the contract is defined once
(`server.py`).

```python
import tingly

@tingly.image("qwen-image-2.1")
def generate(prompt):
    return pipe(prompt).images[0]

tingly.serve()                     # == Server() + @srv.image(...) + srv.run()
```

Build your own `Server` for what the default lacks: `.tb` (a `Client` back
into tb — `Server(tb_base_url=..., tb_token=...)`, falling back to
`TINGLY_BASE_URL` / `TINGLY_TOKEN`), several servers in one process, or test
isolation.

### What a function receives and returns

**Unpack, don't convert.** A function gets its endpoint's positional
field(s) exactly as sent, plus the rest of the body as keyword arguments.

| method (alias) | endpoint | positional | may return |
|---|---|---|---|
| `openai_chat` (`chat`) | `POST /v1/chat/completions` | `messages` | `str` or a ChatCompletion `dict` |
| `openai_responses` (`responses`) | `POST /v1/responses` | `input` (string or item list) | `str` or a Response `dict` |
| `anthropic_message` (`message`) | `POST /v1/messages` | `messages` | `str` or a Message `dict` |
| `image` | `POST /v1/images/generations` | `prompt` | image(s) or an `ImagesResponse` `dict` |
| `image_edit` | `POST /v1/images/edits` | `prompt`, `images` | same as `image` |

- **Only declared keywords are passed.** `def f(prompt)` gets just `prompt`;
  `size=None` also gets `size`; `model` is there if declared; `**kw` gets
  the whole rest. That's what keeps a one-liner one line.
- **Replies:** a `dict` passes through as-is (e.g. `srv.tb.chat(...)`'s
  ChatCompletion); a `str` is wrapped in a minimal envelope of the
  endpoint's own protocol (the Responses one checked against
  `openai.types.responses.Response`).
- **Routing:** `/v1/models` lists every registered name (nothing else
  defines that list). A request goes to the function registered for its
  `model`; if an endpoint has exactly one function, it answers any name;
  otherwise an unknown name is a 500 listing the registered ones. An
  endpoint with no function is a 404; `run()` with none at all raises.
- **Nothing bridges the text protocols.** Each method unpacks only its own
  body, and no reply changes protocol. `/v1/messages` is always treated as
  beta (`?beta=true` / `anthropic-version` accepted, not branched on), as
  tb's vmodel server does.
- **Path leniency:** every route also answers without `/v1`.

**Names.** Text has three wire protocols, so text methods carry the full
protocol name; `chat` / `responses` / `message` are aliases (same
functions). Images have one protocol each, so they're named for the
capability.

**Why one contract.** There briefly were two: a raw layer
(`@srv.chat` → `handler(body)`) and sugar on top (`@tingly.text("m")`).
That meant two ways to write the same provider and one word with two
meanings. The raw layer's extras turned out small — the whole body is
`**rest`, the model is `model`, `.tb` and instances are `Server` itself — so
they were merged. Unpacking is not the `ChatRequest` wrapper that was
rolled back twice: that invented a type to read the request through;
`messages` here *is* the body's list. Type hints are up to the function
author (e.g. `messages: list[ChatCompletionMessageParam]`), at no runtime
cost.

### Streaming: the whole reply, as one chunk

Without it, a client calling tb with `stream: true` (Claude Code always
does) got an **empty stream and no error**. Now, on the three text
endpoints, `stream: true` makes `Server` call the function once as usual,
build the complete reply, then replay it as SSE in the same protocol, all
content in one chunk:

| endpoint | stream |
|---|---|
| `/chat/completions` | one `chat.completion.chunk` (whole message as `delta`, tool calls indexed, `finish_reason`); a usage chunk if present; `[DONE]` |
| `/messages` | `message_start` → per block `content_block_start` / one `text_delta` or `input_json_delta` / `content_block_stop` → `message_delta` → `message_stop` |
| `/responses` | `response.created` → per item `output_item.added`, text parts via `content_part.added` / one `output_text.delta` / `output_text.done` / `content_part.done`, `output_item.done` → `response.completed` |

The reply never changes protocol, only from "whole" to "streamed". It's
built before the first byte, so a function error is still a plain 500.
Token-by-token output (a generator contract) is future work; images never
stream.

## Images

tb forwards its own image routes to the provider's endpoint of the same
name (`internal/protocolserver/openai_image.go`, `openai_image_edit.go`):

| tb route | provider gets |
|---|---|
| `/tingly/imagegen/v1/images/generations` | `POST {api_base}/images/generations`, JSON |
| `/tingly/imagegen/v1/images/edits` | `POST {api_base}/images/edits`, multipart |

A localhost `Server` matches no vendor adapter
(`internal/vision/imagegen/vendor.go` `DetectVendor`), so it always gets
the plain OpenAI call. tb persists only `b64_json` entries.

- **Edits are multipart**, decoded with the stdlib into a dict (the only
  decoding done): text fields stay strings (`"n": "1"`, no coercion), files
  become `bytes`. One image arrives as `image`, several as `image[]` — one
  field in two encodings, so both become the `images` list. `mask`, if
  sent, is `bytes` in the rest.
- **Replies:** `bytes`, a list, or anything with `.save(fp, format)` (a PIL
  image) is wrapped as `{"created", "data": [{"b64_json"}]}`; a `dict`
  passes through. `response_format: "url"` is not honoured.

## Register it with tb

- **OpenAI-protocol functions** (`openai_chat`, `openai_responses`,
  images): **Custom endpoint**, OpenAI, `http://localhost:8765/v1`, no key.
  tb translates Anthropic and Responses clients into Chat for a Chat-mode
  provider, so `openai_chat` alone serves most plugins. `openai_responses`
  needs no extra URL: Chat vs Responses is a path under the same base,
  chosen by the provider's endpoint mode (`.design/openai-endpoint-routing.md`).
- **`anthropic_message`** (Anthropic body natively): **Dual endpoint**
  (`.design/dual-provider.md`), OpenAI URL `…:8765/v1`, Anthropic URL
  `…:8765` (either works with or without `/v1`).
- **Images:** then an `imagegen` rule pointing at the provider and the
  registered model name.
- **No-key + Anthropic-style:** the vendored `anthropic-sdk-go` treats an
  empty key as "discover ambient credentials" and fails before sending.
  Give the provider any placeholder token (`not-required`); self-hosted
  templates keep the token field editable with "no key" checked, and
  `CreateProvider` accepts it. No backend change needed; `Server` ignores
  auth.

## `Client` — call tb

`Client(base_url, token).chat(model=..., messages=..., scenario=...)` POSTs
to `{base_url}/tingly/{scenario}/v1/chat/completions` with the gateway
token (`ModelToken` or a scoped API token) and returns the OpenAI JSON.
OpenAI wire only, no streaming — tb accepts it on any scenario. Routing is
tb's; the SDK doesn't duplicate it.

### Quota — the one generated surface

`list_quota()` / `get_quota(uuid)` / `quota_summary()` return pydantic
models generated from tb's own `openapi.json` (`task gen:py:quota`).
Generating types here doesn't contradict "invent nothing": that rule is
about protocols the SDK doesn't own; quota is tb's own, already-specified
API, where a hand-rolled parse would be the invented shape.

- Generation is scoped: `scripts/extract_quota_schema.py` pulls just the
  provider-quota paths and their schema closure (10 schemas), not the
  whole spec. The output is not committed.
- Quota uses `/api/v1/*`, which checks the **`UserToken`**, a different
  credential from the gateway token: `Client(admin_token=...)`, defaulting
  to `token`.

## End-to-end test against a real tb

`tests/test_e2e_tb.py` starts a real tb (throwaway `--config-dir`, free
port), runs the plugins in-process, registers them through tb's admin API
as a user would, and calls `/tingly/<scenario>/v1/...` — every hop is tb's
real code.

- The image provider is `examples/image.py`, unchanged (a fake model that
  renders a stdlib PNG).
- The same process is registered twice: OpenAI-style (no key) and
  Anthropic-style (placeholder key).
- Covered: model discovery, image generation plus tb's saved PNG, multipart
  edit (`image[]` + mask), chat, an Anthropic client reaching a Chat-only
  plugin, `anthropic_message` natively, and streaming on each text path.
- Stdlib only; skipped unless `TINGLY_TB_BIN` is set. `task test:py:e2e`
  builds tb and runs it.

## Auto-registration — parked

`Server` registering itself with tb on `run()` was designed but not built:
a fixed port registered once by hand already covers the common case, and
the mechanism costs real design weight. If revived, the decisions so far:

- Uses only the existing Provider API (`GET`/`POST`/`PUT
  /api/v2/providers`, `UserToken`). No new endpoint, DB column, heartbeat
  or lease.
- **Identity:** a local record `{uuid, name}` written after the first
  create. Record present → `PUT` that uuid (404 → treat as gone). No record
  → look up by name; **found → refuse** (it may be someone else's), not
  found → `POST` and write the record.
- **Never delete**, not even on clean shutdown: rules reference providers
  by uuid, so deleting would break them. A stale row is the user's to
  clean up, like any offline provider.
- Opt-in only (`auto_register=True` plus an admin token).

## Known limitations

- Streaming is single-chunk (above).
- No serialisation of GPU work: requests run on threads, so a single-GPU
  pipeline needs a lock from its author.
- Registration is by hand: one Connect AI entry and one rule.

## Non-goals (v1)

- Incremental streaming, and `Client.chat` streaming.
- Any bridging between the text protocols (shared request shape,
  content-block flattening, `system`-folding, reply conversion). Tried,
  rolled back; revisit only for a concrete need.
- Typing replies against the real response models (`ChatCompletion` etc.)
  — that means a runtime dependency on `openai` / `anthropic`. Undecided.
- `Client` speaking Anthropic or Responses to tb.
- Quota mutation (`refresh`, `batch`); a generated client for the rest of
  the admin API; a tb-side discovery endpoint.
- Image `response_format: "url"`.
- A CLI, PyPI publishing.

## Key files

| File | Role |
|---|---|
| `sdk/python/tingly/server.py` | `Server`: the contract, unpacking, routing, wrapping, streaming, multipart |
| `sdk/python/tingly/default.py` | `tingly.*` functions and `serve()` on a default `Server` |
| `sdk/python/tingly/client.py` | `Client.chat()` and quota methods |
| `sdk/python/scripts/extract_quota_schema.py`, `Taskfile.yml` (`gen:py:quota`, `test:py:e2e`) | Quota codegen; e2e task |
| `internal/server/module/providerquota/routes.go` | Swagger for provider-quota (ported fix) |
| `internal/protocolserver/openai_image.go`, `openai_image_edit.go` | What tb sends a provider for images |
| `internal/vision/imagegen/vendor.go` | `DetectVendor`: why localhost gets the plain OpenAI call |
| `internal/middleware/auth.go` | `UserAuthMiddleware` vs `ModelAuthMiddleware`: why quota needs `admin_token` |
| `internal/server/module/provider/handler.go` | `CreateProvider` accepts a placeholder token with `no_key_required` |
| `.design/dual-provider.md`, `.design/openai-endpoint-routing.md` | How dual registration and Chat/Responses dispatch work in tb |
