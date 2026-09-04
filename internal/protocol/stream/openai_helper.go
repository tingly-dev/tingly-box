package stream

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	openaistream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

// OpenAIResponsesEvent writes one Responses API SSE event, flushes, and marks
// TTFT on the first content-bearing (*.delta) event. MarkFirstToken is idempotent.
func OpenAIResponsesEvent(c *gin.Context, event string, v any) {
	if isOpenAIResponsesContentEvent(event) {
		protocol.MarkFirstToken(c)
	}
	switch vv := v.(type) {
	case []byte:
		c.Writer.WriteString(fmt.Sprintf("event: %s\ndata: %s\n\n", event, string(vv)))
	case string:
		c.Writer.WriteString(fmt.Sprintf("event: %s\ndata: %s\n\n", event, vv))
	default:
		data, err := json.Marshal(v)
		if err != nil {
			logrus.WithContext(c.Request.Context()).Errorf("OpenAISSE: failed to marshal: %v", err)
			return
		}
		c.Writer.WriteString(fmt.Sprintf("event: %s\ndata: %s\n\n", event, data))
	}
}

// isOpenAIResponsesContentEvent reports whether a Responses API event carries
// content, which always arrives as a *.delta event.
func isOpenAIResponsesContentEvent(eventType string) bool {
	return strings.HasSuffix(eventType, ".delta")
}

// isOpenAIChatContentChunk reports whether an OpenAI Chat chunk carries content
// (text / tool calls / reasoning), skipping the leading role-only delta.
func isOpenAIChatContentChunk(chunk wire.ChatStreamChunk) bool {
	for _, choice := range chunk.Choices {
		if choice.Delta.Content != "" ||
			len(choice.Delta.ToolCalls) > 0 ||
			choice.Delta.ReasoningContent != "" {
			return true
		}
	}
	return false
}

// isOpenAIChatChunkMapContent is the raw-map variant of isOpenAIChatContentChunk
// for handlers that build OpenAI Chat chunks directly as maps.
func isOpenAIChatChunkMapContent(chunk map[string]interface{}) bool {
	choices, ok := chunk["choices"].([]map[string]interface{})
	if !ok {
		return false
	}
	for _, choice := range choices {
		delta, ok := choice["delta"].(map[string]interface{})
		if !ok {
			continue
		}
		if c, _ := delta["content"].(string); c != "" {
			return true
		}
		if tc, _ := delta["tool_calls"].([]map[string]interface{}); len(tc) > 0 {
			return true
		}
		if rc, _ := delta["reasoning_content"].(string); rc != "" {
			return true
		}
		if rf, _ := delta["refusal"].(string); rf != "" {
			return true
		}
	}
	return false
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
			logrus.WithContext(c.Request.Context()).Errorf("OpenAISSE: failed to marshal: %v", err)
			return
		}
		c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", data))
	}
}

// OpenAISSEDone writes the SSE [DONE] terminator and flushes.
func OpenAISSEDone(c *gin.Context) {
	c.Writer.WriteString("data: [DONE]\n\n")
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
	maps.Copy(base, extra)
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
	return id[:maxToolCallIDLength]
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

// openaiChatSSEWriter returns a handleFunc that writes OpenAI Chat wire chunks
// (both normal chunks and error chunks) as SSE, and marks TTFT on the first
// content-bearing chunk.
func openaiChatSSEWriter(c *gin.Context) func(event interface{}) error {
	return func(event interface{}) error {
		if chunk, ok := event.(wire.ChatStreamChunk); ok {
			if isOpenAIChatContentChunk(chunk) {
				protocol.MarkFirstToken(c)
			}
		}
		OpenAISSE(c, event)
		return nil
	}
}

// writeSSEChunk writes a single SSE chunk — kept for callers in other files.
func writeSSEChunk(c *gin.Context, _ interface{ Flush() }, chunk any) {
	OpenAISSE(c, chunk)
}

// SendResponsesStreamFailure ends a Responses SSE stream the way the protocol
// itself ends a failed turn: with a `response.failed` event carrying the reason,
// followed by the legacy `error` frame.
//
// The terminal event is what makes the failure legible. A bare `event: error`
// frame is dropped by the strictest consumer of this surface: Codex's SSE reader
// (codex-rs, codex-api/src/sse/responses.rs) has no match arm for "error", so the
// frame lands in the catch-all, nothing is recorded, and the stream still ends
// with no terminal event — the client reports the generic "stream closed before
// response.completed" and the actual reason (upstream truncation, read error,
// in-band upstream error) is lost. That is also why retrying never helps: the
// user never learns what to fix. "response.failed" IS handled there: the client
// reads response.error.{code,message}, classifies it (context window, quota,
// rate limit, otherwise retryable) and surfaces the message.
//
// responseID may be empty; when the stream already announced one, passing it
// keeps the failure attached to the response the client was streaming.
func SendResponsesStreamFailure(c *gin.Context, responseID, code, message string) {
	response := map[string]interface{}{
		"object": "response",
		"status": "failed",
		"error": map[string]interface{}{
			"type":    "stream_error",
			"code":    code,
			"message": message,
		},
	}
	if responseID != "" {
		response["id"] = responseID
	}
	OpenAIResponsesEvent(c, "response.failed", map[string]interface{}{
		"type":     "response.failed",
		"response": response,
	})
	OpenAIResponsesEvent(c, "error", map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    "stream_error",
			"code":    code,
		},
	})
}

// describeResponsesStreamError restores the detail the OpenAI SDK drops when it
// aborts a stream on an in-band error event.
//
// ssestream ends the stream as soon as an event carries a top-level "error" key
// and uses gjson's string form of that value as the message — which is empty for
// `"error": null` and for `"error": {}`. The result is a bare "received error
// while streaming: " with nothing to act on. Re-attach the raw event so the log
// line and the client both name what actually arrived.
func describeResponsesStreamError(err error) error {
	var streamErr *openaistream.StreamError
	if !errors.As(err, &streamErr) {
		return err
	}
	detail := strings.TrimSpace(strings.TrimPrefix(streamErr.Message, "received error while streaming:"))
	if detail != "" {
		return err
	}
	raw := strings.TrimSpace(string(streamErr.Event.Data))
	if raw == "" {
		return err
	}
	return fmt.Errorf("upstream sent an error event with no message (raw event: %s)", raw)
}
