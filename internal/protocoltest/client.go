package protocoltest

import (
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// harnessPrompt is the single user prompt every matrix request carries.
// The scenario controls the mock response, not the request, so all client
// drivers send the same prompt as buildRequest does.
const harnessPrompt = "What is the capital of France?"

// SendSpec carries everything a client driver needs to issue one request
// through the gateway. Target and ScenarioName are response metadata only;
// the request itself varies only by (Source, RequestModel, Streaming).
type SendSpec struct {
	Source       protocol.APIType
	Target       protocol.APIType
	ScenarioName string
	RequestModel string
	Streaming    bool
	GatewayURL   string // real HTTP base URL of the gateway, e.g. http://127.0.0.1:PORT
	APIKey       string // gateway model token
}

// Client drives a single request through the gateway and returns the
// normalized RoundTripResult the assertion layer consumes. Implementations
// range from the raw in-process HTTP client (default) to official SDKs and
// external subprocess drivers (python/node), so the same matrix exercises
// the gateway through progressively more realistic client stacks.
type Client interface {
	// Name identifies the driver: "http", "gosdk", "python", "node".
	Name() string
	// Supports reports whether the driver can speak the given source protocol.
	Supports(source protocol.APIType) bool
	// Send issues one request and returns the normalized result. Gateway-side
	// API errors (4xx/5xx) must be reported in the result (HTTPStatus/RawBody),
	// not as a returned error; a non-nil error means the driver itself failed.
	Send(env *TestEnv, spec SendSpec) (*RoundTripResult, error)
}

// newRoundTripResult seeds a result with the spec's identifying metadata.
func newRoundTripResult(spec SendSpec) *RoundTripResult {
	return &RoundTripResult{
		SourceProtocol: spec.Source,
		TargetProtocol: spec.Target,
		ScenarioName:   spec.ScenarioName,
		IsStreaming:    spec.Streaming,
	}
}

// normalizeJSON parses a final-response JSON body into the result's semantic
// fields (Content, Role, ToolCalls, Usage, ...). It is the shared
// normalization entry point for drivers that obtain a complete response
// object (SDK accumulators, subprocess drivers re-marshaling).
func normalizeResultJSON(result *RoundTripResult, raw []byte, source protocol.APIType, streaming bool) {
	parsed := parseFromJSON(raw, sourceToStyle(source))
	fillFromParsedResult(result, parsed, sourceToStyle(source), streaming)
}
