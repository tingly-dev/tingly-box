package server

import (
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// EndpointOverride is the typed value of the openai_endpoint_override rule
// flag. It forces an OpenAI request onto a specific endpoint, bypassing the
// adaptive router's capability probe.
type EndpointOverride string

const (
	// OverrideAuto preserves the adaptive router's probe-based decision.
	OverrideAuto EndpointOverride = "auto"
	// OverrideChat forces selection of the Chat Completions endpoint.
	OverrideChat EndpointOverride = "chat"
	// OverrideResponses forces selection of the Responses API endpoint.
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

// isCodexProvider reports whether the provider uses the ChatGPT Codex backend,
// which only exposes the Responses endpoint.
func isCodexProvider(provider *typ.Provider) bool {
	return provider != nil && provider.APIBase == protocol.CodexAPIBase
}

// ResolveOpenAIEndpoint returns the endpoint ("chat" or "responses") to use
// for an OpenAI-style request, honoring an optional rule-level override.
//
// Resolution order:
//  1. Codex providers always return "responses". A "chat" override is logged
//     and ignored because Codex has no Chat endpoint.
//  2. A non-auto override returns the forced endpoint directly, skipping the
//     capability probe.
//  3. Otherwise, defer to the adaptive probe via GetPreferredEndpointForModel.
func (s *Server) ResolveOpenAIEndpoint(provider *typ.Provider, modelID string, override EndpointOverride) string {
	if isCodexProvider(provider) {
		if override == OverrideChat {
			uuid := ""
			if provider != nil {
				uuid = provider.UUID
			}
			logrus.Warnf("rule openai_endpoint_override=chat ignored: provider %s is Codex (Chat unsupported)", uuid)
		}
		return string(db.EndpointTypeResponses)
	}

	switch override {
	case OverrideChat:
		return string(db.EndpointTypeChat)
	case OverrideResponses:
		return string(db.EndpointTypeResponses)
	}

	return s.GetPreferredEndpointForModel(provider, modelID)
}
