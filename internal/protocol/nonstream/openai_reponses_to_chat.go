package nonstream

import (
	"strings"

	"github.com/openai/openai-go/v3/responses"
)

type responsesToChatNonStreamState struct {
	content   strings.Builder
	refusal   strings.Builder
	toolCalls []map[string]any
}

// OpenAIResponsesToChat converts a Responses API response to Chat Completions format.
// This is used when the client expects Chat format but the provider uses Responses API.
func OpenAIResponsesToChat(resp *responses.Response, responseModel string) map[string]any {
	state := buildResponsesToChatNonStreamState(resp)
	message := state.message()

	choices := []map[string]any{
		{
			"index":         0,
			"message":       message,
			"finish_reason": mapResponsesFinishReason(resp, len(state.toolCalls) > 0),
		},
	}

	return map[string]any{
		"id":      resp.ID,
		"object":  "chat.completion",
		"created": int64(resp.CreatedAt),
		"model":   responseModel,
		"choices": choices,
		"usage": map[string]any{
			"prompt_tokens":     resp.Usage.InputTokens,
			"completion_tokens": resp.Usage.OutputTokens,
			"total_tokens":      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}
}

func buildResponsesToChatNonStreamState(resp *responses.Response) *responsesToChatNonStreamState {
	state := &responsesToChatNonStreamState{
		toolCalls: make([]map[string]any, 0),
	}
	if resp == nil {
		return state
	}

	for _, output := range resp.Output {
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

func mapResponsesFinishReason(resp *responses.Response, hasToolCalls bool) string {
	if hasToolCalls {
		return "tool_calls"
	}

	if resp != nil && resp.Status == "incomplete" {
		switch resp.IncompleteDetails.Reason {
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
