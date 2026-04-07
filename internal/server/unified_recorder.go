package server

import (
	"bytes"
	"encoding/json"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// UnifiedRecorder provides unified recording for all scenarios and API types
// It captures:
// - Original request (client -> tingly-box)
// - Transformed request (tingly-box -> provider)
// - Provider response (raw chunks + assembled body)
// - Final response (tingly-box -> client)
type UnifiedRecorder struct {
	sink          *obs.Sink
	scenario      string
	targetAPIType protocol.APIType
	provider      *typ.Provider
	model         string
	requestID     string
	startTime     time.Time
	ctx           *gin.Context

	// Recording state
	originalReq      *obs.RecordRequest
	transformedReq   *obs.RecordRequest
	streamChunks     []obs.StreamChunk
	assembledResp    map[string]any
	nonStreamingBody map[string]any
	statusCode       int
	headers          map[string]string
	isStreaming      bool

	// Assembler for streaming responses
	assembler      StreamAssembler
	transformSteps []string

	// Error tracking
	err error
}

// NewUnifiedRecorder creates a new unified recorder
func NewUnifiedRecorder(
	ctx *gin.Context,
	scenario string,
	targetAPIType protocol.APIType,
	provider *typ.Provider,
	model string,
) *UnifiedRecorder {
	return &UnifiedRecorder{
		sink:           nil, // Will be set from server
		scenario:       scenario,
		targetAPIType:  targetAPIType,
		provider:       provider,
		model:          model,
		requestID:      uuid.New().String(),
		startTime:      time.Now(),
		ctx:            ctx,
		streamChunks:   make([]obs.StreamChunk, 0),
		isStreaming:    false,
		transformSteps: make([]string, 0),
	}
}

// SetSink sets the recording sink
func (r *UnifiedRecorder) SetSink(sink *obs.Sink) {
	r.sink = sink
}

// SetProvider sets the provider (for late binding)
func (r *UnifiedRecorder) SetProvider(provider *typ.Provider) {
	r.provider = provider
}

// SetModel sets the model (for late binding)
func (r *UnifiedRecorder) SetModel(model string) {
	r.model = model
}

// SetScenario sets the scenario (for late binding)
func (r *UnifiedRecorder) SetScenario(scenario string) {
	r.scenario = scenario
}

// SetOriginalRequest records the original client request
func (r *UnifiedRecorder) SetOriginalRequest(req *obs.RecordRequest) {
	r.originalReq = req
}

// SetTransformedRequest records the transformed request sent to provider
func (r *UnifiedRecorder) SetTransformedRequest(req *obs.RecordRequest) {
	r.transformedReq = req
}

// EnableStreaming enables streaming mode and initializes the assembler
func (r *UnifiedRecorder) EnableStreaming() {
	r.isStreaming = true
	r.assembler = NewAssembler(r.targetAPIType)
	if r.assembler == nil {
		logrus.Warnf("No assembler available for target API type: %s", r.targetAPIType)
	}
}

// RecordStreamChunk records a streaming chunk
func (r *UnifiedRecorder) RecordStreamChunk(chunkType string, data []byte, parsed map[string]any) {
	if !r.isStreaming {
		return
	}

	chunk := obs.StreamChunk{
		Type:      chunkType,
		Data:      string(data),
		Parsed:    parsed,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	}
	r.streamChunks = append(r.streamChunks, chunk)

	// Also feed to assembler if available
	if r.assembler != nil {
		r.assembler.AddChunk(chunkType, data, parsed)
	}
}

// SetAssembledResponse sets the assembled complete response for streaming
func (r *UnifiedRecorder) SetAssembledResponse(body map[string]any) {
	if !r.isStreaming {
		return
	}
	r.assembledResp = body
}

// SetNonStreamingResponse sets a non-streaming response
func (r *UnifiedRecorder) SetNonStreamingResponse(statusCode int, headers map[string]string, body map[string]any) {
	r.isStreaming = false
	r.statusCode = statusCode
	r.headers = headers
	r.nonStreamingBody = body
}

// SetStatusCode sets the HTTP status code
func (r *UnifiedRecorder) SetStatusCode(code int) {
	r.statusCode = code
}

// SetHeaders sets the response headers
func (r *UnifiedRecorder) SetHeaders(headers map[string]string) {
	r.headers = headers
}

// AddTransformStep records a transform step
func (r *UnifiedRecorder) AddTransformStep(steps ...string) {
	for _, step := range steps {
		r.transformSteps = append(r.transformSteps, step)
	}
}

// SetError records an error
func (r *UnifiedRecorder) SetError(err error) {
	r.err = err
}

// Finalize writes the recording entry
func (r *UnifiedRecorder) Finalize() {
	if r.sink == nil || !r.sink.IsEnabled() {
		return
	}

	duration := time.Since(r.startTime)

	entry := &obs.RecordEntryV3{
		Timestamp:     r.startTime.UTC().Format(time.RFC3339),
		RequestID:     r.requestID,
		DurationMs:    duration.Milliseconds(),
		Scenario:      r.scenario,
		TargetAPIType: string(r.targetAPIType),
		Provider:      r.provider.Name,
		Model:         r.model,
	}

	// Request stages
	entry.OriginalRequest = r.originalReq
	entry.TransformedRequest = r.transformedReq

	// Response stages
	if r.isStreaming {
		// Provider response (streaming)
		entry.ProviderResponse = &obs.RecordResponseV3{
			StatusCode:    r.statusCode,
			Headers:       r.headers,
			IsStreaming:   true,
			StreamChunks:  r.streamChunks,
			AssembledBody: r.assembledResp,
		}

		// For now, final response is same as provider response
		// In protocol conversion scenarios, this would be the converted response
		entry.FinalResponse = entry.ProviderResponse
	} else {
		// Non-streaming response
		entry.ProviderResponse = &obs.RecordResponseV3{
			StatusCode:  r.statusCode,
			Headers:     r.headers,
			IsStreaming: false,
			Body:        r.nonStreamingBody,
		}
		entry.FinalResponse = entry.ProviderResponse
	}

	// Transform steps
	if len(r.transformSteps) > 0 {
		entry.TransformSteps = r.transformSteps
	}

	// Error
	if r.err != nil {
		entry.Error = r.err.Error()
	}

	r.sink.RecordEntryV3(entry)
}

// RecordError records an error condition and finalizes
func (r *UnifiedRecorder) RecordError(err error) {
	r.err = err
	r.Finalize()
}

// RecordRequestFromContext extracts and records the original request from gin context
func (r *UnifiedRecorder) RecordRequestFromContext() error {
	if r.ctx == nil {
		return nil
	}

	// Read request body
	bodyBytes, err := io.ReadAll(r.ctx.Request.Body)
	if err != nil {
		return err
	}
	// Restore body for subsequent handlers
	r.ctx.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	// Parse body as JSON
	var bodyJSON map[string]interface{}
	if len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, &bodyJSON); err != nil {
			bodyJSON = map[string]interface{}{"raw": string(bodyBytes)}
		}
	}

	req := &obs.RecordRequest{
		Method:  r.ctx.Request.Method,
		URL:     r.ctx.Request.URL.String(),
		Headers: localHeaderToMap(r.ctx.Request.Header),
		Body:    bodyJSON,
	}

	r.SetOriginalRequest(req)
	return nil
}

