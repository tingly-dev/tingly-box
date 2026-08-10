package usecase

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ProviderUseCase implements Provider CRUD plus model-list resolution. It
// holds a *serverconfig.Config directly (not an AppManager — see
// .design/usecase-layer.md, "Construction").
type ProviderUseCase struct {
	cfg *serverconfig.Config
}

// NewProviderUseCase constructs a ProviderUseCase over the given config.
func NewProviderUseCase(cfg *serverconfig.Config) *ProviderUseCase {
	return &ProviderUseCase{cfg: cfg}
}

// ErrProviderNotFound means no provider exists for the given UUID.
type ErrProviderNotFound struct {
	UUID string
}

func (e ErrProviderNotFound) Error() string {
	return fmt.Sprintf("provider not found: %s", e.UUID)
}

// ListProvidersResult is the output of List.
type ListProvidersResult struct {
	Providers []*typ.Provider `json:"providers"`
}

// List returns every configured provider.
func (uc *ProviderUseCase) List() ListProvidersResult {
	return ListProvidersResult{Providers: uc.cfg.ListProviders()}
}

// GetProviderRequest identifies a provider by UUID.
type GetProviderRequest struct {
	UUID string `json:"uuid"`
}

// GetProviderResult is the output of Get.
type GetProviderResult struct {
	Provider *typ.Provider `json:"provider"`
}

// Get returns a provider by UUID, or ErrProviderNotFound.
func (uc *ProviderUseCase) Get(req GetProviderRequest) (GetProviderResult, error) {
	provider, err := uc.cfg.GetProviderByUUID(req.UUID)
	if err != nil || provider == nil {
		return GetProviderResult{}, ErrProviderNotFound{UUID: req.UUID}
	}
	return GetProviderResult{Provider: provider}, nil
}

// CreateProviderRequest is the input to Add.
//
// APIStyle is required — the CLI's name/URL keyword inference ("anthropic"
// in the name → APIStyleAnthropic, etc.) is a CLI-side interactive UX
// convenience, not business logic, and stays in internal/command per the
// no-inference decision recorded in .design/usecase-layer.md.
//
// ProxyURL is optional and included even though the CLI doesn't currently
// expose it — this DTO is the union of what any caller can set, not the
// intersection of what today's CLI happens to ask for; TUI already sets it
// via a separate UpdateProviderByUUID call today, which Add now makes
// unnecessary.
type CreateProviderRequest struct {
	Name     string            `json:"name"`
	APIBase  string            `json:"api_base"`
	Token    string            `json:"token"`
	APIStyle protocol.APIStyle `json:"api_style"`
	ProxyURL string            `json:"proxy_url,omitempty"`
}

// CreateProviderResult is the output of Add.
type CreateProviderResult struct {
	Provider *typ.Provider `json:"provider"`
}

// Add creates a new provider. Provider names are not unique — only UUIDs
// are — so this never rejects on name collision; a fresh UUID is minted
// every call.
func (uc *ProviderUseCase) Add(req CreateProviderRequest) (CreateProviderResult, error) {
	provider := &typ.Provider{
		Name:     req.Name,
		APIBase:  req.APIBase,
		APIStyle: req.APIStyle,
		AuthType: typ.AuthTypeAPIKey,
		Token:    req.Token,
		ProxyURL: req.ProxyURL,
		Enabled:  true,
	}
	if err := uc.cfg.AddProvider(provider); err != nil {
		return CreateProviderResult{}, err
	}
	return CreateProviderResult{Provider: provider}, nil
}

// UpdateProviderRequest replaces a provider's fields by UUID. There is no
// partial-update path today in either CLI or TUI.
//
// Token is the one exception to wholesale replace: an empty Token is ignored
// and the existing token is preserved. This matches the CLI's interactive
// "press Enter to keep current" behavior (the CLI reloads the stored token
// before calling Update when the user leaves the field blank). Clearing a
// token is intentionally unsupported.
type UpdateProviderRequest struct {
	UUID     string            `json:"uuid"`
	Name     string            `json:"name"`
	APIBase  string            `json:"api_base"`
	Token    string            `json:"token"`
	APIStyle protocol.APIStyle `json:"api_style"`
	ProxyURL string            `json:"proxy_url,omitempty"`
}

// UpdateProviderResult is the output of Update.
type UpdateProviderResult struct {
	Provider *typ.Provider `json:"provider"`
}

// Update replaces an existing provider's fields by UUID. Returns
// ErrProviderNotFound if the UUID doesn't match any provider.
func (uc *ProviderUseCase) Update(req UpdateProviderRequest) (UpdateProviderResult, error) {
	existing, err := uc.cfg.GetProviderByUUID(req.UUID)
	if err != nil || existing == nil {
		return UpdateProviderResult{}, ErrProviderNotFound{UUID: req.UUID}
	}

	updated := *existing
	updated.Name = req.Name
	updated.APIBase = req.APIBase
	updated.APIStyle = req.APIStyle
	updated.ProxyURL = req.ProxyURL
	if req.Token != "" {
		updated.Token = req.Token
	}

	if err := uc.cfg.UpdateProvider(req.UUID, &updated); err != nil {
		return UpdateProviderResult{}, err
	}
	return UpdateProviderResult{Provider: &updated}, nil
}

// DeleteProviderRequest identifies the provider to delete.
type DeleteProviderRequest struct {
	UUID string `json:"uuid"`
}

// Delete removes a provider by UUID. Confirmation (if any) and the "rules
// referencing it will be left dangling" warning are caller concerns.
func (uc *ProviderUseCase) Delete(req DeleteProviderRequest) error {
	return uc.cfg.DeleteProvider(req.UUID)
}

// RefreshModelsRequest identifies the provider whose model list should be
// re-fetched from its upstream API.
type RefreshModelsRequest struct {
	UUID string `json:"uuid"`
}

// RefreshModelsResult is the output of RefreshModels.
type RefreshModelsResult struct {
	Models []string `json:"models"`
}

// RefreshModels forces a re-resolution of the provider's model list,
// bypassing the cache (ResolveProviderModels(forceRefresh=true, ...)).
func (uc *ProviderUseCase) RefreshModels(req RefreshModelsRequest) (RefreshModelsResult, error) {
	resolved, err := uc.cfg.ResolveProviderModels(true, false, req.UUID)
	if err != nil {
		return RefreshModelsResult{}, err
	}
	return RefreshModelsResult{Models: resolved.Models}, nil
}

// AvailableModelsRequest identifies the provider whose model list should be
// resolved, preferring cache.
type AvailableModelsRequest struct {
	UUID string `json:"uuid"`
}

// AvailableModelsResult is the output of AvailableModels.
type AvailableModelsResult struct {
	Models []string `json:"models"`
	Source string   `json:"source"`
}

// AvailableModels resolves the provider's model list through the full
// cache → vmodel → API → template fallback chain
// (Config.ResolveProviderModels(forceRefresh=false, ...)). This deliberately
// replaces the TUI's own hand-written cache → template two-level shortcut
// (internal/command/tui/rule_mode.go's availableModels), which missed the
// vmodel level and so could not see virtual providers — see
// .design/usecase-layer.md, "Known behavioral differences not yet resolved".
func (uc *ProviderUseCase) AvailableModels(req AvailableModelsRequest) (AvailableModelsResult, error) {
	resolved, err := uc.cfg.ResolveProviderModels(false, false, req.UUID)
	if err != nil {
		return AvailableModelsResult{}, err
	}
	return AvailableModelsResult{Models: resolved.Models, Source: string(resolved.Source)}, nil
}
