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
                             │  @srv.openai_chat("m")   │
                             │  def reply(messages):    │
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
enough to stand up the loop above end to end. `Server` does answer all
three protocols tb dispatches an outbound provider over (see below), since
tb's own **dual provider** mechanism already exists to carry two of them
and the third rides the same URL — but a function gets its protocol's own
fields exactly as the caller sent them: unpacked from the body, never
wrapped in a type of ours, never normalized across protocols. A prototype
hands over the real wire values and stops there; inventing a shape of our
own to sit in front of them — even a "small" one — is exactly the kind of
forward-looking design this v1 doesn't need yet. Everything else (codegen, discovery,
incremental streaming, helpers) stays a deliberate non-goal until someone has a
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
    __init__.py               # exports Server, Client, and the module-level functions
    client.py                 # Client — call tb from Python; quota methods
    server.py                 # Server — the one provider contract: register functions, serve them
    default.py                # tingly.openai_chat / ... / serve — the same methods on a default Server
    _generated_quota.py       # pydantic v2, from `task gen:py:quota` — NOT committed
  examples/
    relay.py              # pure forwarder, all three text protocols, independently
    fanout.py             # ask N tb models, merge the replies
    image.py              # image provider via tingly.image, with a fake (stdlib PNG) model
  tests/
    test_framework.py     # Server + Client against a stub HTTP server
    test_default.py       # the module-level functions (the default Server)
    test_e2e_tb.py        # against a real tb binary (skipped without TINGLY_TB_BIN)
    helpers.py            # shared HTTP / multipart helpers (not a test module)
```

No CLI, no transports/helpers packages, and — after an earlier draft added
one and rolled it back — no hand-written request/response type module
either. If a function needs the real `anthropic`/`openai` SDK client, it
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
function ever needs them, and there is still no generated control-plane
wrapper for the *rest* of the API surface (see Non-goals).

## `Server` — be a provider

**One contract, reachable two ways.** `Server` is the provider: one method
per endpoint registers a plain Python function under a model name, and
`run()` serves them. The module-level `tingly.openai_chat(...)` & co. are
the same methods on a default `Server` the module creates on first use,
and `tingly.serve()` is that server's `run()`. So what a function receives
and returns is defined in exactly one place (`server.py`); there is no
second, "raw" way to write a provider.

```python
import tingly

@tingly.image("qwen-image-2.1")
def generate(prompt):
    return pipe(prompt).images[0]

tingly.serve()
```

is the same as `srv = tingly.Server()`, `@srv.image("qwen-image-2.1")`,
`srv.run()`. Build your own `Server` when you need what the default one
doesn't have: `.tb` (a `Client` back into tb — `Server(tb_base_url=...,
tb_token=...)`, falling back to `TINGLY_BASE_URL` / `TINGLY_TOKEN`), more
than one server in a process, or isolation in tests:

```python
from tingly import Server, text_of

srv = Server(tb_base_url="http://localhost:12580", tb_token=os.environ["TINGLY_TOKEN"])

@srv.openai_chat("relay")
def relay(messages):
    return srv.tb.chat(model="claude-opus-4-8", messages=messages)   # dict, passed through

@srv.anthropic_message("relay")
def relay_anthropic(messages, system=None):
    return text_of(srv.tb.chat(model="claude-opus-4-8", messages=messages))

