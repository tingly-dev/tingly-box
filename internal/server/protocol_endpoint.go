package server

import (
	"fmt"

	"github.com/sirupsen/logrus"
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
// Rule override is honored unconditionally (per design intent). When an override
// conflicts with the provider's declared mode, a warning is logged but the override
// takes effect. This allows explicit routing control for debugging and special cases.
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

	// Rule override takes first priority (per design intent from .design/openai-endpoint-routing.md)
	// Log warning when override conflicts with provider's declared mode
	switch ParseEndpointOverride(flags.OpenAIEndpointOverride) {
	case OverrideChat:
		if mode == ai.EndpointModeResponses {
			logrus.Warnf("Rule forces chat endpoint on responses-only provider %s", provider.UUID)
		}
		return protocol.TypeOpenAIChat, nil

	case OverrideResponses:
		if mode == ai.EndpointModeChat {
			logrus.Warnf("Rule forces responses endpoint on chat-only provider %s", provider.UUID)
		}
		return protocol.TypeOpenAIResponses, nil
	}

	// Fall back to provider mode when no override specified
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

// EndpointOverride is the typed value of the openai_endpoint_override rule
// flag. It forces an OpenAI request onto a specific endpoint, overriding the
// provider's declared OpenAIEndpointMode default (provider declarations
// trump conflicting overrides — see ResolveOpenAIEndpoint).
type EndpointOverride string

const (
	OverrideAuto      EndpointOverride = "auto"
	OverrideChat      EndpointOverride = "chat"
	OverrideResponses EndpointOverride = "responses"
)

// ParseEndpointOverride coerces a raw rule-flag string to a known
// EndpointOverride. Empty, "auto" and any unrecognized value map to
// OverrideAuto so misconfigured rules degrade safely.
func ParseEndpointOverride(s string) EndpointOverride {
	switch s {
	case string(OverrideChat):
		return OverrideChat
	case string(OverrideResponses):
		return OverrideResponses
	default:
		return OverrideAuto
	}
}
