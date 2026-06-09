package protocoltest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol/sse"
	"github.com/tingly-dev/tingly-box/vmodel"
)

// VirtualServer is a mock provider server backed by httptest.Server.
// It speaks OpenAI, Anthropic, and Google response formats and returns
// pre-configured scenario responses.
//
// **Provider Routes**: This server handles provider-native routes (/v1/chat/completions,
// /v1/messages, /v1beta/models/...), NOT gateway routes (/tingly/{scenario}/v1/...).
// The gateway transforms requests to provider format before forwarding to this server.
// EndpointKind identifies which provider-native endpoint a request hit.
// chat and responses are deliberately distinct: they are two different OpenAI
// protocols, and tests assert which one the gateway actually forwarded to.
type EndpointKind string

const (
	EndpointChat      EndpointKind = "chat"
	EndpointResponses EndpointKind = "responses"
	EndpointAnthropic EndpointKind = "anthropic"
	EndpointGoogle    EndpointKind = "google"
)

type VirtualServer struct {
	server    *httptest.Server
	scenarios *vmodel.GenericRegistry[Scenario]

	mu           sync.RWMutex
	callCount    int
	endpointHits map[EndpointKind]int
	captured     map[EndpointKind]*CapturedRequest
}

// JSON decodes a captured request body into a generic map. It returns an empty
// map if the body is absent or not JSON. Used by flag-behavior tests to assert
// on what the gateway actually forwarded to the upstream provider.
func (cr *CapturedRequest) JSON() map[string]interface{} {
	out := map[string]interface{}{}
	if cr == nil || len(cr.Body) == 0 {
		return out
	}
	_ = json.Unmarshal(cr.Body, &out)
	return out
}

// NewVirtualServer creates a new VirtualServer and registers cleanup with t.
func NewVirtualServer(t *testing.T) *VirtualServer {
	t.Helper()
	vs := &VirtualServer{
		scenarios:    vmodel.NewGenericRegistry[Scenario](),
		endpointHits: make(map[EndpointKind]int),
		captured:     make(map[EndpointKind]*CapturedRequest),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", vs.handleOpenAIChat)
	mux.HandleFunc("/v1/responses", vs.handleOpenAIResponses)
	mux.HandleFunc("/v1/messages", vs.handleAnthropicMessages)
	mux.HandleFunc("/", vs.handleGoogle) // catches /v1beta/models/*/generateContent

	vs.server = httptest.NewServer(mux)
	t.Cleanup(vs.Close)
	return vs
}

// NewVirtualServerForCLI creates a new VirtualServer for CLI use (without testing.T).
// The caller must call Close() to clean up resources.
//
// **Provider Mock**: This is a provider mock, not a gateway. Routes are /v1/...
// (provider-native), not /tingly/... (gateway). The gateway transforms /tingly/{scenario}/v1/...
// requests to provider format before forwarding to this server.
func NewVirtualServerForCLI() *VirtualServer {
	vs := &VirtualServer{
		scenarios:    vmodel.NewGenericRegistry[Scenario](),
		endpointHits: make(map[EndpointKind]int),
		captured:     make(map[EndpointKind]*CapturedRequest),
	}

	mux := http.NewServeMux()

	// Provider-native routes (matching actual provider APIs)
	// Most providers require /v1/ prefix, but we register both for flexibility
	mux.HandleFunc("/v1/chat/completions", vs.handleOpenAIChat)
	mux.HandleFunc("/v1/responses", vs.handleOpenAIResponses)
	mux.HandleFunc("/v1/messages", vs.handleAnthropicMessages)

	// Google API route pattern: /v1beta/models/{model}/generateContent
	// Using catch-all handler since model name is dynamic in the path
	mux.HandleFunc("/v1beta/models/", vs.handleGoogle)

	vs.server = httptest.NewServer(mux)
	return vs
}

// RegisterScenario registers a scenario so the virtual server can serve its
// mock responses. If a scenario with the same name was previously registered
// it is replaced (the registry ordinarily errors on duplicate IDs, so we
// pre-clear).
func (vs *VirtualServer) RegisterScenario(s Scenario) {
	vs.scenarios.Unregister(s.Name)
	_ = vs.scenarios.Register(s)
}

// URL returns the base URL of the virtual server.
func (vs *VirtualServer) URL() string {
	return vs.server.URL
}

// Close shuts down the virtual server.
func (vs *VirtualServer) Close() {
	vs.server.Close()
}

// CallCount returns the total number of requests received.
func (vs *VirtualServer) CallCount() int {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	return vs.callCount
}

// EndpointHits returns how many requests hit a specific provider endpoint.
// Lets tests assert that, e.g., target=openai_responses actually forwarded to
// /v1/responses rather than silently falling back to /v1/chat/completions.
func (vs *VirtualServer) EndpointHits(kind EndpointKind) int {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	return vs.endpointHits[kind]
}

// LastRequest returns the most recent request the gateway forwarded to the
// given provider endpoint, or nil if that endpoint was never hit.
func (vs *VirtualServer) LastRequest(kind EndpointKind) *CapturedRequest {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	return vs.captured[kind]
}

// capture records the outbound request for a provider endpoint so flag tests can
// assert on what the gateway actually forwarded. body must be the already-read
// request body (handlers restore r.Body separately for their own parsing).
func (vs *VirtualServer) capture(kind EndpointKind, r *http.Request, body []byte) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	vs.captured[kind] = &CapturedRequest{
		Method:  r.Method,
		Path:    r.URL.Path,
		Headers: r.Header.Clone(),
		Body:    append([]byte(nil), body...),
	}
}

