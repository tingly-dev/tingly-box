# Vision proxy: inject image descriptions into model response

**Status**: Proposed — revised
**Date**: 2026-06-15
**Branch**: `claude/vision-proxy-inject-description`
**Supersedes**: original revision (commit `379935d`) — replaced after a hook-feasibility audit.

---

## Problem

Today's vision proxy replaces image content parts in the **forwarded request** with a wrapper text containing the vision-upstream description. The client never sees this text. Consequences:

1. The description never lands in the conversation transcript on the client side.
2. On the next turn, the client re-sends the original `image_url` in history; historical-strip replaces it with a fixed marker — **all prior descriptions are lost forever**.
3. Even ignoring cache concerns, future turns lose the ability to reason about previous images.

Goal: preserve descriptions by **emitting them as part of the model's response** so the assistant transcript carries them naturally into subsequent turns.

---

## Design decisions (confirmed with user)

1. **Approach** — inject description text into the model's response.
2. **Wrapper** — `<image-description>…</image-description>`, used by both the request-side replacement *and* the response-side injection (symmetric, greppable).
3. **Position** — prepended before the first valid text content of the model response, with surrounding `\n`s for visual separation.
4. **Multi-image** — each description wrapped in its own pair of tags.
5. **Scope** — all four transports (OpenAI Chat, OpenAI Responses, Anthropic V1, Anthropic Beta), both streaming and non-streaming.

---

## Architectural pivot: response-writer middleware instead of per-handler patches

The first revision proposed eight per-handler injection points (4 transports × 2 modes). User feedback: too invasive on the per-transport handlers — *"more elegant via a hook?"*.

### What the codebase actually offers (hook audit, ranked)

| Mechanism | Where | Read or Mutate? | Transport scope | Verdict |
|---|---|---|---|---|
| `HandleContext.WithOnStreamEvent` | `internal/protocol/context.go:92-97` | Read-only by convention (`func(interface{}) error`, no return-event channel). Mutation works only if event is an addressable pointer — implicit and brittle. | Streaming, all four transports | Not safe for mutation. Use rejected. |
| `responseBodyWriter` (gin.ResponseWriter wrapper) | `internal/server/middleware/utils.go:10-18` | Mutates write stream (today only tees for logging, but the wrapper shape is exactly what we want). | All responses, all transports, all routes | **Recommended.** Centralises the intrusion to one place. |
| `protocol/stream/*MCPHooks.OnToolCallsFinal` | `internal/protocol/stream/{anthropic,openai}_to_*.go` | Mutate, but scoped to tool calls only. | Per-format | Not relevant. |
| Smart-routing `OpProcessor` (where vision proxy lives today) | `internal/smart_routing/processor.go:10-65` | Request-only — `ProcessorContext` has no `Response` field. | Request-only | Not applicable. |
| Guardrails non-stream response funcs | `internal/server/guardrails_runtime.go:361-407` | Mutate non-stream response after assembly. | Non-stream only, per-format, called from `protocol_dispatch.go` | Per-format wiring — not the central chokepoint. |

### Chosen approach: an injecting `gin.ResponseWriter` wrapper, installed once per route

A route-level middleware wraps `c.Writer` with a small, transport-aware injector. Wrappers are installed at the existing route registration sites; handlers themselves are untouched.

```text
                      ┌──────────────────────────────────────┐
                      │ Server.RegisterRoutes()              │
                      │                                      │
                      │  router.POST("/v1/chat/completions", │
                      │      VisionInject(openaiChatHook),   │  ← one line per route
                      │      s.HandleOpenAIChat)             │
                      │  router.POST("/v1/responses", ...)   │
                      │  router.POST("/v1/messages", ...)    │
                      │  router.POST("/v1/messages/beta",...)│
                      └──────────────────────────────────────┘
                                       │
                                       ▼
                    ┌────────────────────────────────────────┐
                    │ VisionInject middleware                │
                    │                                        │
                    │  if no descriptions on c → c.Next()    │
                    │  else c.Writer = &injectingWriter{...} │
                    │                                        │
                    │  c.Next() — handler runs, writes via   │
                    │  wrapped writer; first text payload    │
                    │  gets prefix prepended.                │
                    └────────────────────────────────────────┘
                                       │
                                       ▼
                    ┌────────────────────────────────────────┐
                    │ injectingWriter.Write(b)               │
                    │                                        │
                    │  if injected → forward(b)              │
                    │  else        → hook(b) → ok?           │
                    │                  yes  → forward(b')    │
                    │                  no   → forward(b)     │
                    └────────────────────────────────────────┘
```

`hook(b []byte) (out []byte, injected bool)` is the only per-transport piece. It receives a write payload — for SSE that's `event: ...\ndata: {...}\n\n`; for non-stream that's the full JSON body — and returns the same bytes with the prefix spliced into the first text payload (using `tidwall/sjson`).

### Why this beats eight inline patches

