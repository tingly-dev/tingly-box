package client

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Context key for scenario in request context
type contextKey string

const ScenarioContextKey contextKey = "scenario"

// RecordRoundTripper is an http.RoundTripper that records requests and responses
type RecordRoundTripper struct {
	transport  http.RoundTripper
	recordSink *obs.Sink
	provider   *typ.Provider
	apiStyle   protocol.APIStyle
}

// NewRecordRoundTripper creates a new record round tripper
func NewRecordRoundTripper(transport http.RoundTripper, recordSink *obs.Sink, provider *typ.Provider) *RecordRoundTripper {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &RecordRoundTripper{
		transport:  transport,
		recordSink: recordSink,
		provider:   provider,
		apiStyle:   provider.APIStyle,
	}
}

// RoundTrip executes a single HTTP transaction and records request/response
func (r *RecordRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	startTime := time.Now()

	// Extract scenario from request context
	scenario := ""
	if scenarioVal := req.Context().Value(ScenarioContextKey); scenarioVal != nil {
		if scenarioStr, ok := scenarioVal.(string); ok {
			scenario = scenarioStr
		}
	}

	// Prepare request record
	reqRecord := &obs.RecordRequest{
		Method:  req.Method,
		URL:     req.URL.String(),
		Headers: headerToMap(req.Header),
	}

	// Extract model from request body
	var model string
	if req.Body != nil && req.Body != http.NoBody {
		bodyBytes, err := io.ReadAll(req.Body)
		if err == nil && len(bodyBytes) > 0 {
			req.Body.Close()
			// Try to parse as JSON to extract model
			var jsonObj map[string]interface{}
			if json.Unmarshal(bodyBytes, &jsonObj) == nil {
				reqRecord.Body = jsonObj
				if m, ok := jsonObj["model"].(string); ok {
					model = m
				}
			}
			// Restore the body for the actual request
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}
	}

	// Execute the request
	resp, err := r.transport.RoundTrip(req)

	var respRecord *obs.RecordResponse
	if resp != nil {
		respRecord = &obs.RecordResponse{
			StatusCode: resp.StatusCode,
			Headers:    headerToMap(resp.Header),
		}

		// Check if this is a streaming response
		isStreaming := strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")

		// Handle response body
		if resp.Body != nil && resp.Body != http.NoBody {
			if isStreaming {
				// For streaming responses, delay recording until stream is fully read
				respRecord.IsStreaming = true
				resp.Body = newRecordingReader(resp.Body, r.apiStyle, func(rawContent string, chunks []string, assembledBody map[string]any) {
					// Populate response record in onClose callback
					respRecord.StreamChunks = chunks
					if assembledBody != nil {
						respRecord.Body = assembledBody
					}
					// Record after stream is consumed
					duration := time.Since(startTime)
					if r.recordSink != nil && r.recordSink.IsEnabled() {
						r.recordSink.RecordWithScenario(r.provider.Name, model, scenario, reqRecord, respRecord, duration, err)
					}
				})
				// For streaming, return early - recording will happen in onClose
				return resp, err
			} else {
				// For non-streaming responses, read the entire body
				bodyBytes, readErr := io.ReadAll(resp.Body)
				if readErr == nil && len(bodyBytes) > 0 {
					resp.Body.Close()
					// Try to parse as JSON
					var jsonObj any
					if json.Unmarshal(bodyBytes, &jsonObj) == nil {
						if objMap, ok := jsonObj.(map[string]any); ok {
							respRecord.Body = objMap
						}
					}
					// Restore the body for the actual response
					resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				}
			}
		}
	}

	// Record the request/response (non-streaming or error case)
	duration := time.Since(startTime)
	if r.recordSink != nil && r.recordSink.IsEnabled() {
		r.recordSink.RecordWithScenario(r.provider.Name, model, scenario, reqRecord, respRecord, duration, err)
	}

	return resp, err
}

// recordingReader wraps an io.ReadCloser and records all data read from it
type recordingReader struct {
	source    io.ReadCloser
	buffer    *bytes.Buffer
	apiStyle  protocol.APIStyle
	onClose   func(rawContent string, chunks []string, assembledBody map[string]any)
	closeOnce sync.Once
	closed    bool
}

func newRecordingReader(source io.ReadCloser, apiStyle protocol.APIStyle, onClose func(string, []string, map[string]any)) *recordingReader {
	return &recordingReader{
		source:   source,
		buffer:   &bytes.Buffer{},
		apiStyle: apiStyle,
		onClose:  onClose,
	}
}

