package provider

import (
	"time"

	"github.com/tingly-dev/tingly-box/ai/quota"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ProviderResponse represents a provider configuration with masked token.
type ProviderResponse struct {
	UUID             string            `json:"uuid" example:"0123456789ABCDEF"`
	Name             string            `json:"name" example:"openai"`
	APIBase          string            `json:"api_base" example:"https://api.openai.com/v1"`
	APIStyle         string            `json:"api_style" example:"openai"`
	APIBaseOpenAI    string            `json:"api_base_openai,omitempty" example:"https://api.example.com/v1"`
	APIBaseAnthropic string            `json:"api_base_anthropic,omitempty" example:"https://api.example.com"`
	Token            string            `json:"token" example:"sk-***...***"` // Only populated for api_key auth type
	NoKeyRequired    bool              `json:"no_key_required" example:"false"`
	Enabled          bool              `json:"enabled" example:"true"`
	ProxyURL         string            `json:"proxy_url,omitempty" example:"http://localhost:7890"`
	AuthType         string            `json:"auth_type,omitempty" example:"api_key"` // api_key, oauth, vmodel, aws_sigv4, azure_key, gcp_sa
	OAuthDetail      *typ.OAuthDetail  `json:"oauth_detail,omitempty"`                // OAuth credentials (only for oauth auth type)
	VModelDetail     *typ.VModelDetail `json:"vmodel_detail,omitempty"`               // Virtual-model config (only for vmodel auth type)
	Credential       map[string]string `json:"credential,omitempty"`                  // Multi-field cloud credentials (only for aws_sigv4/azure_key/gcp_sa)
	Source           string            `json:"source,omitempty" example:"user"`       // "user" (default) or "builtin"
	// Flags carries the provider-level flags, ModelFlags the per-model
	// overrides keyed by provider-side model ID. See
	// GET /provider/flags/registry for the catalog. api_key providers only.
	Flags      *typ.ProviderFlags           `json:"flags,omitempty"`
	ModelFlags map[string]typ.ProviderFlags `json:"model_flags,omitempty"`
}

// ProvidersResponse represents the response for listing providers.
type ProvidersResponse struct {
	Success bool               `json:"success" example:"true"`
	Data    []ProviderResponse `json:"data"`
}

// CreateProviderRequest represents the request to add a new provider.
type CreateProviderRequest struct {
	Name             string `json:"name" binding:"required" description:"Provider name" example:"openai"`
	APIBase          string `json:"api_base" binding:"required" description:"API base URL" example:"https://api.openai.com/v1"`
	APIStyle         string `json:"api_style" description:"API style" example:"openai"`
	APIBaseOpenAI    string `json:"api_base_openai,omitempty" description:"Dual-mode OpenAI-compatible base URL (optional, api_key auth only)" example:"https://api.example.com/v1"`
	APIBaseAnthropic string `json:"api_base_anthropic,omitempty" description:"Dual-mode Anthropic-compatible base URL (optional, api_key auth only)" example:"https://api.example.com"`
	Token            string `json:"token" description:"API token" example:"sk-..."`
	NoKeyRequired    bool   `json:"no_key_required" description:"Whether provider requires no API key" example:"false"`
	Enabled          bool   `json:"enabled" description:"Whether provider is enabled" example:"true"`
	ProxyURL         string `json:"proxy_url,omitempty" description:"HTTP or SOCKS proxy URL (e.g., http://localhost:7890 or socks5://localhost:1080)" example:"http://localhost:7890"`
	AuthType         string `json:"auth_type,omitempty" description:"Auth type: api_key, oauth, aws_sigv4, azure_key, or gcp_sa (default: api_key)" example:"api_key"`
	// Credential carries multi-field cloud credentials for auth types
	// aws_sigv4 (AWS Bedrock), azure_key (Azure OpenAI), and gcp_sa (GCP Vertex).
	// Ignored for api_key/oauth/vmodel. See ai.CredentialSchema for the field keys.
	Credential map[string]string `json:"credential,omitempty" description:"Cloud credential fields (aws_sigv4/azure_key/gcp_sa)"`
	// Flags / ModelFlags optionally seed the provider-level flags and
	// per-model overrides (api_key auth only; validated on save).
	Flags      *typ.ProviderFlags           `json:"flags,omitempty" description:"Provider-level flags; see GET /provider/flags/registry (api_key auth only)"`
	ModelFlags map[string]typ.ProviderFlags `json:"model_flags,omitempty" description:"Per-model flag overrides keyed by provider-side model ID; api_key auth only"`
}

// CreateProviderResponse represents the response for adding a provider.
type CreateProviderResponse struct {
	Success bool        `json:"success" example:"true"`
	Message string      `json:"message" example:"Provider added successfully"`
	Data    interface{} `json:"data"`
}

// UpdateProviderRequest represents the request to update a provider.
type UpdateProviderRequest struct {
	Name             *string `json:"name,omitempty" description:"New provider name"`
	APIBase          *string `json:"api_base,omitempty" description:"New API base URL"`
	APIStyle         *string `json:"api_style,omitempty" description:"New API style"`
	APIBaseOpenAI    *string `json:"api_base_openai,omitempty" description:"New dual-mode OpenAI-compatible base URL (empty string clears it)"`
	APIBaseAnthropic *string `json:"api_base_anthropic,omitempty" description:"New dual-mode Anthropic-compatible base URL (empty string clears it)"`
	Token            *string `json:"token,omitempty" description:"New API token"`
	NoKeyRequired    *bool   `json:"no_key_required,omitempty" description:"Whether provider requires no API key"`
	Enabled          *bool   `json:"enabled,omitempty" description:"New enabled status"`
	ProxyURL         *string `json:"proxy_url,omitempty" description:"HTTP or SOCKS proxy URL"`
	// Credential replaces the full multi-field credential bundle for cloud auth
	// types (aws_sigv4/azure_key/gcp_sa). Omit (null) to leave it unchanged; the
	// edit UI resends the complete map since the read path returns it in full.
	Credential map[string]string `json:"credential,omitempty" description:"Replacement cloud credential fields (aws_sigv4/azure_key/gcp_sa)"`
	// Flags / ModelFlags follow the same partial semantics as the other
	// fields: omit (null) to leave the stored value unchanged; a non-null
	// value replaces the corresponding level wholesale (an empty object /
	// map clears it). No deep per-key merge — the edit UI saves whole
	// sections, and deep map patches are ambiguous.
	Flags      *typ.ProviderFlags            `json:"flags,omitempty" description:"Replacement provider-level flags; empty object clears them (api_key auth only)"`
	ModelFlags *map[string]typ.ProviderFlags `json:"model_flags,omitempty" description:"Replacement per-model flag overrides; empty map clears them (api_key auth only)"`
}

// UpdateProviderResponse represents the response for updating a provider.
type UpdateProviderResponse struct {
	Success bool             `json:"success" example:"true"`
	Message string           `json:"message" example:"Provider updated successfully"`
	Data    ProviderResponse `json:"data"`
}

// ToggleProviderResponse represents the response for toggling a provider.
type ToggleProviderResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Provider openai enabled successfully"`
	Data    struct {
		Enabled bool `json:"enabled" example:"true"`
	} `json:"data"`
}

