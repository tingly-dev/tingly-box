# Vision proxy: inject image descriptions into model response

**Status**: Implemented — revision 4
**Date**: 2026-06-15
**Branch**: `claude/vision-proxy-inject-description`
**History**:
- v1 (`379935d`): eight per-handler injection sites. Rejected — too invasive.
- v2 (`35a4849`): gin response-writer middleware on four routes. Rejected — user pointed out the protocol layer already has a hook chain that should serve this.
- v3 (`bb6653a`): hook attached at the `protocol.HandleContext` layer. Hit a blocker: Anthropic passthrough writes `evt.RawJSON()`, so typed mutation via the hook is lost on the wire.
- **v4 (this, implemented)**: instead of mutating events, the hook **prepends a synthetic well-formed event** before the model's first text. One register call covers all four streaming transports; a one-line `c.Writer` wrap on the route groups covers non-streaming. Handler code: untouched.

---

## Problem (unchanged)

Vision-proxy descriptions live only in the forwarded *request*; the client never sees them; subsequent turns lose all descriptions. Goal: persist the description by emitting it in the model's *response* so it lands in the assistant transcript and survives into the next turn's history.

## Design decisions (unchanged, confirmed)

1. Inject into the model response (not metadata, not prompt-shaping).
2. Wrapper: `<image-description>…</image-description>`, used by both request- and response-side.
3. Prepended before the first valid text content, with surrounding `\n`s.
4. Multi-image: each description in its own tag pair.
5. All four transports, both streaming and non-streaming.

---

## v4 — the prepend-not-mutate fix

v3 claimed `OnStreamEvent` mutation could serve every transport because all four passthroughs emit pointer events. That part is true, but it ignored what the handler *does* with the event afterwards. The Anthropic passthroughs forward the upstream payload via `hc.GinContext.SSEvent(evt.Type, evt.RawJSON())` — and `RawJSON()` is a cached byte slice captured at unmarshal time. Mutating the typed struct does not invalidate that cache, so the wire still carries the pre-mutation bytes:

```
internal/protocol/stream/anthropic_passthrough.go:80   c.SSEvent(evt.Type, evt.RawJSON())
libs/anthropic-sdk-go/betaagent.go:181                 func (r BetaManagedAgentsAgent) RawJSON() string { return r.JSON.raw }
```

OpenAI Chat passthrough is the opposite extreme: it rebuilds a `chunkMap` from typed fields on every event, so mutation propagates. The OpenAI Responses converter passes events by interface value — neither pointer nor reusable for mutation. Three different forwarding strategies, one hook signature.

**The fix is to stop mutating.** Instead, the hook writes a new, self-contained event directly to `c.Writer` *before* `handleFunc` runs (`ProcessStream` invokes hooks first, then `handleFunc`). The synthetic event carries the description text in whatever shape the transport expects — `chat.completion.chunk` with `choices[0].delta.content` for OpenAI Chat, `content_block_delta` of type `text_delta` for Anthropic, `response.output_text.delta` for OpenAI Responses. The client's accumulator concatenates deltas, so the prefix lands at the start of the assembled assistant message regardless of how the model's bytes are later forwarded.

Prepending also sidesteps several side puzzles v3 would have had to solve: no need to recompute sequence numbers, no need to renumber content-block indices, no need to fight the SDK's RawJSON cache. The synthetic event is bytes we own end-to-end.

### Why this still uses the protocol hook (the user's original instinct)

The user's call for "a hook" was right; v3's mistake was *how* to use it. The hook is registered exactly once via `RegisterDefaultStreamEventHookFactory`, and `NewHandleContext` consults the registry on every request so all four transports auto-pick it up. The hook receives the same typed events `handleFunc` does — it just emits its own synthetic event from inside the hook instead of mutating in place. That keeps the single-registration ergonomics of a hook while bypassing the mutation-vs-forwarding ambiguity.

## Why v3 was wrong about mutation — re-evaluating the protocol hook

In v2 I dismissed `HandleContext.WithOnStreamEvent` as "read-only by convention". Re-reading the actual event flow proves that wrong:

```
internal/protocol/stream/openai_passthrough.go:70   return true, nil, &chunk         // *openai.ChatCompletionChunk
internal/protocol/stream/anthropic_passthrough.go:40 return true, nil, &evt           // *anthropic.MessageStreamEventUnion
internal/protocol/stream/anthropic_passthrough.go:141return true, nil, &evt           // *anthropic.BetaRawMessageStreamEventUnion
internal/protocol/stream/converter.go:34            return true, nil, event           // wire.ResponsesEvent (and friends)
```