| Per-handler patches (v1)                          | Writer middleware (v2)                                      |
|---------------------------------------------------|-------------------------------------------------------------|
| 8 separate `find first text and prepend` callsites| 4 small `hook` functions, one per transport                 |
| Handler logic interleaved with injection logic    | Handlers untouched; injection lives in `middleware/`        |
| Each retry path needs its own clearing of state   | One `injected bool` per wrapped writer, reset by middleware |
| Hard to disable globally (no opt-out)             | Middleware short-circuits via context flag if no descriptions present |

### Why not pure gin-layer middleware with no per-transport hook?

A truly transport-agnostic wrapper would need to recognise "this slice of bytes is the first model-text payload" without knowing the envelope. SSE/JSON shapes differ enough that one parser doesn't work. The per-transport `hook` function is the irreducible complexity; it just needs **one** owner (the middleware), not eight.

---

## Architecture

### A. Capture (request side)

Identical to v1 — collect descriptions during the existing vision-proxy pass.

1. `internal/server/processor/vision_proxy.go`
   - Add `DescriptionCollector` (a `[]string` with `Append` / `Snapshot`). Constructed by the caller, attached to `ProcessorContext`.
   - `describe()` calls `pctx.Descriptions.Append(wrapped)` on every successful describe AND every fail-strip.
   - Update wrapper strings:
     - success: `"\n<image-description>" + escape(desc) + "</image-description>\n"`
     - unavailable: `"\n<image-description>(description unavailable)</image-description>\n"`
     - historical: `"\n<image-description>(omitted from history)</image-description>\n"`
   - Escape `<`, `>`, `&` in description bodies.

2. `internal/server/vision_proxy.go`
   - `applyVisionProxy` instantiates a fresh `DescriptionCollector`, attaches it to the `ProcessorContext`, then transfers the snapshot to `gin.Context` under `ctxKeyVisionDescriptions`.

### B. Inject (response side, single middleware)

3. `internal/server/middleware/vision_inject.go` (new)
   - `VisionInject(hook InjectHook) gin.HandlerFunc` — returns a middleware that:
     - Reads `gin.Context.Get(ctxKeyVisionDescriptions)` once at entry.
     - If empty or absent → `c.Next()` and return.
     - Else builds the prefix via `BuildVisionDescriptionPrefix(descs)`, wraps `c.Writer` with `&injectingWriter{prefix, hook, false}`, `c.Next()`.
   - `injectingWriter` implements `gin.ResponseWriter` (mostly by embedding). The first `Write` call goes through the hook; subsequent calls are pass-through.
   - `type InjectHook func(prefix string, payload []byte) (out []byte, didInject bool)` — payload-level mutator.

4. `internal/server/middleware/vision_inject_hooks.go` (new)
   - Four small functions, all of shape `InjectHook`:
     - `OpenAIChatStreamHook` — match SSE `data: {...}\n\n`, parse JSON, look for first non-empty `choices.0.delta.content` (or stream-end `choices.0.message.content`), splice prefix in via `sjson.SetBytes`.
     - `OpenAIResponsesStreamHook` — match SSE; for `response.output_text.delta`, splice prefix into the `delta` field; for `response.output_item.added`, splice into the first text part.
     - `AnthropicStreamHook` (covers V1 and Beta — same event shape) — match SSE; for `content_block_delta` with `text_delta`, splice prefix into `delta.text`.
     - `NonStreamJSONHook` — single payload contains the entire body; transport identified by path or content type. Three sub-cases:
       - OpenAI Chat → splice into `choices.0.message.content`.
       - OpenAI Responses → splice into the first `output.*.content.*.text` of type `output_text`.
       - Anthropic V1/Beta → splice into the first text block's `text`.

   In practice the stream hooks are unified by content-type sniff: if the wrapped writer sees `text/event-stream` content-type in headers, it dispatches to the stream variant; else non-stream. So we may collapse to one `InjectHook` per transport (stream + non-stream share the same hook, which branches internally) — that drops the count from "4 hooks × 2 modes" to "4 hooks total".

5. Route wiring — one extra arg per route in `internal/server/server.go` / wherever the four POST routes are registered. Single-line change per route, no logic inside the handler.

### C. Tag escaping

Vision-upstream descriptions are untrusted text. Before wrapping we replace `&`, `<`, `>` with their HTML entities. The decoder side is opt-in; clients that ignore the tag don't need to decode. This prevents a description containing `</image-description>` from prematurely closing the tag in clients that do parse it.

---

## Streaming subtlety: "first text" identification

Not "first SSE event" — the first event is usually a role / start marker without text. The hook returns `(out, didInject=false)` for non-text events and the wrapper forwards the original bytes, leaving `injected = false` so the next event still gets a chance. Once the hook returns `didInject = true` the wrapper flips its flag and all subsequent writes pass through untouched.

