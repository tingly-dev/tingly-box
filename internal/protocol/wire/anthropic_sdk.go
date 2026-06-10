package wire

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/respjson"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

// SDK response structs (anthropic-sdk-go, openai-go) must never be marshaled
// directly onto the client wire: they track field presence in unmarshal-only
// metadata and have no omitempty tags, so encoding/json serializes every unset
// field as its zero value — phantom objects (stop_details refusal, container),
// "" where null is expected, and union noise inside every content block.
//
// The helpers in this file produce clean wire maps instead. Mutable fields
// (model, content text, stop_reason) are rebuilt from current struct state so
// post-forward mutations (model rename, guardrails redaction) are reflected;
// optional provider fields are preserved from the original raw JSON only when
// they were actually present upstream (respjson presence metadata).

// AnthropicMessageMap serializes an anthropic.Message (passthrough or
// hand-built) into the Anthropic v1 wire shape.
func AnthropicMessageMap(m *anthropic.Message) map[string]any {
	out := map[string]any{
		"id":            m.ID,
		"type":          "message",
		"role":          "assistant",
		"model":         string(m.Model),
		"content":       anthropicV1ContentToWire(m.Content),
		"stop_reason":   nullableWireString(string(m.StopReason)),
		"stop_sequence": rawOrNullableString(m.JSON.StopSequence, m.StopSequence),
		"usage":         rawOrUsageMap(m.JSON.Usage, anthropicUsageMap(m.Usage.InputTokens, m.Usage.OutputTokens, m.Usage.CacheCreationInputTokens, m.Usage.CacheReadInputTokens)),
	}
	putRawField(out, "container", m.JSON.Container)
	putRawField(out, "stop_details", m.JSON.StopDetails)
	putExtraRawFields(out, m.JSON.ExtraFields)
	return out
}

// AnthropicBetaMessageMap serializes an anthropic.BetaMessage (passthrough or
// hand-built) into the Anthropic beta wire shape.
func AnthropicBetaMessageMap(m *anthropic.BetaMessage) map[string]any {
	out := map[string]any{
		"id":            m.ID,
		"type":          "message",
		"role":          "assistant",
		"model":         string(m.Model),
		"content":       anthropicBetaContentToWire(m.Content),
		"stop_reason":   nullableWireString(string(m.StopReason)),
		"stop_sequence": rawOrNullableString(m.JSON.StopSequence, m.StopSequence),
		"usage":         rawOrUsageMap(m.JSON.Usage, anthropicUsageMap(m.Usage.InputTokens, m.Usage.OutputTokens, m.Usage.CacheCreationInputTokens, m.Usage.CacheReadInputTokens)),
	}
	putRawField(out, "container", m.JSON.Container)
	putRawField(out, "stop_details", m.JSON.StopDetails)
	putRawField(out, "context_management", m.JSON.ContextManagement)
	putRawField(out, "diagnostics", m.JSON.Diagnostics)
	putExtraRawFields(out, m.JSON.ExtraFields)
	return out
}

func anthropicV1ContentToWire(blocks []anthropic.ContentBlockUnion) []any {
	out := make([]any, 0, len(blocks))
	for i := range blocks {
		b := &blocks[i]
		out = append(out, anthropicBlockToWire(
			b.Type, b.Text, b.Thinking, b.Signature, b.Data, b.ID, b.Name, b.Input,
			b.JSON.Citations, b.RawJSON(), b,
		))
	}
	return out
}

func anthropicBetaContentToWire(blocks []anthropic.BetaContentBlockUnion) []any {
	out := make([]any, 0, len(blocks))
	for i := range blocks {
		b := &blocks[i]
		out = append(out, anthropicBlockToWire(
			b.Type, b.Text, b.Thinking, b.Signature, b.Data, b.ID, b.Name, b.Input,
			b.JSON.Citations, b.RawJSON(), b,
		))
	}
	return out
}

// anthropicBlockToWire rebuilds the common block types from struct state (so
// guardrails edits to text are reflected) and falls back to the block's raw
// upstream JSON for exotic types (server tool results, file ops, …), which
// only occur on passthrough where the raw form is available and unmodified.
func anthropicBlockToWire(
	blockType, text, thinking, signature, data, id, name string,
	input json.RawMessage, citations respjson.Field, raw string, fallback any,
) any {
	switch blockType {
	case "text":
		m := map[string]any{"type": "text", "text": text}
		putRawField(m, "citations", citations)
		return m
	case "thinking":
		return map[string]any{"type": "thinking", "thinking": thinking, "signature": signature}
	case "redacted_thinking":
		return map[string]any{"type": "redacted_thinking", "data": data}
	case "tool_use", "server_tool_use":
		if len(input) == 0 {
			input = json.RawMessage("{}")
		}
		return map[string]any{"type": blockType, "id": id, "name": name, "input": input}
	default:
		if raw != "" {
			return json.RawMessage(raw)
		}
		return fallback
	}
}

func anthropicUsageMap(input, output, cacheCreation, cacheRead int64) map[string]any {
	return map[string]any{
		"input_tokens":                input,
		"output_tokens":               output,
		"cache_creation_input_tokens": cacheCreation,
		"cache_read_input_tokens":     cacheRead,
	}
}

// OpenAIChatCompletionMap serializes an openai.ChatCompletion for the client
// wire, preferring the unmodified upstream JSON when available.
func OpenAIChatCompletionMap(resp *openai.ChatCompletion) (map[string]any, error) {
	return rawOrMarshaledMap(resp.RawJSON(), resp)
}

// OpenAIResponsesMap serializes a responses.Response for the client wire,
// preferring the unmodified upstream JSON when available.
func OpenAIResponsesMap(resp *responses.Response) (map[string]any, error) {
	return rawOrMarshaledMap(resp.RawJSON(), resp)
}

func rawOrMarshaledMap(raw string, v any) (map[string]any, error) {
	data := []byte(raw)
	if raw == "" {
		var err error
		data, err = json.Marshal(v)
		if err != nil {
			return nil, err
		}
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func putRawField(m map[string]any, key string, f respjson.Field) {
	if f.Valid() {
		m[key] = json.RawMessage(f.Raw())
	}
}

func putExtraRawFields(m map[string]any, extras map[string]respjson.Field) {
	for key, f := range extras {
		// The SDK stores unknown fields with "invalid" status (Valid() is
		// false) but keeps the raw JSON — gate on Raw() instead.
		if _, exists := m[key]; !exists && f.Raw() != "" && f.Raw() != respjson.Null {
			m[key] = json.RawMessage(f.Raw())
		}
	}
}

// rawOrUsageMap preserves the upstream usage block verbatim (service_tier,
// cache_creation details, …) and rebuilds the minimal shape for hand-built
// messages. Usage is never mutated after forwarding.
func rawOrUsageMap(f respjson.Field, rebuilt map[string]any) any {
	if f.Valid() {
		return json.RawMessage(f.Raw())
	}
	return rebuilt
}

func rawOrNullableString(f respjson.Field, s string) any {
	if f.Valid() {
		return json.RawMessage(f.Raw())
	}
	return nullableWireString(s)
}

func nullableWireString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
