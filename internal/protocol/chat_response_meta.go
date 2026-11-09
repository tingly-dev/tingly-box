package protocol

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/openai/openai-go/v3/responses"
)

// ChatServiceTierFromAnthropic maps Anthropic's usage.service_tier onto the
// Chat Completions service_tier. "batch" has no Chat counterpart and, like an
// unreported tier, yields nil (emitted as null).
func ChatServiceTierFromAnthropic[T ~string](tier T) *string {
	var v string
	switch string(tier) {
	case string(anthropic.UsageServiceTierStandard):
		v = "default"
	case string(anthropic.UsageServiceTierPriority):
		v = "priority"
	default:
		return nil
	}
	return &v
}

// ChatServiceTierFromResponses carries the Responses service_tier over as-is:
// both APIs share the same tier vocabulary. Unreported yields nil.
func ChatServiceTierFromResponses(tier responses.ResponseServiceTier) *string {
	if tier == "" {
		return nil
	}
	v := string(tier)
	return &v
}

// ChatModerationFromResponses translates Response.moderation into the Chat
// Completions shape. Each side is either an "error" (identical in both APIs) or
// a single "moderation_result", which Chat wraps in a "moderation_results" list.
// Returns nil when the upstream reported no moderation.
func ChatModerationFromResponses(m responses.ResponseModeration) json.RawMessage {
	if m.Input.Type == "" && m.Output.Type == "" {
		return nil
	}
	raw, err := json.Marshal(map[string]json.RawMessage{
		"input":  chatModerationSide(m.Input.Type, m.Input.Model, m.Input.RawJSON()),
		"output": chatModerationSide(m.Output.Type, m.Output.Model, m.Output.RawJSON()),
	})
	if err != nil {
		return nil
	}
	return raw
}

func chatModerationSide(typ, model, raw string) json.RawMessage {
	if raw == "" {
		return json.RawMessage("null")
	}
	if typ != "moderation_result" {
		return json.RawMessage(raw)
	}
	wrapped, err := json.Marshal(map[string]any{
		"type":    "moderation_results",
		"model":   model,
		"results": []json.RawMessage{json.RawMessage(raw)},
	})
	if err != nil {
		return json.RawMessage("null")
	}
	return wrapped
}
