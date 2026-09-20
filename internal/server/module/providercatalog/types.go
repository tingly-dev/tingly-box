package providercatalog

import "github.com/tingly-dev/tingly-box/internal/catalog"

// ProviderCatalogResponse represents the response for provider template endpoints
type ProviderCatalogResponse struct {
	Success bool                                `json:"success"`
	Data    map[string]*catalog.ProviderCatalog `json:"data,omitempty"`
	Message string                              `json:"message,omitempty"`
	Version string                              `json:"version,omitempty"`
}

// SingleProviderCatalogResponse represents the response for a single template
type SingleProviderCatalogResponse struct {
	Success bool                     `json:"success"`
	Data    *catalog.ProviderCatalog `json:"data,omitempty"`
	Message string                   `json:"message,omitempty"`
}
