# internal/protocol/usage

Centralized token extraction and normalization. All handlers call into this package
instead of re-implementing provider rules inline.

---

## Normalization rules

Different providers report tokens with incompatible semantics. We normalize to a
consistent internal representation so the front-end cache-hit formula works correctly:

```
cache_hit_ratio = CacheInputTokens / (InputTokens + CacheInputTokens)
```

| Provider | Wire semantics | InputTokens stored as | CacheInputTokens stored as |
|---|---|---|---|
| **OpenAI Chat / Responses** | `prompt_tokens` = total (cached + uncached) | `prompt_tokens - cached_tokens` | `cached_tokens` |
| **Anthropic** | `input_tokens` = uncached only; `cache_creation_input_tokens` = write cost | `input_tokens + cache_creation_input_tokens` | `cache_read_input_tokens` |

> **Why add `cache_creation` to input?** Creation tokens are billable at a write-cost
> rate, so they belong in the denominator to correctly represent total prompt spend.

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
