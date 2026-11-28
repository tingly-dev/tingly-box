package recording

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ProtocolRecorder captures a single client→tingly-box→provider cycle.
//
// It carries both the scenario-level (client/final) and protocol-level
// (transformed) request/response pairs, plus optional streaming state. The
// recorder is mode-driven: which fields are emitted to the sink is decided
// by RecordMode (set at construction).
//
// Lifecycle:
//  1. EnsureProtocolRecorder at handler entry — captures client request,
//     session, mode.
//  2. Optional: transform pipeline writes SetOriginalRequest /
//     SetTransformedRequest via TransformRecorder.
//  3. For streaming, hooks call EnableStreaming + RecordStreamChunk +
//     SetAssembledResponse.
//  4. RecordResponse (success) or RecordError (failure) emits one *obs.Record.
type ProtocolRecorder struct {
	sink         *obs.Sink
	scenario     string
	startTime    time.Time
	c            *gin.Context
	sessionShort string
	sessionSrc   string

	isStreaming bool
	streamTap   *streamTap

	originalRequest    *obs.RecordRequest
	transformedRequest *obs.RecordRequest
	finalResponse      *obs.RecordResponse
	fingerprint        *obs.ResponseFingerprint

	transformSteps []string

	providerName  string
	providerUUID  string
	providerStyle string // API style (e.g., "openai", "anthropic")
	providerBase  string // Base URL
	model         string
	mode          obs.RecordMode
}

func NewProtocolRecorder(c *gin.Context, sink *obs.Sink, scenario string, mode obs.RecordMode, body []byte) (*ProtocolRecorder, error) {

	var bodyJSON map[string]interface{}
	if err := json.Unmarshal(body, &bodyJSON); err != nil {
		bodyJSON = map[string]interface{}{"raw": string(body)}
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
		mode:            normalizeMode(mode),
	}, nil
}

// normalizeMode canonicalizes a mode value (legacy enum values expand to
// point sets, tokens dedupe into pipeline order) so Wants/emit only ever see
// normalized point sets.
func normalizeMode(mode obs.RecordMode) obs.RecordMode {
	return obs.RecordMode(typ.ParseRecordingMode(string(mode)))
}

// Wants reports whether this recorder's mode selects the given capture
// point. Nil-safe: a nil recorder wants nothing.
func (sr *ProtocolRecorder) Wants(p typ.RecordingPoint) bool {
	if sr == nil {
		return false
	}
	return typ.RecordingMode(sr.mode).Has(p)
}

// SetActiveService re-binds the recorder to a new provider/model. The
// failover orchestrator calls this between attempts so records reflect the
// service currently being attempted. Breaker accounting is owned by the
// failover loop, not by recording.
func (sr *ProtocolRecorder) SetActiveService(provider *typ.Provider, model string) {
	if sr == nil {
		return
	}
	sr.BindProvider(provider, model, "")
}

// GetRecorderFromContext returns the ProtocolRecorder stashed in c by
// EnsureProtocolRecorder, if any. Exported for root callers
// (failover_dispatch.go, protocol_cross.go — both still Step 7-9 territory)
// that need to re-bind or inspect the active recorder mid-request.
func GetRecorderFromContext(c *gin.Context) (*ProtocolRecorder, bool) {
	return getRecorderFromContext(c)
}

func getRecorderFromContext(c *gin.Context) (*ProtocolRecorder, bool) {
	v, exists := c.Get(constant.CtxKeyProtocolRecorder)
	if !exists {
		return nil, false
	}
	rec, ok := v.(*ProtocolRecorder)
	return rec, ok
}

// FromGin is the canonical way protocol code reaches the per-request
// recorder: nil when recording is off, and every recorder method is
// nil-safe, so callers use the result unconditionally. The recorder rides
// the gin context (stashed once by EnsureProtocolRecorder) instead of being
// threaded through protocol call signatures — mirroring how rule flags ride
// the request context rather than parameter lists.
func FromGin(c *gin.Context) *ProtocolRecorder {
	if c == nil {
		return nil
	}
	rec, _ := getRecorderFromContext(c)
	return rec
}

