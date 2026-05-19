package command

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/config"
	exportpkg "github.com/tingly-dev/tingly-box/internal/dataio"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server"
	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// AppManager manages all application state and operations.
// It serves as the single source of truth for business logic that can be
// used by both CLI (cobra commands) and GUI (Wails services).
type AppManager struct {
	appConfig     *config.AppConfig
	serverManager *ServerManager
}

// NewAppManager creates a new AppManager with the given config directory.
func NewAppManager(configDir string) (*AppManager, error) {
	appConfig, err := config.NewAppConfig(config.WithConfigDir(configDir))
	if err != nil {
		return nil, fmt.Errorf("failed to create app config: %w", err)
	}

	return &AppManager{
		appConfig: appConfig,
	}, nil
}

// NewAppManagerWithConfig creates a new AppManager with an existing AppConfig.
func NewAppManagerWithConfig(appConfig *config.AppConfig) *AppManager {
	return &AppManager{
		appConfig: appConfig,
	}
}

// AppConfig returns the underlying AppConfig.
func (am *AppManager) AppConfig() *config.AppConfig {
	return am.appConfig
}

// SaveConfig saves the current configuration to disk.
func (am *AppManager) SaveConfig() error {
	return am.appConfig.Save()
}

// GetGlobalConfig returns the global configuration manager.
func (am *AppManager) GetGlobalConfig() *serverconfig.Config {
	return am.appConfig.GetGlobalConfig()
}

// FetchAndSaveProviderModels fetches models from a provider and saves them.
func (am *AppManager) FetchAndSaveProviderModels(providerUUID string) error {
	return am.appConfig.FetchAndSaveProviderModels(providerUUID)
}

// ============
// Server Management
// ============

// SetupServer initializes the server manager with the given port and options.
func (am *AppManager) SetupServer(port int, opts ...server.ServerOption) error {
	am.serverManager = NewServerManager(am.appConfig, opts...)
	return am.serverManager.Setup(port)
}

// SetupServerWithPort initializes the server manager with just a port (no options).
// This is a convenience method for the TUI wizard.
func (am *AppManager) SetupServerWithPort(port int) error {
	return am.SetupServer(port)
}

// GetServerManager returns the server manager instance.
func (am *AppManager) GetServerManager() *ServerManager {
	return am.serverManager
}

// StartServer starts the server if it has been set up.
func (am *AppManager) StartServer() error {
	if am.serverManager == nil {
		return fmt.Errorf("server manager not initialized - call SetupServer first")
	}
	return am.serverManager.Start()
}

// ============
// Provider Management
// ============

// AddProvider adds a new AI provider with the given configuration.
// Note: Provider name is not used as a unique identifier - multiple providers
// can have the same name. The system automatically generates a unique UUID for each.
// Returns the UUID of the newly created provider.
func (am *AppManager) AddProvider(name, apiBase, token string, apiStyle protocol.APIStyle) (string, error) {
	// Create provider with API style set from the start
	provider := &typ.Provider{
		Name:     name,
		APIBase:  apiBase,
		APIStyle: apiStyle,
		AuthType: typ.AuthTypeAPIKey,
		Token:    token,
		Enabled:  true,
	}

	// Add the provider (UUID will be generated automatically)
	if err := am.appConfig.AddProvider(provider); err != nil {
		return "", fmt.Errorf("failed to add provider: %w", err)
	}

	// Save the configuration
	if err := am.appConfig.Save(); err != nil {
		return "", fmt.Errorf("failed to save configuration: %w", err)
	}

	return provider.UUID, nil
}

// DeleteProvider removes an AI provider by name.
func (am *AppManager) DeleteProvider(name string) error {
	if err := am.appConfig.DeleteProvider(name); err != nil {
		return fmt.Errorf("failed to delete provider: %w", err)
	}
	return nil
}

// DeleteProviderByUUID removes an AI provider by UUID.
func (am *AppManager) DeleteProviderByUUID(uuid string) error {
	globalConfig := am.appConfig.GetGlobalConfig()
	if err := globalConfig.DeleteProvider(uuid); err != nil {
		return fmt.Errorf("failed to delete provider: %w", err)
	}
	return nil
}

// UpdateProviderByUUID updates an existing provider by UUID.
func (am *AppManager) UpdateProviderByUUID(uuid string, provider *typ.Provider) error {
	globalConfig := am.appConfig.GetGlobalConfig()
	if err := globalConfig.UpdateProvider(uuid, provider); err != nil {
		return fmt.Errorf("failed to update provider: %w", err)
	}
	return nil
}

