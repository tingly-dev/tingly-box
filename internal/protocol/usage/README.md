# internal/protocol/usage

Centralized token extraction and normalization. All handlers call into this package
instead of re-implementing provider rules inline.

---

## Normalization rules

**The normalization table lives in [`.design/usage-tracking.md`](../../../.design/usage-tracking.md) §2**, together with the per-field containment rules, the cross-provider invariants, and the gpt-5.6 cache-write background (§12). It is deliberately not duplicated here — this file previously carried a second copy that went stale the moment cache writes landed.

The one-line version, for orientation:

```
InputTokens      = uncached + written   (OpenAI subtracts reads; Anthropic adds creation)
CacheInputTokens = cache reads
CacheWriteTokens = cache writes, a SUBSET of InputTokens — never add it on top
cache_hit_ratio  = CacheInputTokens / (InputTokens + CacheInputTokens)
```

### Anthropic streaming event split

Anthropic splits usage across two events:

- `message_start` → `input_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens`
- `message_delta` → `output_tokens` (some non-standard providers also send `input_tokens` here)

`AnthropicAccumulator` handles this split, priority fallback, and normalization transparently.

---

## API

### Non-streaming (pure functions)

```go
usage.FromOpenAIChatCompletion(resp.Usage)   // openai.CompletionUsage
usage.FromOpenAIResponses(resp.Usage)        // responses.ResponseUsage
usage.FromAnthropicMessage(resp.Usage)       // anthropic.Usage
usage.FromAnthropicBetaMessage(resp.Usage)   // anthropic.BetaUsage
```

### Streaming — Anthropic accumulator

```go
acc := usage.NewAnthropicAccumulator()

// In event loop:
acc.Consume(&evt)      // MessageStreamEventUnion (non-beta)
acc.ConsumeBeta(&evt)  // BetaRawMessageStreamEventUnion (beta)

// At return:
if acc.HasUsage() {
    return acc.Result(), nil
}
return protocol.ZeroTokenUsage(), nil
```

---

## Coverage

### `internal/protocol/nonstream/`

| Function | Extractor |
|---|---|
| `HandleOpenAIChatNonStream` | `FromOpenAIChatCompletion` |
| `HandleOpenAIResponsesNonStream` | `FromOpenAIResponses` |
| `HandleAnthropicV1NonStream` | `FromAnthropicMessage` |
| `HandleAnthropicV1BetaNonStream` | `FromAnthropicBetaMessage` |

### `internal/protocol/stream/`

| Function | Mechanism |
|---|---|
| `HandleAnthropic` | `AnthropicAccumulator.Consume` |
| `HandleAnthropicBeta` | `AnthropicAccumulator.ConsumeBeta` |
| `AnthropicToOpenAIStreamWithMCPHooks` | `AnthropicAccumulator.ConsumeBeta` |
| `HandleAnthropicBetaToOpenAIResponsesStream` | `AnthropicAccumulator.ConsumeBeta` |

### `internal/server/` (dispatch layer)

| Code site | Extractor |
|---|---|
| `protocol_dispatch` — Anthropic Beta non-stream (×2) | `FromAnthropicBetaMessage` |
| `protocol_dispatch` — Responses → Anthropic Beta | `FromAnthropicBetaMessage` |
| `protocol_dispatch` — OpenAI Chat non-stream (×2) | `FromOpenAIChatCompletion` |
| `protocol_dispatch` — OpenAI Responses non-stream (×2) | `FromOpenAIResponses` |
| `anthropic_message_v1` — Responses → Anthropic v1 | `FromOpenAIResponses` |
| `anthropic_message_beta` — Responses → Anthropic Beta | `FromOpenAIResponses` |

### Intentional inline extraction (not migrated)

| File | Reason |
|---|---|
| `stream/openai_passthrough.go` | Per-chunk accumulation + estimated usage injection fallback |
| `stream/openai_to_anthropic*.go` | Uses `StreamTokenCounter` (incremental counting, not extraction) |
| `stream/openai_{chat,responses}_to_*.go` | `state` fields are dual-use: also build the wire response body |
| `stream/google_to_any.go` | Google SDK has no structured cache sub-fields |
| `nonstream/anthropic_to_openai.go` | Returns wire format (`map[string]interface{}`), not `*TokenUsage` |
| `nonstream/openai_to_anthropic.go` | Same |
| `server/protocol_dispatch` — Google non-stream | Google schema, no cached tokens in SDK struct |
