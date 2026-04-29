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
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol/assembler"
	"github.com/tingly-dev/tingly-box/internal/typ"

	"github.com/tingly-dev/tingly-box/internal/obs"
)

// RecordScenarioRequest records the scenario-level request (client → tingly-box).
// It captures the original request before any transformation and returns a
// ProtocolRecorder that callers use to record the eventual response.
func (s *Server) RecordScenarioRequest(c *gin.Context, scenario string) *ProtocolRecorder {
	scenarioType := typ.RuleScenario(scenario)

	sink := s.GetOrCreateScenarioSink(scenarioType)
	if sink == nil {
		return nil
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logrus.Debugf("obs: failed to read request body for recording: %v", err)
		return nil
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	var bodyJSON map[string]interface{}
	if len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, &bodyJSON); err != nil {
			logrus.Debugf("obs: request body is not JSON, storing as raw string")
			bodyJSON = map[string]interface{}{"raw": string(bodyBytes)}
		}
	}

	req := &obs.RecordRequest{
		Method:  c.Request.Method,
		URL:     c.Request.URL.String(),
		Headers: headerToMap(c.Request.Header),
		Body:    bodyJSON,
	}

	sid := typ.GetSessionID(c.Request.Context())
	short, src := obs.SessionShort(sid)

	return &ProtocolRecorder{
		ScenarioRecorder: &ScenarioRecorder{
			sink:         sink,
			scenario:     scenario,
			req:          req,
			startTime:    time.Now(),
			c:            c,
			bodyBytes:    bodyBytes,
			sessionShort: short,
			sessionSrc:   src,
		},
	}
}

// ScenarioRecorder captures scenario-level request/response recording.
type ScenarioRecorder struct {
	sink         *obs.Sink
	scenario     string
	req          *obs.RecordRequest
	startTime    time.Time
	c            *gin.Context
	bodyBytes    []byte
	sessionShort string
	sessionSrc   string

	// Streaming state
	streamChunks      []map[string]interface{}
	isStreaming       bool
	assembledResponse map[string]interface{}
}

// EnableStreaming puts the recorder into streaming mode.
func (sr *ScenarioRecorder) EnableStreaming() {
	if sr != nil {
		sr.isStreaming = true
		sr.streamChunks = make([]map[string]interface{}, 0)
	}
}

// RecordStreamChunk records a single stream chunk.
func (sr *ScenarioRecorder) RecordStreamChunk(eventType string, chunk interface{}) {
	if sr == nil || !sr.isStreaming {
		return
	}

	var chunkJSON []byte
	var err error

	switch v := chunk.(type) {
	case *anthropic.MessageStreamEventUnion:
		chunkJSON = []byte(v.RawJSON())
	case *anthropic.BetaRawMessageStreamEventUnion:
		chunkJSON = []byte(v.RawJSON())
	case interface{ RawJSON() string }:
		chunkJSON = []byte(v.RawJSON())
	default:
		chunkJSON, err = json.Marshal(chunk)
		if err != nil {
			logrus.Debugf("obs: failed to marshal stream chunk: %v", err)
			return
		}
	}

	var chunkData map[string]interface{}
	if err := json.Unmarshal(chunkJSON, &chunkData); err != nil {
		return
	}
	if _, ok := chunkData["type"]; !ok {
		chunkData["type"] = eventType
	}
	sr.streamChunks = append(sr.streamChunks, chunkData)
}

// SetAssembledResponse sets the assembled streaming response.
func (sr *ScenarioRecorder) SetAssembledResponse(response any) {
	if sr == nil {
		return
	}
	var responseMap map[string]interface{}
	switch v := response.(type) {
	case map[string]interface{}:
		responseMap = v
	case []byte:
		if err := json.Unmarshal(v, &responseMap); err != nil {
			logrus.Debugf("obs: failed to unmarshal assembled response: %v", err)
			return
		}
	default:
		data, err := json.Marshal(response)
		if err != nil {
			logrus.Debugf("obs: failed to marshal assembled response: %v", err)
			return
		}
		if err := json.Unmarshal(data, &responseMap); err != nil {
			logrus.Debugf("obs: failed to unmarshal assembled response: %v", err)
			return
		}
	}
	sr.assembledResponse = responseMap
}

// GetStreamChunks returns the collected stream chunks.
func (sr *ScenarioRecorder) GetStreamChunks() []map[string]interface{} {
	if sr == nil {
		return nil
	}
	return sr.streamChunks
}