func (sr *ProtocolRecorder) BindProvider(provider *typ.Provider, model string, mode obs.RecordMode) {
	if sr == nil {
		return
	}
	if provider != nil {
		sr.providerName = provider.Name
		sr.providerUUID = provider.UUID
		sr.providerStyle = string(provider.APIStyle)
		sr.providerBase = provider.APIBase
	}
	if model != "" {
		sr.model = model
	}
	if mode != "" {
		sr.mode = normalizeMode(mode)
	}
}

// EnableStreaming puts the recorder into streaming mode and starts the
// per-stream tap that accumulates the response fingerprint. Chunk bodies are
// not buffered: only shape and timing are kept (bounded memory, no content).
func (sr *ProtocolRecorder) EnableStreaming() {
	if sr == nil {
		return
	}
	sr.isStreaming = true
	if sr.streamTap == nil {
		sr.streamTap = newStreamTap(sr.startTime)
	}
}

// RecordStreamChunk feeds a single stream chunk into the response tap. The
// full chunk payload is deliberately not retained — the tap keeps only counts,
// event names, timing, usage and identity scalars. Chunk bodies can be large
// (tool arguments, long deltas) and are not needed for fingerprinting.
func (sr *ProtocolRecorder) RecordStreamChunk(eventType string, chunk interface{}) {
	if sr == nil || !sr.isStreaming {
		return
	}
	if sr.streamTap == nil {
		sr.streamTap = newStreamTap(sr.startTime)
	}

	var chunkJSON []byte
	var err error

	switch v := chunk.(type) {
	case *anthropic.MessageStreamEventUnion:
		chunkJSON = rawJSONOrMarshal(v.RawJSON(), v)
	case *anthropic.BetaRawMessageStreamEventUnion:
		chunkJSON = rawJSONOrMarshal(v.RawJSON(), v)
	case interface{ RawJSON() string }:
		chunkJSON = rawJSONOrMarshal(v.RawJSON(), v)
	default:
		chunkJSON, err = json.Marshal(chunk)
		if err != nil {
			logrus.Debugf("obs: failed to marshal stream chunk: %v", err)
			return
		}
	}
	if len(chunkJSON) == 0 {
		return
	}

	var chunkData map[string]interface{}
	if err := json.Unmarshal(chunkJSON, &chunkData); err != nil {
		return
	}
	sr.streamTap.record(eventType, chunkData)
}

// rawJSONOrMarshal prefers the SDK's cached raw JSON, falling back to a plain
// marshal when the event was built in memory and has no raw payload (tests,
// synthesised events).
func rawJSONOrMarshal(raw string, v interface{}) []byte {
	if raw != "" && raw != "null" {
		return []byte(raw)
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}

// SetAssembledResponse stores the final assembled (post-stream) response.
// Accepts map, []byte, or any JSON-marshall-able value.
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
	sr.BindProvider(provider, model, "")
	if sr.finalResponse == nil {
		sr.finalResponse = sr.synthesizeFinalResponse()
	}
	sr.emit(nil)
}

// RecordError emits an error record. err may be nil.
func (sr *ProtocolRecorder) RecordError(err error) {
	if sr == nil {
		return
	}
	sr.emit(err)
}

func (sr *ProtocolRecorder) emit(err error) {
	if sr.sink == nil || sr.mode == "" {
		// Still drop buffered request/response payloads: without a sink the
		// recorder would otherwise keep them reachable via the gin context
		// for the remainder of the request.
		sr.release()
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
		APIStyle:   sr.providerStyle,
		BaseURL:    sr.providerBase,
	}
	if err != nil {
		r.Err = err.Error()
	}

	// Capture-point filtering: each selected point contributes its slot.
	//
	// final_response was paused in #1644 because streaming relied on
	// assembly/synthesis fallbacks whose quality was inadequate. It is
	// resumed here for passive response capture: streamed responses now have
	// a chunk tap on every path that records them (native Anthropic hooks and
	// the OpenAI passthrough taps), and the emitted record carries a compact,
	// redacted ResponseFingerprint alongside the (size-capped) body. See
	// .design/recording.md §3.5.
	//
	// upstream_response (Record.ProviderResponse) is still without a producer:
	// provider raw bytes/headers are only available at the wire transport
	// (Phase 3 wire recorder).
	m := typ.RecordingMode(sr.mode)
	if m.Has(typ.RecordClientRequest) {
		r.OriginalRequest = sr.originalRequest
	}
	if m.Has(typ.RecordUpstreamRequest) {
		r.TransformedRequest = sr.transformedRequest
	}
	if m.Has(typ.RecordFinalResponse) && err == nil && sr.finalResponse != nil {
		// Build the fingerprint first (it needs the raw headers), then
		// redact/cap the full response that is persisted for debugging.
		sr.fingerprint = sr.buildFingerprint()
		sr.finalResponse.Headers = obs.RedactFullHeaders(sr.finalResponse.Headers)
		sr.finalResponse.Body = obs.CapResponseBody(sr.finalResponse.Body, obs.DefaultMaxResponseBodyBytes)
		r.FinalResponse = sr.finalResponse
		r.Fingerprint = sr.fingerprint
	}

	sr.sink.Emit(r)
	sr.release()
}