// recordHit increments the total call count and the per-endpoint counter.
func (vs *VirtualServer) recordHit(kind EndpointKind) {
	vs.mu.Lock()
	vs.callCount++
	vs.endpointHits[kind]++
	vs.mu.Unlock()
}

// ─── HTTP handlers ─────────────────────────────────────────────────────────────

// writeBuilderResponse serves a MockResponseBuilder over HTTP. For streaming
// requests it writes a 200 SSE stream, except when the builder declares a
// pre-content HTTP error (StreamHTTPError >= 400) — those fail at the HTTP
// status line, the same way a real provider rejects an auth/rate-limit/5xx error
// before any SSE frame is emitted.
func writeBuilderResponse(w http.ResponseWriter, builder MockResponseBuilder, streaming bool) {
	if streaming && builder.StreamHTTPError < 400 {
		sse.WriteSSEResponse(w, builder.Stream())
		return
	}
	status, body := builder.NonStream()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(body)
}

func (vs *VirtualServer) handleOpenAIChat(w http.ResponseWriter, r *http.Request) {
	vs.recordHit(EndpointChat)

	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	vs.capture(EndpointChat, r, bodyBytes)

	streaming := vs.parseStreamFlagFromBytes(bodyBytes)
	scenario := vs.detectScenario(bodyBytes)

	resp := vs.scenarios.Get(scenario)
	if resp.Name == "" {
		resp = vs.firstScenario()
	}

	builder, ok := resp.MockResponses[FormatOpenAIChat]
	if !ok {
		http.Error(w, "no openai_chat mock response for scenario "+scenario, http.StatusInternalServerError)
		return
	}

	writeBuilderResponse(w, builder, streaming)
}

func (vs *VirtualServer) handleOpenAIResponses(w http.ResponseWriter, r *http.Request) {
	vs.recordHit(EndpointResponses)

	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	vs.capture(EndpointResponses, r, bodyBytes)

	streaming := vs.parseStreamFlagFromBytes(bodyBytes)
	scenario := vs.detectScenario(bodyBytes)

	resp := vs.scenarios.Get(scenario)
	if resp.Name == "" {
		resp = vs.firstScenario()
	}

	builder, ok := resp.MockResponses[FormatOpenAIResponses]
	if !ok {
		// Fallback to FormatOpenAIChat for Responses API when not explicitly defined
		builder, ok = resp.MockResponses[FormatOpenAIChat]
		if !ok {
			http.Error(w, "no openai_responses or openai_chat mock response for scenario "+scenario, http.StatusInternalServerError)
			return
		}
	}

	writeBuilderResponse(w, builder, streaming)
}

