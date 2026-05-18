package server

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// IncomingAPIType describes which OpenAI-style endpoint the client originally
// hit on this gateway. Only consulted when the provider declares
// EndpointModeBoth; otherwise the provider's declared mode dictates the
// upstream endpoint regardless of what the client sent.
type IncomingAPIType string

const (
	IncomingAPIChat      IncomingAPIType = "chat"
	IncomingAPIResponses IncomingAPIType = "responses"
)

// ResolveOpenAIEndpoint picks an OpenAI endpoint using the optional per-rule
// override first, then the provider's declared OpenAIEndpointMode.
//
// Precedence:
//
//  1. Rule flag (flags.OpenAIEndpointOverride). Overrides provider settings.
//  2. provider.OpenAIEndpointMode:
//     EndpointModeUnknown / zero value → Chat
//     EndpointModeChat                 → Chat
//     EndpointModeResponses            → Responses
//     EndpointModeBoth                 → mirror incoming
//
// Defaulting unknown providers to Chat (not "mirror incoming") is intentional:
// most OpenAI-compatible vendors implement only /chat/completions. Providers
// that genuinely support Responses must declare it via template or OAuth.
//
// When an incoming Responses request routes to Chat, Responses-only fields
// (previous_response_id, include, background, truncation, reasoning) are
// silently dropped by ConvertOpenAIResponsesToChat — the same posture as
// Anthropic→Chat downgrades. The user accepts this by declaring the mode.
//
// Pure function: no Server state, no probe lookups, no I/O.
func ResolveOpenAIEndpoint(provider *typ.Provider, flags typ.RuleFlags, incoming IncomingAPIType) (protocol.APIType, error) {
	if provider == nil {
		return "", fmt.Errorf("provider is required for endpoint selection")
	}

	mode := provider.OpenAIEndpointMode

	switch ParseEndpointOverride(flags.OpenAIEndpointOverride) {
	case OverrideChat:
		return protocol.TypeOpenAIChat, nil

	case OverrideResponses:
		return protocol.TypeOpenAIResponses, nil
	}

	switch mode {
	case ai.EndpointModeResponses:
		return protocol.TypeOpenAIResponses, nil
	case ai.EndpointModeBoth:
		if incoming == IncomingAPIResponses {
			return protocol.TypeOpenAIResponses, nil
		}
		return protocol.TypeOpenAIChat, nil
	default: // EndpointModeChat / zero value
		return protocol.TypeOpenAIChat, nil
	}
}