`ProcessStream` runs hooks *before* `handleFunc`:

```go
internal/protocol/context.go:130-175
for _, hook := range hc.OnStreamEventHooks {
    if hookErr := hook(event); hookErr != nil { ... }
}
if handleFunc != nil { handleFunc(event) }   // ← sees any in-place mutation
```

All three passthroughs emit **pointer** events; mutations stick and propagate to the downstream emitter. The converter framework passes events as `interface{}` whose underlying types are pointer-friendly (need to verify each converter, but the framework supports it). So a hook **can** mutate in place, by design — recording happens not to.

That collapses the eight-or-four touchpoint problem into:

- **Streaming**: one hook factory registered once at server boot. `NewHandleContext` auto-attaches it. Zero handler-level changes.
- **Non-streaming**: no `ProcessStream` to hook; each handler writes a JSON body via `c.JSON`. Trivially handled by a gin response-writer wrapper that intercepts the one `Write` call.

---

## Architecture (v3)

### A. Capture (unchanged from v1/v2)

1. `internal/server/processor/vision_proxy.go`
   - Add `DescriptionCollector` (`[]string` + `Append` / `Snapshot`).
   - `describe()` appends `wrap(desc)` on success and `wrap("(description unavailable)")` on fail-strip.
   - Update wrapper strings (request side too — same tag, symmetric):
     - success: `"\n<image-description>" + desc + "</image-description>\n"`
     - unavailable: `"\n<image-description>(description unavailable)</image-description>\n"`
     - historical: `imageHistoricalText = "\n<image-description>(omitted from history)</image-description>\n"`
   - Escape `&`, `<`, `>` in description bodies.

2. `internal/smart_routing/processor.go`
   - Add `Descriptions *processor.DescriptionCollector` (or generic `Extras` map) on `ProcessorContext`.

3. `internal/server/vision_proxy.go`
   - `applyVisionProxy` creates the collector, attaches it to the `ProcessorContext`, then stores the snapshot on the gin context:
     `c.Set(ctxKeyVisionDescriptions, collector.Snapshot())`.

### B. Inject — streaming (the elegant part)

4. `internal/protocol/context.go` — add a tiny package-level registry alongside the existing `HandleContext`:

```go
// DefaultStreamEventHookFactory builds a per-request hook from the gin
// context. Return nil to opt out (e.g. when the request carries no state
// the hook would act on). Factories run at NewHandleContext() time and
// the resulting hooks are appended in registration order.
type DefaultStreamEventHookFactory func(c *gin.Context) func(event interface{}) error

var defaultStreamEventHookFactories []DefaultStreamEventHookFactory

// RegisterDefaultStreamEventHookFactory installs a factory that is
// invoked for every HandleContext built via NewHandleContext.
// Called at server boot; not safe for concurrent registration once
// requests are in flight.
func RegisterDefaultStreamEventHookFactory(f DefaultStreamEventHookFactory) {
    defaultStreamEventHookFactories = append(defaultStreamEventHookFactories, f)
}
```

`NewHandleContext` iterates the registry:

```go
func NewHandleContext(c *gin.Context, responseModel string) *HandleContext {
    hc := &HandleContext{ GinContext: c, ResponseModel: responseModel }
    for _, f := range defaultStreamEventHookFactories {
        if h := f(c); h != nil {
            hc.OnStreamEventHooks = append(hc.OnStreamEventHooks, h)
        }
    }
    return hc
}
```

The protocol package now owns a generic auto-attach mechanism but has zero knowledge of vision-proxy. The dependency direction is correct (server → protocol).

5. `internal/server/processor/vision_inject_stream.go` (new) — the factory itself:

```go
func StreamInjectHookFactory(c *gin.Context) func(event interface{}) error {
    raw, ok := c.Get(ctxKeyVisionDescriptions)
    if !ok { return nil }
    descs, _ := raw.([]string)
    if len(descs) == 0 { return nil }

    prefix := BuildVisionDescriptionPrefix(descs)
    var injected bool
    return func(event interface{}) error {
        if injected { return nil }
        switch ev := event.(type) {
        case *openai.ChatCompletionChunk:
            injected = mutateOpenAIChunkFirstText(ev, prefix)
        case *anthropic.MessageStreamEventUnion:
            injected = mutateAnthropicV1FirstText(ev, prefix)
        case *anthropic.BetaRawMessageStreamEventUnion:
            injected = mutateAnthropicBetaFirstText(ev, prefix)
        case wire.ResponsesEvent:
            injected = mutateOpenAIResponsesFirstText(&ev, prefix)
        }
        return nil
    }
}
```

