package benchmark

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
)

// EndpointKind identifies which provider-native endpoint a request hit. chat and
// responses are deliberately distinct: they are two different OpenAI protocols,
// and observers assert which one was actually forwarded to.
type EndpointKind string

const (
	EndpointChat      EndpointKind = "chat"
	EndpointResponses EndpointKind = "responses"
	EndpointAnthropic EndpointKind = "anthropic"
	EndpointGoogle    EndpointKind = "google"
	EndpointUnknown   EndpointKind = "unknown"
)

// classify maps a provider-native request path to an EndpointKind. It is
// path-prefix based so it works for both the production responder
// (/v1/chat/completions, /openai/v1/..., /anthropic/v1/...) and the scenario
// responder, and for Google's /v1beta/models/{model}:generateContent shape.
func classify(path string) EndpointKind {
	switch {
	case strings.Contains(path, "/chat/completions"):
		return EndpointChat
	case strings.Contains(path, "/responses"):
		return EndpointResponses
	case strings.Contains(path, "/messages"):
		return EndpointAnthropic
	case strings.Contains(path, "generateContent") || strings.Contains(path, "/v1beta/models"):
		return EndpointGoogle
	default:
		return EndpointUnknown
	}
}

// CapturedRequest records what the gateway forwarded to a provider endpoint so
// observers can assert on the outbound request (model, flags, headers, body).
type CapturedRequest struct {
	Method  string
	Path    string
	Headers http.Header
	Body    []byte
}

// JSON decodes the captured body into a generic map. Returns an empty map if the
// body is absent or not JSON.
func (cr *CapturedRequest) JSON() map[string]interface{} {
	out := map[string]interface{}{}
	if cr == nil || len(cr.Body) == 0 {
		return out
	}
	_ = json.Unmarshal(cr.Body, &out)
	return out
}

// recorder is the shared observability state behind a benchmark Server. It is
// safe for concurrent use; the capture middleware writes, observers read.
type recorder struct {
	mu           sync.RWMutex
	callCount    int
	endpointHits map[EndpointKind]int
	captured     map[EndpointKind]*CapturedRequest
}

func newRecorder() *recorder {
	return &recorder{
		endpointHits: make(map[EndpointKind]int),
		captured:     make(map[EndpointKind]*CapturedRequest),
	}
}

// record increments counters and stores the captured request for an endpoint.
// body must be the already-read request body (the caller restores r.Body for
// the inner handler separately).
func (rec *recorder) record(kind EndpointKind, r *http.Request, body []byte) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.callCount++
	rec.endpointHits[kind]++
	rec.captured[kind] = &CapturedRequest{
		Method:  r.Method,
		Path:    r.URL.Path,
		Headers: r.Header.Clone(),
		Body:    append([]byte(nil), body...),
	}
}

func (rec *recorder) totalCalls() int {
	rec.mu.RLock()
	defer rec.mu.RUnlock()
	return rec.callCount
}

func (rec *recorder) hits(kind EndpointKind) int {
	rec.mu.RLock()
	defer rec.mu.RUnlock()
	return rec.endpointHits[kind]
}

func (rec *recorder) lastRequest(kind EndpointKind) *CapturedRequest {
	rec.mu.RLock()
	defer rec.mu.RUnlock()
	return rec.captured[kind]
}

func (rec *recorder) reset() {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.callCount = 0
	rec.endpointHits = make(map[EndpointKind]int)
	rec.captured = make(map[EndpointKind]*CapturedRequest)
}
