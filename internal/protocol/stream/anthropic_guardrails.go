package stream

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	guardrailsmutate "github.com/tingly-dev/tingly-box/internal/guardrails/mutate"
)

// rewriteAnthropicGuardrailsEvent keeps the main Anthropic passthrough loop
// focused on stream orchestration. Guardrails-specific event rewriting, tool_use
// buffering, and credential alias restoration all live here.
func rewriteAnthropicGuardrailsEvent(c *gin.Context, beta bool, eventType string, index int, block interface{}, rawJSON string) (bool, error) {
	if !guardrailsmutate.ShouldRewriteAnthropicEvent(c, eventType, block) {
		return false, nil
	}

	var eventMap map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &eventMap); err != nil {
		return false, err
	}
	if eventType != "" {
		eventMap["type"] = eventType
	}
	guardrailsmutate.RestoreCredentialAliasesInAnthropicEventMap(c, eventMap)
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return false, errors.New("streaming not supported")
	}

	if handled, blockMessage, passthrough := guardrailsmutate.HandleAnthropicToolUseBuffer(c, eventType, index, block, eventMap); handled {
		if blockMessage != "" {
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
					"text": blockMessage,
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
		if len(passthrough) > 0 {
			for _, buffered := range passthrough {
				sendAnthropicGuardrailsMapEvent(c, beta, buffered.EventType, buffered.Payload, flusher)
			}
			return true, nil
		}
		return true, nil
	}

	sendAnthropicGuardrailsMapEvent(c, beta, eventType, eventMap, flusher)
	return true, nil
}

func sendAnthropicGuardrailsMapEvent(c *gin.Context, beta bool, eventType string, eventData map[string]interface{}, flusher http.Flusher) {
	if beta {
		sendAnthropicBetaStreamEvent(c, eventType, eventData, flusher)
		return
	}
	sendAnthropicStreamEvent(c, eventType, eventData, flusher)
}