srv.run(port=8765)
```

### What a function receives, and returns

**Unpack, don't convert.** A function receives the field(s) that are the
point of the call, positionally, plus the rest of the same request body as
keyword arguments — every value exactly as the caller sent it, never
translated, normalized or wrapped.

| method (and `tingly.` function) | endpoint | positional | may return |
|---|---|---|---|
| `openai_chat(model)`, alias `chat` | `POST /v1/chat/completions` | `messages` (the body's list, as-is) | `str` or a ChatCompletion `dict` |
| `openai_responses(model)`, alias `responses` | `POST /v1/responses` | `input` (a string or the item list, as-is) | `str` or a Response `dict` |
| `anthropic_message(model)`, alias `message` | `POST /v1/messages` | `messages` (the body's list, as-is) | `str` or a Message `dict` |
| `image(model)` | `POST /v1/images/generations` | `prompt` | image(s) or an `ImagesResponse` `dict` (see Images) |
| `image_edit(model)` | `POST /v1/images/edits` | `prompt`, `images` (`list[bytes]`) | same as `image` |

- **Only the keyword arguments a function declares are passed.** `def
  generate(prompt)` gets just `prompt`; `def generate(prompt, size=None)`
  also gets `size` when the request carries it; `**kw` gets the whole rest
  of the body. The rest includes `model`, so a function answering several
  names can declare `model` to see which one was asked for. This is what
  lets a one-liner stay one line without its author learning the request
  body first — and what makes a separate whole-body contract unnecessary.
- **A `str` is wrapped** into a minimal envelope of the endpoint's own
  protocol (the Responses one validated against the real
  `openai.types.responses.Response` model while building it — see
  `_wrap_text_openai_responses`). **A `dict` passes through as-is**, e.g.
  straight from `srv.tb.chat(...)` for `openai_chat`, since `.tb` always
  speaks OpenAI.
- **Several models, one process.** Each registration names its model;
  `GET /v1/models` (and `/models`) lists every registered name — nothing
  else defines that list — so tb's model discovery finds them all, exactly
  like Ollama. A request is routed by its `model` field. If exactly one
  function is registered on an endpoint, it answers whatever the name (the
  same leniency as the optional `/v1` prefix, so a mistyped model in a curl
  test doesn't fail); with several, an unknown name is a 500 naming the
  registered ones.
- An endpoint with no function answers 404; `run()` with nothing registered
  raises.
- **Nothing bridges the text protocols.** Each method unpacks only its own
  protocol's body; a function registered with `openai_chat` never sees an
  Anthropic request, and no reply is converted from one protocol's shape
  into another's. Which one tb calls is decided by how the provider is
  registered (below). `/v1/messages` is always treated as beta-shaped — no
  `?beta=true` / `anthropic-version` branching, the same simplification
  tb's own `vmodel` virtual server makes at its HTTP boundary.
- Path leniency is the one deliberate bit of cleverness: every route also
  answers without the `/v1` prefix, since which shape a caller's configured
  base URL expects isn't worth troubleshooting by hand.

**Names.** Text has three wire protocols, so each text method carries its
protocol's full name — `openai_chat`, `openai_responses`,
`anthropic_message` — with the short aliases `chat`, `responses`, `message`
(the same functions under a second name, not a separate contract; the full
names stay canonical in docs and error messages). Images have one protocol
each, so those are named for the capability: `image`, `image_edit`.
Because `srv.chat` and `tingly.chat` are now literally the same method,
one word has one meaning (`.design/ux-principles.md`).

### Why one contract, not two

This SDK briefly had two: a raw layer (`@srv.chat` handed the whole parsed
body to `handler(body)`) and a sugar layer on top (`@tingly.text("m")`
unpacked it). That was two ways to write the same provider, two
registration styles, and the same word meaning two things (`tingly.chat`
vs `srv.chat`). Everything the raw layer offered that sugar didn't turned
out to be small: the whole body is `**rest`; the model name is `model`,
passed when declared; `.tb` and explicit instances are just `Server`
itself. So the two were merged into the unpacked contract above, and the
whole-body handler is gone.

Unpacking is not a return of what was rolled back earlier. Twice, over two
revisions, a shape stood in front of the wire body: first a shared
`ChatRequest` normalizing across protocols (folding Anthropic's `system`
into a message, flattening content blocks, converting an OpenAI dict reply
into an Anthropic envelope via `text_of()`), then a "small" OpenAI-only
`ChatRequest` dataclass. Both invented a type and made the handler read the
request through it. Unpacking invents nothing: `messages` *is* the body's
list, `system` *is* the body's value, content blocks and tool defs included.

Request typing follows from that. Handlers used to be annotated with the
upstream SDKs' `TypedDict` bodies (`CompletionCreateParamsBase` & co.,
`TYPE_CHECKING`-only); a function now receives fields, not a body, so
those no longer attach to anything in the SDK. A function author who wants
types annotates its own parameters (`messages:
list[ChatCompletionMessageParam]`) — still zero runtime cost, still no
dependency on `openai`/`anthropic`.

### Streaming: the whole reply, replayed as one chunk

Streaming used to be a deliberate cut, until an end-to-end run through a
real tb showed what that costs: a client calling tb with `stream: true`
(Claude Code always does) reached the plugin, got a plain JSON reply back,
and tb turned that into an **empty stream with no error anywhere** (tb
logs `upstream 200`). Silent failure on the most common client is not an
acceptable prototype gap.

The fix keeps the function contract untouched. When a request to
`/chat/completions`, `/responses` or `/messages` carries `stream: true`,
`Server` calls the function once exactly as before, builds the complete
reply exactly as before (a `str` wrapped, a `dict` passed through), and
then **replays that complete reply as an SSE stream in the same protocol,
with all of its content in a single chunk**:

| endpoint | stream |
|---|---|
| `/chat/completions` | one `chat.completion.chunk` carrying each choice's whole message as its `delta` (tool calls get their `index`) and `finish_reason`; a usage-only chunk if the reply has `usage`; `data: [DONE]` |
| `/messages` | `message_start` → per content block `content_block_start` / one delta (`text_delta`, or `input_json_delta` for `tool_use`) / `content_block_stop` → `message_delta` (stop reason, output usage) → `message_stop` |
| `/responses` | `response.created` → per output item `output_item.added`, and for message text `content_part.added` / one `output_text.delta` / `output_text.done` / `content_part.done` → `output_item.done` → `response.completed` with the full response |

This is not protocol conversion — the reply never changes protocol, it
changes from "whole" to "streamed" within one. It also means a relay
function that returns `srv.tb.chat(...)`'s dict streams correctly with no
change. What it doesn't give is incremental output: the first byte arrives
when the function has finished. Functions that want to stream token by
token (a generator contract) remain future work. Images are never streamed.

Because the reply is built before the first byte is written, a function
error still comes back as a normal HTTP 500, never a half-written stream.

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

Only OpenAI-protocol functions (`openai_chat`, `openai_responses`, images):
a plain **Custom endpoint** (OpenAI, `http://localhost:8765/v1`, no key).
tb translates Anthropic- and Responses-speaking clients into Chat for a
Chat-mode provider, so `openai_chat` alone serves most plugins.

