package virtualmodel

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// Handler handles HTTP requests for virtual models
type Handler struct {
	registry *Registry
}

// NewHandler creates a new virtual model handler
func NewHandler(registry *Registry) *Handler {
	return &Handler{
		registry: registry,
	}
}

// OpenAIModelsResponse represents OpenAI's models API response format
type OpenAIModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

// ListModels handles the GET /virtual/v1/models endpoint
func (h *Handler) ListModels(c *gin.Context) {
	models := h.registry.ListModels()
	c.JSON(http.StatusOK, OpenAIModelsResponse{
		Object: "list",
		Data:   models,
	})
}

// ChatCompletions handles the POST /virtual/v1/chat/completions endpoint
func (h *Handler) ChatCompletions(c *gin.Context) {
	var req ChatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Invalid request body: " + err.Error(),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Validate model
	if req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Model is required",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Get virtual model
	virtualModel := h.registry.Get(req.Model)
	if virtualModel == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Model not found: %s", req.Model),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	// Check if this is a proxy virtual model
	if virtualModel.IsProxy() {
		if req.Stream {
			h.handleProxyStreaming(c, &req, virtualModel)
		} else {
			h.handleProxyRequest(c, &req, virtualModel)
		}
		return
	}

	// Handle streaming vs non-streaming for mock models
	if req.Stream {
		h.handleStreaming(c, &req, virtualModel)
	} else {
		h.handleNonStreaming(c, &req, virtualModel)
	}
}

// handleProxyRequest handles proxy mode virtual models
func (h *Handler) handleProxyRequest(c *gin.Context, req *ChatCompletionRequest, vm *VirtualModel) {
	// Get transformer
	transformer := vm.GetTransformer()
	if transformer == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Proxy model has no transformer configured",
				"type":    "internal_error",
			},
		})
		return
	}

	logrus.Infof("Proxy model %s with transformer, messages: %d", req.Model, len(req.Messages))

	// For the initial implementation, we return a response indicating compression is available
	// The actual proxying to real LLM provider will be implemented by the user
	c.JSON(http.StatusOK, ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-proxy-%d", time.Now().Unix()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []Choice{{
			Index: 0,
			Message: Message{
				Role:    "assistant",
				Content: fmt.Sprintf("[Proxy Mode: Request was received by %s. The transformer is configured and ready. Implement actual LLM proxying to complete the flow.]", req.Model),
			},
			FinishReason: "stop",
		}},
		Usage: Usage{
			PromptTokens:     estimateTokens(req.Messages),
			CompletionTokens: 50,
			TotalTokens:      estimateTokens(req.Messages) + 50,
		},
	})
}

// handleNonStreaming handles non-streaming requests
func (h *Handler) handleNonStreaming(c *gin.Context, req *ChatCompletionRequest, vm *VirtualModel) {
	// Apply delay if configured
	if delay := vm.GetDelay(); delay > 0 {
		time.Sleep(delay)
	}

	content := vm.GetContent()
	resp := ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-virtual-%d", time.Now().Unix()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []Choice{{
			Index: 0,
			Message: Message{
				Role:    "assistant",
				Content: content,
			},
			FinishReason: "stop",
		}},
		Usage: Usage{
			PromptTokens:     estimateTokens(req.Messages),
			CompletionTokens: estimateTokensString(content),
			TotalTokens:      estimateTokens(req.Messages) + estimateTokensString(content),
		},
	}

	c.JSON(http.StatusOK, resp)
}

