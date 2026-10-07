package protocoltest

import (
	"testing"

	"github.com/tingly-dev/tingly-box/vmodel/benchmark"
)

// VirtualServer is a mock provider server speaking OpenAI, Anthropic, and Google
// response formats, returning pre-configured scenario responses.
//
// As of the benchmark unification it is a thin wrapper over
// benchmark.Server (scenario responder): the scenario-serving handlers, request
// capture, and endpoint-hit counting all live in vmodel/benchmark now. This type
// keeps the protocoltest-facing API (Client(), RegisterScenario, EndpointHits,
// LastRequest, …) stable. See .design/vmodel-benchmark.md.
//
// **Provider Routes**: This server handles provider-native routes
// (/v1/chat/completions, /v1/messages, /v1beta/models/...), NOT gateway routes
// (/tingly/{scenario}/v1/...). The gateway transforms requests to provider
// format before forwarding here.
type VirtualServer struct {
	srv *benchmark.Server
}

// EndpointKind identifies which provider-native endpoint a request hit. Aliased
// to the benchmark foundation so observers share one vocabulary.
type EndpointKind = benchmark.EndpointKind

const (
	EndpointChat      = benchmark.EndpointChat
	EndpointResponses = benchmark.EndpointResponses
	EndpointAnthropic = benchmark.EndpointAnthropic
	EndpointGoogle    = benchmark.EndpointGoogle
)

// CapturedRequest is the request the gateway forwarded to a provider endpoint.
// Aliased to the benchmark foundation.
type CapturedRequest = benchmark.CapturedRequest

// NewVirtualServer creates a new VirtualServer and registers cleanup with t.
func NewVirtualServer(t *testing.T) *VirtualServer {
	t.Helper()
	vs := newVirtualServer()
	t.Cleanup(vs.Close)
	return vs
}

// NewVirtualServerForCLI creates a new VirtualServer for CLI use (without
// testing.T). The caller must call Close() to clean up resources.
func NewVirtualServerForCLI() *VirtualServer {
	return newVirtualServer()
}

func newVirtualServer() *VirtualServer {
	srv := benchmark.NewScenarioServer()
	srv.InProcess()
	return &VirtualServer{srv: srv}
}

// RegisterScenario registers a scenario so the virtual server can serve its mock
// responses. A prior scenario with the same name is replaced.
func (vs *VirtualServer) RegisterScenario(s Scenario) {
	vs.srv.RegisterScenario(s)
}

// URL returns the base URL of the virtual server.
func (vs *VirtualServer) URL() string {
	return vs.srv.URL()
}

// Close shuts down the virtual server.
func (vs *VirtualServer) Close() {
	_ = vs.srv.Close()
}

// CallCount returns the total number of requests received.
func (vs *VirtualServer) CallCount() int {
	return vs.srv.CallCount()
}

// EndpointHits returns how many requests hit a specific provider endpoint. Lets
// tests assert that, e.g., target=openai_responses actually forwarded to
// /v1/responses rather than silently falling back to /v1/chat/completions.
func (vs *VirtualServer) EndpointHits(kind EndpointKind) int {
	return vs.srv.EndpointHits(kind)
}

// LastRequest returns the most recent request the gateway forwarded to the given
// provider endpoint, or nil if that endpoint was never hit.
func (vs *VirtualServer) LastRequest(kind EndpointKind) *CapturedRequest {
	return vs.srv.LastRequest(kind)
}