func (vs *VirtualServer) handleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	vs.recordHit(EndpointAnthropic)

	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	vs.capture(EndpointAnthropic, r, bodyBytes)

	streaming := vs.parseStreamFlagFromBytes(bodyBytes)
	scenario := vs.detectScenario(bodyBytes)

	resp := vs.scenarios.Get(scenario)
	if resp.Name == "" {
		resp = vs.firstScenario()
	}

	builder, ok := resp.MockResponses[FormatAnthropic]
	if !ok {
		http.Error(w, "no anthropic mock response for scenario "+scenario, http.StatusInternalServerError)
		return
	}

	writeBuilderResponse(w, builder, streaming)
}

func (vs *VirtualServer) handleGoogle(w http.ResponseWriter, r *http.Request) {
	vs.recordHit(EndpointGoogle)

	bodyBytes, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	vs.capture(EndpointGoogle, r, bodyBytes)

	streaming := strings.Contains(r.URL.Path, "streamGenerateContent")
	scenario := vs.detectScenarioFromURLOrBody(r.URL.Path, bodyBytes)

	resp := vs.scenarios.Get(scenario)
	if resp.Name == "" {
		resp = vs.firstScenario()
	}

	builder, ok := resp.MockResponses[FormatGoogle]
	if !ok {
		http.Error(w, "no google mock response for scenario "+scenario, http.StatusInternalServerError)
		return
	}

	writeBuilderResponse(w, builder, streaming)
}

// ─── Helpers ───────────────────────────────────────────────────────────────────

func (vs *VirtualServer) parseStreamFlagFromBytes(body []byte) bool {
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return false
	}
	flag, _ := m["stream"].(bool)
	return flag
}

// detectScenarioFromURLOrBody extracts scenario from Google-style URL path or falls back to body.
// Google requests encode the model in the URL: /v1beta/models/{model}/generateContent
func (vs *VirtualServer) detectScenarioFromURLOrBody(urlPath string, body []byte) string {
	const prefix = "virtual-model-"
	// Try URL path first: extract model segment
	parts := strings.Split(urlPath, "/")
	for _, part := range parts {
		if strings.HasPrefix(part, prefix) {
			remaining := part[len(prefix):] // "{target}-{scenario}"
			// Find the last "-" to separate target from scenario
			lastDash := strings.LastIndex(remaining, "-")
			var name string
			if lastDash > 0 {
				name = remaining[lastDash+1:]
			} else {
				name = remaining
			}
			// Strip trailing action suffix if any (e.g. ":generateContent" edge case)
			if idx := strings.IndexByte(name, ':'); idx >= 0 {
				name = name[:idx]
			}
			if vs.scenarios.Has(name) {
				return name
			}
		}
	}
	// Fallback to body parsing
	return vs.detectScenario(body)
}

// detectScenario extracts the scenario name from the request body's model field.
// The model field is expected to be "virtual-model-{scenario}" (set by SetupRoute).
// Falls back to the first registered scenario if extraction fails.
func (vs *VirtualServer) detectScenario(body []byte) string {
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err == nil {
		if model, ok := m["model"].(string); ok {
			const prefix = "virtual-model-"
			if strings.HasPrefix(model, prefix) {
				name := model[len(prefix):]
				if vs.scenarios.Has(name) {
					return name
				}
			}
		}
	}
	// Fallback: return first registered scenario name
	for _, s := range vs.scenarios.List() {
		return s.Name
	}
	return ""
}

func (vs *VirtualServer) firstScenario() Scenario {
	for _, s := range vs.scenarios.List() {
		return s
	}
	return Scenario{}
}