func (r *recordingReader) Read(p []byte) (n int, err error) {
	n, err = r.source.Read(p)
	if n > 0 {
		r.buffer.Write(p[:n])
	}
	// When stream ends (EOF), trigger recording
	if err == io.EOF && !r.closed {
		r.closed = true
		if r.onClose != nil {
			rawContent := r.buffer.String()
			chunks, assembledBody := parseSSEAndAssemble(rawContent, r.apiStyle)
			r.onClose(rawContent, chunks, assembledBody)
		}
	}
	return n, err
}

func (r *recordingReader) Close() error {
	// Close may not be called by SDK for streaming responses
	// Recording is triggered on EOF instead
	err := r.source.Close()
	r.closeOnce.Do(func() {
		// Trigger recording if not already done via EOF
		if !r.closed && r.onClose != nil {
			rawContent := r.buffer.String()
			chunks, assembledBody := parseSSEAndAssemble(rawContent, r.apiStyle)
			r.onClose(rawContent, chunks, assembledBody)
		}
	})
	return err
}

// parseSSEAndAssemble parses SSE content and assembles the complete response
// apiStyle specifies which stream format to use
// Returns: (parsed SSE data chunks, assembled complete body)
func parseSSEAndAssemble(sseContent string, apiStyle protocol.APIStyle) ([]string, map[string]any) {
	chunks := make([]string, 0)

	lines := strings.Split(sseContent, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		// Parse data: lines
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			data = strings.TrimSpace(data)
			// Skip [DONE] marker
			if data == "[DONE]" {
				continue
			}
			chunks = append(chunks, data)
		}
	}

	// Assemble based on apiStyle
	var assembledBody map[string]any
	switch apiStyle {
	case protocol.APIStyleOpenAI:
		assembledBody = assembleOpenAIResponse(chunks)
	case protocol.APIStyleAnthropic:
		assembledBody = assembleAnthropicResponse(chunks)
	default:
		// Unknown apiStyle, try OpenAI format as fallback
		assembledBody = assembleOpenAIResponse(chunks)
	}

	return chunks, assembledBody
}

// assembleOpenAIResponse assembles OpenAI-style SSE chunks into a complete response body
func assembleOpenAIResponse(chunks []string) map[string]any {
	if len(chunks) == 0 {
		return nil
	}

	// Try to parse the first chunk to determine response structure
	var firstChunk map[string]any
	if err := json.Unmarshal([]byte(chunks[0]), &firstChunk); err != nil {
		return nil
	}

	// For OpenAI-style responses, try to assemble choices
	if choices, ok := firstChunk["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			// Get message structure from first chunk
			baseMsg := getBaseMessage(choice)
			if baseMsg == nil {
				return firstChunk
			}

			// Accumulate content from all chunks
			for _, chunk := range chunks {
				var chunkData map[string]any
				if err := json.Unmarshal([]byte(chunk), &chunkData); err != nil {
					continue
				}
				if cs, ok := chunkData["choices"].([]any); ok && len(cs) > 0 {
					if c, ok := cs[0].(map[string]any); ok {
						accumulateContent(baseMsg, c)
					}
				}
			}

			// Build assembled response
			assembled := map[string]any{
				"id":      firstChunk["id"],
				"object":  firstChunk["object"],
				"created": firstChunk["created"],
				"model":   firstChunk["model"],
				"choices": []any{map[string]any{
					"index": 0,
					"message": map[string]any{
						"role":    baseMsg["role"],
						"content": baseMsg["content"],
					},
					"finish_reason": nil, // Will be set from last chunk
				}},
			}

			// Copy usage and finish_reason from last chunk if available
			if len(chunks) > 0 {
				var lastChunk map[string]any
				if err := json.Unmarshal([]byte(chunks[len(chunks)-1]), &lastChunk); err == nil {
					if usage, ok := lastChunk["usage"].(map[string]any); ok {
						assembled["usage"] = usage
					}
					// Get finish_reason from last chunk's choices
					if choices, ok := lastChunk["choices"].([]any); ok && len(choices) > 0 {
						if choice, ok := choices[0].(map[string]any); ok {
							if finishReason, ok := choice["finish_reason"]; ok {
								assembled["choices"].([]any)[0].(map[string]any)["finish_reason"] = finishReason
							}
						}
					}
				}
			}

			return assembled
		}
	}

	// Return first chunk as-is for non-standard formats
	return firstChunk
}

