package stream

import (
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	guardrailsmutate "github.com/tingly-dev/tingly-box/internal/guardrails/mutate"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// rewriteAnthropicGuardrailsEvent keeps the main Anthropic passthrough loop
// focused on stream orchestration. It only intercepts tool_use-related events;
// all other events fall through to the normal passthrough sender.
func rewriteAnthropicGuardrailsEvent(hc *protocol.HandleContext, beta bool, event interface{}) (bool, error) {
	if hc == nil || hc.GinContext == nil {
		return false, nil
	}
	c := hc.GinContext
	var (
		eventType string
		index     int
		block     interface{}
		rawJSON   string
	)
	switch evt := event.(type) {
	case *anthropic.MessageStreamEventUnion:
		if evt == nil {
			return false, nil
		}
		eventType = evt.Type
		index = int(evt.Index)
		block = evt.ContentBlock
		rawJSON = evt.RawJSON()
	case *anthropic.BetaRawMessageStreamEventUnion:
		if evt == nil {
			return false, nil
		}
		eventType = evt.Type
		index = int(evt.Index)
		block = evt.ContentBlock
		rawJSON = evt.RawJSON()
	default:
		return false, nil
	}

	if hc.Guardrails == nil || !guardrailsmutate.ShouldRewriteAnthropicEvent(hc.Guardrails.Stream, eventType, block) {
		return false, nil
	}

	var eventMap map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &eventMap); err != nil {
		return false, err
	}
	if eventType != "" {
		eventMap["type"] = eventType
	}
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return false, errors.New("streaming not supported")
	}

	decision := guardrailsmutate.HandleAnthropicToolUseBuffer(hc.Guardrails.CredentialMask, hc.Guardrails.Stream, eventType, index, block, eventMap)
	switch decision.Kind {
	case guardrailsmutate.AnthropicToolUseDecisionBuffer:
		return true, nil
	case guardrailsmutate.AnthropicToolUseDecisionBlock:
		if decision.BlockMessage != "" {
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
					"text": decision.BlockMessage,
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
			} else {
				sendAnthropicStreamEvent(c, eventTypeContentBlockStart, start, flusher)
				sendAnthropicStreamEvent(c, eventTypeContentBlockDelta, delta, flusher)
				sendAnthropicStreamEvent(c, eventTypeContentBlockStop, stop, flusher)
			}
			return true, nil
		}
		return true, nil
	case guardrailsmutate.AnthropicToolUseDecisionPassthrough:
		for _, buffered := range decision.Passthrough {
			sendAnthropicGuardrailsMapEvent(c, beta, buffered.EventType, buffered.Payload, flusher)
		}
		return true, nil
	}
	return false, nil
}

func sendAnthropicGuardrailsMapEvent(c *gin.Context, beta bool, eventType string, eventData map[string]interface{}, flusher http.Flusher) {
	if beta {
		sendAnthropicBetaStreamEvent(c, eventType, eventData, flusher)
		return
	}
	sendAnthropicStreamEvent(c, eventType, eventData, flusher)
}