To also get the Anthropic body natively (`anthropic_message`), register it
as a **dual** provider (see `.design/dual-provider.md`) — the "Dual
endpoint" card: Connect AI → **Self-hosted** → **Dual endpoint**, OpenAI URL `http://localhost:8765/v1`, Anthropic URL
`http://localhost:8765` (the Anthropic SDK strips a trailing `/v1` if
present, so either works, but the bare host matches how Bedrock/Vertex/
Azure templates present their Anthropic side), same shared token (see
above). tb then dispatches each inbound request to whichever URL matches
the *client's own* protocol — exactly the mechanism dual-provider was built
for, just with a Python process instead of Vertex/Bedrock behind it.

`openai_responses` needs no third URL: per `.design/openai-endpoint-routing.md`,
Responses-vs-Chat is a *path* choice under the same OpenAI base URL
(`EndpointModeChat`/`Responses`/`Both`, a property of the provider row, not
a second `api_base` field — dual-provider only has `APIBaseOpenAI` and
`APIBaseAnthropic`), so the same OpenAI URL registered above already
carries it.

## Images — `image` / `image_edit`

The motivating case: a self-deployable image model (say Qwen-Image 2.1)
that people already drive from a Python script. Before this section,
`Server` had no way to receive an image request at all — tb serves images
on two endpoints of its own, and forwards both to the upstream provider's
endpoint of the same name:

| tb route | forwarded upstream as | body tb sends |
|---|---|---|
| `POST /tingly/imagegen/v1/images/generations` | `POST {api_base}/images/generations` | JSON (`openai.ImageGenerateParams`) |
| `POST /tingly/imagegen/v1/images/edits` | `POST {api_base}/images/edits` | `multipart/form-data` (`openai.ImageEditParams`) |