// RecordResponse emits the scenario-level response record.
func (sr *ScenarioRecorder) RecordResponse(provider *typ.Provider, model string) {
	if sr == nil || sr.sink == nil {
		return
	}

	statusCode := sr.c.Writer.Status()
	headers := headerToMap(sr.c.Writer.Header())

	var bodyJSON map[string]interface{}
	if sr.isStreaming && sr.assembledResponse != nil {
		bodyJSON = sr.assembledResponse
	} else if sr.isStreaming && len(sr.streamChunks) > 0 {
		bodyJSON = map[string]interface{}{
			"id":             fmt.Sprintf("msg_%d", sr.startTime.Unix()),
			"type":           "message",
			"role":           "assistant",
			"content":        []interface{}{},
			"model":          model,
			"_stream_chunks": len(sr.streamChunks),
			"_note":          "assembled response unavailable",
		}
		logrus.Debugf("obs: ScenarioRecorder fallback response, chunks=%d", len(sr.streamChunks))
	} else {
		if responseBody, exists := sr.c.Get("response_body"); exists {
			if b, ok := responseBody.([]byte); ok {
				_ = json.Unmarshal(b, &bodyJSON)
			}
		}
	}

	resp := &obs.RecordResponse{
		StatusCode:  statusCode,
		Headers:     headers,
		Body:        bodyJSON,
		IsStreaming: sr.isStreaming,
	}
	if sr.isStreaming && len(sr.streamChunks) > 0 {
		chunks := make([]string, 0, len(sr.streamChunks))
		for _, chunk := range sr.streamChunks {
			if data, err := json.Marshal(chunk); err == nil {
				chunks = append(chunks, string(data))
			}
		}
		resp.StreamChunks = chunks
	}

	providerName := ""
	if provider != nil {
		providerName = provider.Name
	}

	r := &obs.Record{
		Timestamp:     time.Now().UTC(),
		RequestID:     uuid.New().String(),
		SessionID:     sr.sessionShort,
		SessionSrc:    sr.sessionSrc,
		Provider:      providerName,
		Scenario:      sr.scenario,
		Model:         model,
		Duration:      time.Since(sr.startTime),
		OriginalRequest: sr.req,
		FinalResponse: resp,
	}
	sr.sink.Emit(r)
}

// RecordError emits an error record.
func (sr *ScenarioRecorder) RecordError(err error) {
	if sr == nil || sr.sink == nil {
		return
	}

	model := ""
	if sr.req != nil && sr.req.Body != nil {
		if m, ok := sr.req.Body["model"].(string); ok {
			model = m
		}
	}

	r := &obs.Record{
		Timestamp:     time.Now().UTC(),
		RequestID:     uuid.New().String(),
		SessionID:     sr.sessionShort,
		SessionSrc:    sr.sessionSrc,
		Provider:      "tingly-box",
		Scenario:      sr.scenario,
		Model:         model,
		Duration:      time.Since(sr.startTime),
		Err:           err.Error(),
		OriginalRequest: sr.req,
		FinalResponse: &obs.RecordResponse{
			StatusCode: sr.c.Writer.Status(),
			Headers:    headerToMap(sr.c.Writer.Header()),
		},
	}
	sr.sink.Emit(r)
}

