package adaptor

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/sirupsen/logrus"
	"google.golang.org/genai"
)

// HandleOpenAIToGoogleStreamResponse processes OpenAI streaming events and converts them to Google format
// This handler writes Google-format streaming responses to the gin.Context
func HandleOpenAIToGoogleStreamResponse(c *gin.Context, stream *openaistream.Stream[openai.ChatCompletionChunk], responseModel string) error {
	logrus.Info("Starting OpenAI to Google streaming response handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Panic in OpenAI to Google streaming handler: %v", r)
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				c.Writer.Write([]byte("error: Internal streaming error\n"))
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		if stream != nil {
			if err := stream.Close(); err != nil {
				logrus.Errorf("Error closing OpenAI stream: %v", err)
			}
		}
		logrus.Info("Finished OpenAI to Google streaming response handler")
	}()

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return errors.New("Streaming not supported by this connection")
	}

	// Google format sends GenerateContentResponse objects as JSON
	// Each chunk contains incremental content

	// Track streaming state
	var (
		currentContent   strings.Builder
		currentToolCalls []map[string]interface{}
		outputTokens     int64
	)

	// Process the stream
	for stream.Next() {
		chunk := stream.Current()

		// Check if we have choices
		if len(chunk.Choices) == 0 {
			if chunk.Usage.CompletionTokens > 0 {
				outputTokens = chunk.Usage.CompletionTokens
			}
			continue
		}

		choice := chunk.Choices[0]
		delta := choice.Delta

		// Handle content delta
		if delta.Content != "" {
			currentContent.WriteString(delta.Content)

			// Send incremental response
			googleResp := &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Role: "model",
							Parts: []*genai.Part{
								genai.NewPartFromText(currentContent.String()),
							},
						},
						FinishReason: "",
						Index:        0,
					},
				},
			}
			sendGoogleStreamChunk(c, googleResp, flusher)
		}

		// Handle tool_calls delta
		if len(delta.ToolCalls) > 0 {
			for _, toolCall := range delta.ToolCalls {
				// For simplicity, send the current state of tool calls
				// In a more complex implementation, you'd track incremental updates
				var argsInput map[string]interface{}
				if toolCall.Function.Arguments != "" {
					_ = json.Unmarshal([]byte(toolCall.Function.Arguments), &argsInput)
				}

				currentToolCalls = append(currentToolCalls, map[string]interface{}{
					"id":   toolCall.ID,
					"name": toolCall.Function.Name,
					"args": argsInput,
				})

				// Send incremental response with tool calls
				parts := []*genai.Part{}
				if currentContent.Len() > 0 {
					parts = append(parts, genai.NewPartFromText(currentContent.String()))
				}
				for _, tc := range currentToolCalls {
					if name, ok := tc["name"].(string); ok {
						if args, ok := tc["args"].(map[string]interface{}); ok {
							if id, ok := tc["id"].(string); ok {
								parts = append(parts, &genai.Part{
									FunctionCall: &genai.FunctionCall{
										ID:   id,
										Name: name,
										Args: args,
									},
								})
							}
						}
					}
				}

				googleResp := &genai.GenerateContentResponse{
					Candidates: []*genai.Candidate{
						{
							Content: &genai.Content{
								Role:  "model",
								Parts: parts,
							},
							FinishReason: "",
							Index:        0,
						},
					},
				}
				sendGoogleStreamChunk(c, googleResp, flusher)
			}
		}

		// Track usage
		if chunk.Usage.CompletionTokens > 0 {
			outputTokens = chunk.Usage.CompletionTokens
		}

		// Handle finish_reason (last chunk)
		if choice.FinishReason != "" {
			// Send final response with finish reason
			parts := []*genai.Part{}
			if currentContent.Len() > 0 {
				parts = append(parts, genai.NewPartFromText(currentContent.String()))
			}
			for _, tc := range currentToolCalls {
				if name, ok := tc["name"].(string); ok {
					if args, ok := tc["args"].(map[string]interface{}); ok {
						if id, ok := tc["id"].(string); ok {
							parts = append(parts, &genai.Part{
								FunctionCall: &genai.FunctionCall{
									ID:   id,
									Name: name,
									Args: args,
								},
							})
						}
					}
				}
			}

			googleResp := &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Role:  "model",
							Parts: parts,
						},
						FinishReason: mapOpenAIFinishReasonToGoogle(choice.FinishReason),
						Index:        0,
					},
				},
				UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
					CandidatesTokenCount: int32(outputTokens),
				},
			}
			sendGoogleStreamChunk(c, googleResp, flusher)
			return nil
		}
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		logrus.Errorf("OpenAI stream error: %v", err)
		return nil
	}

	return nil
}

