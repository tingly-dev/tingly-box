package stream

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
)

// StreamAnthropicV1SingleMessage emits a single assembled Anthropic v1 message using SSE events.
func StreamAnthropicV1SingleMessage(c *gin.Context, resp *anthropic.Message, responseModel string) error {
	if resp == nil {
		return errors.New("nil anthropic v1 response")
	}

	setAnthropicSSEHeaders(c)
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return errors.New("streaming not supported by this connection")
	}

	messageID := string(resp.ID)
	if messageID == "" {
		messageID = fmt.Sprintf("msg_%d", time.Now().Unix())
	}

	model := responseModel
	if model == "" {
		model = string(resp.Model)
	}

	state := newStreamState()
	state.inputTokens = int64(resp.Usage.InputTokens)
	state.outputTokens = int64(resp.Usage.OutputTokens)

	messageStart := map[string]interface{}{
		"type": eventTypeMessageStart,
		"message": map[string]interface{}{
			"id":            messageID,
			"type":          "message",
			"role":          "assistant",
			"content":       []interface{}{},
			"model":         model,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]interface{}{
				"input_tokens":  state.inputTokens,
				"output_tokens": 0,
			},
		},
	}
	sendAnthropicStreamEvent(c, eventTypeMessageStart, messageStart, flusher)

	for idx, block := range resp.Content {
		switch v := block.AsAny().(type) {
		case anthropic.TextBlock:
			sendContentBlockStart(c, idx, blockTypeText, map[string]interface{}{"text": ""}, flusher)
			if v.Text != "" {
				sendContentBlockDelta(c, idx, map[string]interface{}{
					"type": deltaTypeTextDelta,
					"text": v.Text,
				}, flusher)
			}
			sendContentBlockStop(c, state, idx, flusher)
		case anthropic.ThinkingBlock:
			sendContentBlockStart(c, idx, blockTypeThinking, map[string]interface{}{"thinking": ""}, flusher)
			if v.Thinking != "" {
				sendContentBlockDelta(c, idx, map[string]interface{}{
					"type":     deltaTypeThinkingDelta,
					"thinking": v.Thinking,
				}, flusher)
			}
			sendContentBlockStop(c, state, idx, flusher)
		case anthropic.ToolUseBlock:
			sendContentBlockStart(c, idx, blockTypeToolUse, map[string]interface{}{
				"id":    v.ID,
				"name":  v.Name,
				"input": v.Input,
			}, flusher)
			sendContentBlockStop(c, state, idx, flusher)
		}
	}

	stopReason := string(resp.StopReason)
	if stopReason == "" {
		stopReason = anthropicStopReasonEndTurn
	}
	sendMessageDelta(c, state, stopReason, flusher)
	sendMessageStop(c, messageID, model, state, stopReason, flusher)
	return nil
}

// StreamAnthropicBetaSingleMessage emits a single assembled Anthropic beta message using SSE events.
func StreamAnthropicBetaSingleMessage(c *gin.Context, resp *anthropic.BetaMessage, responseModel string) error {
	if resp == nil {
		return errors.New("nil anthropic beta response")
	}

	setAnthropicSSEHeaders(c)
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return errors.New("streaming not supported by this connection")
	}

	messageID := string(resp.ID)
	if messageID == "" {
		messageID = fmt.Sprintf("msg_%d", time.Now().Unix())
	}

	model := responseModel
	if model == "" {
		model = string(resp.Model)
	}

	state := newStreamState()
	state.inputTokens = int64(resp.Usage.InputTokens)
	state.outputTokens = int64(resp.Usage.OutputTokens)

	messageStart := map[string]interface{}{
		"type": eventTypeMessageStart,
		"message": map[string]interface{}{
			"id":            messageID,
			"type":          "message",
			"role":          "assistant",
			"content":       []interface{}{},
			"model":         model,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]interface{}{
				"input_tokens":  state.inputTokens,
				"output_tokens": 0,
			},
		},
	}
	sendAnthropicBetaStreamEvent(c, eventTypeMessageStart, messageStart, flusher)

	for idx, block := range resp.Content {
		switch v := block.AsAny().(type) {
		case anthropic.BetaTextBlock:
			sendBetaContentBlockStart(c, idx, blockTypeText, map[string]interface{}{"text": ""}, flusher)
			if v.Text != "" {
				sendBetaContentBlockDelta(c, idx, map[string]interface{}{
					"type": deltaTypeTextDelta,
					"text": v.Text,
				}, flusher)
			}
			sendBetaContentBlockStop(c, state, idx, flusher)
		case anthropic.BetaThinkingBlock:
			sendBetaContentBlockStart(c, idx, blockTypeThinking, map[string]interface{}{"thinking": ""}, flusher)
			if v.Thinking != "" {
				sendBetaContentBlockDelta(c, idx, map[string]interface{}{
					"type":     deltaTypeThinkingDelta,
					"thinking": v.Thinking,
				}, flusher)
			}
			sendBetaContentBlockStop(c, state, idx, flusher)
		case anthropic.BetaToolUseBlock:
			sendBetaContentBlockStart(c, idx, blockTypeToolUse, map[string]interface{}{
				"id":    v.ID,
				"name":  v.Name,
				"input": v.Input,
			}, flusher)
			sendBetaContentBlockStop(c, state, idx, flusher)
		}
	}

	stopReason := string(resp.StopReason)
	if stopReason == "" {
		stopReason = anthropicStopReasonEndTurn
	}
	sendBetaMessageDelta(c, state, stopReason, flusher)
	sendBetaMessageStop(c, messageID, model, state, stopReason, flusher)
	return nil
}

func setAnthropicSSEHeaders(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Headers", "Cache-Control")
}
