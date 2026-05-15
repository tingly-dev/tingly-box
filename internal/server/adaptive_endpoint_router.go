package server

import (
	"context"
	"fmt"

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type IncomingAPIType string

const (
	IncomingAPIChat      IncomingAPIType = "chat"
	IncomingAPIResponses IncomingAPIType = "responses"
)

type EndpointSelection struct {
	Target protocol.APIType
	Reason string
}

func (s *Server) SelectOpenAIEndpoint(ctx context.Context, provider *typ.Provider, modelID string, incoming IncomingAPIType, isStreaming bool, responsesReq *protocol.ResponseCreateRequest) (*EndpointSelection, error) {
	if provider == nil {
		return nil, fmt.Errorf("provider is required for endpoint selection")
	}

	if isCodexProvider(provider) {
		return &EndpointSelection{Target: protocol.TypeOpenAIResponses, Reason: "Codex provider supports Responses API only"}, nil
	}

	capability, err := NewAdaptiveProbe(s).GetModelCapability(provider.UUID, modelID)
	if err != nil || capability == nil {
		// Unknown capability: respect the incoming API instead of blocking the request on probe.
		go func() {
			probeCtx, cancel := context.WithTimeout(context.Background(), DefaultProbeTimeout)
			defer cancel()
			_, _ = NewAdaptiveProbe(s).ProbeModelEndpoints(probeCtx, ModelProbeRequest{ProviderUUID: provider.UUID, ModelID: modelID})
		}()
		return defaultEndpointSelection(incoming, "no cached capability; respecting incoming API"), nil
	}

	switch incoming {
	case IncomingAPIChat:
		if endpointUsable(capability.SupportsChat, capability.ChatSupportsStream, isStreaming) {
			return &EndpointSelection{Target: protocol.TypeOpenAIChat, Reason: "chat endpoint supports incoming chat request"}, nil
		}
		if endpointUsable(capability.SupportsResponses, capability.ResponsesSupportsStream, isStreaming) {
			return &EndpointSelection{Target: protocol.TypeOpenAIResponses, Reason: "chat endpoint unavailable; responses endpoint is usable"}, nil
		}
		return defaultEndpointSelection(IncomingAPIChat, "no usable probed endpoint for chat request; falling back to chat"), nil

	case IncomingAPIResponses:
		if endpointUsable(capability.SupportsResponses, capability.ResponsesSupportsStream, isStreaming) {
			return &EndpointSelection{Target: protocol.TypeOpenAIResponses, Reason: "responses endpoint supports incoming responses request"}, nil
		}
		if endpointUsable(capability.SupportsChat, capability.ChatSupportsStream, isStreaming) {
			if responsesReq != nil {
				if ok, reason := CanDowngradeResponsesToChat(*responsesReq); !ok {
					return nil, fmt.Errorf("responses endpoint unavailable and request cannot be safely downgraded to chat: %s", reason)
				}
			}
			return &EndpointSelection{Target: protocol.TypeOpenAIChat, Reason: "responses endpoint unavailable; chat endpoint is usable and downgrade is safe"}, nil
		}
		return defaultEndpointSelection(IncomingAPIResponses, "no usable probed endpoint for responses request; falling back to responses"), nil

	default:
		return nil, fmt.Errorf("unsupported incoming API type: %s", incoming)
	}
}

func defaultEndpointSelection(incoming IncomingAPIType, reason string) *EndpointSelection {
	if incoming == IncomingAPIResponses {
		return &EndpointSelection{Target: protocol.TypeOpenAIResponses, Reason: reason}
	}
	return &EndpointSelection{Target: protocol.TypeOpenAIChat, Reason: reason}
}

func endpointUsable(available, supportsStream, isStreaming bool) bool {
	if !available {
		return false
	}
	if isStreaming {
		return supportsStream
	}
	return true
}

func isCodexProvider(provider *typ.Provider) bool {
	return provider != nil && provider.OAuthDetail != nil && provider.OAuthDetail.GetIssuer() == ai.IssuerCodex
}

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
