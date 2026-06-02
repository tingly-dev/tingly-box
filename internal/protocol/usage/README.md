# internal/protocol/usage

Centralized token extraction and normalization for all supported provider protocols.
Every handler calls into this package instead of re-implementing provider rules inline.

---

## Why normalization matters

The `TokenUsage` struct (`InputTokens`, `OutputTokens`, `CacheInputTokens`) is used for
cost accounting and cache-hit ratio display. The front-end formula is:

```
cache_hit_ratio = CacheInputTokens / (InputTokens + CacheInputTokens)
```

Different providers report tokens with incompatible semantics, so we normalize to a
consistent internal representation before populating `TokenUsage`.

---

## Provider normalization rules

### OpenAI — Chat Completions & Responses API

| Wire field | Meaning |
|---|---|
| `prompt_tokens` / `input_tokens` | **Total** prompt tokens (cached + uncached) |
| `prompt_tokens_details.cached_tokens` / `input_tokens_details.cached_tokens` | Cached subset (≤ total) |
| `completion_tokens` / `output_tokens` | Output tokens |
| `completion_tokens_details.reasoning_tokens` / `output_tokens_details.reasoning_tokens` | Reasoning subset of output |

**Normalization:**
```
InputTokens     = prompt_tokens - cached_tokens   (uncached only)
CacheInputTokens = cached_tokens
OutputTokens    = completion_tokens
ReasoningTokens = reasoning_tokens
```

Cache hit ratio: `cached / (uncached + cached)` = `cached / prompt_tokens` ✓

### Anthropic — v1 & v1 Beta

| Wire field | Meaning |
|---|---|
| `input_tokens` | **Uncached** prompt tokens only |
| `cache_creation_input_tokens` | Tokens written to cache (priced as write cost) |
| `cache_read_input_tokens` | Tokens read from cache |
| `output_tokens` | Output tokens |

**Normalization:**
```
InputTokens     = input_tokens + cache_creation_input_tokens   (uncached + write cost)
CacheInputTokens = cache_read_input_tokens
OutputTokens    = output_tokens
```

Cache hit ratio: `cache_read / (uncached + creation + cache_read)` ✓

> **Why add `cache_creation`?** Creation tokens are billable at a write-cost rate, so
> they belong in the denominator to correctly represent total prompt spend. Omitting
> them would inflate the apparent hit rate.

### Anthropic Streaming — event split

Anthropic streams usage across two events:

| Event | Fields present |
|---|---|
| `message_start` | `input_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens` |
| `message_delta` | `output_tokens` (occasionally `input_tokens` for non-standard providers) |

`AnthropicAccumulator` handles this split, the priority fallback, and the
normalization transparently.

### Google GenAI

Google usage is extracted and normalized inline in `stream/google_to_any.go` and
`nonstream/any_to_google.go`. The Google protocol uses `promptTokenCount` /
`candidatesTokenCount` with no cache sub-fields exposed in the SDK, so no
centralized extractor is provided here.

---

## API

### Non-streaming (stateless, pure functions)

```go
import "github.com/tingly-dev/tingly-box/internal/protocol/usage"

// OpenAI Chat Completions
tu := usage.FromOpenAIChatCompletion(resp.Usage)

// OpenAI Responses API
tu := usage.FromOpenAIResponses(resp.Usage)

// Anthropic v1 non-beta message
tu := usage.FromAnthropicMessage(resp.Usage)

// Anthropic v1 beta message
tu := usage.FromAnthropicBetaMessage(resp.Usage)
```

Each function returns `*protocol.TokenUsage` with fields already normalized.

### Streaming — Anthropic accumulator

```go
acc := usage.NewAnthropicAccumulator()

// In the event loop:
acc.Consume(&evt)      // non-beta: MessageStreamEventUnion
acc.ConsumeBeta(&evt)  // beta:     BetaRawMessageStreamEventUnion

// At return points:
if acc.HasUsage() {
    return acc.Result(), nil
}
return protocol.ZeroTokenUsage(), nil
```

`Result()` returns `*protocol.TokenUsage` with all normalization applied.
`HasUsage()` is false if no usage-carrying events were seen (use `ZeroTokenUsage()` as
the default).

---

## What is NOT covered here

| Handler | Why inline extraction remains |
|---|---|
| `stream/openai_passthrough.go` | Streams accumulate per-chunk; also drives estimated usage injection when provider omits usage |
| `stream/openai_to_anthropic*.go` | Uses `StreamTokenCounter` from `protocol/token` (incremental counting, not extraction) |
| `stream/openai_chat_to_responses.go` | `state` struct fields are dual-use: also build the wire response body |
| `stream/openai_responses_to_chat.go` | Same dual-use pattern |
| `stream/google_to_any.go` | Google-specific schema with no SDK usage struct |
| `nonstream/anthropic_to_openai.go` | Returns `map[string]interface{}` wire format, not `*TokenUsage` |
| `nonstream/openai_to_anthropic.go` | Same — wire format conversion only |

These files normalize correctly inline; they are just not candidates for the shared
extractors because their token variables serve double duty as wire response fields.
