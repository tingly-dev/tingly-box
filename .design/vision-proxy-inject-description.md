# Vision proxy: inject image descriptions into model response

**Status**: Proposed
**Date**: 2026-06-15
**Branch**: `claude/vision-proxy-inject-description`
**Supersedes**: nothing — extends behaviour defined in `.design/vision-proxy-scenario.md` and `.design/vision-proxy-rule.md`.

---

## Problem

Today's vision proxy replaces image content parts in the **forwarded request** with a wrapper text containing the vision-upstream description. The replacement text is only ever seen by the downstream text-only model; the client never receives it. Consequences:

1. The description never lands in the conversation transcript on the client side.
2. On the next turn, the client re-sends the original `image_url` in history (because it never knew the proxy transformed anything). The historical-strip path replaces those with a fixed marker — **all prior descriptions are lost forever**.
3. Even ignoring cache concerns, future turns lose any ability to reason about previous images: the model has no access to descriptions it (or a sibling turn) saw a moment ago.

User intent: preserve the description by **emitting it as part of the model's response** so the assistant transcript naturally carries the description. Subsequent turns sent by the client will then include the description text in the assistant message history, and a text-only model can reason about prior images by reading these descriptions.

---

## Design decisions (locked in with the user)

1. **Approach** — inject description text into the model's response (not metadata, not prompt-shaping).
2. **Wrapper format** — `<image-description>…</image-description>`. Both the request-side replacement *and* the response-side injection use the same wrapper, so request and response are symmetric and identifiable by any rendering layer.
3. **Position** — prepended to the first text-bearing chunk / first text block of the model response. One injection per request, before any model output.
4. **Multi-image** — each description is wrapped in its own pair of tags. Tags are separated by `\n` so each description is its own line; the whole block is bracketed by `\n\n` against the model's actual output to leave a clean visual break:
   ```
   <image-description>desc 1</image-description>
   <image-description>desc 2</image-description>
   
   ...real model output starts here...
   ```
5. **Transport scope** — all four: OpenAI Chat, OpenAI Responses, Anthropic Messages V1, Anthropic Messages Beta. Both streaming and non-streaming for each.

---

## Architecture

Two layers:

### A. Capture (request side, already mostly in place)

The vision-proxy processor already calls the upstream and gets back a description string in `describe()` (`internal/server/processor/vision_proxy.go:113-135`). Today the string is only used as the replacement text for the request. New: it also needs to be **collected** so the response layer can read it.

- Add a per-request **`DescriptionCollector`** carried on the `smartrouting.ProcessorContext` (or a thin sub-struct attached to it). `describe()` appends each non-empty description to the collector.
- `applyVisionProxy` (the gin-context-aware wrapper, `internal/server/vision_proxy.go:17`) transfers the collected descriptions onto the gin context via a typed key, e.g. `ctxKeyVisionDescriptions`.
- Failed describes (currently collapsed to `[image: (description unavailable)]`) are also stored — they're useful information for the transcript. Stored as the same wrapped string the response layer will emit.

This keeps the processor package pure (no gin dependency) and concentrates the gin coupling at the existing boundary.

### B. Inject (response side, new)

A small shared helper:

```go
// internal/server/processor/vision_inject.go (new file, processor pkg)
// Returns the full prefix string to prepend before the first model text,
// or "" when there are no descriptions to inject.
//
// Each description is on its own line wrapped in <image-description>…</image-description>,
// then a blank line separates the block from the model output.
func BuildVisionDescriptionPrefix(descs []string) string { … }
```

And a tiny gin-context accessor in `internal/server/`:

```go
// internal/server/vision_proxy.go
func consumeVisionDescriptions(c *gin.Context) []string { … }  // returns and clears
```

`consumeVisionDescriptions` is called exactly once per request from the response handler — it returns the slice and removes it from the context so a second handler (e.g. a streaming retry) cannot double-inject.

Then the per-transport patches are mechanical: in the "first text-bearing chunk" path, prepend `BuildVisionDescriptionPrefix(consumeVisionDescriptions(c))` to the first non-empty text and continue with normal handling.

---

## Injection points (eight)

There is no shared chokepoint after upstream conversion — each transport has its own streaming and non-streaming handler. Patching is per-handler but uses the same helper. All eight share the same first-text-only invariant (tracked by a small `injected bool` flag local to the handler scope).

