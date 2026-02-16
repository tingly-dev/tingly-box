package stream

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicstream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// ===================================================================
// Anthropic Handle Functions
// ===================================================================

// HandleAnthropicV1Stream handles Anthropic v1 streaming response.
// Returns (UsageStat, error)
func HandleAnthropicV1Stream(hc *protocol.HandleContext, req anthropic.MessageNewParams, streamResp *anthropicstream.Stream[anthropic.MessageStreamEventUnion]) (protocol.UsageStat, error) {
	defer streamResp.Close()

	hc.SetupSSEHeaders()

	flusher, ok := hc.GinContext.Writer.(http.Flusher)
	if !ok {
		return protocol.ZeroUsageStat(), errors.New("streaming not supported")
	}

	var inputTokens, outputTokens int
	var hasUsage bool

	err := hc.ProcessStream(
		func() (bool, error, interface{}) {
			if !streamResp.Next() {
				return false, nil, nil
			}
			// Current() returns a value, but we need a pointer for modification
			evt := streamResp.Current()
			return true, nil, &evt
		},
		func(event interface{}) error {
			evt := event.(*anthropic.MessageStreamEventUnion)
			evt.Message.Model = anthropic.Model(hc.ResponseModel)

			if evt.Usage.InputTokens > 0 {
				inputTokens = int(evt.Usage.InputTokens)
				hasUsage = true
			}
			if evt.Usage.OutputTokens > 0 {
				outputTokens = int(evt.Usage.OutputTokens)
				hasUsage = true
			}

			eventMap, err := toEventMap(evt, evt.Type)
			if err != nil {
				return err
			}

			if handleToolUseBuffer(hc.GinContext, false, evt.Type, int(evt.Index), evt.ContentBlock, eventMap) {
				return nil
			}

			sendAnthropicStreamEvent(hc.GinContext, evt.Type, eventMap, flusher)
			return nil
		},
	)

	// Handle errors
	if err != nil {
		if errors.Is(err, context.Canceled) || protocol.IsContextCanceled(err) {
			logrus.Debug("Anthropic v1 stream canceled by client")
			if !hasUsage {
				return protocol.ZeroUsageStat(), nil
			}
			return protocol.NewUsageStat(inputTokens, outputTokens), nil
		}

		MarshalAndSendErrorEvent(hc.GinContext, err.Error(), "stream_error", "stream_failed")
		return protocol.NewUsageStat(inputTokens, outputTokens), err
	}

	if err := injectGuardrailsBlock(hc.GinContext, false); err != nil {
		logrus.Debugf("Guardrails inject error: %v", err)
	}

	SendFinishEvent(hc.GinContext)

	return protocol.NewUsageStat(inputTokens, outputTokens), nil
}

// HandleAnthropicV1BetaStream handles Anthropic v1 beta streaming response.
// Returns (UsageStat, error)
func HandleAnthropicV1BetaStream(hc *protocol.HandleContext, req anthropic.BetaMessageNewParams, streamResp *anthropicstream.Stream[anthropic.BetaRawMessageStreamEventUnion]) (protocol.UsageStat, error) {
	defer streamResp.Close()

	hc.SetupSSEHeaders()

	flusher, ok := hc.GinContext.Writer.(http.Flusher)
	if !ok {
		return protocol.ZeroUsageStat(), errors.New("streaming not supported")
	}

	var inputTokens, outputTokens int
	var hasUsage bool

	err := hc.ProcessStream(
		func() (bool, error, interface{}) {
			if !streamResp.Next() {
				return false, nil, nil
			}
			// Current() returns a value, but we need a pointer for modification
			evt := streamResp.Current()
			return true, nil, &evt
		},
		func(event interface{}) error {
			evt := event.(*anthropic.BetaRawMessageStreamEventUnion)
			evt.Message.Model = anthropic.Model(hc.ResponseModel)

			if evt.Usage.InputTokens > 0 {
				inputTokens = int(evt.Usage.InputTokens)
				hasUsage = true
			}
			if evt.Usage.OutputTokens > 0 {
				outputTokens = int(evt.Usage.OutputTokens)
				hasUsage = true
			}

			eventMap, err := toEventMap(evt, evt.Type)
			if err != nil {
				return err
			}

			if handleToolUseBuffer(hc.GinContext, true, evt.Type, int(evt.Index), evt.ContentBlock, eventMap) {
				return nil
			}

			sendAnthropicBetaStreamEvent(hc.GinContext, evt.Type, eventMap, flusher)
			return nil
		},
	)

	// Handle errors
	if err != nil {
		if errors.Is(err, context.Canceled) || protocol.IsContextCanceled(err) {
			logrus.Debug("Anthropic v1 beta stream canceled by client")
			if !hasUsage {
				return protocol.ZeroUsageStat(), nil
			}
			return protocol.NewUsageStat(inputTokens, outputTokens), nil
		}

		MarshalAndSendErrorEvent(hc.GinContext, err.Error(), "stream_error", "stream_failed")
		return protocol.NewUsageStat(inputTokens, outputTokens), err
	}

	if err := injectGuardrailsBlock(hc.GinContext, true); err != nil {
		logrus.Debugf("Guardrails inject error: %v", err)
	}

	SendFinishEvent(hc.GinContext)

	return protocol.NewUsageStat(inputTokens, outputTokens), nil
}

