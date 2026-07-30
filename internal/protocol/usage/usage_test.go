package usage_test

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"

	protocol "github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol/usage"
)

// ---------------------------------------------------------------------------
// FromOpenAIChatCompletion
// ---------------------------------------------------------------------------

func TestFromOpenAIChatCompletion(t *testing.T) {
	tests := []struct {
		name           string
		prompt, cached int64
		completion     int64
		reasoning      int64
		wantInput      int
		wantOutput     int
		wantCache      int
		wantReasoning  int
	}{
		{
			name:   "90% cache hit",
			prompt: 1000, cached: 900, completion: 200,
			wantInput: 100, wantOutput: 200, wantCache: 900,
		},
		{
			name:   "no cache",
			prompt: 500, cached: 0, completion: 100,
			wantInput: 500, wantOutput: 100, wantCache: 0,
		},
		{
			name:   "with reasoning",
			prompt: 200, cached: 50, completion: 80, reasoning: 30,
			wantInput: 150, wantOutput: 80, wantCache: 50, wantReasoning: 30,
		},
		{
			name:      "all zero",
			wantInput: 0, wantOutput: 0, wantCache: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := openai.CompletionUsage{
				PromptTokens:     tc.prompt,
				CompletionTokens: tc.completion,
			}
			u.PromptTokensDetails.CachedTokens = tc.cached
			u.CompletionTokensDetails.ReasoningTokens = tc.reasoning

			got := usage.FromOpenAIChatCompletion(u)
			assert.Equal(t, tc.wantInput, got.InputTokens)
			assert.Equal(t, tc.wantOutput, got.OutputTokens)
			assert.Equal(t, tc.wantCache, got.CacheReadTokens)
			assert.Equal(t, tc.wantReasoning, got.ReasoningTokens)
		})
	}
}

// ---------------------------------------------------------------------------
// FromOpenAIResponses
// ---------------------------------------------------------------------------

func TestFromOpenAIResponses(t *testing.T) {
	tests := []struct {
		name          string
		input, cached int64
		output        int64
		reasoning     int64
		wantInput     int
		wantOutput    int
		wantCache     int
		wantReasoning int
	}{
		{
			name:  "partial cache",
			input: 500, cached: 200, output: 60,
			wantInput: 300, wantOutput: 60, wantCache: 200,
		},
		{
			name:  "full cache",
			input: 800, cached: 800, output: 40,
			wantInput: 0, wantOutput: 40, wantCache: 800,
		},
		{
			name:  "no cache with reasoning",
			input: 300, cached: 0, output: 50, reasoning: 20,
			wantInput: 300, wantOutput: 50, wantCache: 0, wantReasoning: 20,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := responses.ResponseUsage{
				InputTokens:  tc.input,
				OutputTokens: tc.output,
			}
			u.InputTokensDetails.CachedTokens = tc.cached
			u.OutputTokensDetails.ReasoningTokens = tc.reasoning

			got := usage.FromOpenAIResponses(u)
			assert.Equal(t, tc.wantInput, got.InputTokens)
			assert.Equal(t, tc.wantOutput, got.OutputTokens)
			assert.Equal(t, tc.wantCache, got.CacheReadTokens)
			assert.Equal(t, tc.wantReasoning, got.ReasoningTokens)
		})
	}
}

// ---------------------------------------------------------------------------
// FromAnthropicMessage / FromAnthropicBetaMessage
// ---------------------------------------------------------------------------

func TestFromAnthropicMessage(t *testing.T) {
	tests := []struct {
		name                  string
		input, creation, read int64
		output                int64
		wantInput, wantOutput int
		wantCache             int
	}{
		{
			name:  "cache read only",
			input: 100, creation: 0, read: 800, output: 50,
			wantInput: 100, wantOutput: 50, wantCache: 800,
		},
		{
			name:  "cache creation included in denominator",
			input: 100, creation: 900, read: 800, output: 50,
			wantInput: 1000, wantOutput: 50, wantCache: 800,
		},
		{
			name:  "no cache",
			input: 300, creation: 0, read: 0, output: 80,
			wantInput: 300, wantOutput: 80, wantCache: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := anthropic.Usage{
				InputTokens:              tc.input,
				OutputTokens:             tc.output,
				CacheCreationInputTokens: tc.creation,
				CacheReadInputTokens:     tc.read,
			}
			got := usage.FromAnthropicMessage(u)
			assert.Equal(t, tc.wantInput, got.InputTokens)
			assert.Equal(t, tc.wantOutput, got.OutputTokens)
			assert.Equal(t, tc.wantCache, got.CacheReadTokens)
		})
	}
}