// getBaseMessage extracts the base message structure from a choice
func getBaseMessage(choice map[string]any) map[string]any {
	delta, ok := choice["delta"].(map[string]any)
	if !ok {
		return nil
	}

	baseMsg := map[string]any{
		"role":    delta["role"],
		"content": "",
	}
	if baseMsg["role"] == nil {
		baseMsg["role"] = "assistant"
	}
	return baseMsg
}

// accumulateContent accumulates content from a chunk delta into the base message
func accumulateContent(baseMsg, choice map[string]any) {
	delta, ok := choice["delta"].(map[string]any)
	if !ok {
		return
	}

	if content, ok := delta["content"].(string); ok {
		if existing, ok := baseMsg["content"].(string); ok {
			baseMsg["content"] = existing + content
		} else {
			baseMsg["content"] = content
		}
	}
	if role, ok := delta["role"].(string); ok && baseMsg["role"] == nil {
		baseMsg["role"] = role
	}
}

// assembleAnthropicResponse assembles Anthropic-style SSE chunks into a complete response body
func assembleAnthropicResponse(chunks []string) map[string]any {
	if len(chunks) == 0 {
		return nil
	}

	// Anthropic streaming response structure
	type anthropicMessage struct {
		ID           string           `json:"id"`
		Type         string           `json:"type"`
		Role         string           `json:"role"`
		Content      []map[string]any `json:"content"`
		Model        string           `json:"model"`
		StopReason   *string          `json:"stop_reason"`
		StopSequence *string          `json:"stop_sequence"`
		Usage        map[string]any   `json:"usage"`
	}

	var message *anthropicMessage
	contentBlocks := make([]map[string]any, 0)
	currentText := ""

	// Parse all chunks
	for _, chunk := range chunks {
		var chunkData map[string]any
		if err := json.Unmarshal([]byte(chunk), &chunkData); err != nil {
			continue
		}

		eventType, _ := chunkData["type"].(string)

		switch eventType {
		case "message_start":
			// Initialize message from message_start
			if msgData, ok := chunkData["message"].(map[string]any); ok {
				message = &anthropicMessage{
					ID:    toString(msgData["id"]),
					Type:  toString(msgData["type"]),
					Role:  toString(msgData["role"]),
					Model: toString(msgData["model"]),
					Usage: toMap(msgData["usage"]),
				}
			}

		case "content_block_start":
			// Start of a new content block
			if index, ok := chunkData["index"].(float64); ok && int(index) == 0 {
				if blockData, ok := chunkData["content_block"].(map[string]any); ok {
					if blockType, ok := blockData["type"].(string); ok && blockType == "text" {
						contentBlocks = append(contentBlocks, map[string]any{
							"type": "text",
							"text": "",
						})
					}
				}
			}

		case "content_block_delta":
			// Accumulate text content
			if index, ok := chunkData["index"].(float64); ok && int(index) == 0 {
				if delta, ok := chunkData["delta"].(map[string]any); ok {
					if text, ok := delta["text"].(string); ok {
						currentText += text
					}
				}
			}

		case "message_delta":
			// Final message metadata (stop_reason, usage)
			if delta, ok := chunkData["delta"].(map[string]any); ok {
				if stopReason, ok := delta["stop_reason"].(string); ok {
					if message != nil {
						message.StopReason = &stopReason
					}
				}
			}
			if usage, ok := chunkData["usage"].(map[string]any); ok {
				if message != nil {
					message.Usage = usage
				}
			}
		}
	}

	// Build assembled response in Anthropic format
	if message != nil {
		// Update content block with accumulated text
		if len(contentBlocks) > 0 {
			contentBlocks[0]["text"] = currentText
		}
		message.Content = contentBlocks

		// Convert to map for JSON output
		return map[string]any{
			"id":            message.ID,
			"type":          message.Type,
			"role":          message.Role,
			"content":       message.Content,
			"model":         message.Model,
			"stop_reason":   message.StopReason,
			"stop_sequence": message.StopSequence,
			"usage":         message.Usage,
		}
	}

	// Fallback: return first chunk
	var firstChunk map[string]any
	if err := json.Unmarshal([]byte(chunks[0]), &firstChunk); err == nil {
		return firstChunk
	}
	return nil
}

// Helper type conversion functions
func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func toMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

// headerToMap converts http.Header to map[string]string
func headerToMap(h http.Header) map[string]string {
	result := make(map[string]string)
	for k, v := range h {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result
}
