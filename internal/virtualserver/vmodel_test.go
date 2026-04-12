package virtualserver_test

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/server_validate"
	"github.com/tingly-dev/tingly-box/internal/virtualserver"
)

// newTestServer spins up a real Gin httptest.Server with virtualserver routes
// mounted at /v1 to match VirtualClient endpoint paths.
func newTestServer(t *testing.T) *server_validate.VirtualClient {
	t.Helper()
	gin.SetMode(gin.TestMode)

	svc := virtualserver.NewService()

	engine := gin.New()
	group := engine.Group("/v1")
	svc.SetupRoutes(group)

	srv := httptest.NewServer(engine)
	t.Cleanup(srv.Close)

	return server_validate.NewVirtualClient(srv.URL)
}

// ─── Typed response parsers ────────────────────────────────────────────────────

// parseOpenAI deserializes RawBody into ChatCompletionResponse and logs it.
func parseOpenAI(t *testing.T, result *server_validate.ParsedResponse) virtualserver.ChatCompletionResponse {
	t.Helper()
	var resp virtualserver.ChatCompletionResponse
	require.NoError(t, json.Unmarshal(result.RawBody, &resp), "unmarshal OpenAI response")
	logOpenAI(t, &resp)
	return resp
}

// parseAnthropic deserializes RawBody into AnthropicMessageResponse and logs it.
func parseAnthropic(t *testing.T, result *server_validate.ParsedResponse) virtualserver.AnthropicMessageResponse {
	t.Helper()
	var resp virtualserver.AnthropicMessageResponse
	require.NoError(t, json.Unmarshal(result.RawBody, &resp), "unmarshal Anthropic response")
	logAnthropic(t, &resp)
	return resp
}

// ─── Logging helpers ──────────────────────────────────────────────────────────

