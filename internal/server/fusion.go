package server

import (
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// resolveProviderForClient returns a shallow copy of the provider whose
// APIBase and APIStyle reflect the best-fit endpoint for the inbound
// clientStyle.
//
// Fusion-mode providers (api_key auth + APIBaseOpenAI/APIBaseAnthropic set)
// expose two distinct base URLs for the same credentials. When the inbound
// client protocol matches a configured fusion URL, that URL is used directly
// — no protocol translation is needed and the request is passed through
// natively. When no fusion URL matches the inbound style, the legacy
// APIBase/APIStyle pair is preserved so single-protocol providers behave
// exactly as before.
//
// Downstream HTTP clients and protocol-transform code read APIBase and
// APIStyle off of the provider; returning a shallow copy keeps that contract
// without requiring changes to every client constructor.
func (s *Server) resolveProviderForClient(p *typ.Provider, clientStyle protocol.APIStyle) *typ.Provider {
	if p == nil {
		return nil
	}
	baseURL, style := p.ResolveEndpoint(clientStyle)
	if baseURL == p.APIBase && style == p.APIStyle {
		return p
	}
	clone := *p
	clone.APIBase = baseURL
	clone.APIStyle = style
	return &clone
}
