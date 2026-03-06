package virtualmodel

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// AnthropicMessageRequest represents an Anthropic-compatible messages request
type AnthropicMessageRequest struct {
	Model     string             `json:"model"`
	Messages  []AnthropicMessage `json:"messages"`
	MaxTokens int                `json:"max_tokens,omitempty"`
	Stream    bool               `json:"stream,omitempty"`
	Tools     []AnthropicTool    `json:"tools,omitempty"`
}

// AnthropicMessage represents a message in Anthropic format
type AnthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AnthropicTool represents a tool definition in Anthropic format
type AnthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// AnthropicMessageResponse represents an Anthropic-compatible message response
type AnthropicMessageResponse struct {
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	Role       string             `json:"role"`
	Model      string             `json:"model"`
	Content    []AnthropicContent `json:"content"`
	StopReason string             `json:"stop_reason"`
	Usage      AnthropicUsage     `json:"usage"`
}

// AnthropicContent represents content block in Anthropic response
type AnthropicContent struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// AnthropicUsage represents token usage in Anthropic format
type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// AnthropicStreamEvent represents a streaming event in Anthropic format
type AnthropicStreamEvent struct {
	Type    string                    `json:"type"`
	Message *AnthropicMessageResponse `json:"message,omitempty"`
	Index   int                       `json:"index,omitempty"`
	Delta   *AnthropicDelta           `json:"delta,omitempty"`
	Usage   *AnthropicUsage           `json:"usage,omitempty"`
}

// AnthropicDelta represents a delta in streaming response
type AnthropicDelta struct {
	Type       string `json:"type,omitempty"`
	Text       string `json:"text,omitempty"`
	StopReason string `json:"stop_reason,omitempty"`
}

// Messages handles the POST /virtual/v1/messages endpoint (Anthropic format)
func (h *Handler) Messages(c *gin.Context) {
	var req AnthropicMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": "Invalid request body: " + err.Error(),
			},
		})
		return
	}

	if req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": "Model is required",
			},
		})
		return
	}

	virtualModel := h.registry.Get(req.Model)
	if virtualModel == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"type": "error",
			"error": gin.H{
				"type":    "not_found_error",
				"message": fmt.Sprintf("Model not found: %s", req.Model),
			},
		})
		return
	}

	switch virtualModel.GetType() {
	case VirtualModelTypeTool:
		if req.Stream {
			h.handleAnthropicToolModelStreaming(c, &req, virtualModel)
		} else {
			h.handleAnthropicToolModelNonStreaming(c, &req, virtualModel)
		}
		return
	}

	if req.Stream {
		h.handleAnthropicStreaming(c, &req, virtualModel)
	} else {
		h.handleAnthropicNonStreaming(c, &req, virtualModel)
	}
}

func (h *Handler) handleAnthropicNonStreaming(c *gin.Context, req *AnthropicMessageRequest, vm *VirtualModel) {
	if delay := vm.GetDelay(); delay > 0 {
		time.Sleep(delay)
	}

	content := vm.GetContent()
	resp := AnthropicMessageResponse{
		ID:         fmt.Sprintf("msg_virtual_%d", time.Now().Unix()),
		Type:       "message",
		Role:       "assistant",
		Model:      req.Model,
		StopReason: "end_turn",
		Content: []AnthropicContent{
			{Type: "text", Text: content},
		},
		Usage: AnthropicUsage{
			InputTokens:  estimateAnthropicTokens(req.Messages),
			OutputTokens: estimateTokensString(content),
		},
	}

	c.JSON(http.StatusOK, resp)
}

