package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"tingly-box/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/sirupsen/logrus"
)

// forwardOpenAIRequest forwards the request to the selected provider using OpenAI library
func (s *Server) forwardOpenAIRequest(provider *config.Provider, req *RequestWrapper) (*openai.ChatCompletion, error) {
	// Create OpenAI client with provider configuration
	client := openai.NewClient(
		option.WithAPIKey(provider.Token),
		option.WithBaseURL(provider.APIBase),
	)
	logrus.Infof("provider: %s", provider.Name)

	// Since RequestWrapper is a type alias to openai.ChatCompletionNewParams,
	// we can directly use it as the request parameters
	chatReq := *req

	// Make the request using OpenAI library
	chatCompletion, err := client.Chat.Completions.New(context.Background(), chatReq)
	if err != nil {
		logrus.Error(err)
		return nil, fmt.Errorf("failed to create chat completion: %w", err)
	}

	return chatCompletion, nil
}

// Helper functions to convert between formats
func (s *Server) convertAnthropicToOpenAI(anthropicReq *AnthropicMessagesRequest) *RequestWrapper {
	openaiReq := &RequestWrapper{
		Model: anthropicReq.Model,
	}

	// Convert MaxTokens - use option helper functions if available, otherwise skip
	// We'll handle these parameters in the forwardOpenAIRequest function
	s.setRequestParams(openaiReq, anthropicReq)

	// Convert messages
	for _, msg := range anthropicReq.Messages {
		if msg.Role == "user" {
			openaiMsg := openai.UserMessage(msg.Content)
			openaiReq.Messages = append(openaiReq.Messages, openaiMsg)
		} else if msg.Role == "assistant" {
			openaiMsg := openai.AssistantMessage(msg.Content)
			openaiReq.Messages = append(openaiReq.Messages, openaiMsg)
		}
	}

	// Convert system message
	if anthropicReq.System != "" {
		systemMsg := openai.SystemMessage(anthropicReq.System)
		// Add system message at the beginning
		openaiReq.Messages = append([]openai.ChatCompletionMessageParamUnion{systemMsg}, openaiReq.Messages...)
	}

	// We'll handle stop sequences in the forwardOpenAIRequest function

	return openaiReq
}

// setRequestParams handles the optional parameters that need special handling
func (s *Server) setRequestParams(openaiReq *RequestWrapper, anthropicReq *AnthropicMessagesRequest) {
	// Note: This is a placeholder for setting optional parameters
	// In practice, you might need to use the OpenAI SDK's option helpers
	// For now, we'll skip optional parameters and focus on the core functionality
}

func (s *Server) convertOpenAIToAnthropic(openaiResp *openai.ChatCompletion, model string) AnthropicMessagesResponse {
	response := AnthropicMessagesResponse{
		ID:           fmt.Sprintf("msg_%d", time.Now().Unix()),
		Type:         "message",
		Role:         "assistant",
		Model:        model,
		StopReason:   "end_turn",
		StopSequence: "",
		Usage: AnthropicUsage{
			InputTokens:  int(openaiResp.Usage.PromptTokens),
			OutputTokens: int(openaiResp.Usage.CompletionTokens),
		},
	}

	// Convert choices to content
	for _, choice := range openaiResp.Choices {
		content := choice.Message.Content
		if content != "" {
			anthropicContent := AnthropicContent{
				Type: "text",
				Text: content,
			}
			response.Content = append(response.Content, anthropicContent)
		}
	}

	return response
}

// forwardOpenAIStreamRequest forwards the streaming request to the selected provider using OpenAI library
func (s *Server) forwardOpenAIStreamRequest(provider *config.Provider, req *RequestWrapper) (*ssestream.Stream[openai.ChatCompletionChunk], error) {
	// Create OpenAI client with provider configuration
	client := openai.NewClient(
		option.WithAPIKey(provider.Token),
		option.WithBaseURL(provider.APIBase),
	)
	logrus.Infof("provider: %s (streaming)", provider.Name)

	// Since RequestWrapper is a type alias to openai.ChatCompletionNewParams,
	// we can directly use it as the request parameters
	chatReq := *req

	// Make the streaming request using OpenAI library
	stream := client.Chat.Completions.NewStreaming(context.Background(), chatReq)

	return stream, nil
}

// handleOpenAIStreamResponse processes the streaming response and sends it to the client
func (s *Server) handleOpenAIStreamResponse(c *gin.Context, stream *ssestream.Stream[openai.ChatCompletionChunk], responseModel string) {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Panic in streaming handler: %v", r)
			// Try to send an error event if possible
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				c.Writer.Write([]byte("data: {\"error\":{\"message\":\"Internal streaming error\",\"type\":\"internal_error\"}}\n\n"))
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		// Ensure stream is always closed
		if stream != nil {
			if err := stream.Close(); err != nil {
				logrus.Errorf("Error closing stream: %v", err)
			}
		}
	}()

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	// Create a flusher to ensure immediate sending of data
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: "Streaming not supported by this connection",
				Type:    "api_error",
				Code:    "streaming_unsupported",
			},
		})
		return
	}

	// Process the stream
	for stream.Next() {
		chatChunk := stream.Current()

		// Check if we have choices and they're not empty
		if len(chatChunk.Choices) == 0 {
			continue
		}

		choice := chatChunk.Choices[0]

		// Build delta map - include all fields, JSON marshaling will handle empty values
		delta := map[string]interface{}{
			"role":          choice.Delta.Role,
			"content":       choice.Delta.Content,
			"refusal":       choice.Delta.Refusal,
			"function_call": choice.Delta.FunctionCall,
			"tool_calls":    choice.Delta.ToolCalls,
		}

		// Prepare the chunk in OpenAI format
		chunk := map[string]interface{}{
			"id":      chatChunk.ID,
			"object":  "chat.completion.chunk",
			"created": chatChunk.Created,
			"model":   responseModel,
			"choices": []map[string]interface{}{
				{
					"index":         choice.Index,
					"delta":         delta,
					"finish_reason": choice.FinishReason,
					"logprobs":      choice.Logprobs,
				},
			},
		}

		// Add usage if present (usually only in the last chunk)
		if chatChunk.Usage.PromptTokens != 0 || chatChunk.Usage.CompletionTokens != 0 {
			chunk["usage"] = chatChunk.Usage
		}

		// Add system fingerprint if present
		if chatChunk.SystemFingerprint != "" {
			chunk["system_fingerprint"] = chatChunk.SystemFingerprint
		}

		// Convert to JSON and send as SSE
		chunkJSON, err := json.Marshal(chunk)
		if err != nil {
			logrus.Errorf("Failed to marshal chunk: %v", err)
			continue
		}

		// Send the chunk
		c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(chunkJSON))))
		flusher.Flush()
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		logrus.Errorf("Stream error: %v", err)

		// Send error event
		errorChunk := map[string]interface{}{
			"error": map[string]interface{}{
				"message": err.Error(),
				"type":    "stream_error",
				"code":    "stream_failed",
			},
		}

		errorJSON, marshalErr := json.Marshal(errorChunk)
		if marshalErr != nil {
			logrus.Errorf("Failed to marshal error chunk: %v", marshalErr)
			c.Writer.Write([]byte(fmt.Sprintf("data: {\"error\":{\"message\":\"Failed to marshal error\",\"type\":\"internal_error\"}}\n\n")))
		} else {
			c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(errorJSON))))
		}
		flusher.Flush()
		return
	}

	// Send the final [DONE] message
	c.Writer.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()
}
