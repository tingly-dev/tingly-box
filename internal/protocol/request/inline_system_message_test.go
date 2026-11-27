package request

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Claude Code 2.1.x sends role:"system" entries inside messages[] (the
// environment block, <total_tokens> budget notices). They must reach Chat and
// Responses upstreams in place, not be dropped (issue #1921).

const inlineSystemEnv = "# Environment\n - Primary working directory: /work/proj\n - Platform: linux"

func inlineSystemBetaRequest() *anthropic.BetaMessageNewParams {
	return &anthropic.BetaMessageNewParams{
		Model:     "m",
		MaxTokens: 100,
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi")),
			{Role: "system", Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock(inlineSystemEnv)}},
			{Role: anthropic.BetaMessageParamRoleAssistant, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock("hello")}},
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("next")),
		},
	}
}

func inlineSystemV1Request() *anthropic.MessageNewParams {
	return &anthropic.MessageNewParams{
		Model:     "m",
		MaxTokens: 100,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("hi")),
			{Role: "system", Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(inlineSystemEnv)}},
			anthropic.NewAssistantMessage(anthropic.NewTextBlock("hello")),
			anthropic.NewUserMessage(anthropic.NewTextBlock("next")),
		},
	}
}

// wireMessages marshals v and decodes it back as a list of generic objects so
// tests can assert on the serialized role and content.
func wireMessages(t *testing.T, v any) []map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	var out []map[string]any
	require.NoError(t, json.Unmarshal(b, &out))
	return out
}

func wireRoles(msgs []map[string]any) []string {
	roles := make([]string, 0, len(msgs))
	for _, m := range msgs {
		role, _ := m["role"].(string)
		roles = append(roles, role)
	}
	return roles
}

func wireFirstText(t *testing.T, msg map[string]any) string {
	t.Helper()
	parts, ok := msg["content"].([]any)
	require.True(t, ok, "content must be the content-part array form: %v", msg)
	require.NotEmpty(t, parts)
	part := parts[0].(map[string]any)
	text, _ := part["text"].(string)
	return text
}

func TestInlineSystemMessage_BetaToChat(t *testing.T) {
	req, _ := ConvertAnthropicBetaToOpenAIRequest(inlineSystemBetaRequest(), false, true, false)
	msgs := wireMessages(t, req.Messages)

	assert.Equal(t, []string{"user", "system", "assistant", "user"}, wireRoles(msgs))
	assert.Equal(t, inlineSystemEnv, wireFirstText(t, msgs[1]))
}

func TestInlineSystemMessage_V1ToChat(t *testing.T) {
	req, _ := ConvertAnthropicToOpenAIRequest(inlineSystemV1Request(), false, true, false)
	msgs := wireMessages(t, req.Messages)

	assert.Equal(t, []string{"user", "system", "assistant", "user"}, wireRoles(msgs))
	assert.Equal(t, inlineSystemEnv, wireFirstText(t, msgs[1]))
}

func TestInlineSystemMessage_ChatKeepsTopLevelSystemFirst(t *testing.T) {
	anthropicReq := inlineSystemBetaRequest()
	anthropicReq.System = []anthropic.BetaTextBlockParam{{Text: "You are Claude Code."}}

	req, _ := ConvertAnthropicBetaToOpenAIRequest(anthropicReq, false, true, false)
	msgs := wireMessages(t, req.Messages)

	assert.Equal(t, []string{"system", "user", "system", "assistant", "user"}, wireRoles(msgs))
	assert.Equal(t, "You are Claude Code.", wireFirstText(t, msgs[0]))
	assert.Equal(t, inlineSystemEnv, wireFirstText(t, msgs[2]))
}

func TestInlineSystemMessage_BetaToResponses(t *testing.T) {
	params := ConvertAnthropicBetaToResponsesRequest(inlineSystemBetaRequest())
	items := wireMessages(t, params.Input)

	assert.Equal(t, []string{"user", "system", "assistant", "user"}, wireRoles(items))
	assert.Equal(t, inlineSystemEnv, wireFirstText(t, items[1]))
	assert.Equal(t, "input_text", items[1]["content"].([]any)[0].(map[string]any)["type"])
}

func TestInlineSystemMessage_V1ToResponses(t *testing.T) {
	params := ConvertAnthropicV1ToResponsesRequest(inlineSystemV1Request())
	items := wireMessages(t, params.Input)

	assert.Equal(t, []string{"user", "system", "assistant", "user"}, wireRoles(items))
	assert.Equal(t, inlineSystemEnv, wireFirstText(t, items[1]))
}

// Only text is carried; a non-text block in a system message is skipped like
// any other block the target cannot represent, and a system message with no
// text left emits nothing.
func TestInlineSystemMessage_NonTextBlocksSkipped(t *testing.T) {
	anthropicReq := &anthropic.BetaMessageNewParams{
		Model:     "m",
		MaxTokens: 100,
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi")),
			{Role: "system", Content: []anthropic.BetaContentBlockParamUnion{
				anthropic.NewBetaImageBlock(anthropic.BetaURLImageSourceParam{URL: "https://example.com/a.png"}),
				anthropic.NewBetaTextBlock("budget"),
			}},
			{Role: "system", Content: []anthropic.BetaContentBlockParamUnion{
				anthropic.NewBetaImageBlock(anthropic.BetaURLImageSourceParam{URL: "https://example.com/b.png"}),
			}},
		},
	}

	chatReq, _ := ConvertAnthropicBetaToOpenAIRequest(anthropicReq, false, true, false)
	msgs := wireMessages(t, chatReq.Messages)
	require.Equal(t, []string{"user", "system"}, wireRoles(msgs))
	assert.Len(t, msgs[1]["content"], 1)
	assert.Equal(t, "budget", wireFirstText(t, msgs[1]))

	items := wireMessages(t, ConvertAnthropicBetaToResponsesRequest(anthropicReq).Input)
	require.Equal(t, []string{"user", "system"}, wireRoles(items))
	assert.Len(t, items[1]["content"], 1)
	assert.Equal(t, "budget", wireFirstText(t, items[1]))
}

// A cache breakpoint on an inline system block is already counted as
// representable; it must land on the forwarded part.
func TestInlineSystemMessage_CacheBreakpointCarried(t *testing.T) {
	block := anthropic.NewBetaTextBlock(inlineSystemEnv)
	block.OfText.CacheControl = anthropic.NewBetaCacheControlEphemeralParam()
	anthropicReq := &anthropic.BetaMessageNewParams{
		Model:     "m",
		MaxTokens: 100,
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi")),
			{Role: "system", Content: []anthropic.BetaContentBlockParamUnion{block}},
		},
	}

	chatReq, _ := ConvertAnthropicBetaToOpenAIRequest(anthropicReq, false, true, false)
	msgs := wireMessages(t, chatReq.Messages)
	require.Equal(t, []string{"user", "system"}, wireRoles(msgs))
	assert.Contains(t, msgs[1]["content"].([]any)[0], "prompt_cache_breakpoint")

	items := wireMessages(t, ConvertAnthropicBetaToResponsesRequest(anthropicReq).Input)
	require.Equal(t, []string{"user", "system"}, wireRoles(items))
	assert.Contains(t, items[1]["content"].([]any)[0], "prompt_cache_breakpoint")
}
