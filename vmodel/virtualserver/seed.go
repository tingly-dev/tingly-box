package virtualserver

import (
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Sentinel UUIDs and names for builtin virtual-model providers. Stable across
// restarts so seeding is idempotent.
const (
	BuiltinAnthropicUUID = "vmodel-builtin-anthropic"
	BuiltinOpenAIUUID    = "vmodel-builtin-openai"

	BuiltinAnthropicName = "Virtual Models (Anthropic)"
	BuiltinOpenAIName    = "Virtual Models (OpenAI)"

	// vmodelAPIBaseSentinel is a placeholder URL stored on builtin providers.
	// The dispatcher short-circuits to the in-process handler before this URL
	// would ever be dialed; it exists only to satisfy the non-empty-base
	// invariant enforced elsewhere by the provider CRUD layer.
	vmodelAPIBaseSentinel = "vmodel://local"
)

// BuildBuiltinProviders returns the list of builtin virtual-model providers
// that should be present in the ProviderStore. The list is built from the
// service's currently registered models so the seed always reflects the
// in-process registries.
func (s *Service) BuildBuiltinProviders() []*typ.Provider {
	anthropicModels := make([]string, 0)
	for _, m := range s.anthropicReg.ListModels() {
		anthropicModels = append(anthropicModels, m.ID)
	}

	openaiModels := make([]string, 0)
	for _, m := range s.openaiReg.ListModels() {
		openaiModels = append(openaiModels, m.ID)
	}

	return []*typ.Provider{
		{
			UUID:     BuiltinAnthropicUUID,
			Name:     BuiltinAnthropicName,
			APIBase:  vmodelAPIBaseSentinel,
			APIStyle: protocol.APIStyleAnthropic,
			AuthType: typ.AuthTypeVirtual,
			Source:   typ.ProviderSourceBuiltin,
			Enabled:  true,
			VModelDetail: &typ.VModelDetail{
				Models: anthropicModels,
			},
		},
		{
			UUID:     BuiltinOpenAIUUID,
			Name:     BuiltinOpenAIName,
			APIBase:  vmodelAPIBaseSentinel,
			APIStyle: protocol.APIStyleOpenAI,
			AuthType: typ.AuthTypeVirtual,
			Source:   typ.ProviderSourceBuiltin,
			Enabled:  true,
			VModelDetail: &typ.VModelDetail{
				Models: openaiModels,
			},
		},
	}
}

// ProviderSaver is the minimal ProviderStore surface needed for seeding.
// Using an interface lets callers (server.go) pass the live *db.ProviderStore
// without virtualserver depending on the db package.
type ProviderSaver interface {
	GetByUUID(uuid string) (*typ.Provider, error)
	Save(provider *typ.Provider) error
}

// EnsureBuiltinProviders inserts or refreshes the builtin virtual-model
// providers in the given store. It is idempotent and safe to call on every
// startup:
//   - If a builtin provider is missing it is created (Enabled=true).
//   - If a builtin provider already exists its Enabled flag is preserved
//     (users may have disabled it) while the model list is refreshed to match
//     what is currently registered.
func (s *Service) EnsureBuiltinProviders(store ProviderSaver) error {
	for _, p := range s.BuildBuiltinProviders() {
		existing, err := store.GetByUUID(p.UUID)
		if err == nil && existing != nil {
			p.Enabled = existing.Enabled
		}
		if err := store.Save(p); err != nil {
			return err
		}
	}
	return nil
}