// GetResponseHeaders returns headers from the gin context writer
func (r *UnifiedRecorder) GetResponseHeaders() map[string]string {
	if r.ctx == nil {
		return nil
	}
	return localHeaderToMap(r.ctx.Writer.Header())
}

// GetStatusCode returns the status code from the gin context writer
func (r *UnifiedRecorder) GetStatusCode() int {
	if r.ctx == nil {
		return 0
	}
	return r.ctx.Writer.Status()
}

// ===================================================================
// StreamAssembler interface and implementations
// ===================================================================

// StreamAssembler interface for different API types
type StreamAssembler interface {
	// AddChunk adds a chunk and returns true if assembly is complete
	AddChunk(chunkType string, data []byte, parsed map[string]any) bool

	// GetAssembled returns the assembled response
	GetAssembled() map[string]any

	// GetUsage returns token usage if available
	GetUsage() (inputTokens, outputTokens int, hasUsage bool)
}

// NewAssembler creates an assembler for the given API type
// Reuses existing implementations where possible
func NewAssembler(targetAPIType protocol.APIType) StreamAssembler {
	switch targetAPIType {
	case protocol.TypeAnthropicV1, protocol.TypeAnthropicBeta:
		return &anthropicAssemblerWrapper{impl: stream.NewAnthropicStreamAssembler()}
	case protocol.TypeOpenAIChat:
		return &openAIChatAssemblerWrapper{
			chunks:     make([]map[string]any, 0),
			contentMap: make(map[int]string),
			roleMap:    make(map[int]string),
		}
	case protocol.TypeOpenAIResponses:
		return &openAIResponsesAssemblerWrapper{
			chunks:    make([]map[string]any, 0),
			output:    make([]map[string]any, 0),
			deltaText: make(map[int]string),
		}
	case protocol.TypeGoogle:
		// TODO: Implement Google assembler
		return nil
	default:
		return nil
	}
}

