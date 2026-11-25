package protocol

import (
	"encoding/json"
	"strings"
)

// The OpenAI Chat and Responses APIs describe token log probabilities with the
// same wire fields (token, bytes, logprob, top_logprobs). Gateway converters
// move values between the two SDK types, which cannot be assigned across
// packages, so they round-trip through generic JSON: the shapes are identical,
// and the result keeps the SDK's field spelling exactly (including fields a
// hand-written mapper would forget).

// LogprobsAsWire returns a Chat/Responses logprob slice in generic JSON form,
// or nil when there is nothing to carry. It is used to place SDK logprob
// values in a wire DTO whose declared field type differs (Chat stream chunk →
// Responses event, Responses output text → Chat choice).
func LogprobsAsWire[T any](in []T) []any {
	if len(in) == 0 {
		return nil
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return nil
	}
	var out []any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ChatChoiceLogprobsFromContent builds the Chat choice.logprobs object
// ({"content": [...], "refusal": null}) from Responses output-text logprobs.
// Chat logprobs are always "content"; Responses output_text has no refusal
// logprobs. Returns nil when there is nothing to carry, so the field stays
// null instead of an empty object.
func ChatChoiceLogprobsFromContent[T any](content []T) map[string]any {
	wire := LogprobsAsWire(content)
	if wire == nil {
		return nil
	}
	return map[string]any{
		"content": wire,
		"refusal": nil,
	}
}

// ChatChoiceLogprobsFromJSON builds the Chat choice.logprobs object from raw
// Responses logprob JSON. Streaming uses this instead of the SDK struct: the
// Responses stream event type has no `bytes` field, so a typed round-trip would
// silently drop it, while the raw event still carries it.
func ChatChoiceLogprobsFromJSON(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "[]" {
		return nil
	}
	var content []any
	if err := json.Unmarshal([]byte(raw), &content); err != nil || len(content) == 0 {
		return nil
	}
	return map[string]any{
		"content": content,
		"refusal": nil,
	}
}