For non-streaming, the entire response body lands in one `Write` call (Gin's `c.JSON`), so the hook always succeeds on first attempt or never.

---

## Edge cases

| Case | Behaviour |
|---|---|
| No images in request | Collector empty; middleware sees empty slice; `c.Writer` unwrapped; zero overhead. |
| Every vision call fails | `"(description unavailable)"` injected, surfaced to client — explicit failure record. |
| Streaming response has no text (tool-only) | `didInject` never becomes true; prefix silently dropped. Acceptable: the input still saw the descriptions, and there is no visible text turn to anchor a prefix to. |
| Concurrent requests | Collector on per-request `gin.Context`; wrapper state on per-request `injectingWriter`; no cross-request state. |
| Streaming retry mid-response | `injected = true` is local to the wrapped writer. A retry inside the same request reuses the same wrapper, so the prefix only fires once on the visible byte stream. (Need to verify by reading `dispatchWithPriorityFailover` — see open question.) |
| Description body contains `</image-description>` | Escaped to `&lt;/image-description&gt;` before wrapping. |
| Client uses HTTP/2 push or other transports | Wrapper sits at `gin.ResponseWriter` layer; any writer Gin uses goes through the same interface. |

---

## Open questions

1. **Streaming retries** — the existing dispatch loop may reissue mid-stream after partial bytes were already written. Need to confirm that the second attempt's bytes flow through the same `injectingWriter` (they should, since `c.Writer` is replaced once at middleware time and never restored). The local `injected` flag should remain `true` across the retry boundary, preventing double-injection.
2. **Token / usage accounting** — injected bytes are not produced by the upstream model. Today no usage adjustment is made; we keep it that way. Add an optional `vision_proxy_injected_bytes` observability counter (`logrus.WithField`).
3. **Tag-unaware renderers** — CLI tools rendering raw text will see `<image-description>…</image-description>` literally. Acceptable for v1; document in a follow-up to `.design/vision-proxy-scenario.md` so client implementers know to filter or render.
4. **Content-type sniff vs. route-level dispatch** — should each route register a transport-specific `InjectHook` (cleaner, explicit) or should the middleware sniff `Content-Type` header inside the wrapper (one registration, more magic)? Leaning towards explicit per-route registration since the four hooks need to exist anyway; route binding makes the intent obvious.

---

## Implementation plan

### Phase 1 — Capture

1. `internal/server/processor/vision_proxy.go`
   - Add `DescriptionCollector` type.
   - Adapt `describe()` and `imageHistoricalText` to the new wrapper format with escaping.
   - Threading: `Process(pctx)` exposes `pctx.Descriptions` (new field on `smartrouting.ProcessorContext`).

2. `internal/smart_routing/processor.go`
   - Add `Descriptions *DescriptionCollector` (or generic `Extras` map) on `ProcessorContext`. Smallest-surface change preferred.

3. `internal/server/vision_proxy.go`
   - `applyVisionProxy` creates a collector, attaches, then on return stores the snapshot via `c.Set(ctxKeyVisionDescriptions, descs)`.

4. Helper:
   - `internal/server/processor/vision_inject_text.go` (new): `BuildVisionDescriptionPrefix(descs []string) string` pure function.

### Phase 2 — Inject (middleware + 4 hooks)

5. `internal/server/middleware/vision_inject.go` — `VisionInject(hook InjectHook) gin.HandlerFunc` and `injectingWriter`.

6. `internal/server/middleware/vision_inject_hooks.go` — four `InjectHook` implementations (stream + non-stream sharing per-transport functions; sjson-based payload mutation).

7. Route registration — add the middleware to the four POST routes; one line each.

### Phase 3 — Tests

8. `middleware/vision_inject_test.go` — middleware behaviour against a fake handler that emits bytes:
   - No descriptions → c.Writer untouched, byte-identical pass-through.
   - Single description, streaming SSE → first `data:` line has prefix injected; subsequent lines untouched.
   - Single description, non-stream JSON → JSON body has prefix in target field.
   - Multi-description → all stacked in order.
   - Tool-only stream → no injection, no error.
   - Description with `<` / `>` → escaped.
   - Marshal-and-grep regression: output regex `^(?:\n<image-description>[^<]*</image-description>\n)+\n` or empty.

9. End-to-end test (extend `openai_responses_vision_test.go` style) for each transport: send a request with image, run the full handler with the middleware wired, assert injected prefix appears in the response stream/body.

### Phase 4 — Docs

10. Update `.design/vision-proxy-scenario.md` to cross-link this doc; add a section to README / CLAUDE.md for client implementers explaining the `<image-description>` contract.

---

## Out of scope (deferred)

- Returning descriptions as structured metadata in a separate field (kept as a possible add-on).
- Caching descriptions across turns by content hash.
- Adding a system-prompt note instructing the model to use `<image-description>` content.
- Per-format injection toggle in scenario config (could be added later as an extension key).

---

## Risk summary

- **Client-visible output change** — yes, by design. Documented; greppable; suppressible by clients that decode the tag.
- **Per-transport injector functions still required** — irreducible. But isolated to one file (`middleware/vision_inject_hooks.go`) rather than scattered across 8 handler sites.
- **SSE byte-level parsing in the wrapper** — uses `sjson` (already a dependency), avoiding full JSON round-trips. The "match `data: ...\n\n`" framing is standard SSE and tested by the SDK.
- **Retry / failover behaviour** — needs a confirming read of `dispatchWithPriorityFailover` before Phase 2 lands; documented as Open Question 1.