| # | Transport             | Mode      | File                                                              | Function                                    | Field to mutate                                  |
|---|-----------------------|-----------|-------------------------------------------------------------------|---------------------------------------------|--------------------------------------------------|
| 1 | OpenAI Chat           | streaming | `internal/protocol/stream/openai_passthrough.go`                  | `HandleOpenAIChatStream`                    | first non-empty `choices[].delta.content`        |
| 2 | OpenAI Chat           | non-stream| `internal/server/openai_chat.go`                                  | `nonstreamOpenAIChat`                       | `choices[0].message.content`                     |
| 3 | OpenAI Responses      | streaming | `internal/protocol/stream/openai_passthrough.go`                  | `HandleOpenAIResponsesStream`               | first `response.output_text.delta` event's `delta` field (or first text part of `response.output_item.added`, whichever fires first) |
| 4 | OpenAI Responses      | non-stream| `internal/protocol/nonstream/openai_passthrough.go`               | `HandleOpenAIResponsesPassthroughNonStream` | first `output[*].content[*].text` of type `output_text` |
| 5 | Anthropic V1          | streaming | `internal/protocol/stream/anthropic_passthrough.go`               | `HandleAnthropic`                           | first `content_block_delta` event of type `text_delta`, field `delta.text` |
| 6 | Anthropic V1          | non-stream| `internal/protocol/nonstream/openai_to_anthropic.go`              | `ConvertResponsesToAnthropicV1Response`     | first text block's `.Text`                       |
| 7 | Anthropic Beta        | streaming | `internal/protocol/stream/anthropic_passthrough.go`               | `HandleAnthropicBeta`                       | same as V1, beta event type                       |
| 8 | Anthropic Beta        | non-stream| `internal/protocol/nonstream/openai_to_anthropic.go`              | `ConvertResponsesToAnthropicBetaResponse`   | first beta text block's `.Text`                  |

### Streaming subtlety: "first text" identification

For SSE handlers we cannot inject at "the first event" because the first event might be a role / start marker without text. The invariant is "first non-empty text fragment" — handlers maintain an `injected bool` initialised to false, and on each event, if it is a text-bearing delta with a non-empty text payload AND `!injected`, prepend the prefix to that payload and set `injected = true`. If no text fragment ever fires (rare: tool-only responses), the prefix is silently dropped — that's acceptable since there is no visible turn output to attach it to.

For streaming events delivered as raw JSON bytes (`evt.RawJSON()`), the cheapest mutation is `gjson`/`sjson` (the project already uses `tidwall/gjson` and `tidwall/sjson`). Re-encoding the whole event with `encoding/json` round-trips is unnecessary and would lose field ordering.

### Non-streaming subtlety: structure may have no text yet

For non-streaming Anthropic, the response may contain only tool_use blocks. In that case we have two options:
- (a) Silently drop the prefix (consistent with streaming behaviour).
- (b) Prepend a synthetic text block to the `content[]` array.

I propose **(a)** for consistency. If a future agent loop fires a tool turn without any text, the descriptions are still in the *input* of that turn (request-side replacement still happened), so the model already saw them — they just don't appear in the user-visible transcript for this turn. The next text turn will have its own descriptions if any new images are involved.

---

## Wrapper format changes (request side too)

The current request-side text in `describe()` is:

```go
return "Here is an [image] with message and is parsed into description [image: " + desc + "]"
```

And `imageHistoricalText` is:

```go
const imageHistoricalText = "[image: (omitted from history)]"
```

Both change to the unified wrapper (newlines included so the placement inside a content_parts array is stable):

```go
// describe() returns:
"\n<image-description>" + desc + "</image-description>\n"

// describe() failure path:
"\n<image-description>(description unavailable)</image-description>\n"

// imageHistoricalText:
"\n<image-description>(omitted from history)</image-description>\n"
```

This makes the wrapper a single, greppable invariant across the codebase. A client can confidently regex/parse `<image-description>(.+?)</image-description>` to extract or hide descriptions.

(For request side these strings already go inside content parts wrapping is by-token, so the leading/trailing `\n` is the conservative choice — it survives concatenation if the host content joins parts without separators.)

---

## Edge cases

| Case                                                  | Behaviour                                                                                                  |
|-------------------------------------------------------|------------------------------------------------------------------------------------------------------------|
| No images in request                                  | No descriptions collected; helper returns ""; injection is a no-op.                                        |
| Vision upstream fails on every image                  | "(description unavailable)" is what gets stored *and* injected — the transcript explicitly records the failure. |
| Streaming response has no text fragment (tool-only)   | Prefix dropped silently. Acceptable; see above.                                                            |
| Concurrent requests on same Server                    | Collector lives on per-request `gin.Context` / `ProcessorContext`; no cross-request leakage.               |
| Streaming retry after first SSE byte already sent     | `consumeVisionDescriptions` clears the context value, so a re-issued retry must explicitly opt back in. Today's retry loop reissues with the same context; we'll need to verify whether retries can resurrect the collector — see open question below. |
| Description text containing `</image-description>`    | The vision upstream is text we don't trust to be tag-safe. We escape `<` and `>` in description bodies before wrapping (`html.EscapeString`-style; only `<` `>` `&` for minimal damage). Decoder side: opt-in. |