// DeleteProviderResponse represents the response for deleting a provider.
type DeleteProviderResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Provider deleted successfully"`
}

// FlagRegistryResponse exposes the catalog of supported provider/model-level
// flags so the frontend can render the provider Plugins UI generically —
// the supply-side counterpart of the rule module's flag registry endpoint.
type FlagRegistryResponse struct {
	Success bool           `json:"success" example:"true"`
	Data    []typ.FlagSpec `json:"data"`
}

// ModelCacheSource identifies where the model list was sourced from.
type ModelCacheSource string

const (
	ModelCacheSourceDB       ModelCacheSource = "db"
	ModelCacheSourceAPI      ModelCacheSource = "api"
	ModelCacheSourceTemplate ModelCacheSource = "template"
	ModelCacheSourceVModel   ModelCacheSource = "vmodel"
)

// ProviderModelInfo represents model information for a specific provider.
type ProviderModelInfo struct {
	Models      []string             `json:"models" example:"gpt-3.5-turbo,gpt-4"`
	StarModels  []string             `json:"star_models" example:"gpt-4"`
	CustomModel []string             `json:"custom_model" example:"custom-gpt-model"`
	APIBase     string               `json:"api_base" example:"https://api.openai.com/v1"`
	LastUpdated string               `json:"last_updated,omitempty" example:"2024-01-15 10:30:00"`
	Source      ModelCacheSource     `json:"source,omitempty" example:"db"`
	ExpiresAt   time.Time            `json:"expiresAt,omitempty" example:"2024-01-15T11:30:00Z"`
	Quota       *quota.ProviderUsage `json:"quota,omitempty"`
}

// ProviderModelsResponse represents the response for getting provider models.
type ProviderModelsResponse struct {
	Success bool              `json:"success" example:"true"`
	Message string            `json:"message" example:"Provider models successfully"`
	Data    ProviderModelInfo `json:"data"`
}

// FetchProviderModelsResponse represents the response for fetching provider models.
type FetchProviderModelsResponse struct {
	Success bool        `json:"success" example:"true"`
	Message string      `json:"message" example:"Successfully fetched 150 models for provider openai"`
	Data    interface{} `json:"data"`
}

// ImportProvidersRequest represents a request to import providers from a
// base64/JSONL encoded export bundle (see internal/dataio).
type ImportProvidersRequest struct {
	Data string `json:"data" binding:"required" description:"Base64 encoded provider export data" example:"TGB64:1.0:..."`
	// OnProviderConflict specifies what to do when a provider already exists.
	// "use" - use existing provider, "skip" - skip this provider, "suffix" - create with suffixed name
	OnProviderConflict string `json:"on_provider_conflict" description:"How to handle provider conflicts" example:"use"`
}

// ImportProvidersResponse represents the response for importing providers.
type ImportProvidersResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Providers imported successfully"`
	Data    struct {
		ProvidersCreated int                  `json:"providers_created" example:"1"`
		ProvidersUsed    int                  `json:"providers_used" example:"0"`
		Providers        []ProviderImportInfo `json:"providers,omitempty"`
	} `json:"data"`
}

// ProviderImportInfo contains basic information about an imported or used provider.
type ProviderImportInfo struct {
	UUID   string `json:"uuid" example:"123e4567-e89b-12d3-a456-426614174000"`
	Name   string `json:"name" example:"openai"`
	Action string `json:"action" example:"created"` // "created", "used", "skipped"
}

// ExportProviderResponse represents the response for exporting a single provider.
type ExportProviderResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Provider exported successfully"`
	Data    struct {
		Format string `json:"format" example:"base64"`
		Data   string `json:"data" description:"Base64 or JSONL encoded provider export data" example:"TGB64:1.0:..."`
	} `json:"data"`
}
