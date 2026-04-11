// Package virtualserver provides the HTTP handler for virtual model endpoints.
// It serves OpenAI Chat and Anthropic Messages API formats backed by a
// virtualmodel.Registry of deterministic VirtualModel instances.
package virtualserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/virtualmodel"
)

// Handler handles HTTP requests for virtual model endpoints.
type Handler struct {
	registry *virtualmodel.Registry
}

// NewHandler creates a new Handler backed by the given registry.
func NewHandler(registry *virtualmodel.Registry) *Handler {
	return &Handler{registry: registry}
}

// OpenAIModelsResponse is the OpenAI models list response format.
type OpenAIModelsResponse struct {
	Object string               `json:"object"`
	Data   []virtualmodel.Model `json:"data"`
}

// ListModels handles GET /virtual/v1/models.
func (h *Handler) ListModels(c *gin.Context) {
	models := h.registry.ListModels()
	c.JSON(http.StatusOK, OpenAIModelsResponse{
		Object: "list",
		Data:   models,
	})
}

// ChatCompletions handles POST /virtual/v1/chat/completions.
func (h *Handler) ChatCompletions(c *gin.Context) {
	var req ChatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "Invalid request body: " + err.Error(),
			"type":    "invalid_request_error",
		}})
		return
	}

	if req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"message": "Model is required",
			"type":    "invalid_request_error",
		}})
		return
	}

	vm := h.registry.Get(req.Model)
	if vm == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": fmt.Sprintf("Model not found: %s", req.Model),
			"type":    "invalid_request_error",
		}})
		return
	}

	switch vm.GetType() {
	case virtualmodel.VirtualModelTypeProxy:
		if req.Stream {
			h.handleProxyStreaming(c, &req, vm)
		} else {
			h.handleProxyRequest(c, &req, vm)
		}
		return
	case virtualmodel.VirtualModelTypeTool:
		if req.Stream {
			h.handleToolModelStreaming(c, &req, vm)
		} else {
			h.handleToolModelNonStreaming(c, &req, vm)
		}
		return
	}

	if req.Stream {
		h.handleStreaming(c, &req, vm)
	} else {
		h.handleNonStreaming(c, &req, vm)
	}
}

func (h *Handler) handleProxyRequest(c *gin.Context, req *ChatCompletionRequest, vm *virtualmodel.VirtualModel) {
	if vm.GetTransformer() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": "Proxy model has no transformer configured",
			"type":    "internal_error",
		}})
		return
	}

	logrus.Infof("Proxy model %s with transformer, messages: %d", req.Model, len(req.Messages))

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

func (h *Handler) handleNonStreaming(c *gin.Context, req *ChatCompletionRequest, vm *virtualmodel.VirtualModel) {
	if delay := vm.GetDelay(); delay > 0 {
		time.Sleep(delay)
	}

	content := vm.GetContent()
	c.JSON(http.StatusOK, ChatCompletionResponse{
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
	})
}

func (h *Handler) handleStreaming(c *gin.Context, req *ChatCompletionRequest, vm *virtualmodel.VirtualModel) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	if _, ok := c.Writer.(http.Flusher); !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": "Streaming not supported by this connection",
			"type":    "api_error",
			"code":    "streaming_unsupported",
		}})
		return
	}

	c.Stream(func(w io.Writer) bool {
		chunks := vm.GetStreamChunks()
		chunkIndex := 0

		for i, chunk := range chunks {
			select {
			case <-c.Request.Context().Done():
				logrus.Debug("Client disconnected during streaming")
				return false
			default:
			}

			if delay := vm.GetDelay(); delay > 0 {
				time.Sleep(delay / time.Duration(len(chunks)))
			} else {
				time.Sleep(50 * time.Millisecond)
			}

			chunkIndex = i + 1
			data, _ := json.Marshal(ChatCompletionStreamResponse{
				ID:      fmt.Sprintf("chatcmpl-virtual-%d", time.Now().Unix()),
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   req.Model,
				Choices: []StreamChoice{{
					Index:        i,
					Delta:        Delta{Content: chunk},
					FinishReason: nil,
				}},
			})
			c.SSEvent("", string(data))
			c.Writer.Flush()
		}

		data, _ := json.Marshal(ChatCompletionStreamResponse{
			ID:      fmt.Sprintf("chatcmpl-virtual-%d", time.Now().Unix()),
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []StreamChoice{{
				Index:        chunkIndex,
				Delta:        Delta{},
				FinishReason: stringPtr("stop"),
			}},
		})
		c.SSEvent("", string(data))
		c.Writer.Flush()

		c.SSEvent("", "[DONE]")
		c.Writer.Flush()
		return false
	})
}