func toEventMap(evt interface{}, eventType string) (map[string]interface{}, error) {
	raw, err := json.Marshal(evt)
	if err != nil {
		return nil, err
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if eventType != "" {
		payload["type"] = eventType
	}
	return payload, nil
}

type bufferedEvent struct {
	eventType string
	payload   map[string]interface{}
}

type toolUseBufferState struct {
	ByIndex       map[int][]bufferedEvent
	ToolIDByIndex map[int]string
}

func getToolUseBufferState(c *gin.Context) *toolUseBufferState {
	if existing, ok := c.Get("guardrails_tool_buffer"); ok {
		if state, ok := existing.(*toolUseBufferState); ok {
			return state
		}
	}
	state := &toolUseBufferState{
		ByIndex:       make(map[int][]bufferedEvent),
		ToolIDByIndex: make(map[int]string),
	}
	c.Set("guardrails_tool_buffer", state)
	return state
}

type guardrailsBlockState struct {
	ToolMessages map[string]string
	BlockedIndex map[int]string
}

func getGuardrailsBlockState(c *gin.Context) *guardrailsBlockState {
	if existing, ok := c.Get("guardrails_block_state"); ok {
		if state, ok := existing.(*guardrailsBlockState); ok {
			return state
		}
	}
	state := &guardrailsBlockState{
		ToolMessages: make(map[string]string),
		BlockedIndex: make(map[int]string),
	}
	c.Set("guardrails_block_state", state)
	return state
}

// RegisterGuardrailsBlock registers a tool_use block that should be intercepted.
func RegisterGuardrailsBlock(c *gin.Context, toolID string, index int, message string) {
	if toolID == "" || message == "" {
		return
	}
	state := getGuardrailsBlockState(c)
	state.ToolMessages[toolID] = message
	state.BlockedIndex[index] = toolID
}

func extractBlockTypeAndID(block interface{}) (string, string) {
	if block == nil {
		return "", ""
	}
	raw, err := json.Marshal(block)
	if err != nil {
		return "", ""
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", ""
	}
	blockType, _ := payload["type"].(string)
	if id, ok := payload["id"].(string); ok {
		return blockType, id
	}
	return blockType, ""
}

func handleToolUseBuffer(c *gin.Context, beta bool, eventType string, index int, block interface{}, eventMap map[string]interface{}) bool {
	switch eventType {
	case eventTypeContentBlockStart:
		blockType, toolID := extractBlockTypeAndID(block)
		if blockType != "tool_use" {
			return false
		}
		state := getToolUseBufferState(c)
		state.ToolIDByIndex[index] = toolID
		state.ByIndex[index] = append(state.ByIndex[index], bufferedEvent{eventType: eventType, payload: eventMap})
		return true
	case eventTypeContentBlockDelta, eventTypeContentBlockStop:
		state := getToolUseBufferState(c)
		if _, ok := state.ByIndex[index]; !ok {
			return false
		}
		state.ByIndex[index] = append(state.ByIndex[index], bufferedEvent{eventType: eventType, payload: eventMap})
		if eventType != eventTypeContentBlockStop {
			return true
		}

		toolID := state.ToolIDByIndex[index]
		blockState := getGuardrailsBlockState(c)
		if message, ok := blockState.ToolMessages[toolID]; ok {
			flusher, ok := c.Writer.(http.Flusher)
			if ok {
				_ = emitGuardrailsTextBlock(c, beta, index, message, flusher)
			} else {
				logrus.Debug("Guardrails tool buffer: streaming not supported")
			}
			delete(state.ByIndex, index)
			delete(state.ToolIDByIndex, index)
			return true
		}

		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			logrus.Debug("Guardrails tool buffer: streaming not supported")
			delete(state.ByIndex, index)
			delete(state.ToolIDByIndex, index)
			return true
		}
		for _, buffered := range state.ByIndex[index] {
			if beta {
				sendAnthropicBetaStreamEvent(c, buffered.eventType, buffered.payload, flusher)
			} else {
				sendAnthropicStreamEvent(c, buffered.eventType, buffered.payload, flusher)
			}
		}
		delete(state.ByIndex, index)
		delete(state.ToolIDByIndex, index)
		return true
	}
	return false
}

func emitGuardrailsTextBlock(c *gin.Context, beta bool, index int, message string, flusher http.Flusher) error {
	if message == "" {
		return nil
	}

	start := map[string]interface{}{
		"type":  eventTypeContentBlockStart,
		"index": index,
		"content_block": map[string]interface{}{
			"type": "text",
			"text": "",
		},
	}
	delta := map[string]interface{}{
		"type":  eventTypeContentBlockDelta,
		"index": index,
		"delta": map[string]interface{}{
			"type": "text_delta",
			"text": message,
		},
	}
	stop := map[string]interface{}{
		"type":  eventTypeContentBlockStop,
		"index": index,
	}

	if beta {
		sendAnthropicBetaStreamEvent(c, eventTypeContentBlockStart, start, flusher)
		sendAnthropicBetaStreamEvent(c, eventTypeContentBlockDelta, delta, flusher)
		sendAnthropicBetaStreamEvent(c, eventTypeContentBlockStop, stop, flusher)
		return nil
	}

	sendAnthropicStreamEvent(c, eventTypeContentBlockStart, start, flusher)
	sendAnthropicStreamEvent(c, eventTypeContentBlockDelta, delta, flusher)
	sendAnthropicStreamEvent(c, eventTypeContentBlockStop, stop, flusher)
	return nil
}

// ===================================================================
// OpenAI Handle Functions
// ===================================================================