// headerToMap converts http.Header to map[string]string.
func headerToMap(h http.Header) map[string]string {
	result := make(map[string]string)
	for k, v := range h {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result
}

// ===================================================================
// streamRecorder — unified stream event recording + assembly
// ===================================================================

type streamRecorder struct {
	recorder     *ProtocolRecorder
	assembler    *assembler.AnthropicStreamAssembler
	inputTokens  int
	outputTokens int
	hasUsage     bool
}

func newStreamRecorder(recorder *ProtocolRecorder) *streamRecorder {
	if recorder == nil {
		return nil
	}
	recorder.EnableStreaming()
	return &streamRecorder{
		recorder:  recorder,
		assembler: assembler.NewAnthropicStreamAssembler(),
	}
}

func (sr *streamRecorder) RecordV1Event(event *anthropic.MessageStreamEventUnion) {
	if sr == nil {
		return
	}
	sr.recorder.RecordStreamChunk(event.Type, event)
	sr.assembler.RecordV1Event(event)
}

func (sr *streamRecorder) RecordV1BetaEvent(event *anthropic.BetaRawMessageStreamEventUnion) {
	if sr == nil {
		return
	}
	sr.recorder.RecordStreamChunk(event.Type, event)
	sr.assembler.RecordV1BetaEvent(event)
}

func (sr *streamRecorder) Finish(model string, inputTokens, outputTokens int) {
	if sr == nil {
		return
	}
	if inputTokens == 0 && outputTokens == 0 && sr.hasUsage {
		inputTokens = sr.inputTokens
		outputTokens = sr.outputTokens
	}
	assembled := sr.assembler.Finish(model, inputTokens, outputTokens)
	if assembled != nil {
		sr.recorder.SetAssembledResponse(assembled)
	} else if len(sr.recorder.streamChunks) > 0 {
		fallback := map[string]interface{}{
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
		sr.recorder.SetAssembledResponse(fallback)
		logrus.Debugf("obs: streamRecorder using fallback response, chunks=%d", len(sr.recorder.streamChunks))
	}
}

func (sr *streamRecorder) RecordError(err error) {
	if sr == nil {
		return
	}
	sr.recorder.RecordError(err)
}

func (sr *streamRecorder) RecordResponse(provider *typ.Provider, model string) {
	if sr == nil {
		return
	}
	sr.recorder.RecordResponse(provider, model)
}

func (sr *streamRecorder) RecordRawMapEvent(eventType string, event map[string]interface{}) {
	if sr == nil {
		return
	}
	data, err := json.Marshal(event)
	if err == nil {
		var betaEvent anthropic.BetaRawMessageStreamEventUnion
		if err := json.Unmarshal(data, &betaEvent); err == nil {
			betaEvent.Type = eventType
			sr.assembler.RecordV1BetaEvent(&betaEvent)
		}
	}
	sr.recorder.RecordStreamChunk(eventType, event)

	if eventType == "message_delta" {
		if usage, ok := event["usage"].(map[string]interface{}); ok {
			if v, ok := usage["input_tokens"].(float64); ok {
				sr.inputTokens = int(v)
			} else if v, ok := usage["input_tokens"].(int64); ok {
				sr.inputTokens = int(v)
			}
			if v, ok := usage["output_tokens"].(float64); ok {
				sr.outputTokens = int(v)
			} else if v, ok := usage["output_tokens"].(int64); ok {
				sr.outputTokens = int(v)
			}
			sr.hasUsage = true
		}
	}
}

func (sr *streamRecorder) StreamEventRecorder() interface{} {
	if sr == nil {
		return nil
	}
	return sr
}

func (sr *streamRecorder) SetupStreamRecorderInContext(c *gin.Context, key string) {
	if sr == nil {
		return
	}
	c.Set(key, sr)
}

// ===================================================================
// Recorder Hook Builders
// ===================================================================

// NewRecorderHooks creates hook functions from a ProtocolRecorder for use with
// HandleContext. Usage is tracked internally; hooks do not need usage parameters.
func NewRecorderHooks(recorder *ProtocolRecorder) (onStreamEvent func(event interface{}) error, onStreamComplete func(), onStreamError func(err error)) {
	if recorder == nil {
		return nil, nil, nil
	}

	streamRec := newStreamRecorder(recorder)

	onStreamEvent = func(event interface{}) error {
		if streamRec == nil {
			return nil
		}
		switch evt := event.(type) {
		case *anthropic.MessageStreamEventUnion:
			streamRec.RecordV1Event(evt)
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
			if evt.Usage.InputTokens > 0 {
				streamRec.inputTokens = int(evt.Usage.InputTokens)
				streamRec.hasUsage = true
			}
			if evt.Usage.OutputTokens > 0 {
				streamRec.outputTokens = int(evt.Usage.OutputTokens)
				streamRec.hasUsage = true
			}
		case map[string]interface{}:
			if eventType, ok := evt["type"].(string); ok {
				streamRec.RecordRawMapEvent(eventType, evt)
			}
		}
		return nil
	}

	onStreamComplete = func() {
		if streamRec == nil {
			return
		}
		model := ""
		if recorder.c != nil {
			model = recorder.c.Query("model")
		}
		streamRec.Finish(model, streamRec.inputTokens, streamRec.outputTokens)
	}

	onStreamError = func(err error) {
		if streamRec == nil {
			return
		}
		streamRec.RecordError(err)
	}

	return onStreamEvent, onStreamComplete, onStreamError
}

// NewRecorderHooksWithModel creates hooks with an explicit model and provider.
func NewRecorderHooksWithModel(recorder *ProtocolRecorder, model string, provider *typ.Provider) (onStreamEvent func(event interface{}) error, onStreamComplete func(), onStreamError func(err error)) {
	if recorder == nil {
		return nil, nil, nil
	}

	streamRec := newStreamRecorder(recorder)

	onStreamEvent = func(event interface{}) error {
		if streamRec == nil {
			return nil
		}
		switch evt := event.(type) {
		case *anthropic.MessageStreamEventUnion:
			streamRec.RecordV1Event(evt)
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
			if evt.Usage.InputTokens > 0 {
				streamRec.inputTokens = int(evt.Usage.InputTokens)
				streamRec.hasUsage = true
			}
			if evt.Usage.OutputTokens > 0 {
				streamRec.outputTokens = int(evt.Usage.OutputTokens)
				streamRec.hasUsage = true
			}
		case map[string]interface{}:
			if eventType, ok := evt["type"].(string); ok {
				streamRec.RecordRawMapEvent(eventType, evt)
			}
		}
		return nil
	}

	onStreamComplete = func() {
		if streamRec == nil {
			return
		}
		streamRec.Finish(model, streamRec.inputTokens, streamRec.outputTokens)
		streamRec.RecordResponse(provider, model)
	}

	onStreamError = func(err error) {
		if streamRec == nil {
			return
		}
		streamRec.RecordError(err)
	}

	return onStreamEvent, onStreamComplete, onStreamError
}

// NewNonStreamRecorderHook creates a completion hook for non-streaming responses.
func NewNonStreamRecorderHook(recorder *ScenarioRecorder, provider *typ.Provider, model string) func() {
	if recorder == nil {
		return nil
	}
	return func() {
		recorder.RecordResponse(provider, model)
	}
}