func (h *Handler) handleProxyStreaming(c *gin.Context, req *ChatCompletionRequest, vm *virtualmodel.VirtualModel) {
	if vm.GetTransformer() == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": "Proxy model has no transformer configured",
			"type":    "internal_error",
		}})
		return
	}

	logrus.Infof("Proxy model %s with transformer (streaming), messages: %d", req.Model, len(req.Messages))

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	if _, ok := c.Writer.(http.Flusher); !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": "Streaming not supported by this connection",
			"type":    "api_error",
			"code":    "streaming_unsupported",
		}})
		return
	}

	c.Stream(func(w io.Writer) bool {
		content := fmt.Sprintf("[Proxy Mode: Request was received by %s. The transformer is configured and ready. Implement actual LLM proxying to complete the flow.]", req.Model)

		data, _ := json.Marshal(ChatCompletionStreamResponse{
			ID:      fmt.Sprintf("chatcmpl-proxy-%d", time.Now().Unix()),
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []StreamChoice{{
				Index:        0,
				Delta:        Delta{Content: content},
				FinishReason: nil,
			}},
		})
		c.SSEvent("", string(data))
		c.Writer.Flush()

		time.Sleep(10 * time.Millisecond)

		data, _ = json.Marshal(ChatCompletionStreamResponse{
			ID:      fmt.Sprintf("chatcmpl-proxy-%d", time.Now().Unix()),
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []StreamChoice{{
				Index:        0,
				Delta:        Delta{},
				FinishReason: stringPtr("stop"),
			}},
		})
		c.SSEvent("", string(data))
		c.Writer.Flush()

		c.SSEvent("", "[DONE]")
		c.Writer.Flush()
		return false
	})
}

func (h *Handler) handleToolModelNonStreaming(c *gin.Context, req *ChatCompletionRequest, vm *virtualmodel.VirtualModel) {
	if delay := vm.GetDelay(); delay > 0 {
		time.Sleep(delay)
	}

	toolCall := vm.GetToolCall()
	if toolCall == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tool call config not found"})
		return
	}

	argsJSON, _ := json.Marshal(toolCall.Arguments)

	content := ""
	if msg, ok := toolCall.Arguments["message"].(string); ok {
		content = msg
	} else if question, ok := toolCall.Arguments["question"].(string); ok {
		content = question
	}

	c.JSON(http.StatusOK, ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-virtual-%d", time.Now().Unix()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []Choice{{
			Index: 0,
			Message: Message{
				Role:    "assistant",
				Content: content,
				ToolCalls: []ToolCall{{
					ID:   fmt.Sprintf("call_%d", time.Now().Unix()),
					Type: "function",
					Function: FunctionCall{
						Name:      toolCall.Name,
						Arguments: string(argsJSON),
					},
				}},
			},
			FinishReason: "tool_calls",
		}},
		Usage: Usage{
			PromptTokens:     estimateTokens(req.Messages),
			CompletionTokens: estimateTokensString(content) + 20,
			TotalTokens:      estimateTokens(req.Messages) + estimateTokensString(content) + 20,
		},
	})
}

func (h *Handler) handleToolModelStreaming(c *gin.Context, req *ChatCompletionRequest, vm *virtualmodel.VirtualModel) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")

	if _, ok := c.Writer.(http.Flusher); !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": "Streaming not supported",
			"type":    "api_error",
		}})
		return
	}

	toolCall := vm.GetToolCall()
	if toolCall == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tool call config not found"})
		return
	}

	argsJSON, _ := json.Marshal(toolCall.Arguments)

	content := ""
	if msg, ok := toolCall.Arguments["message"].(string); ok {
		content = msg
	} else if question, ok := toolCall.Arguments["question"].(string); ok {
		content = question
	}

	c.Stream(func(w io.Writer) bool {
		if delay := vm.GetDelay(); delay > 0 {
			time.Sleep(delay)
		}

		data, _ := json.Marshal(ChatCompletionStreamResponse{
			ID: fmt.Sprintf("chatcmpl-virtual-%d", time.Now().Unix()), Object: "chat.completion.chunk",
			Created: time.Now().Unix(), Model: req.Model,
			Choices: []StreamChoice{{Index: 0, Delta: Delta{Role: "assistant"}}},
		})
		c.SSEvent("", string(data))
		c.Writer.Flush()
		time.Sleep(50 * time.Millisecond)

		data, _ = json.Marshal(ChatCompletionStreamResponse{
			ID: fmt.Sprintf("chatcmpl-virtual-%d", time.Now().Unix()), Object: "chat.completion.chunk",
			Created: time.Now().Unix(), Model: req.Model,
			Choices: []StreamChoice{{Index: 0, Delta: Delta{Content: content}}},
		})
		c.SSEvent("", string(data))
		c.Writer.Flush()
		time.Sleep(50 * time.Millisecond)

		data, _ = json.Marshal(ChatCompletionStreamResponse{
			ID: fmt.Sprintf("chatcmpl-virtual-%d", time.Now().Unix()), Object: "chat.completion.chunk",
			Created: time.Now().Unix(), Model: req.Model,
			Choices: []StreamChoice{{
				Index: 0,
				Delta: Delta{ToolCalls: []ToolCall{{
					ID:   fmt.Sprintf("call_%d", time.Now().Unix()),
					Type: "function",
					Function: FunctionCall{
						Name:      toolCall.Name,
						Arguments: string(argsJSON),
					},
				}}},
				FinishReason: stringPtr("tool_calls"),
			}},
		})
		c.SSEvent("", string(data))
		c.Writer.Flush()

		c.SSEvent("", "[DONE]")
		c.Writer.Flush()
		return false
	})
}