// anthropicAssemblerWrapper wraps the existing AnthropicStreamAssembler
type anthropicAssemblerWrapper struct {
	impl         *stream.AnthropicStreamAssembler
	inputTokens  int
	outputTokens int
	hasUsage     bool
}

func (w *anthropicAssemblerWrapper) AddChunk(chunkType string, data []byte, parsed map[string]any) bool {
	// For Anthropic, chunks are already handled by the stream handler
	// This wrapper is for compatibility with the interface
	return false
}

func (w *anthropicAssemblerWrapper) GetAssembled() map[string]any {
	if w.impl == nil {
		return nil
	}
	// The Anthropic assembler returns *anthropic.Message, convert to map
	msg := w.impl.Finish("", w.inputTokens, w.outputTokens)
	if msg == nil {
		return nil
	}

	// Convert to map using JSON marshal/unmarshal
	data, err := json.Marshal(msg)
	if err != nil {
		return nil
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}

func (w *anthropicAssemblerWrapper) GetUsage() (int, int, bool) {
	return w.inputTokens, w.outputTokens, w.hasUsage
}

func (w *anthropicAssemblerWrapper) SetUsage(inputTokens, outputTokens int) {
	w.inputTokens = inputTokens
	w.outputTokens = outputTokens
	w.hasUsage = true
}

func (w *anthropicAssemblerWrapper) Finish(model string, inputTokens, outputTokens int) map[string]any {
	if w.impl == nil {
		return nil
	}
	msg := w.impl.Finish(model, inputTokens, outputTokens)
	if msg == nil {
		return nil
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return nil
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}

// Expose the underlying assembler for Anthropic events
func (w *anthropicAssemblerWrapper) GetAnthropicAssembler() *stream.AnthropicStreamAssembler {
	return w.impl
}

// openAIChatAssemblerWrapper assembles OpenAI chat completion streaming responses
type openAIChatAssemblerWrapper struct {
	chunks       []map[string]any
	contentMap   map[int]string
	roleMap      map[int]string
	inputTokens  int
	outputTokens int
	hasUsage     bool
	firstChunk   map[string]any
}

func (w *openAIChatAssemblerWrapper) AddChunk(chunkType string, data []byte, parsed map[string]any) bool {
	if parsed == nil {
		var err error
		data, err = json.Marshal(data)
		if err != nil {
			return false
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return false
		}
	}

	w.chunks = append(w.chunks, parsed)

	// Store first chunk for metadata
	if w.firstChunk == nil {
		w.firstChunk = parsed
	}

	// Extract usage if available
	if usage, ok := parsed["usage"].(map[string]any); ok {
		if promptTokens, ok := usage["prompt_tokens"].(float64); ok {
			w.inputTokens = int(promptTokens)
			w.hasUsage = true
		}
		if promptTokens, ok := usage["prompt_tokens"].(int64); ok {
			w.inputTokens = int(promptTokens)
			w.hasUsage = true
		}
		if completionTokens, ok := usage["completion_tokens"].(float64); ok {
			w.outputTokens = int(completionTokens)
			w.hasUsage = true
		}
		if completionTokens, ok := usage["completion_tokens"].(int64); ok {
			w.outputTokens = int(completionTokens)
			w.hasUsage = true
		}
	}

	// Assemble content from choices
	if choices, ok := parsed["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			index := 0
			if idx, ok := choice["index"].(float64); ok {
				index = int(idx)
			}

			// Extract role from delta if available
			if delta, ok := choice["delta"].(map[string]any); ok {
				if role, ok := delta["role"].(string); ok {
					w.roleMap[index] = role
				}
				if content, ok := delta["content"].(string); ok {
					w.contentMap[index] += content
				}
			}
		}
	}

	return false // Continue assembly
}

func (w *openAIChatAssemblerWrapper) GetAssembled() map[string]any {
	if len(w.chunks) == 0 {
		return nil
	}

	// Build assembled response
	assembled := map[string]any{
		"id":      w.firstChunk["id"],
		"object":  w.firstChunk["object"],
		"created": w.firstChunk["created"],
		"model":   w.firstChunk["model"],
	}

	// Build choices with assembled content
	choices := []map[string]any{}
	for index, content := range w.contentMap {
		choice := map[string]any{
			"index": index,
			"message": map[string]any{
				"role":    w.roleMap[index],
				"content": content,
			},
		}
		choices = append(choices, choice)
	}
	assembled["choices"] = choices

	// Get finish_reason from last chunk
	if lastChunk := w.chunks[len(w.chunks)-1]; lastChunk != nil {
		if choices, ok := lastChunk["choices"].([]any); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]any); ok {
				if finishReason, ok := choice["finish_reason"]; ok {
					if len(assembled["choices"].([]any)) > 0 {
						assembled["choices"].([]any)[0].(map[string]any)["finish_reason"] = finishReason
					}
				}
			}
		}
	}

	// Add usage if available
	if w.hasUsage {
		assembled["usage"] = map[string]any{
			"prompt_tokens":     w.inputTokens,
			"completion_tokens": w.outputTokens,
			"total_tokens":      w.inputTokens + w.outputTokens,
		}
	}

	return assembled
}