func (h *Handler) handleAnthropicStreaming(c *gin.Context, req *AnthropicMessageRequest, vm *VirtualModel) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	c.Stream(func(w io.Writer) bool {
		if delay := vm.GetDelay(); delay > 0 {
			time.Sleep(delay)
		}

		content := vm.GetContent()
		msgID := fmt.Sprintf("msg_virtual_%d", time.Now().Unix())

		startEvent := AnthropicStreamEvent{
			Type: "message_start",
			Message: &AnthropicMessageResponse{
				ID:    msgID,
				Type:  "message",
				Role:  "assistant",
				Model: req.Model,
			},
		}
		data, _ := json.Marshal(startEvent)
		fmt.Fprintf(w, "event: message_start\ndata: %s\n\n", data)
		c.Writer.Flush()

		chunks := splitIntoChunks(content)
		for i, chunk := range chunks {
			time.Sleep(50 * time.Millisecond)

			deltaEvent := AnthropicStreamEvent{
				Type:  "content_block_delta",
				Index: i,
				Delta: &AnthropicDelta{
					Type: "text_delta",
					Text: chunk,
				},
			}
			data, _ := json.Marshal(deltaEvent)
			fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", data)
			c.Writer.Flush()
		}

		stopEvent := AnthropicStreamEvent{Type: "message_stop"}
		data, _ = json.Marshal(stopEvent)
		fmt.Fprintf(w, "event: message_stop\ndata: %s\n\n", data)
		c.Writer.Flush()

		return false
	})
}

func (h *Handler) handleAnthropicToolModelNonStreaming(c *gin.Context, req *AnthropicMessageRequest, vm *VirtualModel) {
	if delay := vm.GetDelay(); delay > 0 {
		time.Sleep(delay)
	}

	toolCall := vm.GetToolCall()
	if toolCall == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tool call config not found"})
		return
	}

	// Use arguments directly as tool input
	inputJSON, _ := json.Marshal(toolCall.Arguments)

	// Extract message content from arguments if available
	content := ""
	if msg, ok := toolCall.Arguments["message"].(string); ok {
		content = msg
	} else if question, ok := toolCall.Arguments["question"].(string); ok {
		content = question
	}

	resp := AnthropicMessageResponse{
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
	}

	c.JSON(http.StatusOK, resp)
}

func (h *Handler) handleAnthropicToolModelStreaming(c *gin.Context, req *AnthropicMessageRequest, vm *VirtualModel) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	toolCall := vm.GetToolCall()
	if toolCall == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "tool call config not found"})
		return
	}

	// Use arguments directly as tool input
	inputJSON, _ := json.Marshal(toolCall.Arguments)

	// Extract message content from arguments if available
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

		startEvent := AnthropicStreamEvent{
			Type: "message_start",
			Message: &AnthropicMessageResponse{
				ID:    msgID,
				Type:  "message",
				Role:  "assistant",
				Model: req.Model,
			},
		}
		data, _ := json.Marshal(startEvent)
		fmt.Fprintf(w, "event: message_start\ndata: %s\n\n", data)
		c.Writer.Flush()

		time.Sleep(50 * time.Millisecond)

		textEvent := AnthropicStreamEvent{
			Type:  "content_block_delta",
			Index: 0,
			Delta: &AnthropicDelta{
				Type: "text_delta",
				Text: content,
			},
		}
		data, _ = json.Marshal(textEvent)
		fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", data)
		c.Writer.Flush()

		time.Sleep(50 * time.Millisecond)

		toolEvent := AnthropicStreamEvent{
			Type:  "content_block_stop",
			Index: 1,
		}
		data, _ = json.Marshal(toolEvent)
		fmt.Fprintf(w, "event: content_block_stop\ndata: %s\n\n", data)
		c.Writer.Flush()

		toolStartEvent := map[string]interface{}{
			"type": "content_block_start",
			"index": 1,
			"content_block": map[string]interface{}{
				"type":  "tool_use",
				"id":    toolID,
				"name":  toolCall.Name,
				"input": inputJSON,
			},
		}
		data, _ = json.Marshal(toolStartEvent)
		fmt.Fprintf(w, "event: content_block_start\ndata: %s\n\n", data)
		c.Writer.Flush()

		stopEvent := AnthropicStreamEvent{
			Type: "message_stop",
			Message: &AnthropicMessageResponse{
				StopReason: "tool_use",
			},
		}
		data, _ = json.Marshal(stopEvent)
		fmt.Fprintf(w, "event: message_stop\ndata: %s\n\n", data)
		c.Writer.Flush()

		return false
	})
}

func estimateAnthropicTokens(messages []AnthropicMessage) int {
	total := 0
	for _, msg := range messages {
		total += estimateTokensString(msg.Content)
		total += 5
	}
	return total
}
