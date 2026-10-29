# Responses `namespace` tools on flat upstreams

> For contributors touching the Responses ↔ Chat / Anthropic converters, or
> debugging "the model never calls my MCP tool" on a Codex session.
>
> Related: the rest of the Responses wire contract is in
> [`protocol-responses.md`](./protocol-responses.md); the regression cases are
> in [`harness-matrix.md`](./harness-matrix.md) §10.1 (issue #2305).

---

## 1. The problem

Codex sends MCP tools to the Responses API grouped in a `namespace` tool, a
sibling of `function` in the tools list, not a variant of it:

```json
{"type": "namespace", "name": "mcp__linear__", "description": "Linear MCP",
 "tools": [{"type": "function", "name": "get_issue", "parameters": {...}}, ...]}
```

and replays an earlier call in `input` as:

```json
{"type": "function_call", "call_id": "call_1", "namespace": "mcp__linear__",
 "name": "get_issue", "arguments": "{...}"}
```

OpenAI Chat Completions and Anthropic Messages have no such grouping: a tool is
one flat function with one name. Until #2305 the Responses→Chat and
Responses→Anthropic converters kept only `type: function` tools and skipped the
rest, so **every tool inside a namespace silently vanished** before the request
reached the upstream. The model never saw them, so it never called them. Only
Force Responses API (passthrough, nothing converted) worked.

The fix is an adaptation, not a passthrough: offer each namespace member
upstream as an ordinary function, and turn the function calls that come back
into Responses `function_call`s with `namespace` and `name` restored.

```
Responses client (Codex)                                   Chat / Anthropic upstream
  tools: namespace "mcp__linear__"                           tools: function "mcp__linear__get_issue"
         └ function "get_issue"          ── flatten ──▶
  ◀── function_call {namespace:"mcp__linear__",     ◀── restore ──   tool call "mcp__linear__get_issue"
                     name:"get_issue"}
```

Nothing namespace-shaped ever reaches the upstream, and the upstream never
returns one. Restoration is a gateway convention, not a protocol feature.

---

## 2. Flat names (`toolnamespace.FlatName`)

Package `internal/protocol/toolnamespace`. A namespace member gets the flat name

| namespace | member | flat name |
|---|---|---|
| `""` (plain function) | `shell` | `shell` |
| `mcp__linear__` (ends in `_`) | `get_issue` | `mcp__linear__get_issue` |
| `crm` (no trailing `_`) | `get_customer` | `crm__get_customer` |

Codex namespaces an MCP server as `mcp__<server>__`, so the usual result is the
familiar `mcp__<server>__<tool>` that Codex itself uses for flat names.

`FlatName` is **not injective**: namespace `a_` with member `_b` and namespace
`a` with member `b` both give `a__b`, and a plain function can share a flat name
too. That is handled by the collision rule in §5, not by the naming scheme.

---

## 3. Request direction: flatten

Both converters call `toolnamespace.Functions(tools)` and convert the returned
`[]responses.FunctionToolParam` exactly as they always converted plain
functions:

- `ConvertResponsesToolsToChatTools` (`internal/protocol/request/openai_responses_to_chat.go`)
- `ConvertResponsesToolsToAnthropicBeta` (`internal/protocol/request/openai_responses_to_anthropic.go`)

`Functions` returns plain top-level functions as declared, and each function
member of a namespace as a `responses.FunctionToolParam` under its flat name,
with its description and `parameters` schema (an untyped `any` on the SDK's
namespace member, turned into a map). Request order is kept.

**History.** A replayed `function_call` carries its own `namespace` field, so
both converters name it `FlatName(namespace, name)` directly. History
conversion never needs the index (§4); this is why multi-turn works even though
the index is rebuilt for every request.

`strict` is copied onto the expanded function but neither converter forwards it
upstream: the Chat converter never set it (it still has its empty `if` with the note
"strict mode is set via ExtraFields if needed", which this change leaves alone),
and Anthropic has no such field. A namespace member
behaves exactly like a plain function here; `TestResponsesStrictBehavesTheSameInANamespace`
pins that.

---

## 4. Response direction: restore (`toolnamespace.Index`)

The upstream answers with a plain tool call named by the flat name. Putting the
namespace back needs to know which flat names were namespace members, and the
response converters do not have the request: their input is only the upstream
response, and the request was long ago converted into another protocol's request
object. So the request side leaves behind an **`Index`**, a map

```
flat name  →  {Namespace, Name}
```

built with `toolnamespace.IndexOf(tools)` from the *original Responses request's*
tools, and carried alongside the request to the response converters, which do
`index.Restore(flatName)`:

- a hit becomes a `function_call` with `namespace` and `name` set;
- a miss (a plain function like `shell`, or a name the model made up) comes
  out unchanged with no namespace. **Restoration is an exact match on a name
  that was offered; it never guesses.**

The index is nil when the request had no namespace tool, and a nil index
restores nothing, so non-namespace traffic takes the old path unchanged.

### Where the index travels

Three paths convert a Responses client's request, each with its own carrier:

| Path | Index is built | Carried by | Restored in |
|---|---|---|---|
| Responses → **Chat**, legacy dispatch | `convertToOpenAIChat` (`internal/protocol/transform/base.go`), stored in `TransformContext.Extra[ExtraResponsesNamespaces]` | `protocol_cross.go` reads `reqCtx.ResponsesNamespaces()` into `HandleContext.Namespaces` | `nonstream.HandleOpenAIChatToResponses`, `stream.HandleOpenAIChatToResponsesStream` |
| Responses → **Anthropic**, Stage route | `serveOpenAIOnAnthropic` (`internal/protocolserver/stage_openai_route.go`) | `StageOpenAIAttempt.Namespaces`, then `HandleContext.Namespaces` in `stage_openai_adapter.go` | `nonstream.HandleAnthropicBetaToResponses`, `stream.HandleAnthropicBetaToOpenAIResponsesStream` |
| Stage **bridges** (`responsesbridge`) | `Open` (`chat.go`, `bridge.go`), from the call's Responses request | the bridge session's `namespaces` field | `ConvertComplete` / `ConvertStream` of the session |

All of them end in the same four converters:

- `nonstream.ConvertChatToResponsesWire`, `nonstream.ConvertAnthropicBetaToResponsesWire`
- `stream.NewChatToResponsesConverter`, `stream.NewAnthropicBetaToOpenAIResponsesConverter`

which take the index as a plain parameter. In streams the namespace is restored
at every place a function-call item is emitted (`response.output_item.added`,
`response.output_item.done` and the output of the final `response.completed`),
so all three agree, and each pending call is restored on its own, so parallel
calls never share or swap a namespace. `wire.ResponsesOutputItemWire` carries
the new `namespace` field.

### The index lives for one request

Nothing is stored between turns. That is sound because a Responses client
resends its full tools list on every request, so the next turn builds a fresh
index, and replayed history needs no index at all (§3).

---

## 5. Collisions

An upstream offered one name twice cannot be restored unambiguously, and an
index that is overwritten on a clash restores a call as the wrong tool. One
function, `expand` (`toolnamespace.go`), decides what a tool list becomes, and
both `Functions` (request side) and `IndexOf` (response side) are thin entry
points onto it, so the names the upstream was offered and the names the index
knows cannot drift apart:

- a **plain function always keeps its name**, whatever the declaration order;
- the **first** namespace member declared keeps a contested flat name;
- any **later** member that collides is **dropped** (it is not offered), with a
  warning naming the namespace, the tool and the flat name.

This favors "one tool is missing, loudly in the log" over "a call is routed to
the wrong tool". It does not reject the request: a clash needs unusual names
(`a_` + `_b`; Codex's `mcp__<server>__` never produces one), and failing the
whole request would take every tool in the session down for it. If that policy
is ever changed, change it in `expand` only.

---

## 6. What is not converted, and what gets logged

A flat upstream cannot take these, so `Functions` drops them:

| Dropped | Log level | Why |
|---|---|---|
| top-level tool types other than `function` / `namespace` (`web_search`, `custom`, `local_shell`, …) | **debug** | Codex sends them on every request and a Chat / Anthropic upstream never had them; a warning per request would be noise |
| a non-function member inside a namespace (`custom`) | warn | new surface, and the namespace is useless to the model without it |
| a namespace member lost to a name clash (§5) | warn | the model will not be offered the tool |

`IndexOf` makes the same decisions silently, so each request logs once.

---

## 7. Not covered

- **`tool_choice`** naming a namespaced function is not remapped.
- **Custom tools inside a namespace** have no flat equivalent and are dropped.
- **Length.** Flat names are not shortened. A name over 64 characters can be
  rejected by the upstream (Codex's ChatGPT backend path has its own
  shortening in `internal/protocol/ops/request_openai_codex.go`; this one does
  not touch it).
- **Other targets.** Only the Chat and Anthropic targets were changed. Responses
  → Responses is a passthrough and needs nothing; Responses → Google was not
  looked at.

---

## 8. Tests

| Level | Where | What it proves |
|---|---|---|
| Unit | `internal/protocol/toolnamespace/toolnamespace_test.go` | flat names, expansion order, `Restore`, same member name in two namespaces, collisions, unknown/empty names, no tools, description / schema / `strict` fidelity |
| Request converters | `openai_responses_to_chat_test.go`, `openai_responses_to_anthropic_test.go` | tools and replayed history flattened; a namespace member converts exactly like a plain function; `strict` parity |
| Response converters | `nonstream/openai_to_responses_namespace_test.go`, `stream/responses_namespace_test.go` | restoration for Chat and Anthropic, streaming and not; parallel calls keep their own namespace; a truncated stream stays `incomplete` |
| Stage bridge | `responsesbridge/chat_test.go` (`TestResponsesToOpenAIChatNamespaceTools`) | a whole bridge round trip |
| Gateway harness | `internal/protocoltest/content_shapes_namespace.go` | the real gateway: `responses_to_{chat,anthropic}/namespace_tools[_stream]` assert both what the upstream was offered and what the client got back |

The harness cases are request-shaped, so they belong to the content-shape suite
(see `harness-matrix.md` §10.1), which is why they run in `harness matrix
--mode=all`, the Harness PR check. Their case definitions are not in a
`_test.go` file on purpose: `cli/harness` imports `protocoltest`, and a
`_test.go` file cannot be imported. The namespace/member pair in that file is
chosen to flatten to `get_weather`, the name the shared tool-use mock already
answers with, so the case needs no mock of its own.

---

## 9. Extending

- A new Responses → X converter must (1) take its function tools from
  `toolnamespace.Functions`, (2) name replayed calls with
  `toolnamespace.FlatName`, and (3) receive an `Index` from `IndexOf` on its
  request path and restore with `Restore` wherever it emits a function call.
  Missing (3) is the easy mistake, and it only shows up as a model that calls
  a tool whose `namespace` the client then cannot route.
- A different collision or logging policy goes in `expand` and nowhere else.
