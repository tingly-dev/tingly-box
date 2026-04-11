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
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"

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

	openaiVM := h.registry.GetOpenAIChatVM(req.Model)
	if openaiVM == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": gin.H{
			"message": fmt.Sprintf("Model %q does not support OpenAI Chat. Use the Anthropic Messages endpoint (/virtual/v1/messages) instead.", req.Model),
			"type":    "not_implemented_error",
		}})
		return
	}

	if req.Stream {
		h.handleOpenAIStreaming(c, &req, openaiVM)
	} else {
		h.handleOpenAINonStreaming(c, &req, openaiVM)
	}
}

// Messages handles POST /virtual/v1/messages (Anthropic format).
func (h *Handler) Messages(c *gin.Context) {
	var req AnthropicMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "error": gin.H{
			"type":    "invalid_request_error",
			"message": "Invalid request body: " + err.Error(),
		}})
		return
	}
	if req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "error": gin.H{
			"type":    "invalid_request_error",
			"message": "Model is required",
		}})
		return
	}

	vm := h.registry.Get(req.Model)
	if vm == nil {
		c.JSON(http.StatusNotFound, gin.H{"type": "error", "error": gin.H{
			"type":    "not_found_error",
			"message": fmt.Sprintf("Model not found: %s", req.Model),
		}})
		return
	}

	anthropicVM := h.registry.GetAnthropicVM(req.Model)
	if anthropicVM == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"type": "error", "error": gin.H{
			"type":    "not_implemented_error",
			"message": fmt.Sprintf("Model %q does not support the Anthropic Messages protocol.", req.Model),
		}})
		return
	}

	if req.Stream {
		h.handleAnthropicStreaming(c, &req, anthropicVM)
	} else {
		h.handleAnthropicNonStreaming(c, &req, anthropicVM)
	}
}

// ── Anthropic handlers ────────────────────────────────────────────────────────

func (h *Handler) handleAnthropicNonStreaming(c *gin.Context, req *AnthropicMessageRequest, vm virtualmodel.AnthropicVirtualModel) {
	if d := vm.SimulatedDelay(); d > 0 {
		time.Sleep(d)
	}

	resp, err := vm.HandleAnthropic(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"type": "error", "error": gin.H{
			"type":    "api_error",
			"message": err.Error(),
		}})
		return
	}

	content := vmodelContentToAnthropic(resp)

	c.JSON(http.StatusOK, AnthropicMessageResponse{
		ID:         fmt.Sprintf("msg_virtual_%d", time.Now().Unix()),
		Type:       "message",
		Role:       "assistant",
		Model:      req.Model,
		StopReason: string(resp.StopReason),
		Content:    content,
		Usage: AnthropicUsage{
			InputTokens:  token.EstimateBetaAnthropicTokens(req.Messages),
			OutputTokens: token.EstimateTokensString(vmodelTextContent(resp)),
		},
	})
}

func (h *Handler) handleAnthropicStreaming(c *gin.Context, req *AnthropicMessageRequest, vm virtualmodel.AnthropicVirtualModel) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	msgID := fmt.Sprintf("msg_virtual_%d", time.Now().Unix())

	c.Stream(func(w io.Writer) bool {
		if d := vm.SimulatedDelay(); d > 0 {
			time.Sleep(d)
		}

		err := vm.HandleAnthropicStream(req, func(ev any) {
			switch e := ev.(type) {
			case virtualmodel.AnthropicStreamStartEvent:
				id := e.MsgID
				if id == "" {
					id = msgID
				}
				data, _ := json.Marshal(AnthropicStreamEvent{
					Type:    "message_start",
					Message: &AnthropicMessageResponse{ID: id, Type: "message", Role: "assistant", Model: req.Model},
				})
				fmt.Fprintf(w, "event: message_start\ndata: %s\n\n", data)
			case virtualmodel.AnthropicTextDeltaEvent:
				data, _ := json.Marshal(AnthropicStreamEvent{
					Type:  "content_block_delta",
					Index: e.Index,
					Delta: &AnthropicDelta{Type: "text_delta", Text: e.Text},
				})
				fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", data)
			case virtualmodel.AnthropicToolUseEvent:
				data, _ := json.Marshal(map[string]interface{}{
					"type":  "content_block_start",
					"index": e.Index,
					"content_block": map[string]interface{}{
						"type":  "tool_use",
						"id":    e.ID,
						"name":  e.Name,
						"input": json.RawMessage(e.Input),
					},
				})
				fmt.Fprintf(w, "event: content_block_start\ndata: %s\n\n", data)
			case virtualmodel.AnthropicDoneEvent:
				data, _ := json.Marshal(AnthropicStreamEvent{Type: "message_stop"})
				fmt.Fprintf(w, "event: message_stop\ndata: %s\n\n", data)
			}
			c.Writer.Flush()
		})
		if err != nil {
			errData, _ := json.Marshal(map[string]interface{}{
				"type": "error",
				"error": map[string]interface{}{
					"type":    "api_error",
					"message": err.Error(),
				},
			})
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", errData)
			c.Writer.Flush()
		}
		return false
	})
}

// ── OpenAI handlers ───────────────────────────────────────────────────────────

