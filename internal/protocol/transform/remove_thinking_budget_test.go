package transform

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

func TestRemoveThinkingBudget(t *testing.T) {
	for name, wrapper := range map[string]interface{}{
		"chat":      &protocol.OpenAIChatCompletionRequest{},
		"responses": &protocol.ResponseCreateRequest{},
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, json.Unmarshal([]byte(`{"model":"kimi-k3","thinking_budget":4096,"reasoning_effort":"high"}`), wrapper))
			var req interface{}
			switch r := wrapper.(type) {
			case *protocol.OpenAIChatCompletionRequest:
				req = r.ChatCompletionNewParams
			case *protocol.ResponseCreateRequest:
				req = r.ResponseNewParams
			}
			before, err := json.Marshal(req)
			require.NoError(t, err)
			require.Contains(t, string(before), `"thinking_budget":4096`)
			require.NoError(t, NewRemoveThinkingBudgetTransform().Apply(&TransformContext{Request: req}))
			after, err := json.Marshal(req)
			require.NoError(t, err)
			require.NotContains(t, string(after), `"thinking_budget"`)
		})
	}
}

func TestRemoveAnthropicThinkingBudget(t *testing.T) {
	for name, req := range map[string]interface{}{
		"v1":   &anthropic.MessageNewParams{Thinking: anthropic.ThinkingConfigParamOfEnabled(4096)},
		"beta": &anthropic.BetaMessageNewParams{Thinking: anthropic.BetaThinkingConfigParamOfEnabled(4096)},
	} {
		t.Run(name, func(t *testing.T) {
			before, err := json.Marshal(req)
			require.NoError(t, err)
			require.Contains(t, string(before), `"budget_tokens":4096`)
			require.NoError(t, NewRemoveThinkingBudgetTransform().Apply(&TransformContext{Request: req}))
			after, err := json.Marshal(req)
			require.NoError(t, err)
			require.NotContains(t, string(after), `"budget_tokens"`)
			require.Contains(t, string(after), `"type":"enabled"`)
		})
	}
}
