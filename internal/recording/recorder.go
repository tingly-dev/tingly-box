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
// Lifecycle — the recorder is a request-scoped observer that protocol code
// only annotates; it emits exactly once per request:
//  1. BeginRuleRecording (protocol_handler.go) at handler entry — the single
//     creation point: captures client request, session, mode; cached on the
//     gin context.
//  2. The transform pipeline writes SetOriginalRequest / SetTransformedRequest
//     via TransformRecorder (chain stages), streaming hooks add
//     EnableStreaming + RecordStreamChunk + SetAssembledResponse, and error
//     paths annotate via RecordError — none of these emit.
//  3. Exactly one *obs.Record is emitted, whichever comes first (the latch
//     makes later triggers no-ops):
//     - RecordResponse on a completed response (terminal success), or
//     - FinalizeIfPending in the dispatch orchestrator
//     (DispatchWithPriorityFailover's defer) — the request-level backstop
//     that flushes the record with the last annotated error when no
//     success emit happened (failed/aborted requests, exhausted failover).
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

	providerName  string
	providerUUID  string
	providerStyle string // API style (e.g., "openai", "anthropic")
	providerBase  string // Base URL
	model         string
	mode          obs.RecordMode

	// emitted latches after the first emit: a request produces exactly one
	// record; later RecordResponse/FinalizeIfPending triggers are no-ops.
	emitted bool
	// lastErr is the most recent error annotated via RecordError. It reaches
	// the sink only if FinalizeIfPending performs the emit (no success emit
	// happened); a terminal RecordResponse emits with no error.
	lastErr error
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

// WantsUpstreamRequest and RecordWireRequest together satisfy
// client.WireRecorder (structurally — internal/client cannot import this
// package, see .design/recording.md). This is the corrected capture point
// for upstream_request: internal/client's wireRecorderTransport calls these
// from the innermost transport layer, right before the request hits the
// wire, so — unlike the old chain-level snapshot — the real upstream URL is
// captured as actually sent. Headers are deliberately not captured (see
// wireRecorderTransport's doc comment); Headers on the resulting
// RecordRequest is always empty.
func (sr *ProtocolRecorder) WantsUpstreamRequest() bool {
	return sr.Wants(typ.RecordUpstreamRequest)
}

// RecordWireRequest stores the final outbound request. body is best-effort
// JSON-decoded into RecordRequest.Body; a non-JSON body leaves Body nil
// (method/URL are still captured).
func (sr *ProtocolRecorder) RecordWireRequest(method, url string, body []byte) {
	if sr == nil {
		return
	}
	var bodyMap map[string]interface{}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &bodyMap)
	}
	sr.SetTransformedRequest(&obs.RecordRequest{
		Method: method,
		URL:    url,
		Body:   bodyMap,
	})
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

// RecordResponse finalises provider/model and emits the request's Record
// (terminal success). Idempotent: the emit latch makes any later trigger a
// no-op, so a path that reports success twice still produces one record.
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

// RecordError annotates the request's pending failure — it does NOT emit.
// Protocol code calls this from attempt-scoped paths (a failover retry, an
// MCP continuation round, a transform failure) where the request may still
// succeed; emitting here used to produce premature/duplicate records. The
// annotated error reaches the sink only if no success emit happens, via
// FinalizeIfPending at the dispatch orchestrator.
func (sr *ProtocolRecorder) RecordError(err error) {
	if sr == nil || err == nil {
		return
	}
	sr.lastErr = err
}

// FinalizeIfPending emits the request's Record if nothing has emitted yet,
// carrying the last annotated error (nil for a request that produced no
// response and no error). The dispatch orchestrator defers this once per
// request; after a terminal RecordResponse it is a no-op.
func (sr *ProtocolRecorder) FinalizeIfPending() {
	if sr == nil || sr.emitted {
		return
	}
	sr.emit(sr.lastErr)
}

func (sr *ProtocolRecorder) emit(err error) {
	if sr.emitted {
		logrus.Debug("obs: ProtocolRecorder emit after latch — dropped (one record per request)")
		return
	}
	sr.emitted = true

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
	// Response-side points are paused for now — request recording is the
	// supported surface (.design/recording.md §3.5):
	//   - upstream_response (Record.ProviderResponse) has no producer until
	//     the wire-level recorder lands (Phase 3).
	//   - final_response (Record.FinalResponse) is captured internally
	//     (SetAssembledResponse / synthesis fallback) but its quality is not
	//     good enough to emit yet; re-enable when the response path is
	//     reworked (Phase 4 EventTap):
	//     if m.Has(typ.RecordFinalResponse) { r.FinalResponse = sr.finalResponse }
	m := typ.RecordingMode(sr.mode)
	if m.Has(typ.RecordClientRequest) {
		r.OriginalRequest = sr.originalRequest
	}
	if m.Has(typ.RecordUpstreamRequest) {
		r.TransformedRequest = sr.transformedRequest
	}

	sr.sink.Emit(r)
	sr.release()
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
	sr.streamChunks = nil
	sr.originalRequest = nil
	sr.transformedRequest = nil
	sr.finalResponse = nil
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
	if sr.isStreaming && len(sr.streamChunks) > 0 {
		bodyJSON = baseMessageMap(sr.model, sr.startTime)
		bodyJSON["_stream_chunks"] = len(sr.streamChunks)
		bodyJSON["_note"] = "assembled response unavailable"
		logrus.Debugf("obs: ProtocolRecorder fallback response, chunks=%d", len(sr.streamChunks))
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