// HandleAnthropicToGoogleStreamResponse processes Anthropic streaming events and converts them to Google format
func HandleAnthropicToGoogleStreamResponse(c *gin.Context, stream *anthropicstream.Stream[anthropic.MessageStreamEventUnion], responseModel string) error {
	logrus.Info("Starting Anthropic to Google streaming response handler")
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Panic in Anthropic to Google streaming handler: %v", r)
			if c.Writer != nil {
				c.Writer.WriteHeader(http.StatusInternalServerError)
				c.Writer.Write([]byte("error: Internal streaming error\n"))
				if flusher, ok := c.Writer.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
		if stream != nil {
			if err := stream.Close(); err != nil {
				logrus.Errorf("Error closing Anthropic stream: %v", err)
			}
		}
		logrus.Info("Finished Anthropic to Google streaming response handler")
	}()

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return errors.New("Streaming not supported by this connection")
	}

	// Google format sends GenerateContentResponse objects as JSON
	var (
		currentContent   strings.Builder
		currentToolCalls []map[string]interface{}
		outputTokens     int64
	)

	// Process the stream
	for stream.Next() {
		event := stream.Current()

		switch event.Type {
		case "content_block_delta":
			// Text delta - send as Google chunk
			if event.Delta.Type == "text_delta" && event.Delta.Text != "" {
				currentContent.WriteString(event.Delta.Text)

				googleResp := &genai.GenerateContentResponse{
					Candidates: []*genai.Candidate{
						{
							Content: &genai.Content{
								Role: "model",
								Parts: []*genai.Part{
									genai.NewPartFromText(currentContent.String()),
								},
							},
							FinishReason: "",
							Index:        0,
						},
					},
				}
				sendGoogleStreamChunk(c, googleResp, flusher)
			}

		case "content_block_start":
			if event.ContentBlock.Type == "tool_use" {
				// New tool use block
				currentToolCalls = append(currentToolCalls, map[string]interface{}{
					"id":   event.ContentBlock.ID,
					"name": event.ContentBlock.Name,
					"args": event.ContentBlock.Input,
				})

				// Send with tool call
				parts := []*genai.Part{}
				if currentContent.Len() > 0 {
					parts = append(parts, genai.NewPartFromText(currentContent.String()))
				}
				for _, tc := range currentToolCalls {
					if name, ok := tc["name"].(string); ok {
						if args, ok := tc["args"].(map[string]interface{}); ok {
							if id, ok := tc["id"].(string); ok {
								parts = append(parts, &genai.Part{
									FunctionCall: &genai.FunctionCall{
										ID:   id,
										Name: name,
										Args: args,
									},
								})
							}
						}
					}
				}

				googleResp := &genai.GenerateContentResponse{
					Candidates: []*genai.Candidate{
						{
							Content: &genai.Content{
								Role:  "model",
								Parts: parts,
							},
							FinishReason: "",
							Index:        0,
						},
					},
				}
				sendGoogleStreamChunk(c, googleResp, flusher)
			}

		case "message_delta":
			// Message delta (includes usage info)
			if event.Usage.OutputTokens != 0 {
				outputTokens = event.Usage.OutputTokens
			}

		case "message_stop":
			// Send final response with finish reason
			parts := []*genai.Part{}
			if currentContent.Len() > 0 {
				parts = append(parts, genai.NewPartFromText(currentContent.String()))
			}
			for _, tc := range currentToolCalls {
				if name, ok := tc["name"].(string); ok {
					if args, ok := tc["args"].(map[string]interface{}); ok {
						if id, ok := tc["id"].(string); ok {
							parts = append(parts, &genai.Part{
								FunctionCall: &genai.FunctionCall{
									ID:   id,
									Name: name,
									Args: args,
								},
							})
						}
					}
				}
			}

			googleResp := &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content: &genai.Content{
							Role:  "model",
							Parts: parts,
						},
						FinishReason: mapAnthropicFinishReasonToGoogle("end_turn"),
						Index:        0,
					},
				},
				UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
					CandidatesTokenCount: int32(outputTokens),
				},
			}
			sendGoogleStreamChunk(c, googleResp, flusher)
			return nil
		}
	}

	// Check for stream errors
	if err := stream.Err(); err != nil {
		logrus.Errorf("Anthropic stream error: %v", err)
		return nil
	}

	return nil
}

// sendGoogleStreamChunk sends a GenerateContentResponse as a JSON chunk
func sendGoogleStreamChunk(c *gin.Context, resp *genai.GenerateContentResponse, flusher http.Flusher) {
	// Use the SDK's JSON marshal to ensure proper format
	chunkJSON, err := json.Marshal(resp)
	if err != nil {
		logrus.Errorf("Failed to marshal Google stream chunk: %v", err)
		return
	}
	c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", string(chunkJSON))))
	flusher.Flush()
}