func TestFromAnthropicBetaMessage(t *testing.T) {
	u := anthropic.BetaUsage{
		InputTokens:              100,
		OutputTokens:             50,
		CacheCreationInputTokens: 200,
		CacheReadInputTokens:     400,
	}
	got := usage.FromAnthropicBetaMessage(u)
	assert.Equal(t, 300, got.InputTokens) // 100 + 200
	assert.Equal(t, 50, got.OutputTokens)
	assert.Equal(t, 400, got.CacheReadTokens)
}

// ---------------------------------------------------------------------------
// AnthropicAccumulator — real streaming format
// ---------------------------------------------------------------------------

// fakeDecoder replays raw JSON events as Anthropic SSE events.
type fakeDecoder struct {
	events  []string
	current int
	next    int
}

func newFakeDecoder(events []string) *fakeDecoder {
	return &fakeDecoder{events: events, current: -1}
}

func (f *fakeDecoder) Next() bool {
	if f.next >= len(f.events) {
		return false
	}
	f.current = f.next
	f.next++
	return true
}

func (f *fakeDecoder) Event() anthropicstream.Event {
	data := []byte(f.events[f.current])
	eventType := gjson.GetBytes(data, "type").String()
	return anthropicstream.Event{Type: eventType, Data: data}
}

func (f *fakeDecoder) Close() error { return nil }
func (f *fakeDecoder) Err() error   { return nil }

