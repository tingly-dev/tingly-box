package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/typ"

	"github.com/tingly-dev/tingly-box/internal/obs"
)

// RecordScenarioRequest records the scenario-level request (client -> tingly-box)
// This captures the original request from the client before any transformation
func (s *Server) RecordScenarioRequest(c *gin.Context, scenario string) *ScenarioRecorder {
	scenarioType := typ.RuleScenario(scenario)

	// Get or create sink for this scenario (on-demand)
	sink := s.GetOrCreateScenarioSink(scenarioType)
	if sink == nil {
		return nil
	}

	// Read and restore the request body

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logrus.Debugf("Failed to read request body for scenario recording: %v", err)
		return nil
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	// Parse request body as JSON
	var bodyJSON map[string]interface{}
	if len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, &bodyJSON); err != nil {
			logrus.Debugf("Failed to parse request body as JSON: %v", err)
			// Keep raw body as string if JSON parsing fails
			bodyJSON = map[string]interface{}{"raw": string(bodyBytes)}
		}
	}

	req := &obs.RecordRequest{
		Method:  c.Request.Method,
		URL:     c.Request.URL.String(),
		Headers: headerToMap(c.Request.Header),
		Body:    bodyJSON,
	}

	return &ScenarioRecorder{
		sink:      sink,
		scenario:  scenario,
		req:       req,
		startTime: time.Now(),
		c:         c,
		bodyBytes: bodyBytes,
	}
}

// ScenarioRecorder captures scenario-level request/response recording
type ScenarioRecorder struct {
	sink      *obs.Sink
	scenario  string
	req       *obs.RecordRequest
	startTime time.Time
	c         *gin.Context
	bodyBytes []byte

	// For streaming responses
	streamChunks      []map[string]interface{} // Collected stream chunks
	isStreaming       bool                     // Whether this is a streaming response
	assembledResponse map[string]interface{}   // Assembled response from stream
}

// EnableStreaming enables streaming mode for this recorder
func (sr *ScenarioRecorder) EnableStreaming() {
	if sr != nil {
		sr.isStreaming = true
		sr.streamChunks = make([]map[string]interface{}, 0)
	}
}

// RecordStreamChunk records a single stream chunk
func (sr *ScenarioRecorder) RecordStreamChunk(eventType string, chunk interface{}) {
	if sr == nil || !sr.isStreaming {
		return
	}

	// Convert chunk to map
	chunkMap, err := json.Marshal(chunk)
	if err != nil {
		logrus.Debugf("Failed to marshal stream chunk: %v", err)
		return
	}

	var chunkData map[string]interface{}
	if err := json.Unmarshal(chunkMap, &chunkData); err != nil {
		return
	}

	// Add event type if not present
	if _, ok := chunkData["type"]; !ok {
		chunkData["type"] = eventType
	}

	sr.streamChunks = append(sr.streamChunks, chunkData)
}

// SetAssembledResponse sets the assembled response for streaming
// Accepts any type (e.g., anthropic.Message) and converts to map for storage
func (sr *ScenarioRecorder) SetAssembledResponse(response any) {
	if sr == nil {
		return
	}

	// Convert response to map[string]interface{}
	var responseMap map[string]interface{}
	switch v := response.(type) {
	case map[string]interface{}:
		responseMap = v
	case []byte:
		if err := json.Unmarshal(v, &responseMap); err != nil {
			logrus.Debugf("Failed to unmarshal response: %v", err)
			return
		}
	default:
		// Marshal to JSON then unmarshal to map
		data, err := json.Marshal(response)
		if err != nil {
			logrus.Debugf("Failed to marshal response: %v", err)
			return
		}
		if err := json.Unmarshal(data, &responseMap); err != nil {
			logrus.Debugf("Failed to unmarshal response: %v", err)
			return
		}
	}

	sr.assembledResponse = responseMap
}

// GetStreamChunks returns the collected stream chunks
func (sr *ScenarioRecorder) GetStreamChunks() []map[string]interface{} {
	if sr == nil {
		return nil
	}
	return sr.streamChunks
}