func logOpenAI(t *testing.T, resp *virtualserver.ChatCompletionResponse) {
	t.Helper()
	var sb strings.Builder
	fmt.Fprintf(&sb, "[OpenAI] id=%s model=%s", resp.ID, resp.Model)
	fmt.Fprintf(&sb, " usage(prompt=%d completion=%d)", resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
	for i, ch := range resp.Choices {
		fmt.Fprintf(&sb, "\n  choice[%d] finish=%s", i, ch.FinishReason)
		if ch.Message.Content != "" {
			preview := ch.Message.Content
			if len(preview) > 80 {
				preview = preview[:80] + "…"
			}
			fmt.Fprintf(&sb, " content=%q", preview)
		}
		for j, tc := range ch.Message.ToolCalls {
			fmt.Fprintf(&sb, "\n    tool[%d] id=%s name=%s args=%s", j, tc.ID, tc.Function.Name, tc.Function.Arguments)
		}
	}
	t.Log(sb.String())
}

func logAnthropic(t *testing.T, resp *virtualserver.AnthropicMessageResponse) {
	t.Helper()
	var sb strings.Builder
	fmt.Fprintf(&sb, "[Anthropic] id=%s model=%s role=%s stop=%s", resp.ID, resp.Model, resp.Role, resp.StopReason)
	fmt.Fprintf(&sb, " usage(in=%d out=%d)", resp.Usage.InputTokens, resp.Usage.OutputTokens)
	for i, blk := range resp.Content {
		switch blk.Type {
		case "text":
			preview := blk.Text
			if len(preview) > 80 {
				preview = preview[:80] + "…"
			}
			fmt.Fprintf(&sb, "\n  block[%d] type=text text=%q", i, preview)
		case "tool_use":
			fmt.Fprintf(&sb, "\n  block[%d] type=tool_use id=%s name=%s input=%s", i, blk.ID, blk.Name, blk.Input)
		}
	}
	t.Log(sb.String())
}

// ─── Stream parsers ───────────────────────────────────────────────────────────

// parseOpenAIStream deserializes each SSE data line into ChatCompletionStreamResponse chunks.
// Lines that are "[DONE]" or event-type lines are skipped.
func parseOpenAIStream(t *testing.T, result *server_validate.ParsedResponse) []virtualserver.ChatCompletionStreamResponse {
	t.Helper()
	var chunks []virtualserver.ChatCompletionStreamResponse
	for _, line := range result.StreamEvents {
		payload, ok := extractSSEData(line)
		if !ok || payload == "[DONE]" {
			continue
		}
		var chunk virtualserver.ChatCompletionStreamResponse
		if err := json.Unmarshal([]byte(payload), &chunk); err == nil {
			chunks = append(chunks, chunk)
		}
	}
	logOpenAIStream(t, chunks)
	return chunks
}

// parseAnthropicStream deserializes each SSE data line into AnthropicStreamEvent items.
func parseAnthropicStream(t *testing.T, result *server_validate.ParsedResponse) []virtualserver.AnthropicStreamEvent {
	t.Helper()
	var events []virtualserver.AnthropicStreamEvent
	for _, line := range result.StreamEvents {
		payload, ok := extractSSEData(line)
		if !ok {
			continue
		}
		var ev virtualserver.AnthropicStreamEvent
		if err := json.Unmarshal([]byte(payload), &ev); err == nil {
			events = append(events, ev)
		}
	}
	logAnthropicStream(t, events)
	return events
}

// extractSSEData strips the "data:" or "data: " prefix from an SSE line.
func extractSSEData(line string) (string, bool) {
	if strings.HasPrefix(line, "data: ") {
		return strings.TrimPrefix(line, "data: "), true
	}
	if strings.HasPrefix(line, "data:") {
		return strings.TrimPrefix(line, "data:"), true
	}
	return "", false
}

func logOpenAIStream(t *testing.T, chunks []virtualserver.ChatCompletionStreamResponse) {
	t.Helper()
	var sb strings.Builder
	fmt.Fprintf(&sb, "[OpenAI stream] chunks=%d", len(chunks))
	var assembled strings.Builder
	var finishReason string
	for _, ch := range chunks {
		if len(ch.Choices) == 0 {
			continue
		}
		c := ch.Choices[0]
		assembled.WriteString(c.Delta.Content)
		if string(c.FinishReason) != "" {
			finishReason = string(c.FinishReason)
		}
	}
	if assembled.Len() > 0 {
		preview := assembled.String()
		if len(preview) > 80 {
			preview = preview[:80] + "…"
		}
		fmt.Fprintf(&sb, " assembled=%q", preview)
	}
	if finishReason != "" {
		fmt.Fprintf(&sb, " finish=%s", finishReason)
	}
	t.Log(sb.String())
}

func logAnthropicStream(t *testing.T, events []virtualserver.AnthropicStreamEvent) {
	t.Helper()
	var sb strings.Builder
	fmt.Fprintf(&sb, "[Anthropic stream] events=%d", len(events))
	var assembled strings.Builder
	for _, ev := range events {
		if ev.Type == "content_block_delta" && ev.Delta != nil && ev.Delta.Type == "text_delta" {
			assembled.WriteString(ev.Delta.Text)
		}
	}
	if assembled.Len() > 0 {
		preview := assembled.String()
		if len(preview) > 80 {
			preview = preview[:80] + "…"
		}
		fmt.Fprintf(&sb, " assembled=%q", preview)
	}
	t.Log(sb.String())
}

// ─── MockModel vmodels (support both OpenAI Chat + Anthropic) ─────────────────

func TestVirtualServer_VModel_VirtualGPT4_OpenAI(t *testing.T) {
	vc := newTestServer(t)
	result := vc.SendOpenAIChatModel(t, "virtual-gpt-4", false)
	require.Equal(t, 200, result.HTTPStatus)

	resp := parseOpenAI(t, result)
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, "stop", string(resp.Choices[0].FinishReason))
	assert.NotEmpty(t, resp.Choices[0].Message.Content)
	assert.Greater(t, resp.Usage.CompletionTokens, int64(0))
}

