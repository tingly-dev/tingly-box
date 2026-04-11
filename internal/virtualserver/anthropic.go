package virtualserver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/virtualmodel"
)

// AnthropicMessageRequest is an Anthropic-compatible messages request.
type AnthropicMessageRequest struct {
	Model     string             `json:"model"`
	Messages  []AnthropicMessage `json:"messages"`
	MaxTokens int                `json:"max_tokens,omitempty"`
	Stream    bool               `json:"stream,omitempty"`
	Tools     []AnthropicTool    `json:"tools,omitempty"`
}

// AnthropicMessage is a message in Anthropic format.
type AnthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AnthropicTool is a tool definition in Anthropic format.
type AnthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// AnthropicMessageResponse is an Anthropic-compatible message response.
type AnthropicMessageResponse struct {
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	Role       string             `json:"role"`
	Model      string             `json:"model"`
	Content    []AnthropicContent `json:"content"`
	StopReason string             `json:"stop_reason"`
	Usage      AnthropicUsage     `json:"usage"`
}

// AnthropicContent is a content block in an Anthropic response.
type AnthropicContent struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// AnthropicUsage holds token usage in Anthropic format.
type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// AnthropicStreamEvent is a streaming event in Anthropic format.
type AnthropicStreamEvent struct {
	Type    string                    `json:"type"`
	Message *AnthropicMessageResponse `json:"message,omitempty"`
	Index   int                       `json:"index,omitempty"`
	Delta   *AnthropicDelta           `json:"delta,omitempty"`
	Usage   *AnthropicUsage           `json:"usage,omitempty"`
}

// AnthropicDelta is a delta in an Anthropic streaming response.
type AnthropicDelta struct {
	Type       string `json:"type,omitempty"`
	Text       string `json:"text,omitempty"`
	StopReason string `json:"stop_reason,omitempty"`
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

	switch vm.GetType() {
	case virtualmodel.VirtualModelTypeTool:
		if req.Stream {
			h.handleAnthropicToolModelStreaming(c, &req, vm)
		} else {
			h.handleAnthropicToolModelNonStreaming(c, &req, vm)
		}
		return
	}

	if req.Stream {
		h.handleAnthropicStreaming(c, &req, vm)
	} else {
		h.handleAnthropicNonStreaming(c, &req, vm)
	}
}

func (h *Handler) handleAnthropicNonStreaming(c *gin.Context, req *AnthropicMessageRequest, vm *virtualmodel.VirtualModel) {
	if delay := vm.GetDelay(); delay > 0 {
		time.Sleep(delay)
	}

	content := vm.GetContent()
	c.JSON(http.StatusOK, AnthropicMessageResponse{
		ID:         fmt.Sprintf("msg_virtual_%d", time.Now().Unix()),
		Type:       "message",
		Role:       "assistant",
		Model:      req.Model,
		StopReason: "end_turn",
		Content:    []AnthropicContent{{Type: "text", Text: content}},
		Usage: AnthropicUsage{
			InputTokens:  estimateAnthropicTokens(req.Messages),
			OutputTokens: estimateTokensString(content),
		},
	})
}

func (h *Handler) handleAnthropicStreaming(c *gin.Context, req *AnthropicMessageRequest, vm *virtualmodel.VirtualModel) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	c.Stream(func(w io.Writer) bool {
		if delay := vm.GetDelay(); delay > 0 {
			time.Sleep(delay)
		}

		content := vm.GetContent()
		msgID := fmt.Sprintf("msg_virtual_%d", time.Now().Unix())

		data, _ := json.Marshal(AnthropicStreamEvent{
			Type: "message_start",
			Message: &AnthropicMessageResponse{
				ID: msgID, Type: "message", Role: "assistant", Model: req.Model,
			},
		})
		fmt.Fprintf(w, "event: message_start\ndata: %s\n\n", data)
		c.Writer.Flush()

		for i, chunk := range splitIntoChunks(content) {
			time.Sleep(50 * time.Millisecond)
			data, _ := json.Marshal(AnthropicStreamEvent{
				Type:  "content_block_delta",
				Index: i,
				Delta: &AnthropicDelta{Type: "text_delta", Text: chunk},
			})
			fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", data)
			c.Writer.Flush()
		}

		data, _ = json.Marshal(AnthropicStreamEvent{Type: "message_stop"})
		fmt.Fprintf(w, "event: message_stop\ndata: %s\n\n", data)
		c.Writer.Flush()
		return false
	})
}

