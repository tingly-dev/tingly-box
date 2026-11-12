package client

// AnthropicWire identifies which Anthropic API wire version a client
// speaks. The versions differ in wire details (API version, auth header
// shape, sampling-parameter acceptance), and future API versions extend
// this enum rather than adding more boolean flags.
type AnthropicWire int

const (
	// WireV1 is the classic 2023-06-01 Messages API: x-api-key auth,
	// ?beta=true on beta endpoints, temperature/top_p/top_k accepted. The
	// wire the third-party compatible ecosystem is on.
	WireV1 AnthropicWire = iota
	// WireV2 is the 2026-10-21 Messages API: Authorization: Bearer auth,
	// no ?beta=true, sampling parameters rejected. See NewAnthropicClientV2 /
	// NewClaudeClientV2.
	WireV2
)
