package server

import (
	"fmt"

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// IncomingAPIType describes which OpenAI-style endpoint the client originally
// hit on this gateway. The resolver only mirrors when the provider declares
// EndpointModeBoth; otherwise the provider's declared mode dictates the
// upstream endpoint regardless of what the client sent.
type IncomingAPIType string

const (
	IncomingAPIChat      IncomingAPIType = "chat"
	IncomingAPIResponses IncomingAPIType = "responses"
)

// EndpointSelection is the resolver's decision for a single request.
type EndpointSelection struct {
	Target protocol.APIType
	Reason string
}

func mirrorIncoming(incoming IncomingAPIType, reason string) *EndpointSelection {
	if incoming == IncomingAPIResponses {
		return &EndpointSelection{Target: protocol.TypeOpenAIResponses, Reason: reason}
	}
	return &EndpointSelection{Target: protocol.TypeOpenAIChat, Reason: reason}
}

// CanDowngradeResponsesToChat reports whether a Responses request can be
// safely emitted as Chat Completions instead. Called by the openai_responses
// handler after routing to reject requests whose Responses-only features
// would be silently dropped by the downgrade.
func CanDowngradeResponsesToChat(req protocol.ResponseCreateRequest) (bool, string) {
	p := req.ResponseNewParams
	switch {
	case p.PreviousResponseID.Valid():
		return false, "previous_response_id cannot be represented in Chat Completions"
	case len(p.Include) > 0:
		return false, "include cannot be represented in Chat Completions"
	case p.Background.Valid() && p.Background.Value:
		return false, "background mode is not supported by Chat Completions"
	case !param.IsOmitted(p.Truncation):
		return false, "Responses truncation cannot be safely represented in Chat Completions"
	case !param.IsOmitted(p.Reasoning):
		return false, "Responses reasoning configuration cannot be safely represented in Chat Completions"
	}
	return true, ""
}

// ResolveOpenAIEndpoint picks an OpenAI endpoint using only the provider's
// declared OpenAIEndpointMode and the optional per-rule override.
//
// Precedence:
//
//  1. Rule flag (flags.OpenAIEndpointOverride). Provider declarations trump
//     overrides — asking for Chat on a Responses-only provider (or vice versa)
//     logs a warning and uses the provider's endpoint instead.
//  2. provider.OpenAIEndpointMode:
//       EndpointModeChat (default) → Chat
//       EndpointModeResponses      → Responses
//       EndpointModeBoth           → mirror incoming
//
// Defaulting to Chat (not "mirror incoming") is intentional: most
// OpenAI-compatible vendors implement only /chat/completions, and silently
// trying /responses against them fails. Providers that genuinely support
// Responses must declare it via the template or OAuth instantiation.
//
// Callers receiving an OpenAI Responses request that ends up routed to Chat
// must run CanDowngradeResponsesToChat against the request body and reject
// if it returns false. The resolver doesn't inspect request bodies — that
// concern belongs to the protocol handler that knows the request shape.
//
// The function is pure: no Server state, no probe lookups, no I/O.
func ResolveOpenAIEndpoint(provider *typ.Provider, flags typ.RuleFlags, incoming IncomingAPIType) (*EndpointSelection, error) {
	if provider == nil {
		return nil, fmt.Errorf("provider is required for endpoint selection")
	}

	mode := provider.OpenAIEndpointMode

	switch ParseEndpointOverride(flags.OpenAIEndpointOverride) {
	case OverrideChat:
		if mode == ai.EndpointModeResponses {
			logModeOverrideIgnored(provider, "chat")
			return &EndpointSelection{
				Target: protocol.TypeOpenAIResponses,
				Reason: "provider mode=responses; rule override=chat ignored",
			}, nil
		}
		return &EndpointSelection{
			Target: protocol.TypeOpenAIChat,
			Reason: "rule override: openai_endpoint_override=chat",
		}, nil

	case OverrideResponses:
		if mode == ai.EndpointModeChat {
			logModeOverrideIgnored(provider, "responses")
			return &EndpointSelection{
				Target: protocol.TypeOpenAIChat,
				Reason: "provider mode=chat; rule override=responses ignored",
			}, nil
		}
		return &EndpointSelection{
			Target: protocol.TypeOpenAIResponses,
			Reason: "rule override: openai_endpoint_override=responses",
		}, nil
	}

	switch mode {
	case ai.EndpointModeResponses:
		return &EndpointSelection{
			Target: protocol.TypeOpenAIResponses,
			Reason: "provider mode=responses",
		}, nil
	case ai.EndpointModeBoth:
		return mirrorIncoming(incoming, "provider mode=both; mirroring incoming API"), nil
	default: // EndpointModeChat / zero value
		return &EndpointSelection{
			Target: protocol.TypeOpenAIChat,
			Reason: "provider mode=chat (default)",
		}, nil
	}
}