func (h *Handler) handleAnthropicToolModelNonStreaming(c *gin.Context, req *AnthropicMessageRequest, vm *virtualmodel.VirtualModel) {
	if delay := vm.GetDelay(); delay > 0 {
		time.Sleep(delay)
	}

	toolCall := vm.GetToolCall()
	if toolCall == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tool call config not found"})
		return
	}

	inputJSON, _ := json.Marshal(toolCall.Arguments)
	content := ""
	if msg, ok := toolCall.Arguments["message"].(string); ok {
		content = msg
	} else if question, ok := toolCall.Arguments["question"].(string); ok {
		content = question
	}

	c.JSON(http.StatusOK, AnthropicMessageResponse{
		ID:         fmt.Sprintf("msg_virtual_%d", time.Now().Unix()),
		Type:       "message",
		Role:       "assistant",
		Model:      req.Model,
		StopReason: "tool_use",
		Content: []AnthropicContent{
			{Type: "text", Text: content},
			{
				Type:  "tool_use",
				ID:    fmt.Sprintf("toolu_%d", time.Now().Unix()),
				Name:  toolCall.Name,
				Input: inputJSON,
			},
		},
		Usage: AnthropicUsage{
			InputTokens:  estimateAnthropicTokens(req.Messages),
			OutputTokens: estimateTokensString(content) + 20,
		},
	})
}

func (h *Handler) handleAnthropicToolModelStreaming(c *gin.Context, req *AnthropicMessageRequest, vm *virtualmodel.VirtualModel) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	toolCall := vm.GetToolCall()
	if toolCall == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tool call config not found"})
		return
	}

	inputJSON, _ := json.Marshal(toolCall.Arguments)
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

		msgID := fmt.Sprintf("msg_virtual_%d", time.Now().Unix())
		toolID := fmt.Sprintf("toolu_%d", time.Now().Unix())

		data, _ := json.Marshal(AnthropicStreamEvent{
			Type:    "message_start",
			Message: &AnthropicMessageResponse{ID: msgID, Type: "message", Role: "assistant", Model: req.Model},
		})
		fmt.Fprintf(w, "event: message_start\ndata: %s\n\n", data)
		c.Writer.Flush()
		time.Sleep(50 * time.Millisecond)

		data, _ = json.Marshal(AnthropicStreamEvent{
			Type:  "content_block_delta",
			Index: 0,
			Delta: &AnthropicDelta{Type: "text_delta", Text: content},
		})
		fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", data)
		c.Writer.Flush()
		time.Sleep(50 * time.Millisecond)

		data, _ = json.Marshal(AnthropicStreamEvent{Type: "content_block_stop", Index: 1})
		fmt.Fprintf(w, "event: content_block_stop\ndata: %s\n\n", data)
		c.Writer.Flush()

		data, _ = json.Marshal(map[string]interface{}{
			"type":  "content_block_start",
			"index": 1,
			"content_block": map[string]interface{}{
				"type":  "tool_use",
				"id":    toolID,
				"name":  toolCall.Name,
				"input": inputJSON,
			},
		})
		fmt.Fprintf(w, "event: content_block_start\ndata: %s\n\n", data)
		c.Writer.Flush()

		data, _ = json.Marshal(AnthropicStreamEvent{
			Type:    "message_stop",
			Message: &AnthropicMessageResponse{StopReason: "tool_use"},
		})
		fmt.Fprintf(w, "event: message_stop\ndata: %s\n\n", data)
		c.Writer.Flush()
		return false
	})
}

func estimateAnthropicTokens(messages []AnthropicMessage) int {
	total := 0
	for _, msg := range messages {
		total += estimateTokensString(msg.Content) + 5
	}
	return total
}