func messageStartJSON(t *testing.T, inputTokens, cacheCreation, cacheRead int64) string {
	t.Helper()
	ev := map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id": "msg_test", "type": "message", "role": "assistant",
			"content": []interface{}{}, "model": "claude-3-5-sonnet",
			"usage": map[string]interface{}{
				"input_tokens":                inputTokens,
				"output_tokens":               0,
				"cache_creation_input_tokens": cacheCreation,
				"cache_read_input_tokens":     cacheRead,
			},
		},
	}
	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func outputOnlyDeltaJSON(t *testing.T, outputTokens int64) string {
	t.Helper()
	ev := map[string]interface{}{
		"type":  "message_delta",
		"delta": map[string]interface{}{"stop_reason": "end_turn"},
		"usage": map[string]interface{}{"output_tokens": outputTokens},
	}
	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func messageDeltaFullJSON(t *testing.T, inputTokens, outputTokens, cacheRead int64) string {
	t.Helper()
	ev := map[string]interface{}{
		"type":  "message_delta",
		"delta": map[string]interface{}{"stop_reason": "end_turn"},
		"usage": map[string]interface{}{
			"input_tokens":            inputTokens,
			"output_tokens":           outputTokens,
			"cache_read_input_tokens": cacheRead,
		},
	}
	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestAnthropicAccumulator_RealFormat verifies the real Anthropic API format:
// input_tokens in message_start, output_tokens in message_delta.
func TestAnthropicAccumulator_RealFormat(t *testing.T) {
	events := []string{
		messageStartJSON(t, 35, 0, 5), // input=35, creation=0, read=5
		outputOnlyDeltaJSON(t, 18),
	}
	dec := newFakeDecoder(events)
	stream := anthropicstream.NewStream[anthropic.MessageStreamEventUnion](dec, nil)

	acc := usage.NewAnthropicAccumulator()
	for stream.Next() {
		evt := stream.Current()
		acc.Consume(&evt)
	}

	got := acc.Result()
	assert.Equal(t, 35, got.InputTokens)
	assert.Equal(t, 18, got.OutputTokens)
	assert.Equal(t, 5, got.CacheReadTokens)
	assert.True(t, acc.HasUsage())
}

// TestAnthropicAccumulator_WithCacheCreation verifies that cache_creation_input_tokens
// is added to inputTokens so the denominator covers total prompt cost.
func TestAnthropicAccumulator_WithCacheCreation(t *testing.T) {
	events := []string{
		messageStartJSON(t, 100, 900, 800), // input=100, creation=900, read=800
		outputOnlyDeltaJSON(t, 50),
	}
	dec := newFakeDecoder(events)
	stream := anthropicstream.NewStream[anthropic.MessageStreamEventUnion](dec, nil)

	acc := usage.NewAnthropicAccumulator()
	for stream.Next() {
		evt := stream.Current()
		acc.Consume(&evt)
	}

	got := acc.Result()
	assert.Equal(t, 1000, got.InputTokens) // 100 + 900
	assert.Equal(t, 50, got.OutputTokens)
	assert.Equal(t, 800, got.CacheReadTokens)
}

// TestAnthropicAccumulator_NonStandardDelta verifies backward compat for providers
// that send input_tokens in message_delta instead of message_start.
func TestAnthropicAccumulator_NonStandardDelta(t *testing.T) {
	events := []string{
		// message_start with zero input (non-standard)
		`{"type":"message_start","message":{"id":"m","type":"message","role":"assistant","content":[],"model":"custom","usage":{"input_tokens":0,"output_tokens":0}}}`,
		messageDeltaFullJSON(t, 40, 20, 0),
	}
	dec := newFakeDecoder(events)
	stream := anthropicstream.NewStream[anthropic.MessageStreamEventUnion](dec, nil)

	acc := usage.NewAnthropicAccumulator()
	for stream.Next() {
		evt := stream.Current()
		acc.Consume(&evt)
	}

	got := acc.Result()
	assert.Equal(t, 40, got.InputTokens)
	assert.Equal(t, 20, got.OutputTokens)
}

// TestAnthropicAccumulator_Beta verifies ConsumeBeta works for beta streams.
func TestAnthropicAccumulator_Beta(t *testing.T) {
	events := []string{
		messageStartJSON(t, 40, 5, 8), // input=40, creation=5, read=8
		outputOnlyDeltaJSON(t, 22),
	}
	dec := newFakeDecoder(events)
	stream := anthropicstream.NewStream[anthropic.BetaRawMessageStreamEventUnion](dec, nil)

	acc := usage.NewAnthropicAccumulator()
	for stream.Next() {
		evt := stream.Current()
		acc.ConsumeBeta(&evt)
	}

	got := acc.Result()
	assert.Equal(t, 45, got.InputTokens) // 40 + 5
	assert.Equal(t, 22, got.OutputTokens)
	assert.Equal(t, 8, got.CacheReadTokens)
}

// TestAnthropicAccumulator_NoUsage verifies HasUsage is false when no usage seen.
func TestAnthropicAccumulator_NoUsage(t *testing.T) {
	acc := usage.NewAnthropicAccumulator()
	assert.False(t, acc.HasUsage())
	got := acc.Result()
	assert.Equal(t, 0, got.InputTokens)
	assert.Equal(t, 0, got.OutputTokens)
}

// TestAnthropicAccumulator_UsageOnNonStandardEventType verifies the gjson
// fallback still catches usage attached to event types outside the standard
// message_start/message_delta pair (e.g. a non-compliant provider putting
// final usage on message_stop).
func TestAnthropicAccumulator_UsageOnNonStandardEventType(t *testing.T) {
	events := []string{
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`,
		`{"type":"message_stop","usage":{"input_tokens":40,"output_tokens":423}}`,
	}
	dec := newFakeDecoder(events)
	stream := anthropicstream.NewStream[anthropic.MessageStreamEventUnion](dec, nil)

	acc := usage.NewAnthropicAccumulator()
	for stream.Next() {
		evt := stream.Current()
		acc.Consume(&evt)
	}

	got := acc.Result()
	assert.Equal(t, 40, got.InputTokens)
	assert.Equal(t, 423, got.OutputTokens)
	assert.True(t, acc.HasUsage())
}

// ---------------------------------------------------------------------------
// Cache-write normalization (OpenAI gpt-5.6+)
// ---------------------------------------------------------------------------

// TestFromOpenAI_CacheWriteStaysInsideInput pins the rule that separates cache
// reads from cache writes: reads are subtracted out of InputTokens (they are
// billed at a discount and belong in the hit-rate numerator), writes are NOT
// (they are billed at 1.25x and are part of this prompt's cost). Subtracting
// writes too would silently drop the write cost from every downstream total.
func TestFromOpenAI_CacheWriteStaysInsideInput(t *testing.T) {
	const (
		prompt     = int64(1000)
		cached     = int64(600)
		cacheWrite = int64(150)
		completion = int64(80)
	)

	chat := openai.CompletionUsage{PromptTokens: prompt, CompletionTokens: completion}
	chat.PromptTokensDetails.CachedTokens = cached
	chat.PromptTokensDetails.CacheWriteTokens = cacheWrite

	resp := responses.ResponseUsage{InputTokens: prompt, OutputTokens: completion}
	resp.InputTokensDetails.CachedTokens = cached
	resp.InputTokensDetails.CacheWriteTokens = cacheWrite

	for name, got := range map[string]*protocol.TokenUsage{
		"chat":      usage.FromOpenAIChatCompletion(chat),
		"responses": usage.FromOpenAIResponses(resp),
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, 400, got.InputTokens, "only cached_tokens is subtracted; writes stay in")
			assert.Equal(t, 600, got.CacheReadTokens)
			assert.Equal(t, 600, got.CacheReadTokens)
			assert.Equal(t, 150, got.CacheWriteTokens)
			assert.Equal(t, 80, got.OutputTokens)
			// The invariant every downstream consumer relies on.
			assert.Equal(t, int(prompt), got.InputTokens+got.CacheReadTokens)
			assert.LessOrEqual(t, got.CacheWriteTokens, got.InputTokens,
				"CacheWriteTokens is a subset of InputTokens, never an addition")
		})
	}
}

// TestFromOpenAI_NoCacheWriteReported covers pre-gpt-5.6 models and channels
// such as Azure that never surface writes: the field simply stays zero.
func TestFromOpenAI_NoCacheWriteReported(t *testing.T) {
	chat := openai.CompletionUsage{PromptTokens: 500, CompletionTokens: 40}
	chat.PromptTokensDetails.CachedTokens = 200

	got := usage.FromOpenAIChatCompletion(chat)
	assert.Equal(t, 300, got.InputTokens)
	assert.Equal(t, 200, got.CacheReadTokens)
	assert.Zero(t, got.CacheWriteTokens)
}

// TestChatUsage_RoundTripsCacheWrite verifies the canonical -> wire direction
// puts writes back where OpenAI expects them.
func TestChatUsage_RoundTripsCacheWrite(t *testing.T) {
	u := protocol.NewTokenUsageFull(400, 80, 600, 150, 12)

	cu := usage.ChatUsage(u)
	assert.Equal(t, int64(1000), cu.PromptTokens, "prompt_tokens = uncached + written + cached")
	assert.Equal(t, int64(600), cu.PromptTokensDetails.CachedTokens)
	assert.Equal(t, int64(150), cu.PromptTokensDetails.CacheWriteTokens)
	assert.Equal(t, int64(12), cu.CompletionTokensDetails.ReasoningTokens)

	// Round trip back through normalization must be lossless.
	back := usage.FromOpenAIChatCompletion(cu)
	assert.Equal(t, u.InputTokens, back.InputTokens)
	assert.Equal(t, u.CacheReadTokens, back.CacheReadTokens)
	assert.Equal(t, u.CacheWriteTokens, back.CacheWriteTokens)
}

// TestAnthropicAndOpenAI_NormalizeToSameShape is the cross-provider contract
// from .design/stream-usage-tracking.md §2.1: OpenAI subtracts, Anthropic adds,
// and both land on InputTokens = uncached + written.
func TestAnthropicAndOpenAI_NormalizeToSameShape(t *testing.T) {
	// 200 uncached + 50 written + 800 read, 500 output.
	chat := openai.CompletionUsage{PromptTokens: 1050, CompletionTokens: 500}
	chat.PromptTokensDetails.CachedTokens = 800
	chat.PromptTokensDetails.CacheWriteTokens = 50

	anth := anthropic.Usage{
		InputTokens:              200,
		OutputTokens:             500,
		CacheCreationInputTokens: 50,
		CacheReadInputTokens:     800,
	}

	fromOpenAI := usage.FromOpenAIChatCompletion(chat)
	fromAnthropic := usage.FromAnthropicMessage(anth)

	assert.Equal(t, 250, fromOpenAI.InputTokens)
	assert.Equal(t, fromOpenAI.InputTokens, fromAnthropic.InputTokens)
	assert.Equal(t, fromOpenAI.CacheReadTokens, fromAnthropic.CacheReadTokens)
	assert.Equal(t, fromOpenAI.CacheWriteTokens, fromAnthropic.CacheWriteTokens)
	assert.Equal(t, fromOpenAI.OutputTokens, fromAnthropic.OutputTokens)
}

// TestToAnthropicUsageMap_UnfoldsCacheWrite guards against double counting:
// canonical InputTokens folds the write cost in, Anthropic's wire input_tokens
// does not, so emitting both unchanged would bill the writes twice.
func TestToAnthropicUsageMap_UnfoldsCacheWrite(t *testing.T) {
	u := protocol.NewTokenUsageFull(250, 500, 800, 50, 0)

	m := u.ToAnthropicUsageMap()
	assert.Equal(t, 200, m["input_tokens"], "input_tokens excludes the write portion")
	assert.Equal(t, 50, m["cache_creation_input_tokens"])
	assert.Equal(t, 800, m["cache_read_input_tokens"])

	// input + creation + read must reconstruct the original prompt total.
	assert.Equal(t, u.InputTokens+u.CacheReadTokens,
		m["input_tokens"].(int)+m["cache_creation_input_tokens"].(int)+m["cache_read_input_tokens"].(int))
}
