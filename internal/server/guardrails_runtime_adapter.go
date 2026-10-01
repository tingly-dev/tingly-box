package server

import "github.com/tingly-dev/tingly-box/internal/guardrails"

// The exported methods below adapt the protocolserver-owned GuardrailsState to
// the GuardrailsRuntime interface the guardrails admin handler consumes, so the
// handler can drive the runtime without depending on *Server. Root lifecycle
// code (server.go, server_flags.go, server_options.go) talks to
// s.guardrailsState directly.

// CurrentGuardrailsRuntime returns the active guardrails runtime snapshot.
func (s *Server) CurrentGuardrailsRuntime() *guardrails.Guardrails {
	return s.guardrailsState.Current()
}

// SetGuardrailsRuntime swaps in a new guardrails runtime, preserving history
// and credential-cache state carried over from the previous runtime.
func (s *Server) SetGuardrailsRuntime(runtime *guardrails.Guardrails, context string) {
	s.guardrailsState.Set(runtime, context)
}

// GetGuardrailsSupportedScenarios returns the scenarios guardrails can gate.
func (s *Server) GetGuardrailsSupportedScenarios() []string {
	return s.guardrailsState.SupportedScenarios()
}

// RefreshGuardrailsCredentialCacheOrWarn rebuilds the protected-credential
// cache, logging (rather than returning) any failure.
func (s *Server) RefreshGuardrailsCredentialCacheOrWarn(context string) {
	s.guardrailsState.RefreshCredentialCacheOrWarn(context)
}