func (h *Handler) handleOpenAINonStreaming(c *gin.Context, req *ChatCompletionRequest, vm virtualmodel.OpenAIChatVirtualModel) {
	if d := vm.SimulatedDelay(); d > 0 {
		time.Sleep(d)
	}

	resp, err := vm.HandleOpenAIChat(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"message": err.Error(),
			"type":    "api_error",
		}})
		return
	}

	finishReason := resp.FinishReason
	outputTokens := token.EstimateTokensString(resp.Content)
	toolCalls := vmodelToolCallsToOpenAI(resp.ToolCalls)

	c.JSON(http.StatusOK, ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-virtual-%d", time.Now().Unix()),
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []Choice{{
			Index:        0,
			Message:      Message{Content: resp.Content, ToolCalls: toolCalls},
			FinishReason: finishReason,
		}},
		Usage: Usage{
			PromptTokens:     token.EstimateMessagesTokens(req.Messages),
			CompletionTokens: outputTokens,
			TotalTokens:      token.EstimateMessagesTokens(req.Messages) + outputTokens,
		},
	})
}

func (h *Handler) handleOpenAIStreaming(c *gin.Context, req *ChatCompletionRequest, vm virtualmodel.OpenAIChatVirtualModel) {
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
		if d := vm.SimulatedDelay(); d > 0 {
			time.Sleep(d)
		}

		chunkIndex := 0
		var finishReason string

		err := vm.HandleOpenAIChatStream(req, func(ev any) {
			select {
			case <-c.Request.Context().Done():
				logrus.Debug("Client disconnected during streaming")
				return
			default:
			}
			switch e := ev.(type) {
			case virtualmodel.OpenAIChatDeltaEvent:
				chunkIndex = e.Index + 1
				data, _ := json.Marshal(ChatCompletionStreamResponse{
					ID:      fmt.Sprintf("chatcmpl-virtual-%d", time.Now().Unix()),
					Created: time.Now().Unix(),
					Model:   req.Model,
					Choices: []StreamChoice{{
						Index:        int64(e.Index),
						Delta:        Delta{Content: e.Content},
						FinishReason: "",
					}},
				})
				c.SSEvent("", string(data))
			case virtualmodel.OpenAIChatToolEvent:
				tc := vmodelToolCallsToOpenAI([]virtualmodel.VToolCall{e.ToolCall})
				if len(tc) > 0 {
					data, _ := json.Marshal(ChatCompletionStreamResponse{
						ID:      fmt.Sprintf("chatcmpl-virtual-%d", time.Now().Unix()),
						Created: time.Now().Unix(),
						Model:   req.Model,
						Choices: []StreamChoice{{
							Index: 0,
							Delta: Delta{ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{{
								Index:    int64(e.Index),
								ID:       tc[0].ID,
								Type:     "function",
								Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{Name: tc[0].Function.Name, Arguments: tc[0].Function.Arguments},
							}}},
						}},
					})
					c.SSEvent("", string(data))
				}
			case virtualmodel.OpenAIChatDoneEvent:
				finishReason = e.FinishReason
				data, _ := json.Marshal(ChatCompletionStreamResponse{
					ID:      fmt.Sprintf("chatcmpl-virtual-%d", time.Now().Unix()),
					Created: time.Now().Unix(),
					Model:   req.Model,
					Choices: []StreamChoice{{Index: int64(chunkIndex), Delta: Delta{}, FinishReason: finishReason}},
				})
				c.SSEvent("", string(data))
			}
			c.Writer.Flush()
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{
				"message": err.Error(),
				"type":    "api_error",
			}})
			return false
		}

		c.SSEvent("", "[DONE]")
		c.Writer.Flush()
		return false
	})
}

// ── response conversion helpers ───────────────────────────────────────────────

// vmodelTextContent extracts concatenated text from Anthropic response content blocks.
func vmodelTextContent(resp virtualmodel.VModelResponse) string {
	s := ""
	for _, blk := range resp.Content {
		if blk.OfText != nil {
			s += blk.OfText.Text
		}
	}
	return s
}

func vmodelContentToAnthropic(resp virtualmodel.VModelResponse) []AnthropicContent {
	out := make([]AnthropicContent, 0, len(resp.Content))
	for _, blk := range resp.Content {
		if blk.OfText != nil {
			out = append(out, AnthropicContent{Type: "text", Text: blk.OfText.Text})
		} else if blk.OfToolUse != nil {
			inputJSON, _ := json.Marshal(blk.OfToolUse.Input)
			out = append(out, AnthropicContent{
				Type:  "tool_use",
				ID:    blk.OfToolUse.ID,
				Name:  blk.OfToolUse.Name,
				Input: inputJSON,
			})
		}
	}
	return out
}

// vmodelToolCallsToOpenAI converts VToolCall slice to OpenAI SDK tool call format.
func vmodelToolCallsToOpenAI(calls []virtualmodel.VToolCall) []openai.ChatCompletionMessageToolCallUnion {
	if len(calls) == 0 {
		return nil
	}
	out := make([]openai.ChatCompletionMessageToolCallUnion, 0, len(calls))
	for _, tc := range calls {
		out = append(out, openai.ChatCompletionMessageToolCallUnion{
			ID:   tc.ID,
			Type: "function",
			Function: openai.ChatCompletionMessageFunctionToolCallFunction{
				Name:      tc.Name,
				Arguments: tc.Arguments,
			},
		})
	}
	return out
}