// ListProviders returns all configured providers.
func (am *AppManager) ListProviders() []*typ.Provider {
	return am.appConfig.ListProviders()
}

// GetProviderByUUID returns a provider by UUID (implements quota.ProviderManager interface)
func (am *AppManager) GetProviderByUUID(uuid string) (*typ.Provider, error) {
	return am.GetProvider(uuid)
}

// GetProvider returns a provider by UUID, or nil if not found.
func (am *AppManager) GetProvider(uuid string) (*typ.Provider, error) {
	return am.appConfig.GetProviderByUUID(uuid)
}

// GetProviderByName returns a provider by name, or nil if not found.
func (am *AppManager) GetProviderByName(name string) (*typ.Provider, error) {
	return am.appConfig.GetProviderByName(name)
}

// ============
// Rule Management
// ============

// AddRule adds a new routing rule.
func (am *AppManager) AddRule(rule typ.Rule) error {
	globalConfig := am.appConfig.GetGlobalConfig()
	if err := globalConfig.AddRule(rule); err != nil {
		return fmt.Errorf("failed to add rule: %w", err)
	}
	return nil
}

// ListRules returns all configured rules.
func (am *AppManager) ListRules() []typ.Rule {
	globalConfig := am.appConfig.GetGlobalConfig()
	return globalConfig.Rules
}

// GetRuleByRequestModelAndScenario returns a rule by request model and scenario.
func (am *AppManager) GetRuleByRequestModelAndScenario(requestModel string, scenario typ.RuleScenario) *typ.Rule {
	globalConfig := am.appConfig.GetGlobalConfig()
	return globalConfig.GetRuleByRequestModelAndScenario(requestModel, scenario)
}

// UpdateRule updates an existing rule by UUID.
func (am *AppManager) UpdateRule(uuid string, rule typ.Rule) error {
	globalConfig := am.appConfig.GetGlobalConfig()
	if err := globalConfig.UpdateRule(uuid, rule); err != nil {
		return fmt.Errorf("failed to update rule: %w", err)
	}
	return nil
}

// DeleteRule removes a rule by UUID.
func (am *AppManager) DeleteRule(uuid string) error {
	globalConfig := am.appConfig.GetGlobalConfig()
	if err := globalConfig.DeleteRule(uuid); err != nil {
		return fmt.Errorf("failed to delete rule: %w", err)
	}
	return nil
}

// GetRuleByUUID returns the rule for the given UUID, or nil if not found.
func (am *AppManager) GetRuleByUUID(uuid string) *typ.Rule {
	globalConfig := am.appConfig.GetGlobalConfig()
	return globalConfig.GetRuleByUUID(uuid)
}

// ============
// Configuration Accessors
// ============

// GetServerPort returns the configured server port.
func (am *AppManager) GetServerPort() int {
	return am.appConfig.GetServerPort()
}

// SetServerPort sets the server port.
func (am *AppManager) SetServerPort(port int) error {
	return am.appConfig.SetServerPort(port)
}

// GetUserToken returns the user authentication token.
func (am *AppManager) GetUserToken() string {
	return am.appConfig.GetGlobalConfig().GetUserToken()
}

// GetModelToken returns the model API token.
func (am *AppManager) GetModelToken() string {
	return am.appConfig.GetGlobalConfig().GetModelToken()
}

// HasModelToken returns true if a model token is configured.
func (am *AppManager) HasModelToken() bool {
	return am.appConfig.GetGlobalConfig().HasModelToken()
}

// ============
// Import/Export Types
// ============

// ImportOptions controls how imports are handled when conflicts occur.
type ImportOptions struct {
	// OnProviderConflict specifies what to do when a provider already exists.
	// "use" - use existing provider, "skip" - skip this provider, "suffix" - create with suffixed name
	OnProviderConflict string
	// OnRuleConflict specifies what to do when a rule already exists.
	// "skip" - skip import, "update" - update existing rule, "new" - create with new name
	OnRuleConflict string
	// Quiet suppresses progress output
	Quiet bool
}

// ProviderImportInfo contains information about an imported or used provider
type ProviderImportInfo struct {
	UUID   string
	Name   string
	Action string // "created", "used", "skipped"
}

// ImportResult contains the results of an import operation.
type ImportResult struct {
	RuleCreated      bool
	RuleUpdated      bool
	ProvidersCreated int
	ProvidersUsed    int
	Providers        []ProviderImportInfo
	ProviderMap      map[string]string // old UUID -> new UUID
}

