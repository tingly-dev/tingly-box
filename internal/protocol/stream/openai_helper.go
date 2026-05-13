package stream

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func OpenAIResponsesEvent(c *gin.Context, event string, v any) {
	switch vv := v.(type) {
	case []byte:
		c.Writer.WriteString(fmt.Sprintf("event: %s\ndata: %s\n\n", event, string(vv)))
	case string:
		c.Writer.WriteString(fmt.Sprintf("event: %s\ndata: %s\n\n", event, vv))
	default:
		data, err := json.Marshal(v)
		if err != nil {
			logrus.Errorf("OpenAISSE: failed to marshal: %v", err)
			return
		}
		c.Writer.WriteString(fmt.Sprintf("event: %s\ndata: %s\n\n", event, data))
	}
}

// OpenAISSE marshals v to JSON and writes it as an OpenAI-style SSE data line, then flushes.
// MENTION: Must keep extra space after "data:" to match OpenAI wire format.
func OpenAISSE(c *gin.Context, v any) {
	switch vv := v.(type) {
	case []byte:
		c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", string(vv)))
	case string:
		c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", vv))
	default:
		data, err := json.Marshal(v)
		if err != nil {
			logrus.Errorf("OpenAISSE: failed to marshal: %v", err)
			return
		}
		c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", data))
	}
}

// OpenAISSEDone writes the SSE [DONE] terminator and flushes.
func OpenAISSEDone(c *gin.Context) {
	c.Writer.WriteString("data: [DONE]\n\n")
}

func nextSequenceNumber(state *chatToResponsesState) int64 {
	state.sequenceNumber++
	return state.sequenceNumber
}

// FilterSpecialFields removes special fields that have dedicated content blocks
// e.g., reasoning_content is handled as thinking block, not merged into text_delta
func FilterSpecialFields(extras map[string]interface{}) map[string]interface{} {
	if extras == nil || len(extras) == 0 {
		return extras
	}
	result := make(map[string]interface{})
	for k, v := range extras {
		if k != OpenaiFieldReasoningContent {
			result[k] = v
		}
	}
	return result
}

// FilterOpenAIProtocolFields removes OpenAI protocol fields that should NOT appear in Anthropic message_delta.
// These fields are already properly handled via content_block events and should not be duplicated.
func FilterOpenAIProtocolFields(extras map[string]interface{}) map[string]interface{} {
	if extras == nil || len(extras) == 0 {
		return extras
	}
	result := make(map[string]interface{})
	// OpenAI protocol fields that must not appear in Anthropic message_delta
	// - content: handled via content_block_start/delta for text
	// - role: always "assistant" in responses, not needed in delta
	// - tool_calls: handled via content_block_start/delta/stop for tool_use
	// - refusal: handled via content_block for refusal text
	openAIProtocolFields := map[string]bool{
		"content":    true,
		"role":       true,
		"tool_calls": true,
		"refusal":    true,
	}
	for k, v := range extras {
		if !openAIProtocolFields[k] {
			result[k] = v
		}
	}
	return result
}

// GenerateObfuscationString generates a random string similar to "KOJz1A"
func GenerateObfuscationString() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based if crypto rand fails
		return base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))[:6]
	}
	return base64.URLEncoding.EncodeToString(b)[:6]
}

// responsesAPIEventSenders defines callbacks for sending Anthropic events in a specific format (v1 or beta)
type responsesAPIEventSenders struct {
	SendMessageStart      func(event map[string]interface{}, flusher http.Flusher)
	SendContentBlockStart func(index int, blockType string, content map[string]interface{}, flusher http.Flusher)
	SendContentBlockDelta func(index int, content map[string]interface{}, flusher http.Flusher)
	SendContentBlockStop  func(state *streamState, index int, flusher http.Flusher)
	SendStopEvents        func(state *streamState, flusher http.Flusher)
	SendMessageDelta      func(state *streamState, stopReason string, flusher http.Flusher)
	SendMessageStop       func(messageID, model string, state *streamState, stopReason string, flusher http.Flusher)
	SendErrorEvent        func(event map[string]interface{}, flusher http.Flusher)
}

// parseRawJSON parses raw JSON string into map[string]interface{}
func parseRawJSON(rawJSON string) map[string]interface{} {
	if rawJSON == "" {
		return nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &result); err != nil {
		return nil
	}
	return result
}

// mergeMaps merges extra fields into the base map
func mergeMaps(base map[string]interface{}, extra map[string]interface{}) map[string]interface{} {
	if extra == nil || len(extra) == 0 {
		return base
	}
	if base == nil {
		base = make(map[string]interface{})
	}
	for k, v := range extra {
		base[k] = v
	}
	return base
}

// extractString extracts string value from interface{}, handling different types
func extractString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch tv := v.(type) {
	case string:
		return tv
	case []byte:
		return string(tv)
	default:
		return fmt.Sprintf("%v", tv)
	}
}

// truncateToolCallID ensures tool call ID doesn't exceed OpenAI's 40 character limit
// OpenAI API requires tool_call.id to be <= 40 characters
func truncateToolCallID(id string) string {
	if len(id) <= maxToolCallIDLength {
		return id
	}
	// Truncate to max length and add a suffix to indicate truncation
	return id[:maxToolCallIDLength-3] + "..."
}

// rewriteToolCallIDForAnthropic converts an OpenAI-style tool call ID (call_...) to an
// Anthropic-style ID (toolu_...) for protocol compliance, then truncates if necessary.
func rewriteToolCallIDForAnthropic(id string) string {
	// MENTION: we keep this in comment but do not use it for loose.
	//const openAIPrefix = "call_"
	//const anthropicPrefix = "toolu_"
	//if len(id) >= len(openAIPrefix) && id[:len(openAIPrefix)] == openAIPrefix {
	//	id = anthropicPrefix + id[len(openAIPrefix):]
	//}
	return truncateToolCallID(id)
}

// pendingToolCall tracks a tool call being assembled from stream chunks
type pendingToolCall struct {
	id    string
	name  string
	input string
	emit  bool
}