// handleStreaming handles streaming requests with SSE
func (h *Handler) handleStreaming(c *gin.Context, req *ChatCompletionRequest, vm *VirtualModel) {
	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	// Check if streaming is supported
	_, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Streaming not supported by this connection",
				"type":    "api_error",
				"code":    "streaming_unsupported",
			},
		})
		return
	}

	// Use gin.Stream for proper streaming handling
	c.Stream(func(w io.Writer) bool {
		chunks := vm.GetStreamChunks()
		content := ""
		chunkIndex := 0

		for i, chunk := range chunks {
			// Check if client disconnected
			select {
			case <-c.Request.Context().Done():
				logrus.Debug("Client disconnected during streaming")
				return false
			default:
			}

			// Apply delay between chunks
			if delay := vm.GetDelay(); delay > 0 {
				time.Sleep(delay / time.Duration(len(chunks)))
			} else {
				time.Sleep(50 * time.Millisecond) // Default chunk delay
			}

			content += chunk
			chunkIndex = i + 1

			// Create streaming chunk response
			streamResp := ChatCompletionStreamResponse{
				ID:      fmt.Sprintf("chatcmpl-virtual-%d", time.Now().Unix()),
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   req.Model,
				Choices: []StreamChoice{{
					Index: i,
					Delta: Delta{
						Content: chunk,
					},
					FinishReason: nil,
				}},
			}

			// Send SSE event
			data, _ := json.Marshal(streamResp)
			c.SSEvent("", string(data))
			c.Writer.Flush()
		}

		// Send final chunk with finish_reason
		finalResp := ChatCompletionStreamResponse{
			ID:      fmt.Sprintf("chatcmpl-virtual-%d", time.Now().Unix()),
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []StreamChoice{{
				Index:        chunkIndex,
				Delta:        Delta{},
				FinishReason: stringPtr("stop"),
			}},
		}

		data, _ := json.Marshal(finalResp)
		c.SSEvent("", string(data))
		c.Writer.Flush()

		// Send [DONE] message
		c.SSEvent("", "[DONE]")
		c.Writer.Flush()

		return false // Stop streaming
	})
}

// handleProxyStreaming handles proxy mode with streaming response
func (h *Handler) handleProxyStreaming(c *gin.Context, req *ChatCompletionRequest, vm *VirtualModel) {
	// Get transformer
	transformer := vm.GetTransformer()
	if transformer == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Proxy model has no transformer configured",
				"type":    "internal_error",
			},
		})
		return
	}

	logrus.Infof("Proxy model %s with transformer (streaming), messages: %d", req.Model, len(req.Messages))

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	// Check if streaming is supported
	_, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Streaming not supported by this connection",
				"type":    "api_error",
				"code":    "streaming_unsupported",
			},
		})
		return
	}

	// Use gin.Stream for proper streaming handling
	c.Stream(func(w io.Writer) bool {
		// Create the response content
		content := fmt.Sprintf("[Proxy Mode: Request was received by %s. The transformer is configured and ready. Implement actual LLM proxying to complete the flow.]", req.Model)

		// Create streaming chunk response
		streamResp := ChatCompletionStreamResponse{
			ID:      fmt.Sprintf("chatcmpl-proxy-%d", time.Now().Unix()),
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []StreamChoice{{
				Index: 0,
				Delta: Delta{
					Content: content,
				},
				FinishReason: nil,
			}},
		}

		// Send SSE event
		data, _ := json.Marshal(streamResp)
		c.SSEvent("", string(data))
		c.Writer.Flush()

		// Small delay to simulate streaming
		time.Sleep(10 * time.Millisecond)

		// Send final chunk with finish_reason
		finalResp := ChatCompletionStreamResponse{
			ID:      fmt.Sprintf("chatcmpl-proxy-%d", time.Now().Unix()),
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []StreamChoice{{
				Index:        0,
				Delta:        Delta{},
				FinishReason: stringPtr("stop"),
			}},
		}

		data, _ = json.Marshal(finalResp)
		c.SSEvent("", string(data))
		c.Writer.Flush()

		// Send [DONE] message
		c.SSEvent("", "[DONE]")
		c.Writer.Flush()

		return false // Stop streaming
	})
}

// estimateTokens estimates token count (rough approximation)
func estimateTokens(messages []Message) int {
	total := 0
	for _, msg := range messages {
		total += estimateTokensString(msg.Content)
		total += 5 // Overhead for role, etc.
	}
	return total
}

// estimateTokensString estimates token count for a string
func estimateTokensString(s string) int {
	// Rough estimate: ~4 characters per token
	if len(s) == 0 {
		return 0
	}
	return (len(s) + 3) / 4
}

// stringPtr returns a pointer to a string
func stringPtr(s string) *string {
	return &s
}
