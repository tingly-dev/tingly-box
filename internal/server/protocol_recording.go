package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/server/middleware"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// recorderContextKey is the gin context key under which the active
// ProtocolRecorder is stored so later handler stages can reuse it.
const recorderContextKey = "protocol_recorder"

// ProtocolRecorder captures a single client→tingly-box→provider cycle.
//
// It carries both the scenario-level (client/final) and protocol-level
// (transformed) request/response pairs, plus optional streaming state. The
// recorder is mode-driven: which fields are emitted to the sink is decided
// by RecordMode (set at construction).
//
// Lifecycle:
//   1. EnsureProtocolRecorder at handler entry — captures client request,
//      session, mode.
//   2. Optional: transform pipeline writes SetOriginalRequest /
//      SetTransformedRequest via TransformRecorder.
//   3. For streaming, hooks call EnableStreaming + RecordStreamChunk +
//      SetAssembledResponse.
//   4. RecordResponse (success) or RecordError (failure) emits one *obs.Record.
type ProtocolRecorder struct {
	sink         *obs.Sink
	scenario     string
	startTime    time.Time
	c            *gin.Context
	sessionShort string
	sessionSrc   string

	streamChunks []map[string]interface{}
	isStreaming  bool

	originalRequest    *obs.RecordRequest
	transformedRequest *obs.RecordRequest
	finalResponse      *obs.RecordResponse

	transformSteps []string

	providerName string
	providerUUID string
	model        string
	mode         obs.RecordMode
}

// breakerServiceID returns the loadbalance service identifier for the
// active provider+model, or "" if either is unknown. The ID format must
// match Service.ServiceID() so circuit-breaker lookups line up with the
// keys used by the priority tactic.
func (sr *ProtocolRecorder) breakerServiceID() string {
	if sr == nil || sr.providerUUID == "" || sr.model == "" {
		return ""
	}
	return sr.providerUUID + ":" + sr.model
}

// EnsureProtocolRecorder returns a ProtocolRecorder for the given scenario,
// reusing any recorder already stored in the gin context. Returns nil when
// recording is disabled (no sink) or the request body cannot be read.
func (s *Server) EnsureProtocolRecorder(c *gin.Context, scenario string, provider *typ.Provider, model string, mode obs.RecordMode) *ProtocolRecorder {
	if rec, ok := getRecorderFromContext(c); ok {
		rec.bindProvider(provider, model, mode)
		return rec
	}

	scenarioType := typ.RuleScenario(scenario)
	sink := s.GetOrCreateScenarioSink(scenarioType)
	if sink == nil {
		return nil
	}

	rec, err := newProtocolRecorder(c, sink, scenario, mode)
	if err != nil {
		logrus.Debugf("obs: failed to build ProtocolRecorder: %v", err)
		return nil
	}
	rec.bindProvider(provider, model, mode)
	c.Set(recorderContextKey, rec)
	return rec
}

// BeginProtocolRecording is the entry-time constructor used before provider
// routing has resolved. Provider/model are filled in later via
// EnsureProtocolRecorder.
func (s *Server) BeginProtocolRecording(c *gin.Context, scenario string) *ProtocolRecorder {
	scenarioType := typ.RuleScenario(scenario)
	sink := s.GetOrCreateScenarioSink(scenarioType)
	if sink == nil {
		return nil
	}

	mode := s.GetScenarioRecordMode(scenarioType)
	rec, err := newProtocolRecorder(c, sink, scenario, mode)
	if err != nil {
		logrus.Debugf("obs: failed to build ProtocolRecorder: %v", err)
		return nil
	}
	return rec
}

func newProtocolRecorder(c *gin.Context, sink *obs.Sink, scenario string, mode obs.RecordMode) (*ProtocolRecorder, error) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, err
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	var bodyJSON map[string]interface{}
	if len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, &bodyJSON); err != nil {
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
		sink:            sink,
		scenario:        scenario,
		startTime:       time.Now(),
		c:               c,
		sessionShort:    short,
		sessionSrc:      src,
		originalRequest: req,
		mode:            mode,
	}, nil
}

func getRecorderFromContext(c *gin.Context) (*ProtocolRecorder, bool) {
	v, exists := c.Get(recorderContextKey)
	if !exists {
		return nil, false
	}
	rec, ok := v.(*ProtocolRecorder)
	return rec, ok
}

func (sr *ProtocolRecorder) bindProvider(provider *typ.Provider, model string, mode obs.RecordMode) {
	if sr == nil {
		return
	}
	if provider != nil {
		sr.providerName = provider.Name
		sr.providerUUID = provider.UUID
	}
	if model != "" {
		sr.model = model
	}
	if mode != "" {
		sr.mode = mode
	}
}

// EnableStreaming puts the recorder into streaming mode.
func (sr *ProtocolRecorder) EnableStreaming() {
	if sr == nil {
		return
	}
	sr.isStreaming = true
	if sr.streamChunks == nil {
		sr.streamChunks = make([]map[string]interface{}, 0)
	}
}

