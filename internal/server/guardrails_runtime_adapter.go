package server

import "github.com/tingly-dev/tingly-box/internal/guardrails"

// The exported methods below adapt *Server's unexported guardrails-runtime
// accessors (guardrails_runtime.go) to the webui.GuardrailsRuntime interface,
// so the WebUI Management API's guardrails admin handler
// (internal/server/webui.GuardrailsHandler) can drive the runtime without
// root server depending on webui's types.

// CurrentGuardrailsRuntime returns the active guardrails runtime snapshot.
func (s *Server) CurrentGuardrailsRuntime() *guardrails.Guardrails {
	return s.currentGuardrailsRuntime()
}

// SetGuardrailsRuntime swaps in a new guardrails runtime, preserving history
// and credential-cache state carried over from the previous runtime.
func (s *Server) SetGuardrailsRuntime(runtime *guardrails.Guardrails, context string) {
	s.setGuardrailsRuntime(runtime, context)
}

// GetGuardrailsSupportedScenarios returns the scenarios guardrails can gate.
func (s *Server) GetGuardrailsSupportedScenarios() []string {
	return s.getGuardrailsSupportedScenarios()
}

// RefreshGuardrailsCredentialCacheOrWarn rebuilds the protected-credential
// cache, logging (rather than returning) any failure.
func (s *Server) RefreshGuardrailsCredentialCacheOrWarn(context string) {
	s.refreshGuardrailsCredentialCacheOrWarn(context)
}