(`internal/protocolserver/routes.go`, `openai_image.go`,
`openai_image_edit.go`; `internal/client/openai.go` `ImagesGenerate` /
`ImagesEdit`.) tb only swaps in a vendor-specific adapter for hosts it
recognises (`internal/vision/imagegen/vendor.go` `DetectVendor`: DashScope,
MiniMax, xAI, Qianfan, Codex); a localhost `Server` matches none of them, so
it always gets the plain OpenAI SDK call — exactly the two shapes above.
tb reads the reply as `openai.ImagesResponse`
(`{"created", "data": [{"b64_json" | "url"}], "usage"?}`), and only
`b64_json` entries are persisted to tb's image directory.

So `Server` has two image methods, same contract as the text ones:

- `image(model)` → `POST /v1/images/generations` (and `/images/generations`).
  The body is JSON; the function gets `prompt`, and the rest (`size`, `n`,
  `quality`, …) as declared keywords.
- `image_edit(model)` → `POST /v1/images/edits` (and `/images/edits`). The
  body is multipart, not JSON, so the form is first decoded into a dict —
  the only decoding done, the multipart counterpart of `json.loads`: text
  fields stay strings under their own names (`"prompt"`, `"model"`, `"n":
  "1"` — no coercion to int), file fields become `bytes`. The SDK sends one
  image as `image` and several as `image[]`; both arrive as one
  `list[bytes]`, since that's one field on the wire in two encodings, not
  two fields. The function gets `prompt` and that list as `images`; `mask`,
  when sent, is `bytes` in the rest.

Convenience return for both: `bytes` (one image), a `list` of them, or any
object with a `.save(fp, format)` method (a PIL image — what a diffusers
pipeline returns) — wrapped into `{"created", "data": [{"b64_json"}]}`.
A `dict` is passed through as-is. `response_format: "url"` is not honoured:
the reply is always `b64_json`, which is also the only kind tb persists.

Multipart is parsed with the stdlib (`email` package), so `Server` keeps
zero runtime dependencies.

### Register it with tb

A plain **Custom endpoint** (OpenAI, `http://localhost:8765/v1`, no key),
as for text; the no-key Anthropic footgun above doesn't apply. Then point a
rule in the `imagegen` scenario at that provider and the model name the
function was registered under (listed on `/v1/models`).

## Known limitations, not addressed yet

- **Streaming is single-chunk** (see "Streaming" above): streaming clients
  work, but the whole reply arrives at once when the function returns.
- **No serialisation of GPU work.** Requests are handled on threads; two
  concurrent image requests call the same pipeline concurrently. A
  single-GPU pipeline usually needs a lock — the author's to add for now.
- **Registration is still by hand** (auto-registration stays parked, below).
  So "one line" is one line of code plus one Connect AI entry and one
  rule, not zero setup.

The image example (`examples/image.py`) fakes the model: it renders a
solid-colour PNG with the stdlib instead of loading a real pipeline, so it
runs anywhere and still returns a real image tb can persist.

## End-to-end test against a real tb

Unit tests stub tb out. `tests/test_e2e_tb.py` doesn't: it starts a real
tb binary with a throwaway `--config-dir` on a free port, starts the
plugins in-process, registers them through tb's own admin API exactly as a
user would (Custom endpoint, no key; `imagegen` and text rules), and sends
requests to tb's `/tingly/<scenario>/v1/...` — so every hop is real: tb's
routing, its openai-go/anthropic-go upstream clients, its multipart
encoding, its stream re-emission, its image persistence.

