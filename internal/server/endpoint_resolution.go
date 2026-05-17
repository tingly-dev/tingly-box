package server

import (
	"fmt"

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// IncomingAPIType describes which OpenAI-style endpoint the client originally
// hit on this gateway. Used by the endpoint resolver to mirror the incoming
// API when no override or provider declaration forces otherwise.
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

// OpenAIEndpointOptions bundles the per-call inputs to ResolveOpenAIEndpoint
// that aren't already on Provider or RuleFlags.
type OpenAIEndpointOptions struct {
	Incoming IncomingAPIType
	// RequireResponses indicates the incoming Responses request uses features
	// (previous_response_id / include / background / truncation / reasoning)
	// that cannot be represented in Chat Completions. When set, the resolver
	// refuses to honor a "chat" override and returns an error.
	RequireResponses bool
}

func defaultEndpointSelection(incoming IncomingAPIType, reason string) *EndpointSelection {
	if incoming == IncomingAPIResponses {
		return &EndpointSelection{Target: protocol.TypeOpenAIResponses, Reason: reason}
	}
	return &EndpointSelection{Target: protocol.TypeOpenAIChat, Reason: reason}
}

// CanDowngradeResponsesToChat reports whether a Responses request can be
// safely emitted as Chat Completions instead. Used by the openai_responses
// handler to compute OpenAIEndpointOptions.RequireResponses.
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

// ResolveOpenAIEndpoint picks an OpenAI endpoint (Chat vs Responses) using
// only declared configuration, in this precedence order:
//
//  1. Rule flag (flags.OpenAIEndpointOverride):
//     - "chat" forces Chat. Refused with error when opts.RequireResponses=true.
//     If the provider is ResponsesOnly, the override is logged and ignored
//     (the provider's declaration wins).
//     - "responses" forces Responses.
//  2. provider.ResponsesOnly=true → Responses.
//  3. Default → mirror opts.Incoming (chat → Chat, responses → Responses).
//
// The function is pure: no Server state, no probe lookups, no I/O.
func ResolveOpenAIEndpoint(provider *typ.Provider, flags typ.RuleFlags, opts OpenAIEndpointOptions) (*EndpointSelection, error) {
	if provider == nil {
		return nil, fmt.Errorf("provider is required for endpoint selection")
	}

	switch ParseEndpointOverride(flags.OpenAIEndpointOverride) {
	case OverrideChat:
		if provider.ResponsesOnly {
			logResponsesOnlyOverrideIgnored(provider)
			return &EndpointSelection{
				Target: protocol.TypeOpenAIResponses,
				Reason: "provider declared responses_only; rule override=chat ignored",
			}, nil
		}
		if opts.RequireResponses {
			return nil, fmt.Errorf("rule override requests Chat Completions but the incoming Responses request uses features that cannot be downgraded")
		}
		return &EndpointSelection{
			Target: protocol.TypeOpenAIChat,
			Reason: "rule override: openai_endpoint_override=chat",
		}, nil

	case OverrideResponses:
		return &EndpointSelection{
			Target: protocol.TypeOpenAIResponses,
			Reason: "rule override: openai_endpoint_override=responses",
		}, nil
	}

	if provider.ResponsesOnly {
		return &EndpointSelection{
			Target: protocol.TypeOpenAIResponses,
			Reason: "provider declared responses_only",
		}, nil
	}

	return defaultEndpointSelection(opts.Incoming, "mirroring incoming API"), nil
}
