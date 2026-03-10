package tbclient

import (
	"context"
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TBClient defines the interface for remote control interactions
type TBClient interface {
	// GetProviders returns all configured providers
	GetProviders(ctx context.Context) ([]ProviderInfo, error)

	// GetServices returns all services from routing rules
	GetServices(ctx context.Context) ([]ServiceInfo, error)

	// GetDefaultService returns the default service configuration
	// This reuses the ClaudeCode scenario's active service
	// Returns base URL (ClaudeCode scenario), API key, provider, and model
	GetDefaultService(ctx context.Context) (*DefaultServiceConfig, error)

	// GetConnectionConfig returns base URL and API key
	// Base URL defaults to ClaudeCode scenario URL if not configured
	GetConnectionConfig(ctx context.Context) (*ConnectionConfig, error)

	// SelectModel returns model configuration for @tb execution
	SelectModel(ctx context.Context, req ModelSelectionRequest) (*ModelConfig, error)
}

// TBClientImpl implements TBClient interface
type TBClientImpl struct {
	config         *config.AppConfig
	providerDB     *db.ProviderStore
	router         *smartrouting.Router
	serverHost     string
	serverPort     int
	defaultBaseURL string
}

// NewTBClient creates a new TB client instance
func NewTBClient(
	cfg *config.AppConfig,
	providerDB *db.ProviderStore,
	router *smartrouting.Router,
	serverHost string,
	serverPort int,
) *TBClientImpl {
	return &TBClientImpl{
		config:         cfg,
		providerDB:     providerDB,
		router:         router,
		serverHost:     serverHost,
		serverPort:     serverPort,
		defaultBaseURL: "http://localhost:12580/tingly/claude_code",
	}
}

// GetProviders returns all configured providers
func (c *TBClientImpl) GetProviders(ctx context.Context) ([]ProviderInfo, error) {
	providers := c.config.ListProviders()
	result := make([]ProviderInfo, 0, len(providers))

	for _, p := range providers {
		result = append(result, ProviderInfo{
			UUID:     p.UUID,
			Name:     p.Name,
			APIBase:  p.APIBase,
			APIStyle: string(p.APIStyle),
			Enabled:  p.Enabled,
			Models:   p.Models, // Include cached models if available
		})
	}

	return result, nil
}

// GetServices returns all services from routing rules
func (c *TBClientImpl) GetServices(ctx context.Context) ([]ServiceInfo, error) {
	// Access router's rules and extract services
	rules := c.router.GetRules()
	result := make([]ServiceInfo, 0)

	for _, rule := range rules {
		for _, svc := range rule.Services {
			result = append(result, ServiceInfo{
				ProviderID: svc.Provider,
				Model:      svc.Model,
			})
		}
	}

	return result, nil
}

// GetConnectionConfig returns base URL and API key
func (c *TBClientImpl) GetConnectionConfig(ctx context.Context) (*ConnectionConfig, error) {
	// For @tb, we use the ClaudeCode scenario URL as default
	// API key comes from the default or configured provider

	// Try to find an Anthropic provider
	providers, err := c.providerDB.ListEnabled()
	if err != nil {
		return nil, fmt.Errorf("failed to list providers: %w", err)
	}

	var apiKey string
	for _, p := range providers {
		if p.APIStyle == "anthropic" && p.Token != "" {
			apiKey = p.Token
			break
		}
	}

	// Build base URL from server config
	port := c.serverPort
	if port == 0 {
		port = 12580
	}
	baseURL := fmt.Sprintf("http://%s:%d/tingly/claude_code", c.serverHost, port)

	return &ConnectionConfig{
		BaseURL: baseURL,
		APIKey:  apiKey,
	}, nil
}

// GetDefaultService returns the default service configuration
// This reuses the ClaudeCode scenario's active service
func (c *TBClientImpl) GetDefaultService(ctx context.Context) (*DefaultServiceConfig, error) {
	// Get the first active ClaudeCode rule (same logic as ApplyClaudeConfig)
	globalConfig := c.config.GetGlobalConfig()
	rules := globalConfig.GetRequestConfigs()
	var firstRule *typ.Rule

	for i, rule := range rules {
		if rule.GetScenario() == typ.ScenarioClaudeCode && rule.Active {
			firstRule = &rules[i]
			break
		}
	}

	if firstRule == nil {
		return nil, fmt.Errorf("no active ClaudeCode rules found")
	}

	services := firstRule.GetServices()
	if len(services) == 0 {
		return nil, fmt.Errorf("no services configured in ClaudeCode rule")
	}

	firstService := services[0]
	provider, err := c.config.GetProviderByUUID(firstService.Provider)
	if err != nil || provider == nil {
		return nil, fmt.Errorf("provider not found for ClaudeCode rule: %s", firstService.Provider)
	}

	// Build base URL from server config
	port := c.serverPort
	if port == 0 {
		port = 12580
	}
	baseURL := fmt.Sprintf("http://%s:%d/tingly/claude_code", c.serverHost, port)

	return &DefaultServiceConfig{
		ProviderUUID: provider.UUID,
		ProviderName: provider.Name,
		ModelID:      firstService.Model,
		BaseURL:      baseURL,
		APIKey:       provider.GetAccessToken(),
		APIStyle:     string(provider.APIStyle),
	}, nil
}

// SelectModel returns model configuration for @tb execution
func (c *TBClientImpl) SelectModel(ctx context.Context, req ModelSelectionRequest) (*ModelConfig, error) {
	var provider *typ.Provider
	var modelID string

	// Strategy 1: Use service name from routing rules
	if req.ServiceName != "" {
		// Find service in rules and get its provider/model
		rules := c.router.GetRules()
		for _, rule := range rules {
			if rule.Description == req.ServiceName {
				if len(rule.Services) > 0 {
					svc := rule.Services[0]
					provider, err := c.config.GetProviderByUUID(svc.Provider)
					if err == nil && provider != nil {
						modelID = svc.Model
						return &ModelConfig{
							ProviderUUID: provider.UUID,
							ModelID:      modelID,
							BaseURL:      provider.APIBase,
							APIKey:       provider.GetAccessToken(),
							APIStyle:     string(provider.APIStyle),
						}, nil
					}
				}
			}
		}
		return nil, fmt.Errorf("service not found: %s", req.ServiceName)
	}

	// Strategy 2: Use provider UUID
	if provider == nil && req.ProviderUUID != "" {
		var err error
		provider, err = c.config.GetProviderByUUID(req.ProviderUUID)
		if err != nil || provider == nil {
			return nil, fmt.Errorf("provider not found: %s", req.ProviderUUID)
		}
		if req.ModelID != "" {
			modelID = req.ModelID
		} else {
			// Use first model from provider if available
			if len(provider.Models) > 0 {
				modelID = provider.Models[0]
			} else {
				modelID = "claude-sonnet-4-6" // Default fallback
			}
		}
	}

	// Strategy 3: Use first available Anthropic provider
	if provider == nil {
		providers, err := c.providerDB.ListEnabled()
		if err != nil {
			return nil, fmt.Errorf("failed to list providers: %w", err)
		}

		for _, p := range providers {
			if p.APIStyle == "anthropic" {
				provider = p
				modelID = req.ModelID
				if modelID == "" {
					// Use first model from provider if available
					if len(p.Models) > 0 {
						modelID = p.Models[0]
					} else {
						modelID = "claude-sonnet-4-6" // Default fallback
					}
				}
				break
			}
		}
	}

	if provider == nil {
		return nil, fmt.Errorf("no suitable provider found")
	}

	return &ModelConfig{
		ProviderUUID: provider.UUID,
		ModelID:      modelID,
		BaseURL:      provider.APIBase,
		APIKey:       provider.GetAccessToken(),
		APIStyle:     string(provider.APIStyle),
	}, nil
}
