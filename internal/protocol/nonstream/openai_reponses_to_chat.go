package nonstream

import "github.com/openai/openai-go/v3/responses"

// OpenAIResponsesToChat converts a Responses API response to Chat Completions format.
// This is used when the client expects Chat format but the provider uses Responses API.
func OpenAIResponsesToChat(resp *responses.Response, responseModel string) map[string]any {
	// Extract text content from output
	// Response output can have various types: "message", "function_call", etc.
	content := ""
	for _, output := range resp.Output {
		if output.Type == "message" {
			// For message type, extract text from content array
			for _, contentItem := range output.Content {
				// ContentItemUnion has a Type field to distinguish variants
				if contentItem.Type == "output_text" || contentItem.Type == "text" {
					content += contentItem.Text
				}
			}
		}
	}

	// Build Chat Completion response structure
	choices := []map[string]any{
		{
			"index": 0,
			"message": map[string]any{
				"role":    "assistant",
				"content": content,
			},
			"finish_reason": "stop",
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