func (w *openAIChatAssemblerWrapper) GetUsage() (int, int, bool) {
	return w.inputTokens, w.outputTokens, w.hasUsage
}

func (w *openAIChatAssemblerWrapper) Finish(model string, inputTokens, outputTokens int) map[string]any {
	if inputTokens > 0 {
		w.inputTokens = inputTokens
		w.hasUsage = true
	}
	if outputTokens > 0 {
		w.outputTokens = outputTokens
		w.hasUsage = true
	}
	return w.GetAssembled()
}

// openAIResponsesAssemblerWrapper assembles OpenAI Responses API streaming responses
type openAIResponsesAssemblerWrapper struct {
	chunks       []map[string]any
	responseID   string
	status       string
	output       []map[string]any
	inputTokens  int
	outputTokens int
	cacheTokens  int
	hasUsage     bool
	deltaText    map[int]string // Accumulate text deltas per output index
}

func (w *openAIResponsesAssemblerWrapper) AddChunk(chunkType string, data []byte, parsed map[string]any) bool {
	if parsed == nil {
		var err error
		data, err = json.Marshal(data)
		if err != nil {
			return false
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			return false
		}
	}

	w.chunks = append(w.chunks, parsed)

	// Extract event type from parsed data
	// OpenAI Responses events have structure: {"type": "response.created", "response": {...}}
	if eventTypeVal, ok := parsed["type"].(string); ok {
		switch eventTypeVal {
		case "response.created":
			// Initial response object
			if response, ok := parsed["response"].(map[string]any); ok {
				if id, ok := response["id"].(string); ok {
					w.responseID = id
				}
				if status, ok := response["status"].(string); ok {
					w.status = status
				}
			}
		case "response.output_text.added":
			// Text output added
			if response, ok := parsed["response"].(map[string]any); ok {
				if output, ok := response["output"].([]map[string]any); ok {
					for _, item := range output {
						if itemType, ok := item["type"].(string); ok && itemType == "output_text" {
							if outputIndex, ok := item["output_index"].(float64); ok {
								if text, ok := item["text"].(string); ok {
									idx := int(outputIndex)
									w.deltaText[idx] = text
									// Build output item
									outputItem := map[string]any{
										"type":         "output_text",
										"text":         text,
										"output_index": outputIndex,
									}
									w.output = append(w.output, outputItem)
								}
							}
						}
					}
				}
			}
		case "response.output_text.delta":
			// Text delta - accumulate
			if response, ok := parsed["response"].(map[string]any); ok {
				if delta, ok := response["delta"].(map[string]any); ok {
					if outputIndex, ok := delta["output_index"].(float64); ok {
						if text, ok := delta["text"].(string); ok {
							idx := int(outputIndex)
							w.deltaText[idx] += text
						}
					}
				}
			}
		case "response.output_text.done":
			// Text output completed - update with accumulated text
			if response, ok := parsed["response"].(map[string]any); ok {
				if output, ok := response["output"].([]map[string]any); ok {
					for _, item := range output {
						if itemType, ok := item["type"].(string); ok && itemType == "output_text" {
							if outputIndex, ok := item["output_index"].(float64); ok {
								idx := int(outputIndex)
								if accumulatedText, exists := w.deltaText[idx]; exists {
									item["text"] = accumulatedText
								}
								w.output = append(w.output, item)
							}
						}
					}
				}
			}
		case "response.in_progress":
			w.status = "in_progress"
		case "response.completed":
			w.status = "completed"
			// Extract usage from completed event
			if response, ok := parsed["response"].(map[string]any); ok {
				if usage, ok := response["usage"].(map[string]any); ok {
					if inputTokens, ok := usage["input_tokens"].(float64); ok {
						w.inputTokens = int(inputTokens)
						w.hasUsage = true
					}
					if inputTokens, ok := usage["input_tokens"].(int64); ok {
						w.inputTokens = int(inputTokens)
						w.hasUsage = true
					}
					if outputTokens, ok := usage["output_tokens"].(float64); ok {
						w.outputTokens = int(outputTokens)
						w.hasUsage = true
					}
					if outputTokens, ok := usage["output_tokens"].(int64); ok {
						w.outputTokens = int(outputTokens)
						w.hasUsage = true
					}
					// Cache tokens from details
					if details, ok := usage["input_tokens_details"].(map[string]any); ok {
						if cachedTokens, ok := details["cached_tokens"].(float64); ok {
							w.cacheTokens = int(cachedTokens)
						}
						if cachedTokens, ok := details["cached_tokens"].(int64); ok {
							w.cacheTokens = int(cachedTokens)
						}
					}
				}
			}
		case "response.failed":
			w.status = "failed"
		case "error":
			w.status = "failed"
		}
	}

	return false // Continue assembly
}