- The image provider is `examples/image.py` itself, loaded unchanged.
- The same plugin process is registered twice — OpenAI-style (no key) and
  Anthropic-style (a placeholder key, which tb's Anthropic client needs).
- Covered: model discovery (`/v1/models`), image generation (and that tb
  saved the PNG), image edit (multipart, `image[]` + mask), chat,
  an Anthropic client reaching a Chat-only plugin, an Anthropic-style
  provider getting the Anthropic body natively (`anthropic_message`), and
  streaming for each text path.
- Stdlib only, like the rest of the tests. Skipped unless `TINGLY_TB_BIN`
  points at a tb binary; `task test:py:e2e` builds one and runs it.

## Auto-registration (parked — recorded for later, not being built now)

Today, wiring a running `Server` into tb is a manual step: open Connect AI,
pick Self-hosted → Dual endpoint, type the name and both URLs, save. The
question that started this section was whether `Server` should do that
step itself. Revisiting it: **it shouldn't, not yet.** A user who picks a
fixed port once already has everything they need — register that port in
Connect AI a single time, and every later run of the same script reuses the
same port and needs nothing further; only *changing* the port means
touching Connect AI again, which is exactly as much friction as changing
any other provider setting. Auto-registration's entire value is saving that
one manual step, at the cost of everything below it (identity, the local
record, the refuse-on-collision rule, the no-delete rule) — real design
weight for a save that a fixed port already makes rare. That's not what a
prototype needs a mechanism for.

The design is kept below, not deleted, because the underlying idea isn't
wrong — a future need (many short-lived `Server` instances, ports that
must be dynamic, onboarding someone who won't want Connect AI's picker)
could revive it. Until then: **not implemented, not default-on, not shown
in `examples/`.** A user who wants this today picks a port, registers it by
hand once, same as any other self-hosted provider.

<details>
<summary>Design, as discussed (parked)</summary>

Today, wiring a running `Server` into tb is a manual step: open Connect AI,
pick Self-hosted → Dual endpoint, type the name and both URLs, save. Every
restart on a different port means redoing it. The proposal: given
`srv.tb` (an admin-capable `Client`), `Server` registers itself with tb on
`run()` — so the port can be `0` (OS-assigned, no longer something a human
ever has to pick or type) and the plugin is usable the moment it starts.

### What already exists to build this from

No new tb backend work — the provider CRUD API is already complete and
already in `openapi.json` (`internal/server/module/provider/{types,routes}.go`):

| Call | Use |
|---|---|
| `GET /api/v2/providers` | find an existing row by name |
| `POST /api/v2/providers` | create |
| `PUT /api/v2/providers/:uuid` | update (port changed since last run) |

All three already require `UserToken` — the same `admin_token` `Client`'s
quota methods added. `DELETE /api/v2/providers/:uuid` exists too, but
auto-registration never calls it — see "no delete, ever" below.

### Identity: name, guarded by a local record of what we created

Matching by `name` alone is the simplest possible identity, and needs no
new tb-side concept — but on its own it means two `Server`s (or one
`Server` and one manually-created provider) sharing a name collide: the
second one to start would silently overwrite the first's URLs on its next
`PUT`.

Resolved: **name decides identity only on a `Server` that has never
registered before; every later run trusts a small local record instead.**
No backend change (`CreateProviderRequest` has no client-supplied-UUID
field to build on, unlike `EnsureSmartGuideRuleForBot`'s rule-UUID pattern —
adding one was considered and set aside; the record below gets the same
result without touching the generic Provider API's shape). Concretely, a
small JSON file next to wherever `Server` runs (path configurable, default
derived from the first registered model name) stores `{"uuid": "...", "name": "..."}` after the
first successful registration, and is kept **permanently** — nothing ever
deletes it (see "no delete, ever" below):

1. Bind the HTTP server first (`port=0` is the point — the OS picks a free
   one; auto-registration is what removes the reason to ever hardcode a
   port).
2. Local record present → `PUT /api/v2/providers/{recorded uuid}` directly.
   No name lookup, no collision check: this `Server` made that row, full
   stop. If the `PUT` 404s (the row was deleted out from under it, e.g. by
   hand in Connect AI), treat it as gone and fall through to step 3.
3. No local record (or it just went stale per step 2) → `GET
   /api/v2/providers`, look for a row with the provider name (by default
   the first registered model name):
   - **Found → refuse.** Raise, naming the conflicting provider; do not
     touch it. This `Server` has no record of having created it, so it
     might be a real stranger's row that merely happens to share a name.
   - **Not found → `POST /api/v2/providers`**: `name=<provider name>`,
     `auth_type="api_key"`, `no_key_required=True`, `token=<placeholder>`
     (sidesteps the anthropic-sdk-go empty-key footgun above),
     `api_style="openai"`, `api_base`/`api_base_openai` =
     `http://{advertise_host}:{port}/v1` when an OpenAI-protocol function
     is registered, `api_base_anthropic` = the bare URL when an
     `anthropic_message` function is registered — only the URL(s) for protocols the `Server` actually
     serves. Write the returned UUID to the local record.

That's the whole lifecycle — there is no step 4. `run()` only ever creates
or updates; nothing in `Server` calls `DELETE`, on shutdown or otherwise.

`advertise_host` defaults to `127.0.0.1` regardless of what host the HTTP
server itself binds to — this stays a same-box prototype (tb and the
plugin on one machine); reaching a plugin across machines is out of scope.

### No delete, ever

An earlier draft of this design had a clean shutdown delete the provider
row (and its local record) so a stopped `Server` left nothing behind. Wrong:
a provider is not a leaf node. Rules reference it by UUID; deleting it out
from under a rule the user built on top of this provider breaks that rule
or silently orphans it — a cascading, destructive side effect for
`srv.run()` to trigger on nothing more than the process exiting normally.
`Server` **only ever registers** — creates once, updates on every later
run, never deletes. A provider (and the local record pointing at it) that
outlives its `Server` process is the same as any other stale provider a
human forgot to remove: the user's call to clean up in Connect AI, not a
risk this SDK takes on their behalf. This is also simpler, not just safer:
no "did the delete succeed," no "should the local record survive a failed
delete" — the local record is just permanent.

The rejected alternative — a backend-accepted, caller-pinned UUID
(`Server(uuid="...")`, no lookup, no collision handling, "wrong or reused
UUID is the caller's problem") — was the more literal reading of "preset a
UUID," and is genuinely simpler at the call site. It was set aside because
its simplicity is paid for by a backend change to a generic, already-stable
API (`CreateProviderRequest` growing a co-owned-identity field used by
exactly one caller), where the local-record approach gets the same
recoverable-identity result entirely client-side.

### What this deliberately does not do

Mirrors the four discarded iterations the prior branch's own design notes
already worked through (see "Prior attempt" above) — restating them here so
this proposal doesn't quietly re-invent any of them:

- **No heartbeat, lease, or TTL.** A stopped `Server` — crashed or cleanly
  exited, no distinction now that there's no delete step — leaves a stale
  provider row, the same failure mode as any other misconfigured or offline
  provider today. tb's existing provider-error surfacing is what a user
  checks; this does not add a liveness system on top of it.
- **No new "plugin" concept, DB column, or dedicated registration
  endpoint.** Auto-registration is entirely the generic Provider CRUD API
  that already exists — the row it creates is indistinguishable from one a
  human made by hand through Connect AI.
- **No sub-process supervision, no reverse-proxy mounting.**
- **Opt-in, not automatic-automatic.** Requires `auto_register=True`
  explicitly, and only runs when `srv.tb.admin_token` is actually set — a
  `Server` with no admin token behaves exactly as it does today: unregistered,
  wire it up by hand.

</details>

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

This is a deliberate exception to `Server`'s "hand over the wire values as
sent, invent nothing" rule, and the two aren't in tension: that rule is about a
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

- Incremental streaming (a generator contract yielding tokens) —
  `Server` streams, but always as one chunk; and `Client.chat` as a stream.
- Serialising work per model in `Server` (a lock around a single-GPU
  pipeline) — left to the function author for now.
- Image `response_format: "url"` — replies are always `b64_json`.
- Any bridging between `openai_chat`, `openai_responses` and
  `anthropic_message` — a shared request shape, content-block flattening,
  `system`-folding, or converting one protocol's reply into another's.
  Tried, rolled back (see above); revisit only once a concrete function
  needs to serve more than one protocol from one piece of logic, not
  preemptively.
- Typing a function's return value, or `Client.chat()`'s, against the real
  response Pydantic models (`ChatCompletion`, `Response`, `Message`) —
  doing this for real means depending on `openai`/`anthropic` at runtime.
  Raised, not decided.
- `Client` speaking Anthropic or Responses wire to tb (`.tb` is OpenAI Chat
  only; tb accepts that on any scenario regardless of the original caller's
  protocol, so there's no loss in staying single-shape outbound).
- Quota *mutation* (`refresh`, `batch`) — `Client` only exposes the three
  read calls; the two POST endpoints stay unused (their types are generated
  as part of the same schema closure regardless, so adding the methods
  later is a `client.py` change, not a codegen change).
- A generated control-plane client for the rest of the admin API (create
  providers/rules, etc.) — quota is the one surface with a concrete need
  today; everything else stays a plain `httpx`/`requests` call against tb's
  already-public API if a function ever needs it.
- Auto-registration — parked (see above): a fixed port registered once by
  hand in Connect AI already covers the common case cheaply enough that
  the mechanism isn't worth building yet. Registering the provider stays a
  one-time manual step.
- Discovery via a dedicated tb-side endpoint (`POST /sdk/session` or
  similar) — the parked auto-registration design builds on the generic
  Provider CRUD API instead of one of these, so this stays out of scope
  regardless of whether that design is ever revived.
- A CLI, packaging/publishing to PyPI.

## Key files

| File | Role |
|---|---|
| `sdk/python/tingly/client.py` | `Client.chat()` — call any tb scenario/model; quota methods |
| `sdk/python/tingly/server.py` | `Server` — the one contract: `openai_chat` / `openai_responses` / `anthropic_message` (+ aliases) / `image` / `image_edit`, unpacking, model routing, `models`, `.tb`, `.run()`, wrapping, streaming, path leniency |
| `sdk/python/tingly/default.py` | `tingly.openai_chat` & co. and `tingly.serve()` — the same methods on a lazily created default `Server` |
| `.design/openai-endpoint-routing.md` | Why `openai_responses` exists and needs no second URL — tb's Chat/Responses/Both provider dispatch model |
| `sdk/python/tingly/_generated_quota.py` | Generated (`task gen:py:quota`), not committed |
| `sdk/python/scripts/extract_quota_schema.py` | `openapi.json` → provider-quota schema closure, for the generator |
| `Taskfile.yml` (`gen:py:quota`) | The generation task itself |
| `sdk/python/examples/relay.py` | Pure forwarder, all three text protocols, independently |
| `sdk/python/examples/image.py` | Image provider via `tingly.image`, fake stdlib-PNG model |
| `internal/protocolserver/openai_image.go`, `openai_image_edit.go` | What tb sends a provider for `/images/generations` (JSON) and `/images/edits` (multipart) |
| `internal/vision/imagegen/vendor.go` | `DetectVendor` — why a localhost `Server` always gets the plain OpenAI images call |
| `sdk/python/examples/fanout.py` | Multi-model ask + merge (OpenAI only) |
| `internal/data/providers.json` | Self-hosted provider templates (Ollama et al.) — the mechanism this SDK plugs into, unchanged |
| `.design/dual-provider.md` | The tb-side mechanism `Server`'s dual registration plugs into |
| `frontend/src/components/ProviderFormDialog.tsx` | `optionalEditableToken` — why the no-key footgun has a UI-level workaround, not a backend one |
| `internal/server/module/provider/handler.go` | `CreateProvider`'s `!req.NoKeyRequired && req.Token == ""` check — confirms a placeholder token is accepted |
| `internal/server/module/providerquota/routes.go` | Swagger declarations for provider-quota — ported from the prior branch, the one piece of its backend work worth keeping |
| `internal/server/swagger.go`, `internal/server/server_control.go` | Register provider-quota routes unconditionally (nil-manager-safe) so they always reach `openapi.json` |
| `internal/middleware/auth.go` | `UserAuthMiddleware` vs `ModelAuthMiddleware` — why quota needs `admin_token` and `.chat()` doesn't |
| `internal/server/module/provider/{types,routes}.go` | Provider CRUD API — what the parked auto-registration design would have called; no backend change needed for it |
