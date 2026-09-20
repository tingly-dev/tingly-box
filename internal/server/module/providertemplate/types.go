package providertemplate

import "github.com/tingly-dev/tingly-box/internal/catalog"

// TemplateResponse represents the response for provider template endpoints
type TemplateResponse struct {
	Success bool                              `json:"success"`
	Data    map[string]*catalog.ProviderTemplate `json:"data,omitempty"`
	Message string                            `json:"message,omitempty"`
	Version string                            `json:"version,omitempty"`
}

// SingleTemplateResponse represents the response for a single template
type SingleTemplateResponse struct {
	Success bool                   `json:"success"`
	Data    *catalog.ProviderTemplate `json:"data,omitempty"`
	Message string                 `json:"message,omitempty"`
}