func (w *openAIResponsesAssemblerWrapper) GetAssembled() map[string]any {
	if len(w.chunks) == 0 {
		return nil
	}

	// Build assembled response from first chunk's metadata
	firstChunk := w.chunks[0]
	assembled := make(map[string]any)

	// Copy top-level fields from first chunk
	if id, ok := firstChunk["response"].(map[string]any)["id"].(string); ok {
		assembled["id"] = id
	}
	if response, ok := firstChunk["response"].(map[string]any); ok {
		if model, ok := response["model"].(string); ok {
			assembled["model"] = model
		}
		if status, ok := response["status"].(string); ok {
			assembled["status"] = status
		}
	}

	assembled["object"] = "response"
	assembled["status"] = w.status

	// Build output array from accumulated outputs
	if len(w.output) > 0 {
		assembled["output"] = w.output
	}

	// Add usage if available
	if w.hasUsage {
		usage := map[string]any{
			"input_tokens":  w.inputTokens,
			"output_tokens": w.outputTokens,
			"total_tokens":  w.inputTokens + w.outputTokens,
		}
		if w.cacheTokens > 0 {
			usage["input_tokens_details"] = map[string]any{
				"cached_tokens": w.cacheTokens,
			}
		}
		assembled["usage"] = usage
	}

	return assembled
}

func (w *openAIResponsesAssemblerWrapper) GetUsage() (int, int, bool) {
	return w.inputTokens, w.outputTokens, w.hasUsage
}