---

## Open questions

1. **Streaming retries** — when the protocol-dispatch retry loop reissues a request mid-stream, do we want descriptions injected again at the start of the second attempt? Probably yes (since the client only sees one consolidated stream). Need to confirm by reading `dispatchWithPriorityFailover`.
2. **Token accounting** — the injected prefix consumes output tokens from the model's accounting perspective if we count it. Today injection bytes are not produced by the model; they are not counted in upstream usage. Should they be added to usage tracked locally? Default: no — usage is "what the upstream charged us"; injection is overhead we shoulder. We may want a separate counter (`vision_proxy_injected_bytes`) for observability.
3. **Renderers that don't know the tag** — for SDK clients that render raw text (CLI tools, plain webhooks), `<image-description>…</image-description>` will appear literally. Acceptable for v1; document in `.design/`. If readability becomes an issue, we add a flag (`extensions.vision_proxy_injection_format = "tag"|"markdown"|"plain"`) later.

---

## Implementation plan

### Phase 1 — Capture path

1. `internal/server/processor/vision_proxy.go`
   - Add `DescriptionCollector` (a slice with `Append` / `Snapshot` methods, no map needed).
   - `describe()` calls `collector.Append(wrappedString)` on every successful describe *and* every fail-strip (so both surfaces in the transcript).
   - Drop the verbose `"Here is an [image] with message…"` wrapper; use `<image-description>…</image-description>\n` with leading `\n`.
   - Update `imageHistoricalText` to the same wrapper.
   - Escape `<`, `>`, `&` in description bodies.

2. `internal/server/vision_proxy.go`
   - `applyVisionProxy` attaches a fresh collector to the `ProcessorContext` before calling `Process`; afterwards stores `collector.Snapshot()` on `c` under `ctxKeyVisionDescriptions`.
   - New `consumeVisionDescriptions(c) []string` accessor that reads-and-clears.

3. `internal/server/processor/vision_inject.go` (new)
   - `BuildVisionDescriptionPrefix([]string) string` — the shared helper; pure function.
   - Unit tested with empty/single/multi/escaped inputs.

### Phase 2 — Per-transport injection (eight patches, all using the helper)

Patches in the table above. Each handler:
- Reads `consumeVisionDescriptions(c)` once at entry.
- For non-streaming: prepends the prefix to the first text field, marshals, sends.
- For streaming: holds a local `injected bool`; on the first text fragment whose payload is non-empty, prepend the prefix to that payload, set `injected = true`.

### Phase 3 — Tests

Per-transport, both modes — 8 happy-path tests:
- Send a request with one image; assert the response (assembled / streamed) starts with `\n<image-description>…</image-description>\n\n` before the model's output.
- Plus three behavioural tests shared across transports:
  - **Multi-image**: two images → two stacked `<image-description>` tags, in encounter order.
  - **No image**: assert no prefix is injected.
  - **Tool-only response (streaming)**: assert no crash, descriptions silently dropped.
- Plus a marshal-and-grep contract test on the helper: output regex matches `^(?:\n<image-description>[^<]*</image-description>\n)+\n$` or is empty.

### Phase 4 — Docs

- Update `.design/vision-proxy-scenario.md` to mention the response-injection contract (cross-link this doc).
- Add a CLAUDE.md / README note for client implementers: descriptions appear in the assistant transcript wrapped in `<image-description>…</image-description>`; suppress rendering if you want, but keep them in any history you replay to the model.

---

## Out of scope (deferred)

- Returning descriptions as structured metadata (Approach 3 from the design discussion) — kept as a possible add-on; current design doesn't preclude it.
- Caching descriptions across turns by content hash — orthogonal optimisation.
- Telling the model in the system prompt that `<image-description>` blocks are proxy-generated context — could improve grounding but is a behaviour change in model output we don't want to commit to in v1.

---

## Risk summary

- **User-visible output change**: yes — clients will see `<image-description>…</image-description>` strings in the transcript. This is the *point* of the feature. Mitigation: documented, greppable, easy to filter or render.
- **Multi-injection on retry**: handled by clear-on-read on the gin context.
- **Tag-injection from description body**: handled by escaping `<` `>` `&` before wrapping.
- **Per-transport drift**: eight patches mean eight places to keep in sync. Mitigation: every handler calls the same `BuildVisionDescriptionPrefix` helper and the same `consumeVisionDescriptions` accessor; the only handler-specific code is "find first text payload and prepend".
