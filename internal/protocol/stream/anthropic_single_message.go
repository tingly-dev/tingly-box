package stream

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
)

// AnthropicSingleMessage emits a single assembled Anthropic v1 message using SSE events.
func AnthropicSingleMessage(c *gin.Context, resp *anthropic.Message, responseModel string) error {
	if resp == nil {
		return errors.New("nil anthropic v1 response")
	}

	setAnthropicSSEHeaders(c)
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return errors.New("streaming not supported by this connection")
	}

	model := responseModel
	if model == "" {
		model = resp.Model
	}

	state := buildStreamState(resp.Usage.InputTokens, resp.Usage.OutputTokens)
	sendMessageStart(c, flusher, model, eventTypeMessageStart, sendAnthropicStreamEvent, state.inputTokens)

	hasToolUse := false
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
			hasToolUse = true
			sendContentBlockStart(c, idx, blockTypeToolUse, map[string]interface{}{
				"id":    v.ID,
				"name":  v.Name,
				"input": map[string]any{},
			}, flusher)
			if inputBytes, err := json.Marshal(v.Input); err == nil {
				sendContentBlockDelta(c, idx, map[string]interface{}{
					"type":         deltaTypeInputJSONDelta,
					"partial_json": string(inputBytes),
				}, flusher)
			}
			sendContentBlockStop(c, state, idx, flusher)
		}
	}

	stopReason := anthropicStopReasonEndTurn
	if hasToolUse {
		stopReason = anthropicStopReasonToolUse
	}
	sendMessageDelta(c, state, stopReason, flusher)
	sendMessageStop(c, fmt.Sprintf("msg_%d", time.Now().Unix()), model, state, stopReason, flusher)
	return nil
}

// AnthropicSingleBetaMessage emits a single assembled Anthropic beta message using SSE events.
func AnthropicSingleBetaMessage(c *gin.Context, resp *anthropic.BetaMessage, responseModel string) error {
	if resp == nil {
		return errors.New("nil anthropic beta response")
	}

	setAnthropicSSEHeaders(c)
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return errors.New("streaming not supported by this connection")
	}

	model := responseModel
	if model == "" {
		model = string(resp.Model)
	}

	state := buildStreamState(resp.Usage.InputTokens, resp.Usage.OutputTokens)
	sendMessageStart(c, flusher, model, eventTypeMessageStart, sendAnthropicStreamEvent, state.inputTokens)

	hasToolUse := false
	for idx, block := range resp.Content {
		switch v := block.AsAny().(type) {
		case anthropic.BetaTextBlock:
			sendContentBlockStart(c, idx, blockTypeText, map[string]interface{}{"text": ""}, flusher)
			if v.Text != "" {
				sendContentBlockDelta(c, idx, map[string]interface{}{
					"type": deltaTypeTextDelta,
					"text": v.Text,
				}, flusher)
			}
			sendContentBlockStop(c, state, idx, flusher)
		case anthropic.BetaThinkingBlock:
			sendContentBlockStart(c, idx, blockTypeThinking, map[string]interface{}{"thinking": ""}, flusher)
			if v.Thinking != "" {
				sendContentBlockDelta(c, idx, map[string]interface{}{
					"type":     deltaTypeThinkingDelta,
					"thinking": v.Thinking,
				}, flusher)
			}
			sendContentBlockStop(c, state, idx, flusher)
		case anthropic.BetaToolUseBlock:
			hasToolUse = true
			sendContentBlockStart(c, idx, blockTypeToolUse, map[string]interface{}{
				"id":    v.ID,
				"name":  v.Name,
				"input": map[string]any{},
			}, flusher)
			if inputBytes, err := json.Marshal(v.Input); err == nil {
				sendContentBlockDelta(c, idx, map[string]interface{}{
					"type":         deltaTypeInputJSONDelta,
					"partial_json": string(inputBytes),
				}, flusher)
			}
			sendContentBlockStop(c, state, idx, flusher)
		}
	}

	stopReason := anthropicStopReasonEndTurn
	if hasToolUse {
		stopReason = anthropicStopReasonToolUse
	}
	sendMessageDelta(c, state, stopReason, flusher)
	sendMessageStop(c, fmt.Sprintf("msg_%d", time.Now().Unix()), model, state, stopReason, flusher)
	return nil
}
