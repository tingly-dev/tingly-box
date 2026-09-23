package protocolserver

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
// override first, then a per-model table entry, then the provider's declared
// OpenAIEndpointMode.
//
// Precedence:
//
//  1. Rule flag (flags.OpenAIEndpointOverride). Overrides everything below.
//  2. modelOverride: a per-model table entry (data.ModelInfo.OpenAIEndpoints)
//     for relays whose catalog mixes vendors, which provider.OpenAIEndpointMode
//     can't express (one value per provider). ai.EndpointModeUnknown means no
//     entry for this model. The caller looks it up (data.TemplateManager) and
//     passes it in, keeping this function pure.
//  3. provider.OpenAIEndpointMode.
//
// Both #2 and #3 resolve through the same rule (see resolveEndpointMode):
//
//	EndpointModeUnknown / zero value → Chat
//	EndpointModeChat                 → Chat
//	EndpointModeResponses            → Responses
//	EndpointModeBoth                 → mirror incoming
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
func ResolveOpenAIEndpoint(provider *typ.Provider, flags typ.RuleFlags, incoming IncomingAPIType, modelOverride ai.OpenAIEndpointMode) (protocol.APIType, error) {
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

	// Per-model table entry (see precedence #2 above).
	if modelOverride != ai.EndpointModeUnknown {
		return resolveEndpointMode(modelOverride, incoming), nil
	}

	// Fall back to provider mode when no override specified
	return resolveEndpointMode(mode, incoming), nil
}

// resolveEndpointMode maps a declared OpenAIEndpointMode — either the
// provider's own or a per-model table entry — plus the incoming protocol to a
// concrete upstream endpoint. Shared by both layers so "both means mirror
// incoming" is defined exactly once.
func resolveEndpointMode(mode ai.OpenAIEndpointMode, incoming IncomingAPIType) protocol.APIType {
	switch mode {
	case ai.EndpointModeResponses:
		return protocol.TypeOpenAIResponses
	case ai.EndpointModeBoth:
		if incoming == IncomingAPIResponses {
			return protocol.TypeOpenAIResponses
		}
		return protocol.TypeOpenAIChat
	default: // EndpointModeChat / EndpointModeUnknown
		return protocol.TypeOpenAIChat
	}
}

// EndpointOverride is the typed value of the openai_endpoint_override rule
// flag. It forces an OpenAI request onto a specific endpoint, overriding the
// model Catalog entry and the provider's declared OpenAIEndpointMode default.
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