// ImportRuleFromJSONL imports a rule from JSONL format (either file content or stdin format).
// The data should be line-delimited JSON with:
// - Line 1: metadata (type="metadata")
// - Line 2: rule data (type="rule")
// - Subsequent lines: provider data (type="provider")
//
// Deprecated: Use ImportRule with FormatAuto instead. This method is kept for backward compatibility.
func (am *AppManager) ImportRuleFromJSONL(data string, opts ImportOptions) (*ImportResult, error) {
	// Convert command.ImportOptions to dataio.ImportOptions
	importOpts := exportpkg.ImportOptions{
		OnProviderConflict: opts.OnProviderConflict,
		OnRuleConflict:     opts.OnRuleConflict,
		Quiet:              opts.Quiet,
	}

	// Use dataio.Import with FormatAuto (will detect JSONL automatically)
	result, err := exportpkg.Import(data, am.appConfig.GetGlobalConfig(), exportpkg.FormatAuto, importOpts)
	if err != nil {
		return nil, err
	}

	// Convert dataio.ImportResult to command.ImportResult
	return &ImportResult{
		RuleCreated:      result.RuleCreated,
		RuleUpdated:      result.RuleUpdated,
		ProvidersCreated: result.ProvidersCreated,
		ProvidersUsed:    result.ProvidersUsed,
		Providers:        convertProviderInfoList(result.Providers),
		ProviderMap:      result.ProviderMap,
	}, nil
}

// convertProviderInfoList converts dataio.ProviderImportInfo to command.ProviderImportInfo
func convertProviderInfoList(dataioList []exportpkg.ProviderImportInfo) []ProviderImportInfo {
	result := make([]ProviderImportInfo, len(dataioList))
	for i, p := range dataioList {
		result[i] = ProviderImportInfo{
			UUID:   p.UUID,
			Name:   p.Name,
			Action: p.Action,
		}
	}
	return result
}

// ============
// Export
// ============

// CollectProvidersFromRule collects all providers referenced by the rule's services.
// This is a helper function for gathering providers to export with a rule.
func (am *AppManager) CollectProvidersFromRule(rule *typ.Rule) ([]*typ.Provider, error) {
	globalConfig := am.appConfig.GetGlobalConfig()

	providerUUIDs := am.getProviderUUIDsFromRule(rule)
	providers := make([]*typ.Provider, 0, len(providerUUIDs))

	for _, providerUUID := range providerUUIDs {
		provider, err := globalConfig.GetProviderByUUID(providerUUID)
		if err == nil && provider != nil {
			providers = append(providers, provider)
		}
	}

	return providers, nil
}

// ExportRule exports a rule with its providers, or providers only, in the specified format.
// At least one of rule or providers must be specified.
func (am *AppManager) ExportRule(rule *typ.Rule, providers []*typ.Provider, format exportpkg.Format) (string, error) {
	if rule == nil && len(providers) == 0 {
		return "", fmt.Errorf("either rule or providers must be specified for export")
	}

	// Build export request
	req := &exportpkg.ExportRequest{
		Rule:      rule,
		Providers: providers,
	}

	// Perform export
	result, err := exportpkg.Export(req, format)
	if err != nil {
		return "", fmt.Errorf("failed to export: %w", err)
	}

	return result.Content, nil
}

// getProviderUUIDsFromRule extracts all provider UUIDs from the rule's services
func (am *AppManager) getProviderUUIDsFromRule(rule *typ.Rule) []string {
	uuids := make(map[string]bool)
	for _, service := range rule.Services {
		if service.Provider != "" {
			uuids[service.Provider] = true
		}
	}

	result := make([]string, 0, len(uuids))
	for uuid := range uuids {
		result = append(result, uuid)
	}
	return result
}

// ============
// Import
// ============

// ImportRule imports a rule from data in the specified format
func (am *AppManager) ImportRule(data string, format exportpkg.Format, opts ImportOptions) (*ImportResult, error) {
	globalConfig := am.appConfig.GetGlobalConfig()

	// Convert command.ImportOptions to import.ImportOptions
	importOpts := exportpkg.ImportOptions{
		OnProviderConflict: opts.OnProviderConflict,
		OnRuleConflict:     opts.OnRuleConflict,
		Quiet:              opts.Quiet,
	}

	// Perform import
	result, err := exportpkg.Import(data, globalConfig, format, importOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to import rule: %w", err)
	}

	// Convert import.ImportResult to command.ImportResult
	return &ImportResult{
		RuleCreated:      result.RuleCreated,
		RuleUpdated:      result.RuleUpdated,
		ProvidersCreated: result.ProvidersCreated,
		ProvidersUsed:    result.ProvidersUsed,
		ProviderMap:      result.ProviderMap,
	}, nil
}