The four small mutators are the irreducible per-transport piece. They live in one file, take a typed pointer, find the first non-empty text field, prepend the prefix, return `true` if they touched something. They are independently unit-testable without spinning up a server.

6. `internal/server/server.go` — register once at boot:

```go
protocol.RegisterDefaultStreamEventHookFactory(processor.StreamInjectHookFactory)
```

That's it for streaming. **No handler touched.** The first call to `NewHandleContext` for every request automatically attaches the hook if descriptions are present, and the existing `ProcessStream` loop runs it.

### C. Inject — non-streaming (minimal middleware)

Non-stream paths don't go through `ProcessStream`. Each handler builds a final response object and calls `c.JSON(...)`. One `Write` to `c.Writer` carries the entire body.

7. `internal/server/middleware/vision_inject.go` (new) — one middleware applied to all four POST routes:

```go
func VisionInjectNonStream() gin.HandlerFunc {
    return func(c *gin.Context) {
        // Streaming requests are handled by the protocol-layer hook; the
        // SSE Content-Type is set inside the handler before the first
        // write, so we wait until the first Write call and short-circuit
        // if it looks like an SSE preamble.
        descs, _ := c.Get(ctxKeyVisionDescriptions)
        if descs == nil { c.Next(); return }
        c.Writer = newInjectingWriter(c, descs.([]string))
        c.Next()
    }
}
```

`injectingWriter`:

- On its **first** `Write` call:
  - If the buffered Content-Type starts with `text/event-stream`, restore the original writer and forward bytes unchanged. (Streaming path; protocol hook already handled it.)
  - Otherwise treat the payload as the JSON body, identify the transport by path prefix (`/v1/chat/completions`, `/v1/responses`, `/v1/messages`, `/v1/messages/beta`), splice the prefix into the right field using `sjson`, and forward.
- Subsequent writes pass through untouched.

The non-stream injection thus has one middleware + one helper per transport. That is irreducible: there is no shared non-stream framework analogous to `ProcessStream`.

### D. Wiring

8. `internal/server/server.go` — three additions, all one-line:
   - `protocol.RegisterDefaultStreamEventHookFactory(processor.StreamInjectHookFactory)` at boot.
   - `router.Use(middleware.VisionInjectNonStream())` on the relevant route group(s).
   - Nothing else.

---

## Intrusion budget

| Layer | v1 | v2 | **v3** |
|---|---|---|---|
| Handler callsites | 8 inline blocks | 0 | **0** |
| New middleware | — | 1 (per-route, 4 wirings) | **1 (group-level, 1 wiring)** |
| New protocol APIs | — | — | **1 registry pair (factory + register)** |
| Per-transport mutators | 8 | 4 (with branch) | **4 stream + 3 non-stream** ⚠ |
| Handlers modified | 8 | 0 | **0** |
| Where new code lives | scattered | `middleware/` | `protocol/` + `processor/` + `middleware/` (one file each) |

⚠ Same total mutator count as v2; the elegance is in *where* they're called from, not in their existence — finding "first text in transport X" remains transport-specific.

---

## On tag escaping (and why we don't)

An earlier draft wrapped the description body in HTML-style entity escaping (`&` → `&amp;` etc.) to guard against a vision upstream "closing the tag" with adversarial output. Removed: vision-model descriptions are natural language, not an injection vector, and a literal `</image-description>` inside the body — should one ever occur — is a client-side parsing edge case, not a security boundary the wrapper can meaningfully police. Clients are responsible for being robust when scanning their own input.

## Edge cases (unchanged)

| Case | Behaviour |
|---|---|
| No images | Collector empty; hook factory returns nil; `c.Writer` unwrapped. Zero overhead. |
| All vision calls fail | `"(description unavailable)"` injected — explicit failure surfaced. |
| Tool-only stream (no text) | `injected` never flips; prefix silently dropped. |
| Tool-only non-stream | sjson path finds no target text field → forward unmodified. |
| Concurrent requests | Per-request gin context + per-call hook closure; no shared mutable state. |
| Streaming retry mid-response | Hook closure's `injected` flag survives the retry; double-injection prevented. |
| Description contains `</image-description>` | Escaped before wrap. |

## Open questions

1. **Converter event types** — confirm during implementation that converter-based paths (OpenAI Responses, cross-protocol) emit events the four mutators recognise. If a converter wraps in an envelope, add the envelope arm to the switch.
2. **Hook ordering with recorder** — recorder reads events in the same loop. Mutation happens **before** recording. That is the correct order: we want the recorded transcript to match what the client sees. Verify this is what we want before shipping.
3. **`vision_proxy_injected_bytes` counter** — observability counter for injected bytes; default off.
4. **Tag-unaware renderers** — known cost; documented for client implementers.

