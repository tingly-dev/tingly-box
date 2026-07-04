package usage

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tidwall/gjson"

	protocol "github.com/tingly-dev/tingly-box/ai"
)

// AnthropicAccumulator accumulates token usage across a streaming Anthropic
// response. The Anthropic protocol splits usage across two event types:
//   - message_start → input_tokens, cache_creation, cache_read
//   - message_delta → output_tokens (occasionally input for non-standard providers)
//
// Both non-beta (MessageStreamEventUnion) and beta
// (BetaRawMessageStreamEventUnion) streams are supported via Consume and
// ConsumeBeta respectively.
type AnthropicAccumulator struct {
	usage    *protocol.TokenUsage
	hasUsage bool
}

// NewAnthropicAccumulator returns a zeroed accumulator ready to consume events.
func NewAnthropicAccumulator() *AnthropicAccumulator {
	return &AnthropicAccumulator{usage: protocol.ZeroTokenUsage()}
}

// Consume updates the accumulator from a non-beta streaming event.
// It is safe to call on every event in the stream; only usage-carrying
// events (message_start, message_delta) have any effect.
func (a *AnthropicAccumulator) Consume(evt *anthropic.MessageStreamEventUnion) {
	raw := strings.Clone(evt.RawJSON())
	a.consumeRaw(
		evt.Message.Usage.InputTokens,
		gjson.Get(raw, "message.usage.input_tokens").Int(),
		evt.Usage.InputTokens,
		gjson.Get(raw, "usage.input_tokens").Int(),
		evt.Usage.OutputTokens,
		gjson.Get(raw, "usage.output_tokens").Int(),
		evt.Message.Usage.CacheReadInputTokens,
		evt.Usage.CacheReadInputTokens,
		gjson.Get(raw, "message.usage.cache_read_input_tokens").Int(),
		gjson.Get(raw, "usage.cache_read_input_tokens").Int(),
		evt.Message.Usage.CacheCreationInputTokens,
		evt.Usage.CacheCreationInputTokens,
	)
}

// ConsumeBeta updates the accumulator from a beta streaming event.
func (a *AnthropicAccumulator) ConsumeBeta(evt *anthropic.BetaRawMessageStreamEventUnion) {
	// Beta events follow the same protocol as non-beta; no gjson fallback
	// needed since BetaUsage fields are directly accessible.
	a.consumeRaw(
		evt.Message.Usage.InputTokens,
		0, // no gjson fallback for beta — SDK fields are reliable
		evt.Usage.InputTokens,
		0,
		evt.Usage.OutputTokens,
		0,
		evt.Message.Usage.CacheReadInputTokens,
		evt.Usage.CacheReadInputTokens,
		0,
		0,
		evt.Message.Usage.CacheCreationInputTokens,
		evt.Usage.CacheCreationInputTokens,
	)
}

// consumeRaw merges one event's usage fields into the accumulator.
// Parameters follow the priority order used in Anthropic streaming:
//   - Input: prefer message_start (msgStartInput / msgStartInputRaw), fall
//     back to message_delta (deltaInput / deltaInputRaw)
//   - Output: delta path only
//   - Cache read: prefer message_start, fall back to delta
//   - Cache creation: added to inputTokens (normalization)
func (a *AnthropicAccumulator) consumeRaw(
	msgStartInput, msgStartInputRaw int64,
	deltaInput, deltaInputRaw int64,
	deltaOutput, deltaOutputRaw int64,
	msgStartCacheRead, deltaCacheRead int64,
	msgStartCacheReadRaw, deltaCacheReadRaw int64,
	msgStartCacheCreation, deltaCacheCreation int64,
) {
	if a.usage == nil {
		a.usage = protocol.ZeroTokenUsage()
	}

	// Input tokens — prefer message_start, fall back to message_delta
	switch {
	case msgStartInput > 0:
		a.usage.InputTokens = int(msgStartInput)
		a.hasUsage = true
	case msgStartInputRaw > 0:
		a.usage.InputTokens = int(msgStartInputRaw)
		a.hasUsage = true
	case deltaInput > 0:
		a.usage.InputTokens = int(deltaInput)
		a.hasUsage = true
	case deltaInputRaw > 0:
		a.usage.InputTokens = int(deltaInputRaw)
		a.hasUsage = true
	}

	// Output tokens
	switch {
	case deltaOutput > 0:
		a.usage.OutputTokens = int(deltaOutput)
		a.hasUsage = true
	case deltaOutputRaw > 0:
		a.usage.OutputTokens = int(deltaOutputRaw)
		a.hasUsage = true
	}

	// Cache read tokens — stored separately as cache read details
	switch {
	case msgStartCacheRead > 0:
		a.usage.CacheReadTokens = int(msgStartCacheRead)
		a.hasUsage = true
	case deltaCacheRead > 0:
		a.usage.CacheReadTokens = int(deltaCacheRead)
		a.hasUsage = true
	case msgStartCacheReadRaw > 0:
		a.usage.CacheReadTokens = int(msgStartCacheReadRaw)
		a.hasUsage = true
	case deltaCacheReadRaw > 0:
		a.usage.CacheReadTokens = int(deltaCacheReadRaw)
		a.hasUsage = true
	}

	// Normalize: add cache_creation to inputTokens so denominator =
	// input (uncached) + creation (write cost). Cache reads stay in cache details.
	switch {
	case msgStartCacheCreation > 0:
		a.usage.CacheWriteTokens = int(msgStartCacheCreation)
		a.usage.InputTokens += int(msgStartCacheCreation)
	case deltaCacheCreation > 0:
		a.usage.CacheWriteTokens = int(deltaCacheCreation)
		a.usage.InputTokens += int(deltaCacheCreation)
	}
}

// Result returns the normalized TokenUsage built from accumulated events.
func (a *AnthropicAccumulator) Result() *protocol.TokenUsage {
	if a.usage == nil {
		return protocol.ZeroTokenUsage()
	}
	result := *a.usage
	result.CacheInputTokens = result.CacheReadTokens
	return &result
}

// HasUsage reports whether any non-zero usage was observed.
func (a *AnthropicAccumulator) HasUsage() bool {
	return a.hasUsage
}