// RecordStreamChunk records a single stream chunk.
func (sr *ProtocolRecorder) RecordStreamChunk(eventType string, chunk interface{}) {
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

// SetAssembledResponse stores the final assembled (post-stream) response.
// Accepts map, []byte, or any JSON-marshallable value.
func (sr *ProtocolRecorder) SetAssembledResponse(response any) {
	if sr == nil {
		return
	}
	responseMap, ok := coerceToMap(response)
	if !ok {
		return
	}

	statusCode := 200
	headers := map[string]string{}
	if sr.c != nil {
		statusCode = sr.c.Writer.Status()
		headers = headerToMap(sr.c.Writer.Header())
	}
	sr.finalResponse = &obs.RecordResponse{
		StatusCode:  statusCode,
		Headers:     headers,
		Body:        responseMap,
		IsStreaming: sr.isStreaming,
	}
}

// SetOriginalRequest stores the pre-transform request.
func (sr *ProtocolRecorder) SetOriginalRequest(req *obs.RecordRequest) {
	if sr == nil {
		return
	}
	sr.originalRequest = req
}

// SetTransformedRequest stores the post-transform request.
func (sr *ProtocolRecorder) SetTransformedRequest(req *obs.RecordRequest) {
	if sr == nil {
		return
	}
	sr.transformedRequest = req
}

// SetTransformSteps records which transforms were applied.
func (sr *ProtocolRecorder) SetTransformSteps(steps []string) {
	if sr == nil {
		return
	}
	sr.transformSteps = steps
}

// RecordResponse finalises provider/model and emits a Record to the sink.
func (sr *ProtocolRecorder) RecordResponse(provider *typ.Provider, model string) {
	if sr == nil {
		return
	}
	sr.bindProvider(provider, model, "")
	if sr.finalResponse == nil {
		sr.finalResponse = sr.synthesizeFinalResponse()
	}
	if id := sr.breakerServiceID(); id != "" {
		loadbalance.RecordServiceSuccess(id)
	}
	sr.emit(nil)
}

// RecordError emits an error record. err may be nil.
func (sr *ProtocolRecorder) RecordError(err error) {
	if sr == nil {
		return
	}
	if err != nil {
		if id := sr.breakerServiceID(); id != "" {
			loadbalance.RecordServiceFailure(id)
		}
	}
	sr.emit(err)
}

func (sr *ProtocolRecorder) emit(err error) {
	if sr.sink == nil || sr.mode == "" {
		return
	}

	r := &obs.Record{
		Timestamp:  time.Now().UTC(),
		RequestID:  sr.resolveRequestID(),
		SessionID:  sr.sessionShort,
		SessionSrc: sr.sessionSrc,
		Provider:   sr.providerName,
		Scenario:   sr.scenario,
		Model:      sr.resolveModel(),
		Steps:      sr.transformSteps,
		Duration:   time.Since(sr.startTime),
	}
	if err != nil {
		r.Err = err.Error()
	}

	switch sr.mode {
	case obs.RecordModeAll, obs.RecordModeScenario, obs.RecordModeStagedRequestResponse:
		r.OriginalRequest = sr.originalRequest
		r.TransformedRequest = sr.transformedRequest
		r.FinalResponse = sr.finalResponse
	case obs.RecordModeRequestOnly:
		r.TransformedRequest = sr.transformedRequest
	case obs.RecordModeRequestResponse:
		r.TransformedRequest = sr.transformedRequest
		r.FinalResponse = sr.finalResponse
	}

	sr.sink.Emit(r)
}

// resolveRequestID returns the request correlation id established by the
// access-log middleware so the recording (system B) shares an id with the
// logrus traces (system A). Falls back to a fresh uuid when the recorder
// runs outside an HTTP request.
func (sr *ProtocolRecorder) resolveRequestID() string {
	if sr.c != nil {
		if id := sr.c.GetString(middleware.GinRequestIDKey); id != "" {
			return id
		}
	}
	return uuid.New().String()
}

func (sr *ProtocolRecorder) resolveModel() string {
	if sr.model != "" {
		return sr.model
	}
	if sr.originalRequest != nil && sr.originalRequest.Body != nil {
		if m, ok := sr.originalRequest.Body["model"].(string); ok {
			return m
		}
	}
	return ""
}

// synthesizeFinalResponse builds a final response from the gin writer or a
// streaming fallback, used when RecordResponse runs without an earlier
// SetAssembledResponse.
func (sr *ProtocolRecorder) synthesizeFinalResponse() *obs.RecordResponse {
	statusCode := 0
	var headers map[string]string
	if sr.c != nil {
		statusCode = sr.c.Writer.Status()
		headers = headerToMap(sr.c.Writer.Header())
	}

	var bodyJSON map[string]interface{}
	if sr.isStreaming && len(sr.streamChunks) > 0 {
		bodyJSON = baseMessageMap(sr.model, sr.startTime)
		bodyJSON["_stream_chunks"] = len(sr.streamChunks)
		bodyJSON["_note"] = "assembled response unavailable"
		logrus.Debugf("obs: ProtocolRecorder fallback response, chunks=%d", len(sr.streamChunks))
	} else if sr.c != nil {
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
	return resp
}

// baseMessageMap builds the common skeleton of a synthesised assistant
// message used by streaming fallbacks.
func baseMessageMap(model string, startTime time.Time) map[string]interface{} {
	return map[string]interface{}{
		"id":      "msg_" + strconv.FormatInt(startTime.Unix(), 10),
		"type":    "message",
		"role":    "assistant",
		"content": []interface{}{},
		"model":   model,
	}
}

// coerceToMap normalises an arbitrary value to map[string]interface{}.
func coerceToMap(v any) (map[string]interface{}, bool) {
	switch x := v.(type) {
	case nil:
		return nil, false
	case map[string]interface{}:
		return x, true
	case []byte:
		var m map[string]interface{}
		if err := json.Unmarshal(x, &m); err != nil {
			logrus.Debugf("obs: failed to unmarshal response bytes: %v", err)
			return nil, false
		}
		return m, true
	default:
		data, err := json.Marshal(v)
		if err != nil {
			logrus.Debugf("obs: failed to marshal response: %v", err)
			return nil, false
		}
		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			logrus.Debugf("obs: failed to unmarshal response: %v", err)
			return nil, false
		}
		return m, true
	}
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
