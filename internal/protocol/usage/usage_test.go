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

	"github.com/tingly-dev/tingly-box/internal/protocol/usage"
)

// ---------------------------------------------------------------------------
// FromOpenAIChatCompletion
// ---------------------------------------------------------------------------

func TestFromOpenAIChatCompletion(t *testing.T) {
	tests := []struct {
		name             string
		prompt, cached   int64
		completion       int64
		reasoning        int64
		wantInput        int
		wantOutput       int
		wantCache        int
		wantReasoning    int
	}{
		{
			name: "90% cache hit",
			prompt: 1000, cached: 900, completion: 200,
			wantInput: 100, wantOutput: 200, wantCache: 900,
		},
		{
			name: "no cache",
			prompt: 500, cached: 0, completion: 100,
			wantInput: 500, wantOutput: 100, wantCache: 0,
		},
		{
			name: "with reasoning",
			prompt: 200, cached: 50, completion: 80, reasoning: 30,
			wantInput: 150, wantOutput: 80, wantCache: 50, wantReasoning: 30,
		},
		{
			name: "all zero",
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
			assert.Equal(t, tc.wantCache, got.CacheInputTokens)
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
			name: "partial cache",
			input: 500, cached: 200, output: 60,
			wantInput: 300, wantOutput: 60, wantCache: 200,
		},
		{
			name: "full cache",
			input: 800, cached: 800, output: 40,
			wantInput: 0, wantOutput: 40, wantCache: 800,
		},
		{
			name: "no cache with reasoning",
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
			assert.Equal(t, tc.wantCache, got.CacheInputTokens)
			assert.Equal(t, tc.wantReasoning, got.ReasoningTokens)
		})
	}
}

// ---------------------------------------------------------------------------
// FromAnthropicMessage / FromAnthropicBetaMessage
// ---------------------------------------------------------------------------

func TestFromAnthropicMessage(t *testing.T) {
	tests := []struct {
		name                       string
		input, creation, read      int64
		output                     int64
		wantInput, wantOutput      int
		wantCache                  int
	}{
		{
			name: "cache read only",
			input: 100, creation: 0, read: 800, output: 50,
			wantInput: 100, wantOutput: 50, wantCache: 800,
		},
		{
			name: "cache creation included in denominator",
			input: 100, creation: 900, read: 800, output: 50,
			wantInput: 1000, wantOutput: 50, wantCache: 800,
		},
		{
			name: "no cache",
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
			assert.Equal(t, tc.wantCache, got.CacheInputTokens)
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
	assert.Equal(t, 400, got.CacheInputTokens)
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
				"input_tokens":              inputTokens,
				"output_tokens":             0,
				"cache_creation_input_tokens": cacheCreation,
				"cache_read_input_tokens":   cacheRead,
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
		messageStartJSON(t, 35, 0, 5),  // input=35, creation=0, read=5
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
	assert.Equal(t, 5, got.CacheInputTokens)
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
	assert.Equal(t, 800, got.CacheInputTokens)
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
	assert.Equal(t, 8, got.CacheInputTokens)
}

// TestAnthropicAccumulator_NoUsage verifies HasUsage is false when no usage seen.
func TestAnthropicAccumulator_NoUsage(t *testing.T) {
	acc := usage.NewAnthropicAccumulator()
	assert.False(t, acc.HasUsage())
	got := acc.Result()
	assert.Equal(t, 0, got.InputTokens)
	assert.Equal(t, 0, got.OutputTokens)
}
