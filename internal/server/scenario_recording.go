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
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/typ"

	"github.com/tingly-dev/tingly-box/internal/obs"
)

// RecordScenarioRequest records the scenario-level request (client -> tingly-box)
// This captures the original request from the client before any transformation
// protocolType is used to select the appropriate extractor for prompt recording
func (s *Server) RecordScenarioRequest(c *gin.Context, scenario string, protocolType db.ProtocolType) *ScenarioRecorder {
	scenarioType := typ.RuleScenario(scenario)

	// Get or create sink for this scenario (on-demand)
	sink := s.GetOrCreateScenarioSink(scenarioType)
	if sink == nil {
		return nil
	}

	// Get prompt store if prompt recording is enabled
	var memoryStore *db.MemoryStore
	memoryStore = s.config.GetMemoryStore()

	// Get the appropriate extractor for this protocol
	var extractor MemoryExtractor
	extractor = PromptExtractors[protocolType]

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
		sink:        sink,
		memoryStore: memoryStore,
		extractor:   extractor,
		protocol:    protocolType,
		scenario:    scenario,
		req:         req,
		startTime:   time.Now(),
		c:           c,
		bodyBytes:   bodyBytes,
	}
}

// ScenarioRecorder captures scenario-level request/response recording
// Enhanced to support prompt recording to database when enabled
type ScenarioRecorder struct {
	sink        *obs.Sink
	memoryStore *db.MemoryStore
	extractor   MemoryExtractor
	protocol    db.ProtocolType
	scenario    string
	req         *obs.RecordRequest
	startTime   time.Time
	c           *gin.Context
	bodyBytes   []byte

	// parsedRequest holds the pre-parsed request from upstream handlers.
	// This avoids re-parsing the bodyBytes for prompt recording.
	// Type can be: *protocol.AnthropicBetaMessagesRequest, *protocol.AnthropicMessagesRequest, etc.
	parsedRequest interface{}

	// For streaming responses
	streamChunks      []map[string]interface{} // Collected stream chunks
	isStreaming       bool                     // Whether this is a streaming response
	assembledResponse map[string]interface{}   // Assembled response from stream

	// Token counts for prompt recording
	inputTokens  int
	outputTokens int
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

// SetParsedRequest sets the pre-parsed request from upstream handlers.
// This avoids re-parsing the bodyBytes for prompt recording.
// Supported types: *protocol.AnthropicBetaMessagesRequest, *protocol.AnthropicMessagesRequest
func (sr *ScenarioRecorder) SetParsedRequest(req interface{}) {
	if sr == nil {
		return
	}
	sr.parsedRequest = req
}

// RecordResponse records the scenario-level response (tingly-box -> client)
// This captures the response sent back to the client
// Enhanced to also record prompt rounds to database when prompt_recording is enabled
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

	// Enhanced: Also record to database if prompt recording is enabled
	sr.recordPromptRounds(provider, model, headers, duration)
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
	// Store token counts on the scenario recorder for prompt recording
	if sr.recorder != nil {
		sr.recorder.inputTokens = inputTokens
		sr.recorder.outputTokens = outputTokens
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
// Usage is tracked internally in the event hook, so complete hooks don't need usage parameters.
//
// Returns:
// - onStreamEvent: Hook for each stream event
// - onStreamComplete: Hook for stream completion
// - onStreamError: Hook for stream errors
func NewRecorderHooks(recorder *ScenarioRecorder) (onStreamEvent func(event interface{}) error, onStreamComplete func(), onStreamError func(err error)) {
	if recorder == nil {
		return nil, nil, nil
	}

	streamRec := newStreamRecorder(recorder)

	// OnStreamEvent hook - records each stream event and tracks usage
	onStreamEvent = func(event interface{}) error {
		if streamRec == nil {
			return nil
		}
		switch evt := event.(type) {
		case *anthropic.MessageStreamEventUnion:
			streamRec.RecordV1Event(evt)
			// Track usage from events
			if evt.Usage.InputTokens > 0 {
				streamRec.inputTokens = int(evt.Usage.InputTokens)
				streamRec.hasUsage = true
			}
			if evt.Usage.OutputTokens > 0 {
				streamRec.outputTokens = int(evt.Usage.OutputTokens)
				streamRec.hasUsage = true
			}
		case *anthropic.BetaRawMessageStreamEventUnion:
			streamRec.RecordV1BetaEvent(evt)
			// Track usage from events
			if evt.Usage.InputTokens > 0 {
				streamRec.inputTokens = int(evt.Usage.InputTokens)
				streamRec.hasUsage = true
			}
			if evt.Usage.OutputTokens > 0 {
				streamRec.outputTokens = int(evt.Usage.OutputTokens)
				streamRec.hasUsage = true
			}
		case map[string]interface{}:
			// For raw map events (protocol conversion scenarios)
			if eventType, ok := evt["type"].(string); ok {
				streamRec.RecordRawMapEvent(eventType, evt)
			}
		}
		return nil
	}

	// OnStreamComplete hook - finalizes recording using internally tracked usage
	onStreamComplete = func() {
		if streamRec == nil {
			return
		}
		// Model is not available here, it needs to be set externally
		// or we can retrieve it from the recorder's gin context
		model := ""
		if recorder.c != nil {
			model = recorder.c.Query("model")
		}
		streamRec.Finish(model, streamRec.inputTokens, streamRec.outputTokens)
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
// Usage is tracked internally in the event hook.
func NewRecorderHooksWithModel(recorder *ScenarioRecorder, model string, provider *typ.Provider) (onStreamEvent func(event interface{}) error, onStreamComplete func(), onStreamError func(err error)) {
	if recorder == nil {
		return nil, nil, nil
	}

	streamRec := newStreamRecorder(recorder)

	// OnStreamEvent hook - records each stream event and tracks usage
	onStreamEvent = func(event interface{}) error {
		if streamRec == nil {
			return nil
		}
		switch evt := event.(type) {
		case *anthropic.MessageStreamEventUnion:
			streamRec.RecordV1Event(evt)
			// Track usage from events
			if evt.Usage.InputTokens > 0 {
				streamRec.inputTokens = int(evt.Usage.InputTokens)
				streamRec.hasUsage = true
			}
			if evt.Usage.OutputTokens > 0 {
				streamRec.outputTokens = int(evt.Usage.OutputTokens)
				streamRec.hasUsage = true
			}
		case *anthropic.BetaRawMessageStreamEventUnion:
			streamRec.RecordV1BetaEvent(evt)
			// Track usage from events
			if evt.Usage.InputTokens > 0 {
				streamRec.inputTokens = int(evt.Usage.InputTokens)
				streamRec.hasUsage = true
			}
			if evt.Usage.OutputTokens > 0 {
				streamRec.outputTokens = int(evt.Usage.OutputTokens)
				streamRec.hasUsage = true
			}
		case map[string]interface{}:
			// For raw map events (protocol conversion scenarios)
			if eventType, ok := evt["type"].(string); ok {
				streamRec.RecordRawMapEvent(eventType, evt)
			}
		}
		return nil
	}

	// OnStreamComplete hook - finalizes recording with model and provider using internally tracked usage
	onStreamComplete = func() {
		if streamRec == nil {
			return
		}
		streamRec.Finish(model, streamRec.inputTokens, streamRec.outputTokens)
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

// recordPromptRounds records prompt rounds to database when prompt_recording is enabled
// This is called automatically by RecordResponse when memoryStore is available
func (sr *ScenarioRecorder) recordPromptRounds(provider *typ.Provider, model string, headers map[string]string, duration time.Duration) {
	if sr == nil || sr.memoryStore == nil || sr.extractor == nil {
		return
	}

	// Extract rounds - try pre-parsed request first, then fallback to bodyBytes
	var rounds []db.RoundData
	var err error

	// Use switch to handle different pre-parsed request types
	switch req := sr.parsedRequest.(type) {
	case *protocol.AnthropicBetaMessagesRequest:
		// Use pre-parsed beta messages directly
		rounds = sr.extractRoundsFromBetaMessages(req.Messages)
	case *protocol.AnthropicMessagesRequest:
		// Use pre-parsed v1 messages directly
		rounds = sr.extractRoundsFromV1Messages(req.Messages)
	default:
		// Fallback to extractor parsing bodyBytes
		rounds, err = sr.extractor.ExtractRounds(sr.bodyBytes)
		if err != nil {
			logrus.Debugf("Failed to extract rounds for prompt recording: %v", err)
			return
		}
	}

	if len(rounds) == 0 {
		return
	}

	// Convert map[string]string to http.Header for metadata extraction
	httpHeaders := make(http.Header)
	for k, v := range headers {
		httpHeaders.Set(k, v)
	}

	// Extract metadata from response headers
	metadata, err := sr.extractor.ExtractMetadata(httpHeaders)
	if err != nil {
		logrus.Debugf("Failed to extract metadata: %v", err)
		// Continue without metadata
	}

	// Extract common correlation IDs (project_id, session_id, request_id)
	projectID := ""
	sessionID := ""
	requestID := ""

	if extractorWithMeta, ok := sr.extractor.(interface {
		ExtractProjectID(http.Header) string
		ExtractSessionID(http.Header) string
		ExtractRequestID(http.Header) string
	}); ok {
		projectID = extractorWithMeta.ExtractProjectID(httpHeaders)
		sessionID = extractorWithMeta.ExtractSessionID(httpHeaders)
		requestID = extractorWithMeta.ExtractRequestID(httpHeaders)
	}

	// Special handling for claude_code scenario: parse request metadata user_id to extract session_id
	// The user_id comes from the request body metadata field, not HTTP headers
	// Format: user_{hash}_account__session_{uuid}
	// Example: user_5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907_account__session_81ca9881-6299-46c2-ae66-7bb28357034f
	if sr.scenario == "claude_code" {
		// Extract user_id from parsed request metadata (not from HTTP headers)
		var anthropicUserID string
		switch req := sr.parsedRequest.(type) {
		case *protocol.AnthropicBetaMessagesRequest:
			if req.BetaMessageNewParams.Metadata.UserID.Valid() {
				anthropicUserID = req.BetaMessageNewParams.Metadata.UserID.Value
			}
		case *protocol.AnthropicMessagesRequest:
			if req.MessageNewParams.Metadata.UserID.Valid() {
				anthropicUserID = req.MessageNewParams.Metadata.UserID.Value
			}
		}

		if anthropicUserID != "" {
			// Try to parse claude_code format
			if parsed := TryParseClaudeCodeUserID(anthropicUserID); parsed != nil {
				// Use extracted session_id from claude_code user_id
				sessionID = parsed.SessionID
				// Store the full user_id hash in metadata
				if metadata == nil {
					metadata = make(map[string]interface{})
				}
				metadata["claude_code_user_id"] = parsed.UserID
				metadata["claude_code_user_id_full"] = anthropicUserID
			} else {
				// Pattern doesn't match, generate UUID for session_id
				if sessionID == "" {
					sessionID = GenerateSessionID()
				}
				// Store original user_id in metadata
				if metadata == nil {
					metadata = make(map[string]interface{})
				}
				metadata["anthropic_user_id"] = anthropicUserID
			}
		} else {
			// No user_id in request metadata, generate session_id
			if sessionID == "" {
				sessionID = GenerateSessionID()
			}
		}
	}

	// Ensure session_id is never empty (generate if needed)
	if sessionID == "" {
		sessionID = GenerateSessionID()
	}

	// Extract working directory from system prompt
	workingDir := sr.extractWorkingDirFromRequest()

	// Convert metadata to JSON string
	metadataJSON := ""
	if len(metadata) > 0 {
		data, err := json.Marshal(metadata)
		if err != nil {
			logrus.Debugf("Failed to marshal metadata: %v", err)
		} else {
			metadataJSON = string(data)
		}
	}

	// Determine input/output tokens (use tracked values or try to extract from response)
	inputTokens := sr.inputTokens
	outputTokens := sr.outputTokens

	// If tokens not tracked, try to extract from assembled response
	if inputTokens == 0 && outputTokens == 0 && sr.assembledResponse != nil {
		if usage, ok := sr.assembledResponse["usage"].(map[string]interface{}); ok {
			if in, ok := usage["input_tokens"].(float64); ok {
				inputTokens = int(in)
			}
			if out, ok := usage["output_tokens"].(float64); ok {
				outputTokens = int(out)
			}
		}
	}

	// Extract assistant's output from assembled response (only for current round)
	currentRoundResult := sr.extractRoundResultFromResponse()

	// Create prompt round records
	records := make([]*db.MemoryRoundRecord, len(rounds))
	for i, round := range rounds {
		// Convert full messages to JSON
		fullMessagesJSON := ""
		if round.FullMessages != nil {
			data, err := json.Marshal(round.FullMessages)
			if err != nil {
				logrus.Debugf("Failed to marshal full messages: %v", err)
			} else {
				fullMessagesJSON = string(data)
			}
		}

		// Extract RoundResult:
		// - For current round (last one): use response (assembledResponse)
		// - For historical rounds: extract from request messages
		var result string
		if i == len(rounds)-1 {
			// Current round - use response
			result = currentRoundResult
		} else {
			// Historical rounds - extract from request messages
			result = round.RoundResult // Already extracted by extractRoundsFromV1Messages
		}

		records[i] = &db.MemoryRoundRecord{
			Scenario:     sr.scenario,
			ProviderUUID: provider.UUID,
			ProviderName: provider.Name,
			Model:        model,

			Protocol:  sr.protocol,
			RequestID: requestID,

			ProjectID:  projectID,
			SessionID:  sessionID,
			WorkingDir: workingDir,

			Metadata: metadataJSON,

			RoundIndex:    round.RoundIndex,
			UserInput:     round.UserInput,
			UserInputHash: round.UserInputHash,
			RoundResult:   result,
			FullMessages:  fullMessagesJSON,

			InputTokens:  inputTokens,
			OutputTokens: outputTokens,

			IsStreaming:  sr.isStreaming,
			ToolUseCount: round.ToolUseCount,
		}
	}

	// Save all records in a single transaction
	if err := sr.memoryStore.RecordRounds(records); err != nil {
		logrus.Errorf("Failed to record prompt rounds: %v", err)
		return
	}

	logrus.Debugf("Recorded %d prompt rounds for scenario %s", len(records), sr.scenario)
}

// countToolUseInMessages counts the number of tool_use blocks in messages
func (sr *ScenarioRecorder) countToolUseInMessages(messages []map[string]interface{}) int {
	count := 0
	for _, msg := range messages {
		if role, ok := msg["role"].(string); ok && role == "assistant" {
			if content, ok := msg["content"].([]interface{}); ok {
				for _, block := range content {
					if blockMap, ok := block.(map[string]interface{}); ok {
						if blockType, ok := blockMap["type"].(string); ok {
							if blockType == "tool_use" {
								count++
							}
						}
					}
				}
			}
		}
	}
	return count
}

// extractRoundsFromV1Messages extracts rounds from pre-parsed v1 messages
func (sr *ScenarioRecorder) extractRoundsFromV1Messages(messages []anthropic.MessageParam) []db.RoundData {
	grouper := protocol.NewGrouper()
	rounds := grouper.GroupV1(messages)

	result := make([]db.RoundData, 0, len(rounds))
	for i, round := range rounds {
		userInput := sr.extractUserInputFromV1(round.Messages)
		// Extract RoundResult from request messages (for historical rounds)
		// Current round's result will be filled from response
		roundResult := sr.extractRoundResultFromV1Messages(round.Messages)

		result = append(result, db.RoundData{
			RoundIndex:    i,
			UserInput:     userInput,
			UserInputHash: db.ComputeUserInputHash(userInput),
			RoundResult:   roundResult,
			FullMessages:  sr.normalizeV1Messages(round.Messages),
			ToolUseCount:  sr.countToolUseInV1Messages(round.Messages),
		})
	}
	return result
}

// extractRoundsFromBetaMessages extracts rounds from pre-parsed beta messages
func (sr *ScenarioRecorder) extractRoundsFromBetaMessages(messages []anthropic.BetaMessageParam) []db.RoundData {
	grouper := protocol.NewGrouper()
	rounds := grouper.GroupBeta(messages)

	result := make([]db.RoundData, 0, len(rounds))
	for i, round := range rounds {
		userInput := sr.extractUserInputFromBeta(round.Messages)
		// Extract RoundResult from request messages (for historical rounds)
		// Current round's result will be filled from response
		roundResult := sr.extractRoundResultFromBetaMessages(round.Messages)

		result = append(result, db.RoundData{
			RoundIndex:    i,
			UserInput:     userInput,
			UserInputHash: db.ComputeUserInputHash(userInput),
			RoundResult:   roundResult,
			FullMessages:  sr.normalizeBetaMessages(round.Messages),
			ToolUseCount:  sr.countToolUseInBetaMessages(round.Messages),
		})
	}
	return result
}

// Helper methods for v1 messages
func (sr *ScenarioRecorder) extractUserInputFromV1(messages []anthropic.MessageParam) string {
	for _, msg := range messages {
		if string(msg.Role) == "user" {
			var input string
			for _, block := range msg.Content {
				if block.OfText != nil {
					input += block.OfText.Text + "\n"
				}
			}
			return input
		}
	}
	return ""
}

func (sr *ScenarioRecorder) normalizeV1Messages(messages []anthropic.MessageParam) []map[string]interface{} {
	result := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		result[i] = sr.normalizeV1Message(msg)
	}
	return result
}

func (sr *ScenarioRecorder) normalizeV1Message(msg anthropic.MessageParam) map[string]interface{} {
	normalized := map[string]interface{}{
		"role": string(msg.Role),
	}

	content := make([]map[string]interface{}, len(msg.Content))
	for i, block := range msg.Content {
		content[i] = sr.normalizeV1ContentBlock(block)
	}
	normalized["content"] = content

	return normalized
}

func (sr *ScenarioRecorder) normalizeV1ContentBlock(block anthropic.ContentBlockParamUnion) map[string]interface{} {
	result := make(map[string]interface{})

	switch {
	case block.OfText != nil:
		result["type"] = "text"
		result["text"] = block.OfText.Text
	case block.OfImage != nil:
		result["type"] = "image"
		img := block.OfImage
		result["source"] = map[string]interface{}{
			"type":       "image_source",
			"media_type": "image",
			"data":       img.Source,
		}
	case block.OfToolUse != nil:
		result["type"] = "tool_use"
		result["id"] = block.OfToolUse.ID
		result["name"] = block.OfToolUse.Name
		result["input"] = block.OfToolUse.Input
	case block.OfToolResult != nil:
		result["type"] = "tool_result"
		result["tool_use_id"] = block.OfToolResult.ToolUseID
		result["content"] = block.OfToolResult.Content
	}
	return result
}

// Helper methods for beta messages
func (sr *ScenarioRecorder) extractUserInputFromBeta(messages []anthropic.BetaMessageParam) string {
	for _, msg := range messages {
		if string(msg.Role) == "user" {
			var input string
			for _, block := range msg.Content {
				if block.OfText != nil {
					input += block.OfText.Text + "\n"
				}
			}
			return input
		}
	}
	return ""
}

// extractRoundResultFromV1Messages extracts assistant's output from v1 messages in a round
// For rounds with multiple assistant messages (e.g., with tool use), returns only the last one
// which typically contains the final answer after tool execution
func (sr *ScenarioRecorder) extractRoundResultFromV1Messages(messages []anthropic.MessageParam) string {
	var lastAssistantResult string
	for _, msg := range messages {
		if string(msg.Role) == "assistant" {
			var result string
			for _, block := range msg.Content {
				if block.OfText != nil {
					result += block.OfText.Text + "\n"
				}
			}
			if result != "" {
				lastAssistantResult = result
			}
		}
	}
	return lastAssistantResult
}

// extractRoundResultFromBetaMessages extracts assistant's output from beta messages in a round
// For rounds with multiple assistant messages (e.g., with tool use), returns only the last one
// which typically contains the final answer after tool execution
func (sr *ScenarioRecorder) extractRoundResultFromBetaMessages(messages []anthropic.BetaMessageParam) string {
	var lastAssistantResult string
	for _, msg := range messages {
		if string(msg.Role) == "assistant" {
			var result string
			for _, block := range msg.Content {
				if block.OfText != nil {
					result += block.OfText.Text + "\n"
				}
			}
			if result != "" {
				lastAssistantResult = result
			}
		}
	}
	return lastAssistantResult
}

// countToolUseInV1Messages counts the number of tool_use blocks in v1 messages
func (sr *ScenarioRecorder) countToolUseInV1Messages(messages []anthropic.MessageParam) int {
	count := 0
	for _, msg := range messages {
		if string(msg.Role) == "assistant" {
			for _, block := range msg.Content {
				if block.OfToolUse != nil {
					count++
				}
			}
		}
	}
	return count
}

// countToolUseInBetaMessages counts the number of tool_use blocks in beta messages
func (sr *ScenarioRecorder) countToolUseInBetaMessages(messages []anthropic.BetaMessageParam) int {
	count := 0
	for _, msg := range messages {
		if string(msg.Role) == "assistant" {
			for _, block := range msg.Content {
				if block.OfToolUse != nil {
					count++
				}
			}
		}
	}
	return count
}

func (sr *ScenarioRecorder) normalizeBetaMessages(messages []anthropic.BetaMessageParam) []map[string]interface{} {
	result := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		result[i] = sr.normalizeBetaMessage(msg)
	}
	return result
}

func (sr *ScenarioRecorder) normalizeBetaMessage(msg anthropic.BetaMessageParam) map[string]interface{} {
	normalized := map[string]interface{}{
		"role": string(msg.Role),
	}

	content := make([]map[string]interface{}, len(msg.Content))
	for i, block := range msg.Content {
		content[i] = sr.normalizeBetaContentBlock(block)
	}
	normalized["content"] = content

	return normalized
}

func (sr *ScenarioRecorder) normalizeBetaContentBlock(block anthropic.BetaContentBlockParamUnion) map[string]interface{} {
	result := make(map[string]interface{})

	switch {
	case block.OfText != nil:
		result["type"] = "text"
		result["text"] = block.OfText.Text
	case block.OfImage != nil:
		result["type"] = "image"
		img := block.OfImage
		result["source"] = map[string]interface{}{
			"type":       "image_source",
			"media_type": "image",
			"data":       img.Source,
		}
	case block.OfToolUse != nil:
		result["type"] = "tool_use"
		result["id"] = block.OfToolUse.ID
		result["name"] = block.OfToolUse.Name
		result["input"] = block.OfToolUse.Input
	case block.OfToolResult != nil:
		result["type"] = "tool_result"
		result["tool_use_id"] = block.OfToolResult.ToolUseID
		result["content"] = block.OfToolResult.Content
	}
	return result
}

// extractRoundResultFromResponse extracts assistant's output from the assembled response
func (sr *ScenarioRecorder) extractRoundResultFromResponse() string {
	if sr == nil || sr.assembledResponse == nil {
		return ""
	}

	// Try to extract content from the response
	// Anthropic response format: { "content": [...], ... }
	if content, ok := sr.assembledResponse["content"].([]interface{}); ok {
		var result string
		for _, block := range content {
			if blockMap, ok := block.(map[string]interface{}); ok {
				if blockType, ok := blockMap["type"].(string); ok {
					switch blockType {
					case "text":
						if text, ok := blockMap["text"].(string); ok {
							result += text + "\n"
						}
					case "tool_use":
						// For tool use, we might want to record the tool name and result
						// For now, just skip or add a placeholder
					}
				}
			}
		}
		return result
	}

	return ""
}

// extractWorkingDirFromRequest extracts working directory from system prompt
func (sr *ScenarioRecorder) extractWorkingDirFromRequest() string {
	if sr == nil {
		return ""
	}

	// Extract system prompt based on request type
	var systemPrompt string
	switch req := sr.parsedRequest.(type) {
	case *protocol.AnthropicBetaMessagesRequest:
		systemPrompt = sr.extractSystemPromptFromBeta(req.System)
	case *protocol.AnthropicMessagesRequest:
		systemPrompt = sr.extractSystemPromptFromV1(req.System)
	default:
		// Try to extract from bodyBytes
		systemPrompt = sr.extractSystemPromptFromBody()
	}

	if systemPrompt == "" {
		return ""
	}

	return TryParseWorkingDir(systemPrompt)
}

// extractSystemPromptFromBeta extracts system prompt from beta request system blocks
func (sr *ScenarioRecorder) extractSystemPromptFromBeta(blocks []anthropic.BetaTextBlockParam) string {
	var result string
	for _, block := range blocks {
		result += block.Text + "\n"
	}
	return result
}

// extractSystemPromptFromV1 extracts system prompt from v1 request system blocks
func (sr *ScenarioRecorder) extractSystemPromptFromV1(blocks []anthropic.TextBlockParam) string {
	var result string
	for _, block := range blocks {
		result += block.Text + "\n"
	}
	return result
}

// extractSystemPromptFromBody extracts system prompt from raw body bytes (fallback)
func (sr *ScenarioRecorder) extractSystemPromptFromBody() string {
	if len(sr.bodyBytes) == 0 {
		return ""
	}

	var body struct {
		System interface{} `json:"system"`
	}
	if err := json.Unmarshal(sr.bodyBytes, &body); err != nil {
		return ""
	}

	// Handle different system prompt formats
	switch s := body.System.(type) {
	case string:
		return s
	case []interface{}:
		var result string
		for _, block := range s {
			if blockMap, ok := block.(map[string]interface{}); ok {
				if text, ok := blockMap["text"].(string); ok {
					result += text + "\n"
				}
			}
		}
		return result
	}
	return ""
}