func TestVirtualServer_VModel_VirtualGPT4_OpenAI_Stream(t *testing.T) {
	vc := newTestServer(t)
	result := vc.SendOpenAIChatModel(t, "virtual-gpt-4", true)
	require.Equal(t, 200, result.HTTPStatus)

	chunks := parseOpenAIStream(t, result)
	require.NotEmpty(t, chunks)

	// All chunks carry the correct model
	for _, ch := range chunks {
		assert.Equal(t, "virtual-gpt-4", ch.Model)
	}

	// Assemble delta content and verify finish reason
	var assembled strings.Builder
	var finishReason string
	for _, ch := range chunks {
		if len(ch.Choices) == 0 {
			continue
		}
		assembled.WriteString(ch.Choices[0].Delta.Content)
		if fr := string(ch.Choices[0].FinishReason); fr != "" {
			finishReason = fr
		}
	}
	assert.NotEmpty(t, assembled.String())
	assert.Equal(t, "stop", finishReason)
}

func TestVirtualServer_VModel_VirtualGPT4_Anthropic(t *testing.T) {
	vc := newTestServer(t)
	result := vc.SendAnthropicV1Model(t, "virtual-gpt-4", false)
	require.Equal(t, 200, result.HTTPStatus)

	resp := parseAnthropic(t, result)
	assert.Equal(t, "message", resp.Type)
	assert.Equal(t, "assistant", resp.Role)
	assert.Equal(t, "stop", resp.StopReason)
	require.NotEmpty(t, resp.Content)
	assert.Equal(t, "text", resp.Content[0].Type)
	assert.NotEmpty(t, resp.Content[0].Text)
	assert.Greater(t, resp.Usage.OutputTokens, int64(0))
}

func TestVirtualServer_VModel_VirtualGPT4_Anthropic_Stream(t *testing.T) {
	vc := newTestServer(t)
	result := vc.SendAnthropicV1Model(t, "virtual-gpt-4", true)
	require.Equal(t, 200, result.HTTPStatus)

	events := parseAnthropicStream(t, result)
	require.NotEmpty(t, events)

	// Must have a message_start event
	var hasStart bool
	for _, ev := range events {
		if ev.Type == "message_start" {
			hasStart = true
			require.NotNil(t, ev.Message)
			assert.Equal(t, "assistant", ev.Message.Role)
		}
	}
	assert.True(t, hasStart, "expected message_start event")

	// Must have at least one content_block_delta with text
	var assembled strings.Builder
	for _, ev := range events {
		if ev.Type == "content_block_delta" && ev.Delta != nil {
			assembled.WriteString(ev.Delta.Text)
		}
	}
	assert.NotEmpty(t, assembled.String())

	// Must end with message_stop
	last := events[len(events)-1]
	assert.Equal(t, "message_stop", last.Type)
}

func TestVirtualServer_VModel_VirtualClaude3_OpenAI(t *testing.T) {
	vc := newTestServer(t)
	result := vc.SendOpenAIChatModel(t, "virtual-claude-3", false)
	require.Equal(t, 200, result.HTTPStatus)

	resp := parseOpenAI(t, result)
	require.Len(t, resp.Choices, 1)
	assert.NotEmpty(t, resp.Choices[0].Message.Content)
}

func TestVirtualServer_VModel_VirtualClaude3_Anthropic(t *testing.T) {
	vc := newTestServer(t)
	result := vc.SendAnthropicV1Model(t, "virtual-claude-3", false)
	require.Equal(t, 200, result.HTTPStatus)

	resp := parseAnthropic(t, result)
	assert.Equal(t, "assistant", resp.Role)
	require.NotEmpty(t, resp.Content)
	assert.Equal(t, "text", resp.Content[0].Type)
	assert.NotEmpty(t, resp.Content[0].Text)
}

func TestVirtualServer_VModel_EchoModel_OpenAI(t *testing.T) {
	vc := newTestServer(t)
	result := vc.SendOpenAIChatModel(t, "echo-model", false)
	require.Equal(t, 200, result.HTTPStatus)

	resp := parseOpenAI(t, result)
	require.Len(t, resp.Choices, 1)
	assert.NotEmpty(t, resp.Choices[0].Message.Content)
}

