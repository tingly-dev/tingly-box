package mcp

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func extractMessagesForToolCall(req any) []map[string]any {
	switch r := req.(type) {
	case *anthropic.MessageNewParams:
		return extractMessageMaps(r.Messages)
	case *anthropic.BetaMessageNewParams:
		return extractMessageMaps(r.Messages)
	case *openai.ChatCompletionNewParams:
		return extractMessageMaps(r.Messages)
	default:
		return nil
	}
}

func extractMessageMaps[T any](messages []T) []map[string]any {
	if len(messages) == 0 {
		return nil
	}
	b, err := json.Marshal(messages)
	if err != nil {
		return nil
	}
	var out []map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

func extractModelFromRequest(req any, provider *typ.Provider) string {
	switch r := req.(type) {
	case *anthropic.MessageNewParams:
		return string(r.Model)
	case *anthropic.BetaMessageNewParams:
		return string(r.Model)
	case *openai.ChatCompletionNewParams:
		return string(r.Model)
	default:
		if provider != nil && len(provider.Models) > 0 {
			return provider.Models[0]
		}
		return ""
	}
}