// RecordResponse records the scenario-level response (tingly-box -> client)
// This captures the response sent back to the client
func (sr *ScenarioRecorder) RecordResponse(provider *typ.Provider, model string) {
	if sr == nil || sr.sink == nil {
		return
	}

	// Get response info from the context
	statusCode := sr.c.Writer.Status()
	headers := headerToMap(sr.c.Writer.Header())

	var bodyJSON map[string]interface{}

	// If this was a streaming response, use the assembled response
	if sr.isStreaming && sr.assembledResponse != nil {
		bodyJSON = sr.assembledResponse
	} else if sr.isStreaming && len(sr.streamChunks) > 0 {
		// Fallback for streaming: if no assembled response but we have chunks,
		// create a minimal response with the chunks
		bodyJSON = map[string]interface{}{
			"id":             fmt.Sprintf("msg_%d", sr.startTime.Unix()),
			"type":           "message",
			"role":           "assistant",
			"content":        []interface{}{},
			"model":          model,
			"_stream_chunks": len(sr.streamChunks),
			"_note":          "Assembled response not available, using fallback",
		}
		logrus.Debugf("ScenarioRecorder: using fallback in RecordResponse, chunks=%d", len(sr.streamChunks))
	} else {
		// Try to get response body if it was captured
		if responseBody, exists := sr.c.Get("response_body"); exists {
			if bytes, ok := responseBody.([]byte); ok {
				if err := json.Unmarshal(bytes, &bodyJSON); err == nil {
					bodyJSON = map[string]interface{}{"raw": string(bytes)}
				}
			}
		}
	}

	resp := &obs.RecordResponse{
		StatusCode: statusCode,
		Headers:    headers,
		Body:       bodyJSON,
	}

	// Mark as streaming if applicable
	if sr.isStreaming {
		resp.IsStreaming = true
		if len(sr.streamChunks) > 0 {
			// Store raw chunks for reference
			chunksJSON := make([]string, 0, len(sr.streamChunks))
			for _, chunk := range sr.streamChunks {
				if data, err := json.Marshal(chunk); err == nil {
					chunksJSON = append(chunksJSON, string(data))
				}
			}
			resp.StreamChunks = chunksJSON
		}
	}

	// Record with scenario-based file naming
	duration := time.Since(sr.startTime)
	sr.sink.RecordWithScenario(provider.Name, model, sr.scenario, sr.req, resp, duration, nil)
}