// buildFingerprint derives the compact response fingerprint from the assembled
// response body (when present) and the stream tap. It must run before the
// final_response headers/body are redacted and capped.
func (sr *ProtocolRecorder) buildFingerprint() *obs.ResponseFingerprint {
	status := 0
	var headers map[string]string
	var body map[string]interface{}
	if sr.finalResponse != nil {
		status = sr.finalResponse.StatusCode
		headers = sr.finalResponse.Headers
		body = sr.finalResponse.Body
	}
	if status == 0 && sr.c != nil {
		status = sr.c.Writer.Status()
	}
	fp := obs.BuildResponseFingerprint(sr.providerStyle, status, headers, body, time.Since(sr.startTime))
	if sr.streamTap != nil && sr.isStreaming {
		tap := sr.streamTap
		fp.Stream = tap.snapshot(time.Since(sr.startTime))
		// Prefer values seen on the upstream event stream over body-derived
		// ones: the body may be a synthesis stub (its id is a placeholder) or
		// have been rewritten by the gateway/assembler.
		if tap.model != "" {
			fp.Model = tap.model
		}
		if tap.id != "" {
			fp.IDPattern = obs.IDPattern(tap.id)
		}
		if tap.systemFingerprint != "" {
			fp.SystemFingerprint = tap.systemFingerprint
		}
		if tap.serviceTier != "" {
			fp.ServiceTier = tap.serviceTier
		}
		if tap.usage != nil {
			fp.Usage = tap.usage
		}
		if len(tap.finishReasons) > 0 {
			fp.FinishReasons = tap.finishReasons
		}
	}
	return fp
}

// release drops per-request recorder state once the obs.Record has
// been handed to the sink. This prevents completed protocol stream responses
// from remaining reachable through gin.Context keys or recorder references; the
// emitted record owns whatever payload the selected record mode intentionally
// preserves.
func (sr *ProtocolRecorder) release() {
	if sr == nil {
		return
	}
	sr.streamTap = nil
	sr.originalRequest = nil
	sr.transformedRequest = nil
	sr.finalResponse = nil
	sr.fingerprint = nil
	sr.transformSteps = nil
	sr.c = nil
}

// resolveRequestID returns the request correlation id established by the
// access-log middleware so the recording (system B) shares an id with the
// logrus traces (system A). Falls back to a fresh uuid when the recorder
// runs outside an HTTP request.
func (sr *ProtocolRecorder) resolveRequestID() string {
	if sr.c != nil {
		if id := sr.c.GetString(constant.CtxKeyRequestID); id != "" {
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
	if sr.isStreaming && sr.streamTap != nil && sr.streamTap.chunkCount > 0 {
		bodyJSON = baseMessageMap(sr.model, sr.startTime)
		bodyJSON["_stream_chunks"] = sr.streamTap.chunkCount
		bodyJSON["_note"] = "assembled response unavailable"
		logrus.Debugf("obs: ProtocolRecorder fallback response, chunks=%d", sr.streamTap.chunkCount)
	}

	resp := &obs.RecordResponse{
		StatusCode:  statusCode,
		Headers:     headers,
		Body:        bodyJSON,
		IsStreaming: sr.isStreaming,
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
