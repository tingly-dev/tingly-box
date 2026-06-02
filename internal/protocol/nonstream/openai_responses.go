package nonstream

import (
	"encoding/json"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/usage"
)

// BuildResponsesPayloadFromChat converts a Chat completion response to Responses API format.
func BuildResponsesPayloadFromChat(resp *openai.ChatCompletion, responseModel, actualModel string) map[string]any {
	model := responseModel
	if model == "" {
		model = actualModel
	}

	messageContent := ""
	if len(resp.Choices) > 0 {
		messageContent = resp.Choices[0].Message.Content
	}

	output := []map[string]any{}
	if messageContent != "" {
		output = append(output, map[string]any{
			"type":         "output_text",
			"text":         messageContent,
			"output_index": 0,
		})
	}

	return map[string]any{
		"id":     resp.ID,
		"object": "response",
		"model":  model,
		"status": "completed",
		"output": output,
		"usage": map[string]any{
			"input_tokens":  resp.Usage.PromptTokens,
			"output_tokens": resp.Usage.CompletionTokens,
			"total_tokens":  resp.Usage.PromptTokens + resp.Usage.CompletionTokens,
		},
	}
}

// BuildResponsesPayloadFromAnthropicBeta converts an Anthropic Beta message response to Responses API format.
func BuildResponsesPayloadFromAnthropicBeta(resp *anthropic.BetaMessage, responseModel, actualModel string) map[string]any {
	model := responseModel
	if model == "" {
		model = actualModel
	}

	output := []map[string]any{}
	outputIndex := 0
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			if block.Text == "" {
				continue
			}
			output = append(output, map[string]any{
				"type":         "output_text",
				"text":         block.Text,
				"output_index": outputIndex,
			})
			outputIndex++
		case "tool_use":
			argsJSON := "{}"
			if block.Input != nil {
				if raw, err := json.Marshal(block.Input); err == nil {
					argsJSON = string(raw)
				}
			}
			output = append(output, map[string]any{
				"type":         "function_call",
				"id":           block.ID,
				"name":         block.Name,
				"arguments":    argsJSON,
				"output_index": outputIndex,
			})
			outputIndex++
		}
	}

	return map[string]any{
		"id":     resp.ID,
		"object": "response",
		"model":  model,
		"status": "completed",
		"output": output,
		"usage": map[string]any{
			"input_tokens":  resp.Usage.InputTokens,
			"output_tokens": resp.Usage.OutputTokens,
			"total_tokens":  resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}
}

// HandleResponsesToOpenAIChatNonStream writes a Responses API response as OpenAI Chat format.
// Corresponds to stream.HandleResponsesToOpenAIChatStream.
func HandleResponsesToOpenAIChatNonStream(hc *protocol.HandleContext, resp *responses.Response) (*protocol.TokenUsage, error) {
	chatResp := OpenAIResponsesToChat(resp, hc.ResponseModel)
	hc.GinContext.JSON(http.StatusOK, chatResp)
	return usage.FromOpenAIResponses(resp.Usage), nil
}

// HandleOpenAIChatToResponsesNonStream writes an OpenAI Chat response as Responses API format.
// Corresponds to stream.HandleOpenAIChatToResponsesStream.
func HandleOpenAIChatToResponsesNonStream(hc *protocol.HandleContext, resp *openai.ChatCompletion, requestModel string) (*protocol.TokenUsage, error) {
	payload := BuildResponsesPayloadFromChat(resp, hc.ResponseModel, requestModel)
	hc.GinContext.JSON(http.StatusOK, payload)
	return usage.FromOpenAIChatCompletion(resp.Usage), nil
}

// HandleAnthropicBetaToResponsesNonStream writes an Anthropic Beta response as Responses API format.
// Corresponds to stream.HandleAnthropicBetaToOpenAIResponsesStream.
func HandleAnthropicBetaToResponsesNonStream(hc *protocol.HandleContext, resp *anthropic.BetaMessage, requestModel string) (*protocol.TokenUsage, error) {
	payload := BuildResponsesPayloadFromAnthropicBeta(resp, hc.ResponseModel, requestModel)
	hc.GinContext.JSON(http.StatusOK, payload)
	return usage.FromAnthropicBetaMessage(resp.Usage), nil
}
