package request

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// claudeCodeStyleRequest builds an Anthropic beta request shaped like a Claude
// Code turn — a two-block system prompt, a cached tool definition, and a
// history of user text / tool_use / tool_result — with the rolling ephemeral
// breakpoints parked on the message block at breakpointAt (-1 for none).
//
// Claude Code carries a fixed number of breakpoints and moves them to the tail
// of the conversation each turn, so breakpointAt is exactly the axis that
// varies between two consecutive requests over the same history.
func claudeCodeStyleRequest(breakpointAt int) *anthropic.BetaMessageNewParams {
	cache := func(on bool) anthropic.BetaCacheControlEphemeralParam {
		if on {
			return anthropic.NewBetaCacheControlEphemeralParam()
		}
		return anthropic.BetaCacheControlEphemeralParam{}
	}

	toolResult := &anthropic.BetaToolResultBlockParam{
		ToolUseID:    "toolu_1",
		CacheControl: cache(breakpointAt == 2),
		Content: []anthropic.BetaToolResultBlockParamContentUnion{
			{OfText: &anthropic.BetaTextBlockParam{Text: "file contents"}},
		},
	}

	return &anthropic.BetaMessageNewParams{
		Model:     "gpt-5.6-sol",
		MaxTokens: 4096,
		System: []anthropic.BetaTextBlockParam{
			{Text: "You are Claude Code."},
			{Text: "BIG SYSTEM PROMPT", CacheControl: anthropic.NewBetaCacheControlEphemeralParam()},
		},
		Tools: []anthropic.BetaToolUnionParam{{
			OfTool: &anthropic.BetaToolParam{
				Name:         "Read",
				InputSchema:  anthropic.BetaToolInputSchemaParam{Type: "object"},
				CacheControl: anthropic.NewBetaCacheControlEphemeralParam(),
			},
		}},
		Messages: []anthropic.BetaMessageParam{
			{Role: "user", Content: []anthropic.BetaContentBlockParamUnion{
				{OfText: &anthropic.BetaTextBlockParam{Text: "read a file", CacheControl: cache(breakpointAt == 0)}},
			}},
			{Role: "assistant", Content: []anthropic.BetaContentBlockParamUnion{
				{OfText: &anthropic.BetaTextBlockParam{Text: "on it", CacheControl: cache(breakpointAt == 1)}},
				{OfToolUse: &anthropic.BetaToolUseBlockParam{ID: "toolu_1", Name: "Read", Input: map[string]any{"p": "x"}}},
			}},
			{Role: "user", Content: []anthropic.BetaContentBlockParamUnion{{OfToolResult: toolResult}}},
		},
	}
}

// stripPromptCacheFields removes every prompt-cache directive from a decoded
// request body, leaving the structure a prompt cache is actually keyed on.
func stripPromptCacheFields(v any) any {
	switch node := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(node))
		for k, child := range node {
			if slices.Contains(protocol.PromptCacheHintFields, k) {
				continue
			}
			out[k] = stripPromptCacheFields(child)
		}
		return out
	case []any:
		out := make([]any, 0, len(node))
		for _, child := range node {
			out = append(out, stripPromptCacheFields(child))
		}
		return out
	default:
		return v
	}
}

func marshalWithoutCacheFields(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	var decoded any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	out, err := json.Marshal(stripPromptCacheFields(decoded))
	require.NoError(t, err)
	return string(out)
}

