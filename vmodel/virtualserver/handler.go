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
	"github.com/tingly-dev/tingly-box/vmodel"

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
//
// Deprecated: prefer ListOpenAIModels / ListAnthropicModels for the
// protocol-split entrypoints. Retained for the legacy mixed-protocol route
// and for test fixtures that want both registries on one endpoint.
func (h *Handler) ListModels(c *gin.Context) {
	models := h.anthropicReg.ListModels()
	models = append(models, h.openaiReg.ListModels()...)
	c.JSON(http.StatusOK, OpenAIModelsResponse{
		Object: "list",
		Data:   models,
	})
}

// ListOpenAIModels handles GET /virtual/openai/v1/models — returns only the
// OpenAI-protocol registry so clients pointed at the OpenAI base URL don't
// see Anthropic-only model IDs they cannot dispatch.
func (h *Handler) ListOpenAIModels(c *gin.Context) {
	c.JSON(http.StatusOK, OpenAIModelsResponse{
		Object: "list",
		Data:   h.openaiReg.ListModels(),
	})
}

// ListAnthropicModels handles GET /virtual/anthropic/v1/models — returns
// only the Anthropic-protocol registry in Anthropic's native envelope shape
// (data + first_id/last_id/has_more, no "object" field).
func (h *Handler) ListAnthropicModels(c *gin.Context) {
	models := h.anthropicReg.ListModels()
	resp := AnthropicModelsResponse{Data: models, HasMore: false}
	if len(models) > 0 {
		resp.FirstID = models[0].ID
		resp.LastID = models[len(models)-1].ID
	}
	c.JSON(http.StatusOK, resp)
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

	if e := vmodel.ExtractErrorInjection(vm); e != nil && e.Stage == vmodel.ErrorStagePreContent {
		writePreContentErrorOpenAI(c, e)
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

	if e := vmodel.ExtractErrorInjection(vm); e != nil && e.Stage == vmodel.ErrorStagePreContent {
		writePreContentErrorAnthropic(c, e)
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
	midInj := midStreamInjection(vm)
	gate := newMidStreamGate(midInj)

	c.Stream(func(w io.Writer) bool {
		if d := vm.SimulatedDelay(); d > 0 {
			time.Sleep(d)
		}

		var explicitUsage *vmodel.MockUsage
		var stopReason string

		err := vm.HandleAnthropicStream(req, func(ev any) {
			if gate != nil {
				switch ev.(type) {
				case anthropicvm.DoneEvent, anthropicvm.UsageEvent:
					return // suppress terminal events; handler applies break instead
				default:
					if !gate.Allow() {
						return
					}
				}
			}
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
			case anthropicvm.UsageEvent:
				u := e.Usage
				explicitUsage = &u
			case anthropicvm.DoneEvent:
				stopReason = e.StopReason
				// Emit message_delta with stop_reason + usage (Anthropic
				// places terminal usage on message_delta, not message_stop).
				deltaPayload := map[string]interface{}{
					"type": "message_delta",
					"delta": map[string]interface{}{
						"stop_reason":   stopReason,
						"stop_sequence": nil,
					},
				}
				usageMap := map[string]interface{}{}
				if explicitUsage != nil {
					usageMap["input_tokens"] = explicitUsage.PromptTokens
					usageMap["output_tokens"] = explicitUsage.CompletionTokens
					if explicitUsage.CachedInputTokens > 0 {
						usageMap["cache_read_input_tokens"] = explicitUsage.CachedInputTokens
					}
					if explicitUsage.CacheCreationInputTokens > 0 {
						usageMap["cache_creation_input_tokens"] = explicitUsage.CacheCreationInputTokens
					}
					if explicitUsage.ReasoningTokens > 0 {
						usageMap["reasoning_tokens"] = explicitUsage.ReasoningTokens
					}
				}
				deltaPayload["usage"] = usageMap
				data, _ := json.Marshal(deltaPayload)
				fmt.Fprintf(w, "event: message_delta\ndata: %s\n\n", data)

				stopData, _ := json.Marshal(AnthropicStreamEvent{Type: "message_stop"})
				fmt.Fprintf(w, "event: message_stop\ndata: %s\n\n", stopData)
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
		} else if midInj != nil {
			applyMidStreamBreakAnthropic(c, w, midInj)
		}
		return false
	})
}

// newMidStreamGate constructs a counting gate for mid-stream injection, or
// returns nil when no injection is configured. The gate is owned by the
// handler (not the model): the model's stream loop emits freely; the handler's
// emit wrapper intercepts after the configured event count.
func newMidStreamGate(midInj *vmodel.ErrorInjection) *vmodel.EmitGate {
	if midInj == nil {
		return nil
	}
	cutoff := midInj.AfterEvents
	if cutoff <= 0 {
		cutoff = 1
	}
	return vmodel.NewEmitGate(cutoff)
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
	midInj := midStreamInjection(vm)
	gate := newMidStreamGate(midInj)

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
		var completionText string
		var explicitUsage *vmodel.MockUsage

		err := vm.HandleOpenAIChatStream(req, func(ev any) {
			select {
			case <-c.Request.Context().Done():
				logrus.Debug("Client disconnected during streaming")
				return
			default:
			}
			if gate != nil {
				switch ev.(type) {
				case openaivm.DoneEvent, openaivm.UsageEvent:
					return // suppress terminal events; handler applies break instead
				default:
					if !gate.Allow() {
						return
					}
				}
			}
			switch e := ev.(type) {
			case openaivm.DeltaEvent:
				chunkIndex = e.Index + 1
				completionText += e.Content
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
			case openaivm.UsageEvent:
				u := e.Usage
				explicitUsage = &u
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

		if midInj != nil {
			applyMidStreamBreakOpenAI(c, w, midInj)
			return false
		}

		// Mirror real OpenAI: when stream_options.include_usage=true (or the
		// mock advertised an explicit UsageEvent), emit a trailing usage-only
		// chunk (choices:[], usage:{...}) after the finish_reason chunk and
		// before [DONE]. Explicit MockUsage takes precedence so tests get
		// deterministic, fully-populated values (including cached prompt
		// tokens and reasoning tokens).
		if req.StreamOptions.IncludeUsage.Value || explicitUsage != nil {
			usage := openai.CompletionUsage{}
			if explicitUsage != nil {
				usage.PromptTokens = explicitUsage.PromptTokens
				usage.CompletionTokens = explicitUsage.CompletionTokens
				usage.TotalTokens = explicitUsage.PromptTokens + explicitUsage.CompletionTokens
				if explicitUsage.CachedInputTokens > 0 {
					usage.PromptTokensDetails.CachedTokens = explicitUsage.CachedInputTokens
				}
				if explicitUsage.ReasoningTokens > 0 {
					usage.CompletionTokensDetails.ReasoningTokens = explicitUsage.ReasoningTokens
				}
			} else {
				promptTokens := token.EstimateMessagesTokens(req.Messages)
				completionTokens := token.EstimateTokensString(completionText)
				usage.PromptTokens = int64(promptTokens)
				usage.CompletionTokens = int64(completionTokens)
				usage.TotalTokens = int64(promptTokens + completionTokens)
			}
			usageChunk := ChatCompletionStreamResponse{
				ID:      fmt.Sprintf("chatcmpl-virtual-%d", time.Now().Unix()),
				Created: time.Now().Unix(),
				Model:   req.Model,
				Choices: []StreamChoice{},
				Usage:   usage,
			}
			data, _ := json.Marshal(usageChunk)
			c.SSEvent("", string(data))
			c.Writer.Flush()
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
