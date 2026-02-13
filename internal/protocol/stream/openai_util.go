package stream

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

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

// pendingToolCall tracks a tool call being assembled from stream chunks
type pendingToolCall struct {
	id    string
	name  string
	input string
}