func TestVirtualServer_VModel_EchoModel_Anthropic(t *testing.T) {
	vc := newTestServer(t)
	result := vc.SendAnthropicV1Model(t, "echo-model", false)
	require.Equal(t, 200, result.HTTPStatus)

	resp := parseAnthropic(t, result)
	assert.Equal(t, "assistant", resp.Role)
	require.NotEmpty(t, resp.Content)
	assert.NotEmpty(t, resp.Content[0].Text)
}

// ─── Tool-returning MockModels ─────────────────────────────────────────────────

func TestVirtualServer_VModel_AskUserQuestion_OpenAI(t *testing.T) {
	vc := newTestServer(t)
	result := vc.SendOpenAIChatModel(t, "ask-user-question", false)
	require.Equal(t, 200, result.HTTPStatus)

	resp := parseOpenAI(t, result)
	require.Len(t, resp.Choices, 1)
	ch := resp.Choices[0]
	assert.Equal(t, "tool_calls", string(ch.FinishReason))
	require.NotEmpty(t, ch.Message.ToolCalls)
	tc := ch.Message.ToolCalls[0]
	assert.Equal(t, "ask_user_question", tc.Function.Name)

	var args map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(tc.Function.Arguments), &args))
	assert.NotEmpty(t, args["question"])
	assert.NotEmpty(t, args["options"])
}

func TestVirtualServer_VModel_AskUserQuestion_Anthropic(t *testing.T) {
	vc := newTestServer(t)
	result := vc.SendAnthropicV1Model(t, "ask-user-question", false)
	require.Equal(t, 200, result.HTTPStatus)

	resp := parseAnthropic(t, result)
	assert.Equal(t, "tool_use", resp.StopReason)

	var toolBlock *virtualserver.AnthropicContent
	for i := range resp.Content {
		if resp.Content[i].Type == "tool_use" {
			toolBlock = &resp.Content[i]
			break
		}
	}
	require.NotNil(t, toolBlock, "expected tool_use block")
	assert.Equal(t, "ask_user_question", toolBlock.Name)
	assert.NotEmpty(t, toolBlock.ID)

	var input map[string]interface{}
	require.NoError(t, json.Unmarshal(toolBlock.Input, &input))
	assert.NotEmpty(t, input["question"])
	assert.NotEmpty(t, input["options"])
}

func TestVirtualServer_VModel_AskConfirmation_Anthropic(t *testing.T) {
	vc := newTestServer(t)
	result := vc.SendAnthropicV1Model(t, "ask-confirmation", false)
	require.Equal(t, 200, result.HTTPStatus)

	resp := parseAnthropic(t, result)
	assert.Equal(t, "tool_use", resp.StopReason)

	var toolBlock *virtualserver.AnthropicContent
	for i := range resp.Content {
		if resp.Content[i].Type == "tool_use" {
			toolBlock = &resp.Content[i]
			break
		}
	}
	require.NotNil(t, toolBlock)
	assert.Equal(t, "ask_user_question", toolBlock.Name)

	var input map[string]interface{}
	require.NoError(t, json.Unmarshal(toolBlock.Input, &input))
	assert.NotEmpty(t, input["question"])
	options, ok := input["options"].([]interface{})
	require.True(t, ok, "options should be a list")
	assert.Len(t, options, 2) // Yes / No
}

func TestVirtualServer_VModel_WebSearchExample_OpenAI(t *testing.T) {
	vc := newTestServer(t)
	result := vc.SendOpenAIChatModel(t, "web-search-example", false)
	require.Equal(t, 200, result.HTTPStatus)

	resp := parseOpenAI(t, result)
	require.Len(t, resp.Choices, 1)
	ch := resp.Choices[0]
	assert.Equal(t, "tool_calls", string(ch.FinishReason))
	require.NotEmpty(t, ch.Message.ToolCalls)
	tc := ch.Message.ToolCalls[0]
	assert.Equal(t, "web_search", tc.Function.Name)

	var args map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(tc.Function.Arguments), &args))
	assert.NotEmpty(t, args["query"])
}

