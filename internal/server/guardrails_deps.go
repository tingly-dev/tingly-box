package server

import (
	"sync"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailsutils "github.com/tingly-dev/tingly-box/internal/guardrails/utils"
	"github.com/tingly-dev/tingly-box/internal/server/config"
)

// GuardrailsRuntime is the narrow slice of the root server's guardrails
// runtime state (internal/server.guardrails_runtime.go) that the admin
// surface needs: the current runtime snapshot, the ability to swap it after
// a config edit, and the small set of gating/derived helpers. Declared as an
// interface — rather than depending on *server.Server — to avoid an import
// cycle, since root server already imports this webui package.
type GuardrailsRuntime interface {
	CurrentGuardrailsRuntime() *guardrails.Guardrails
	SetGuardrailsRuntime(runtime *guardrails.Guardrails, context string)
	GetGuardrailsSupportedScenarios() []string
	RefreshGuardrailsCredentialCacheOrWarn(context string)
}

// GuardrailsDeps declares exactly what the guardrails admin handlers need
// from the host server.
type GuardrailsDeps struct {
	Config  *config.Config
	Runtime GuardrailsRuntime

	// GuardrailsConfigMu serializes config/policy/group file edits. It is the
	// SAME mutex instance as root server's Server.guardrailsConfigMu (passed
	// in by pointer) so admin edits and any other root-side writer are
	// mutually exclusive.
	GuardrailsConfigMu *sync.Mutex
}

// GuardrailsHandler is the aggregate handler for the guardrails admin surface
// (config editor, policy/group CRUD, protected credentials, registry
// install, history).
type GuardrailsHandler struct {
	deps GuardrailsDeps
}

// NewGuardrailsHandler constructs the guardrails admin handler.
func NewGuardrailsHandler(deps GuardrailsDeps) *GuardrailsHandler {
	return &GuardrailsHandler{deps: deps}
}

func (h *GuardrailsHandler) credentialStore() (*guardrailsutils.ProtectedCredentialStore, error) {
	return config.CredentialStore(h.deps.Config.ConfigDir)
}
