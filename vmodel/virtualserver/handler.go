// Package virtualserver provides the HTTP handler for virtual model endpoints.
// It serves OpenAI Chat and Anthropic Messages API formats backed by separate
// per-provider virtual model registries.
//
// This is the production HTTP surface for internal/virtualmodel: the parent
// server mounts it under /virtual/v1/* so end users can reach the synthetic
// provider for onboarding, demos, and dry-runs without configuring a real
// upstream provider. Test consumers do not depend on this package; they use
// the registry primitives in internal/virtualmodel directly.
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
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"

	anthropicvm "github.com/tingly-dev/tingly-box/vmodel/anthropic"
	openaivm "github.com/tingly-dev/tingly-box/vmodel/openai"
)

// Handler handles HTTP requests for virtual model endpoints.
type Handler struct {
	anthropicReg *anthropicvm.Registry
	openaiReg    *openaivm.Registry
}

// NewHandler creates a new Handler backed by the given per-provider registries.
func NewHandler(anthropicReg *anthropicvm.Registry, openaiReg *openaivm.Registry) *Handler {
	return &Handler{anthropicReg: anthropicReg, openaiReg: openaiReg}
}

// ListModels handles GET /virtual/v1/models — returns the union of both registries.
func (h *Handler) ListModels(c *gin.Context) {
	models := h.anthropicReg.ListModels()
	models = append(models, h.openaiReg.ListModels()...)
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

	vm := h.openaiReg.Get(req.Model)
	if vm == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{
			"message": fmt.Sprintf("Model not found: %s", req.Model),
			"type":    "invalid_request_error",
		}})
		return
	}

	if req.Stream {
		h.handleOpenAIStreaming(c, &req, vm)
	} else {
		h.handleOpenAINonStreaming(c, &req, vm)
	}
}

// Messages handles POST /virtual/v1/messages (Anthropic format).
//
// Accepts both Anthropic v1 and beta wire formats. The query parameter
// "beta=true" mirrors the real Anthropic API gating used in
// internal/server/anthropic.go. Internally everything is canonicalized to
// the beta superset struct so vmodel implementations only deal with one
// request shape.
func (h *Handler) Messages(c *gin.Context) {
	var req AnthropicMessageRequest
	if c.Query("beta") == "true" {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"type": "error", "error": gin.H{
				"type":    "invalid_request_error",
				"message": "Invalid request body: " + err.Error(),
			}})
			return
		}
	} else {
		var v1 protocol.AnthropicMessagesRequest
		if err := c.ShouldBindJSON(&v1); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"type": "error", "error": gin.H{
				"type":    "invalid_request_error",
				"message": "Invalid request body: " + err.Error(),
			}})
			return
		}
		lifted, err := liftAnthropicV1ToBeta(v1)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"type": "error", "error": gin.H{
				"type":    "invalid_request_error",
				"message": "Invalid request body: " + err.Error(),
			}})
			return
		}
		req = lifted
	}
	if req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"type": "error", "error": gin.H{
			"type":    "invalid_request_error",
			"message": "Model is required",
		}})
		return
	}

	vm := h.anthropicReg.Get(req.Model)
	if vm == nil {
		c.JSON(http.StatusNotFound, gin.H{"type": "error", "error": gin.H{
			"type":    "not_found_error",
			"message": fmt.Sprintf("Model not found: %s", req.Model),
		}})
		return
	}

	if req.Stream {
		h.handleAnthropicStreaming(c, &req, vm)
	} else {
		h.handleAnthropicNonStreaming(c, &req, vm)
	}
}

// ── Anthropic handlers ────────────────────────────────────────────────────────

func (h *Handler) handleAnthropicNonStreaming(c *gin.Context, req *AnthropicMessageRequest, vm anthropicvm.VirtualModel) {
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

func (h *Handler) handleAnthropicStreaming(c *gin.Context, req *AnthropicMessageRequest, vm anthropicvm.VirtualModel) {
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
			case anthropicvm.StreamStartEvent:
				id := e.MsgID
				if id == "" {
					id = msgID
				}
				data, _ := json.Marshal(AnthropicStreamEvent{
					Type:    "message_start",
					Message: &AnthropicMessageResponse{ID: id, Type: "message", Role: "assistant", Model: req.Model},
				})
				fmt.Fprintf(w, "event: message_start\ndata: %s\n\n", data)
			case anthropicvm.TextDeltaEvent:
				data, _ := json.Marshal(AnthropicStreamEvent{
					Type:  "content_block_delta",
					Index: e.Index,
					Delta: &AnthropicDelta{Type: "text_delta", Text: e.Text},
				})
				fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", data)
			case anthropicvm.ToolUseEvent:
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
			case anthropicvm.DoneEvent:
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

func (h *Handler) handleOpenAINonStreaming(c *gin.Context, req *ChatCompletionRequest, vm openaivm.VirtualModel) {
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

func (h *Handler) handleOpenAIStreaming(c *gin.Context, req *ChatCompletionRequest, vm openaivm.VirtualModel) {
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
			case openaivm.DeltaEvent:
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
			case openaivm.ToolEvent:
				tc := vmodelToolCallsToOpenAI([]openaivm.VToolCall{e.ToolCall})
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
			case openaivm.DoneEvent:
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
func vmodelTextContent(resp anthropicvm.VModelResponse) string {
	s := ""
	for _, blk := range resp.Content {
		if blk.OfText != nil {
			s += blk.OfText.Text
		}
	}
	return s
}

func vmodelContentToAnthropic(resp anthropicvm.VModelResponse) []AnthropicContent {
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

// liftAnthropicV1ToBeta canonicalizes an Anthropic v1 request into the beta
// superset struct via JSON round-trip. Beta extends v1 strictly (extra fields
// like Container, Speed, ServiceTier), so the v1 wire format is a valid subset
// of the beta one.
func liftAnthropicV1ToBeta(v1 protocol.AnthropicMessagesRequest) (AnthropicMessageRequest, error) {
	raw, err := json.Marshal(v1)
	if err != nil {
		return AnthropicMessageRequest{}, err
	}
	var beta AnthropicMessageRequest
	if err := json.Unmarshal(raw, &beta); err != nil {
		return AnthropicMessageRequest{}, err
	}
	return beta, nil
}

// vmodelToolCallsToOpenAI converts VToolCall slice to OpenAI SDK tool call format.
func vmodelToolCallsToOpenAI(calls []openaivm.VToolCall) []openai.ChatCompletionMessageToolCallUnion {
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