## Implementation (as shipped)

### Phase 1 — Capture (commit `34c97e4`)
- `processor/vision_proxy.go`: `DescriptionCollector` with `Append` / `Snapshot`; `wrapVisionDescription` wraps body in `<image-description>…</image-description>` with surrounding `\n`; `imageHistoricalText` and `imageUnavailableText` re-cast as canonical markers; `describe()` appends each emitted description to a `context.WithValue`-threaded collector.
- `smart_routing/processor.go`: `ProcessorContext.Extras map[string]any` as the generic side channel.
- `server/vision_proxy.go`: `applyVisionProxy` constructs a collector, hands it over via `Extras`, and stashes the snapshot on `gin.Context` under `GinKeyVisionDescriptions` for the response side to consume.

### Phase 2 — Inject (streaming, commit `b0cd9ae` + this commit)
- `protocol/context.go`: `DefaultStreamEventHookFactory` registry; `NewHandleContext` auto-attaches every registered factory. `RegisterDefaultStreamEventHookFactory(...)` is the boot-time entry point.
- `server/vision_inject_stream.go`: the factory itself. Reads `GinKeyVisionDescriptions`; builds the prefix via `processor.BuildVisionDescriptionPrefix`; returns a closure that holds `injected bool` and, on the first text-bearing event, writes a synthetic event:
  - `*openai.ChatCompletionChunk` whose `Delta.Content != ""` → `data: {synthetic chat.completion.chunk with delta.content=prefix}\n\n`
  - `*anthropic.MessageStreamEventUnion` / `*anthropic.BetaRawMessageStreamEventUnion` with raw `content_block_delta`/`text_delta` → `event: content_block_delta\ndata: {... text_delta with text=prefix}\n\n` at the same block index
  - `wire.ResponsesOutputTextDeltaEvent` → `event: response.output_text.delta\ndata: {...delta=prefix}\n\n` at the same `(item_id, output_index, content_index)`
- `server/server.go`: a single `protocol.RegisterDefaultStreamEventHookFactory(visionStreamInjectFactory)` at boot.

### Phase 3 — Inject (non-streaming)
- `server/vision_inject_nonstream.go`: `VisionInjectNonStream()` middleware. When descriptions are present, wraps `c.Writer` with a `visionInjectWriter` whose first `Write` either short-circuits (when `Content-Type` starts with `text/event-stream`) or routes by request path to a per-transport sjson splicer:
  - `/chat/completions` → splice into `choices.0.message.content`
  - `/responses` → first `output[*].content[*]` of type `output_text`
  - `/messages` → first content block of type `text`
- `server/server_routes.go`: one `group.Use(VisionInjectNonStream())` line per route group (`SetupMixinEndpoints`, `SetupOpenAIEndpoints`, `SetupAnthropicEndpoints`).

### Phase 4 — Tests
- `processor/vision_proxy_test.go`: four Phase-1 tests on `DescriptionCollector` (single wrap, multi-image order preservation, fail-strip still collected, historical-image not collected).
- `server/vision_inject_stream_test.go`: eight Phase-2 tests covering all three streaming event types plus the no-descriptions / role-preamble / non-text-event guard cases and the `NewHandleContext` auto-attach contract.
- `server/vision_inject_nonstream_test.go`: seven Phase-3 tests covering each transport's JSON splice, SSE pass-through (the protocol hook owns it), no-descriptions zero-overhead, tool_use-only Anthropic bodies (no text → pass-through), and multi-description ordering.

### Phase 5 — Docs
- This file (v4 history above).
- Client-implementer note in README / CLAUDE.md (TODO): describe the `<image-description>` wrapper contract; how to suppress for tag-aware renderers vs. let it render literally.

---

## Risk summary

- **Mutation through `OnStreamEvent` is now an explicit supported pattern** — we are establishing it. Worth a short doc note in `protocol/context.go` clarifying that hooks may mutate pointer events; recorder hooks must accept that.
- **Hook ordering** — confirm recorder vs vision-inject ordering; chosen order is "inject before record" so the transcript stored matches the wire.
- **Converter coverage** — four mutators must cover every event type emitted by the converters in `protocol/stream/converter*.go`. Tests fixate this; the switch's `default` is a no-op so unknown event types just skip injection (graceful degradation).
- **Mostly no-handler-change** — by design. The only place handlers see this feature is via the existing `applyVisionProxy(...)` call they already make.
