package transform

import (
	"testing"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
)

// A third-party Anthropic-compatible provider, not a Claude Code backend.
const thirdPartyAnthropicURL = "https://api.z.ai/api/anthropic"

// toolTurnWithoutThinking is what an Anthropic client sends back after the
// provider's model answered a tool call without a leading thinking block,
// in the shape the vendor stage sees for target.
func toolTurnWithoutThinking(target protocol.APIType) any {
	v1 := &anthropic.MessageNewParams{
		Model:       anthropic.Model("glm-5"),
		MaxTokens:   32000,
		Temperature: param.NewOpt(0.6),
		Thinking:    anthropic.ThinkingConfigParamOfEnabled(16000),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("list files")),
			anthropic.NewAssistantMessage(
				anthropic.NewTextBlock("ok"),
				anthropic.NewToolUseBlock("t1", map[string]any{"cmd": "ls"}, "Bash"),
			),
			anthropic.NewUserMessage(anthropic.NewToolResultBlock("t1", "a.txt", false)),
		},
	}
	if target == protocol.TypeAnthropicBeta {
		return request.ConvertAnthropicV1ToBetaRequest(v1)
	}
	return v1
}

// thinkingOf projects the fields the thinking guard rewrites out of either
// Anthropic request shape.
func thinkingOf(t *testing.T, req any) (enabled bool, temperature float64) {
	t.Helper()
	switch r := req.(type) {
	case *anthropic.MessageNewParams:
		return r.Thinking.OfEnabled != nil, r.Temperature.Value
	case *anthropic.BetaMessageNewParams:
		return r.Thinking.OfEnabled != nil, r.Temperature.Value
	}
	t.Fatalf("unexpected request type %T", req)
	return false, 0
}

var anthropicTargets = []protocol.APIType{protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta}

// A third-party Anthropic-compatible provider gets the request as it came,
// whoever the client is: reconciling it flipped thinking between requests of
// one conversation and invalidated the provider's prompt cache. Thinking the
// gateway produces from an OpenAI client's effort is reconciled by the
// conversion, not here. A rule's thinking_effort changes nothing here either.
func TestVendorTransform_ThirdPartyAnthropicKeepsThinking(t *testing.T) {
	sources := []protocol.APIType{protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta, protocol.TypeOpenAIChat, protocol.TypeOpenAIResponses}
	for _, target := range anthropicTargets {
		for _, source := range sources {
			for _, rule := range []string{"", "high"} {
				t.Run(string(source)+"->"+string(target)+"/rule="+rule, func(t *testing.T) {
					ctx := newFullChainContext(toolTurnWithoutThinking(target), thirdPartyAnthropicURL, nil)
					ctx.SourceAPI, ctx.TargetAPI = source, target
					if rule != "" {
						require.NoError(t, NewRuleThinkingTransform(rule).Apply(ctx))
					}
					require.NoError(t, NewVendorTransform().Apply(ctx))
					enabled, temperature := thinkingOf(t, ctx.Request)
					assert.True(t, enabled, "thinking must stay enabled")
					assert.Equal(t, 0.6, temperature, "sampling must stay as sent")
				})
			}
		}
	}
}

// A Claude Code backend enforces Anthropic's wire rules, so the vendor stage
// reconciles there: a tool turn without a thinking block turns thinking off.
func TestVendorTransform_ClaudeCodeBackendReconcilesThinking(t *testing.T) {
	for _, target := range anthropicTargets {
		t.Run(string(target), func(t *testing.T) {
			ctx := newFullChainContext(toolTurnWithoutThinking(target), "https://api.anthropic.com", anthropicExtra())
			ctx.SourceAPI, ctx.TargetAPI = protocol.TypeAnthropicBeta, target
			require.NoError(t, NewVendorTransform().Apply(ctx))
			enabled, _ := thinkingOf(t, ctx.Request)
			assert.False(t, enabled, "tool turn without a thinking block turns budget thinking off")
		})
	}
}