// RecordError records an error for the scenario-level request
func (sr *ScenarioRecorder) RecordError(err error) {
	if sr == nil || sr.sink == nil {
		return
	}

	resp := &obs.RecordResponse{
		StatusCode: sr.c.Writer.Status(),
		Headers:    headerToMap(sr.c.Writer.Header()),
	}

	// Extract model from request if available
	model := ""
	if sr.req.Body != nil {
		if m, ok := sr.req.Body["model"].(string); ok {
			model = m
		}
	}

	// Record with error
	duration := time.Since(sr.startTime)
	sr.sink.RecordWithScenario("tingly-box", model, sr.scenario, sr.req, resp, duration, err)
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

// streamRecorder encapsulates recording and assembly logic for streaming responses
// It provides a unified way to handle both v1 and v1beta Anthropic streaming events
type streamRecorder struct {
	recorder     *ScenarioRecorder
	assembler    *stream.AnthropicStreamAssembler
	inputTokens  int
	outputTokens int
	hasUsage     bool
}

// newStreamRecorder creates a new streamRecorder
func newStreamRecorder(recorder *ScenarioRecorder) *streamRecorder {
	if recorder == nil {
		return nil
	}
	recorder.EnableStreaming()
	return &streamRecorder{
		recorder:  recorder,
		assembler: stream.NewAnthropicStreamAssembler(),
	}
}

// RecordV1Event records a v1 stream event
func (sr *streamRecorder) RecordV1Event(event *anthropic.MessageStreamEventUnion) {
	if sr == nil {
		return
	}
	sr.recorder.RecordStreamChunk(event.Type, event)
	sr.assembler.RecordV1Event(event)
}

// RecordV1BetaEvent records a v1beta stream event
func (sr *streamRecorder) RecordV1BetaEvent(event *anthropic.BetaRawMessageStreamEventUnion) {
	if sr == nil {
		return
	}
	sr.recorder.RecordStreamChunk(event.Type, event)
	sr.assembler.RecordV1BetaEvent(event)
}

// Finish finishes recording and sets the assembled response
// For protocol conversion scenarios, it uses the tracked usage information
// If the assembler returns nil, it creates a fallback response from collected chunks
func (sr *streamRecorder) Finish(model string, inputTokens, outputTokens int) {
	if sr == nil {
		return
	}
	// Use tracked usage if provided values are zero and we have tracked usage
	if inputTokens == 0 && outputTokens == 0 && sr.hasUsage {
		inputTokens = sr.inputTokens
		outputTokens = sr.outputTokens
	}
	assembled := sr.assembler.Finish(model, inputTokens, outputTokens)
	if assembled != nil {
		sr.recorder.SetAssembledResponse(assembled)
	} else {
		// Fallback: if assembler returned nil but we have chunks, create a minimal response
		if len(sr.recorder.streamChunks) > 0 {
			fallbackResp := map[string]interface{}{
				"id":          fmt.Sprintf("msg_%d", sr.recorder.startTime.Unix()),
				"type":        "message",
				"role":        "assistant",
				"content":     []interface{}{},
				"model":       model,
				"stop_reason": sr.recorder.c.Query("stop_reason"),
				"usage": map[string]interface{}{
					"input_tokens":  inputTokens,
					"output_tokens": outputTokens,
				},
			}
			sr.recorder.SetAssembledResponse(fallbackResp)
			logrus.Debugf("StreamRecorder: using fallback response, chunks=%d", len(sr.recorder.streamChunks))
		}
	}
}

// RecordError records an error
func (sr *streamRecorder) RecordError(err error) {
	if sr == nil {
		return
	}
	sr.recorder.RecordError(err)
}

// RecordResponse records the final response
func (sr *streamRecorder) RecordResponse(provider *typ.Provider, model string) {
	if sr == nil {
		return
	}
	sr.recorder.RecordResponse(provider, model)
}

// RecordRawMapEvent records a raw map-based event (for protocol conversion scenarios)
// This is used when converting between different API formats (e.g., OpenAI -> Anthropic)
// It also extracts usage information from message_delta and message_stop events
func (sr *streamRecorder) RecordRawMapEvent(eventType string, event map[string]interface{}) {
	if sr == nil {
		return
	}

	// Convert map to BetaRawMessageStreamEventUnion
	data, err := json.Marshal(event)
	if err == nil {
		var betaEvent anthropic.BetaRawMessageStreamEventUnion
		if err := json.Unmarshal(data, &betaEvent); err == nil {
			betaEvent.Type = eventType
			sr.assembler.RecordV1BetaEvent(&betaEvent)
		}
	}

	sr.recorder.RecordStreamChunk(eventType, event)

	// Extract usage from message_delta event
	if eventType == "message_delta" {
		if usage, ok := event["usage"].(map[string]interface{}); ok {
			if inputTokens, ok := usage["input_tokens"].(float64); ok {
				sr.inputTokens = int(inputTokens)
			} else if inputTokens, ok := usage["input_tokens"].(int64); ok {
				sr.inputTokens = int(inputTokens)
			}
			if outputTokens, ok := usage["output_tokens"].(float64); ok {
				sr.outputTokens = int(outputTokens)
			} else if outputTokens, ok := usage["output_tokens"].(int64); ok {
				sr.outputTokens = int(outputTokens)
			}
			sr.hasUsage = true
		}
	}
}

// StreamEventRecorder returns the StreamEventRecorder interface for use in protocol packages
func (sr *streamRecorder) StreamEventRecorder() interface{} {
	if sr == nil {
		return nil
	}
	return sr
}

// SetupStreamRecorderInContext sets up the stream recorder in gin context for protocol conversion handlers
// This allows protocol handlers in the stream package to record events without direct dependency on server package
func (sr *streamRecorder) SetupStreamRecorderInContext(c *gin.Context, key string) {
	if sr == nil {
		return
	}
	c.Set(key, sr)
}

// ===================================================================
// Recorder Hook Builders
// ===================================================================

// NewRecorderHooks creates hook functions from a ScenarioRecorder for use with HandleContext.
// This allows decoupling the recorder from the handle context while maintaining recording functionality.
//
// Returns:
// - onStreamEvent: Hook for each stream event
// - onStreamComplete: Hook for stream completion
// - onStreamError: Hook for stream errors
func NewRecorderHooks(recorder *ScenarioRecorder) (onStreamEvent func(event interface{}) error, onStreamComplete func(inputTokens, outputTokens int), onStreamError func(err error)) {
	if recorder == nil {
		return nil, nil, nil
	}

	streamRec := newStreamRecorder(recorder)

	// OnStreamEvent hook - records each stream event
	onStreamEvent = func(event interface{}) error {
		if streamRec == nil {
			return nil
		}
		switch evt := event.(type) {
		case *anthropic.MessageStreamEventUnion:
			streamRec.RecordV1Event(evt)
		case *anthropic.BetaRawMessageStreamEventUnion:
			streamRec.RecordV1BetaEvent(evt)
		case map[string]interface{}:
			// For raw map events (protocol conversion scenarios)
			if eventType, ok := evt["type"].(string); ok {
				streamRec.RecordRawMapEvent(eventType, evt)
			}
		}
		return nil
	}

	// OnStreamComplete hook - finalizes recording
	onStreamComplete = func(inputTokens, outputTokens int) {
		if streamRec == nil {
			return
		}
		// Model is not available here, it needs to be set externally
		// or we can retrieve it from the recorder's gin context
		model := ""
		if recorder.c != nil {
			model = recorder.c.Query("model")
		}
		streamRec.Finish(model, inputTokens, outputTokens)
	}

	// OnStreamError hook - records errors
	onStreamError = func(err error) {
		if streamRec == nil {
			return
		}
		streamRec.RecordError(err)
	}

	return onStreamEvent, onStreamComplete, onStreamError
}

// NewRecorderHooksWithModel creates hook functions with an explicit model parameter.
// This is preferred when the model is known at hook creation time.
func NewRecorderHooksWithModel(recorder *ScenarioRecorder, model string, provider *typ.Provider) (onStreamEvent func(event interface{}) error, onStreamComplete func(inputTokens, outputTokens int), onStreamError func(err error)) {
	if recorder == nil {
		return nil, nil, nil
	}

	streamRec := newStreamRecorder(recorder)

	// OnStreamEvent hook - records each stream event
	onStreamEvent = func(event interface{}) error {
		if streamRec == nil {
			return nil
		}
		switch evt := event.(type) {
		case *anthropic.MessageStreamEventUnion:
			streamRec.RecordV1Event(evt)
		case *anthropic.BetaRawMessageStreamEventUnion:
			streamRec.RecordV1BetaEvent(evt)
		case map[string]interface{}:
			// For raw map events (protocol conversion scenarios)
			if eventType, ok := evt["type"].(string); ok {
				streamRec.RecordRawMapEvent(eventType, evt)
			}
		}
		return nil
	}

	// OnStreamComplete hook - finalizes recording with model and provider
	onStreamComplete = func(inputTokens, outputTokens int) {
		if streamRec == nil {
			return
		}
		streamRec.Finish(model, inputTokens, outputTokens)
		streamRec.RecordResponse(provider, model)
	}

	// OnStreamError hook - records errors
	onStreamError = func(err error) {
		if streamRec == nil {
			return
		}
		streamRec.RecordError(err)
	}

	return onStreamEvent, onStreamComplete, onStreamError
}

// NewNonStreamRecorderHook creates a hook for non-streaming responses.
func NewNonStreamRecorderHook(recorder *ScenarioRecorder, provider *typ.Provider, model string) func() {
	if recorder == nil {
		return nil
	}

	return func() {
		recorder.RecordResponse(provider, model)
	}
}
