package nonstream

import (
	"net/http"
	"strings"

	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	usageconv "github.com/tingly-dev/tingly-box/internal/protocol/usage"
)

type responsesToChatNonStreamState struct {
	content   strings.Builder
	refusal   strings.Builder
	toolCalls []map[string]any
}

// HandleResponsesToOpenAIChat writes a Responses API response as OpenAI Chat format.
// Corresponds to stream.HandleResponsesToOpenAIChatStream.
func HandleResponsesToOpenAIChat(hc *protocol.HandleContext, rs *responses.Response) (map[string]any, *protocol.TokenUsage, error) {
	state := buildResponsesToChatNonStreamState(rs)
	message := state.message()

	choices := []map[string]any{
		{
			"index":         0,
			"message":       message,
			"finish_reason": mapResponsesFinishReason(rs, len(state.toolCalls) > 0),
		},
	}

	normalizedUsage := usageconv.FromOpenAIResponses(rs.Usage)
	totalInputTokens := normalizedUsage.InputTokens + normalizedUsage.CacheInputTokens
	totalTokens := int(rs.Usage.TotalTokens)
	if totalTokens == 0 {
		totalTokens = totalInputTokens + normalizedUsage.OutputTokens
	}
	usage := map[string]any{
		"prompt_tokens":     totalInputTokens,
		"completion_tokens": normalizedUsage.OutputTokens,
		"total_tokens":      totalTokens,
	}
	if normalizedUsage.CacheInputTokens > 0 {
		usage["prompt_tokens_details"] = map[string]any{
			"cached_tokens": normalizedUsage.CacheInputTokens,
		}
	}
	if normalizedUsage.ReasoningTokens > 0 {
		usage["completion_tokens_details"] = map[string]any{
			"reasoning_tokens": normalizedUsage.ReasoningTokens,
		}
	}

	chatResp := map[string]any{
		"id":      rs.ID,
		"object":  "chat.completion",
		"created": int64(rs.CreatedAt),
		"model":   hc.ResponseModel,
		"choices": choices,
		"usage":   usage,
	}
	hc.GinContext.JSON(http.StatusOK, chatResp)
	return chatResp, usageconv.FromOpenAIResponses(rs.Usage), nil
}

func buildResponsesToChatNonStreamState(rs *responses.Response) *responsesToChatNonStreamState {
	state := &responsesToChatNonStreamState{
		toolCalls: make([]map[string]any, 0),
	}
	if rs == nil {
		return state
	}

	for _, output := range rs.Output {
		switch output.Type {
		case "message":
			for _, contentItem := range output.Content {
				switch contentItem.Type {
				case "output_text", "text":
					state.content.WriteString(contentItem.Text)
				case "refusal":
					state.refusal.WriteString(contentItem.Refusal)
				}
			}
		case "function_call", "custom_tool_call", "mcp_call":
			state.toolCalls = append(state.toolCalls, map[string]any{
				"id":   firstNonEmpty(output.CallID, output.ID),
				"type": "function",
				"function": map[string]any{
					"name":      output.Name,
					"arguments": output.Arguments.OfString,
				},
			})
		}
	}

	return state
}

func (s *responsesToChatNonStreamState) message() map[string]any {
	message := map[string]any{
		"role": "assistant",
	}
	if s == nil {
		return message
	}

	if content := s.content.String(); content != "" {
		message["content"] = content
	}
	if refusal := s.refusal.String(); refusal != "" {
		message["refusal"] = refusal
	}
	if len(s.toolCalls) > 0 {
		message["tool_calls"] = s.toolCalls
	}

	return message
}

func mapResponsesFinishReason(rs *responses.Response, hasToolCalls bool) string {
	if hasToolCalls {
		return "tool_calls"
	}

	if rs != nil && rs.Status == "incomplete" {
		switch rs.IncompleteDetails.Reason {
		case "max_output_tokens":
			return "length"
		case "content_filter":
			return "content_filter"
		}
	}

	return "stop"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