// TestResponsesShapeIsStableAcrossBreakpointRotation is the regression test for
// the Codex prompt-cache collapse: converted history used to switch between the
// compact string form and the content-part list form depending on where the
// client's rolling breakpoints happened to sit, so every turn rewrote the
// conversation prefix the upstream cache had been keyed on and the hit rate
// fell off from the oldest moved breakpoint onward.
//
// Breakpoints must now be purely additive: strip the cache directives and every
// placement must produce the identical request.
func TestResponsesShapeIsStableAcrossBreakpointRotation(t *testing.T) {
	baseline := marshalWithoutCacheFields(t, ConvertAnthropicBetaToResponsesRequest(claudeCodeStyleRequest(-1)))

	for _, at := range []int{0, 1, 2} {
		got := marshalWithoutCacheFields(t, ConvertAnthropicBetaToResponsesRequest(claudeCodeStyleRequest(at)))
		require.Equal(t, baseline, got, "breakpoint on message block %d changed the request shape", at)
	}

	// The breakpoints themselves still reach the upstream, on the blocks that
	// carried them.
	withToolResultBreakpoint := ConvertAnthropicBetaToResponsesRequest(claudeCodeStyleRequest(2))
	raw, err := json.Marshal(withToolResultBreakpoint)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"prompt_cache_breakpoint"`)
}

// TestResponsesShapeIsStableForV1 covers the same invariant on the Anthropic v1
// converter, which shares the tool_result conversion with the beta path.
func TestResponsesShapeIsStableForV1(t *testing.T) {
	build := func(cached bool) *anthropic.MessageNewParams {
		toolResult := anthropic.NewToolResultBlock("toolu_1", "file contents", false)
		if cached {
			toolResult.OfToolResult.CacheControl = anthropic.NewCacheControlEphemeralParam()
		}
		userText := anthropic.NewTextBlock("read a file")
		if !cached {
			userText.OfText.CacheControl = anthropic.NewCacheControlEphemeralParam()
		}
		return &anthropic.MessageNewParams{
			Model:     "gpt-5.6-sol",
			MaxTokens: 4096,
			System:    []anthropic.TextBlockParam{{Text: "SYS"}},
			Messages: []anthropic.MessageParam{
				anthropic.NewUserMessage(userText),
				anthropic.NewAssistantMessage(anthropic.NewToolUseBlock("toolu_1", map[string]any{}, "Read")),
				anthropic.NewUserMessage(toolResult),
			},
		}
	}

	require.Equal(t,
		marshalWithoutCacheFields(t, ConvertAnthropicV1ToResponsesRequest(build(false))),
		marshalWithoutCacheFields(t, ConvertAnthropicV1ToResponsesRequest(build(true))),
	)
}

func TestResponsesPromptCacheKeyFromMetadata(t *testing.T) {
	t.Run("claude code session id", func(t *testing.T) {
		req := claudeCodeStyleRequest(-1)
		req.Metadata.UserID = anthropic.String(
			`{"device_id":"dev","account_uuid":"acct","session_id":"16d97292-8713-438b-ad2e-76f495717258"}`)

		out := ConvertAnthropicBetaToResponsesRequest(req)
		require.Equal(t, "16d97292-8713-438b-ad2e-76f495717258", out.PromptCacheKey.Value)

		raw, err := json.Marshal(out)
		require.NoError(t, err)
		require.NotContains(t, string(raw), "acct", "only the session id is forwarded upstream")
		require.NotContains(t, string(raw), `"dev"`)
	})

	t.Run("unrecognized user_id is hashed, not forwarded", func(t *testing.T) {
		req := claudeCodeStyleRequest(-1)
		req.Metadata.UserID = anthropic.String("someone@example.com")

		out := ConvertAnthropicBetaToResponsesRequest(req)
		require.NotEmpty(t, out.PromptCacheKey.Value)
		require.NotContains(t, out.PromptCacheKey.Value, "example.com")
		require.Equal(t, out.PromptCacheKey.Value,
			ConvertAnthropicBetaToResponsesRequest(req).PromptCacheKey.Value, "must be stable")
	})

	t.Run("absent metadata leaves the key unset", func(t *testing.T) {
		out := ConvertAnthropicBetaToResponsesRequest(claudeCodeStyleRequest(-1))
		require.False(t, out.PromptCacheKey.Valid())
	})
}
