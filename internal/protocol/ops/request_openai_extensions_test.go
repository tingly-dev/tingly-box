package ops

import (
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// TestProviderDispatchDoesNotMatchHostnameMentionedElsewhereInURL proves the
// fix for the old strings.Contains(url, "...") dispatch: a base URL whose
// path or query merely *mentions* a vendor's hostname as text (e.g. a proxy
// relaying to a target named in a query parameter) must not be mistaken for
// that vendor. Only the parsed host is matched.
func TestProviderDispatchDoesNotMatchHostnameMentionedElsewhereInURL(t *testing.T) {
	msg := assistantToolCallMessage(t)
	msg.OfAssistant.SetExtraFields(map[string]any{"x_thinking": "should not be converted"})

	req := &openai.ChatCompletionNewParams{
		Model:    openai.ChatModel("gpt-4o"),
		Messages: []openai.ChatCompletionMessageParamUnion{msg},
	}

	ApplyProviderTransforms(req, "https://gateway.example.com/relay?target=api.deepseek.com", string(req.Model), &protocol.OpenAIConfig{})

	raw := marshalMessage(t, req.Messages[0])
	assert.Equal(t, "should not be converted", raw["x_thinking"],
		"a URL that merely mentions a vendor hostname in its path/query must not trigger that vendor's transform")
	assert.NotContains(t, raw, "reasoning_content")
}

// TestProviderDispatchMatchesBareHostnameWithoutScheme proves dispatch still
// works when Provider.APIBase is stored without a scheme (e.g.
// "api.deepseek.com" rather than "https://api.deepseek.com"), which
// net/url.Parse alone would treat as a relative path, not a host.
func TestProviderDispatchMatchesBareHostnameWithoutScheme(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		Model: openai.ChatModel("deepseek-v4-flash"),
	}

	ApplyProviderTransforms(req, "api.deepseek.com", string(req.Model), &protocol.OpenAIConfig{
		HasThinking:     true,
		ReasoningEffort: "high",
	})

	assert.Equal(t, "high", string(req.ReasoningEffort))
}

// TestDefaultTransformCollapsesExtendedEffortForUnverifiedVendor proves the
// fix for a regression introduced when the ladder was extended to six levels
// (#1524/#1528): a relay like opencode.ai/zen/go whose model name gives no
// vendor hint (e.g. a codenamed model that isn't literally "deepseek") falls
// through to applyDefaultTransform, which used to forward "minimal"/"xhigh"
// verbatim — values only api.openai.com is confirmed to accept — causing the
// downstream vendor to 400 on the unrecognized reasoning_effort enum member.
func TestDefaultTransformCollapsesExtendedEffortForUnverifiedVendor(t *testing.T) {
	tests := []struct {
		ladderEffort string
		want         string
	}{
		{"minimal", "low"},
		{"low", "low"},
		{"medium", "medium"},
		{"high", "high"},
		{"xhigh", "high"},
		{"max", "high"},
	}

	for _, tt := range tests {
		t.Run(tt.ladderEffort, func(t *testing.T) {
			req := &openai.ChatCompletionNewParams{
				Model: openai.ChatModel("gpt-5.6-luna"),
			}

			ApplyProviderTransforms(req, "https://opencode.ai/zen/go/v1", string(req.Model), &protocol.OpenAIConfig{
				HasThinking:     true,
				ReasoningEffort: openai.ReasoningEffort(tt.ladderEffort),
			})

			assert.Equal(t, tt.want, string(req.ReasoningEffort))
		})
	}
}

// TestDefaultTransformKeepsExtendedEffortForVerifiedOpenAI proves that
// api.openai.com — the one host confirmed to accept the full six-level
// ladder — still gets "minimal"/"xhigh" verbatim, unaffected by the
// unverified-vendor collapse above.
func TestDefaultTransformKeepsExtendedEffortForVerifiedOpenAI(t *testing.T) {
	req := &openai.ChatCompletionNewParams{
		Model: openai.ChatModel("gpt-5.6"),
	}

	ApplyProviderTransforms(req, "https://api.openai.com/v1", string(req.Model), &protocol.OpenAIConfig{
		HasThinking:     true,
		ReasoningEffort: "xhigh",
	})

	assert.Equal(t, "xhigh", string(req.ReasoningEffort))
}