func TestVirtualServer_VModel_WebSearchExample_Anthropic(t *testing.T) {
	vc := newTestServer(t)
	result := vc.SendAnthropicV1Model(t, "web-search-example", false)
	require.Equal(t, 200, result.HTTPStatus)

	resp := parseAnthropic(t, result)
	assert.Equal(t, "tool_use", resp.StopReason)

	var toolBlock *virtualserver.AnthropicContent
	for i := range resp.Content {
		if resp.Content[i].Type == "tool_use" {
			toolBlock = &resp.Content[i]
			break
		}
	}
	require.NotNil(t, toolBlock)
	assert.Equal(t, "web_search", toolBlock.Name)

	var input map[string]interface{}
	require.NoError(t, json.Unmarshal(toolBlock.Input, &input))
	assert.NotEmpty(t, input["query"])
}

// ─── TransformModels (Anthropic only) ─────────────────────────────────────────

func TestVirtualServer_VModel_CompactThinking_Anthropic(t *testing.T) {
	vc := newTestServer(t)
	result := vc.SendAnthropicV1Model(t, "compact-thinking", false)
	require.Equal(t, 200, result.HTTPStatus)

	resp := parseAnthropic(t, result)
	// TransformModel passes through — with a single-turn input it echoes the user message.
	assert.Equal(t, "assistant", resp.Role)
}

func TestVirtualServer_VModel_CompactRoundOnly_Anthropic(t *testing.T) {
	vc := newTestServer(t)
	result := vc.SendAnthropicV1Model(t, "compact-round-only", false)
	require.Equal(t, 200, result.HTTPStatus)

	resp := parseAnthropic(t, result)
	assert.Equal(t, "assistant", resp.Role)
}

func TestVirtualServer_VModel_CompactRoundFiles_Anthropic(t *testing.T) {
	vc := newTestServer(t)
	result := vc.SendAnthropicV1Model(t, "compact-round-files", false)
	require.Equal(t, 200, result.HTTPStatus)

	resp := parseAnthropic(t, result)
	assert.Equal(t, "assistant", resp.Role)
}

func TestVirtualServer_VModel_ClaudeCodeCompact_Anthropic(t *testing.T) {
	vc := newTestServer(t)
	result := vc.SendAnthropicV1Model(t, "claude-code-compact", false)
	require.Equal(t, 200, result.HTTPStatus)

	resp := parseAnthropic(t, result)
	assert.Equal(t, "assistant", resp.Role)
}

func TestVirtualServer_VModel_ClaudeCodeStrategy_Anthropic(t *testing.T) {
	vc := newTestServer(t)
	result := vc.SendAnthropicV1Model(t, "claude-code-strategy", false)
	require.Equal(t, 200, result.HTTPStatus)

	resp := parseAnthropic(t, result)
	assert.Equal(t, "assistant", resp.Role)
}

// ─── TransformModels must reject OpenAI Chat requests ─────────────────────────

func TestVirtualServer_VModel_CompactThinking_OpenAI_NotImplemented(t *testing.T) {
	vc := newTestServer(t)
	result := vc.SendOpenAIChatModel(t, "compact-thinking", false)
	assert.Equal(t, 501, result.HTTPStatus)
	t.Logf("[OpenAI 501] body=%s", result.RawBody)
}

// ─── Unknown model returns 404 ────────────────────────────────────────────────

func TestVirtualServer_UnknownModel_OpenAI(t *testing.T) {
	vc := newTestServer(t)
	result := vc.SendOpenAIChatModel(t, "no-such-model", false)
	assert.Equal(t, 404, result.HTTPStatus)
	t.Logf("[OpenAI 404] body=%s", result.RawBody)
}

func TestVirtualServer_UnknownModel_Anthropic(t *testing.T) {
	vc := newTestServer(t)
	result := vc.SendAnthropicV1Model(t, "no-such-model", false)
	assert.Equal(t, 404, result.HTTPStatus)
	t.Logf("[Anthropic 404] body=%s", result.RawBody)
}
