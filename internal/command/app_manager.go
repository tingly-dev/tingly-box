package command

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/dataexport"
	exportpkg "github.com/tingly-dev/tingly-box/internal/dataexport"
	importpkg "github.com/tingly-dev/tingly-box/internal/dataimport"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
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
func (am *AppManager) AddProvider(name, apiBase, token string, apiStyle protocol.APIStyle) error {
	// Check if provider already exists
	if existingProvider, err := am.appConfig.GetProviderByName(name); err == nil && existingProvider != nil {
		return fmt.Errorf("provider '%s' already exists", name)
	}

	// Add the provider
	if err := am.appConfig.AddProviderByName(name, apiBase, token); err != nil {
		return fmt.Errorf("failed to add provider: %w", err)
	}

	// Update the provider to set the API style
	provider, err := am.appConfig.GetProviderByName(name)
	if err == nil {
		provider.APIStyle = apiStyle
		// Save the configuration
		if saveErr := am.appConfig.Save(); saveErr != nil {
			return fmt.Errorf("failed to save API style configuration: %w", saveErr)
		}
	}

	return nil
}

// DeleteProvider removes an AI provider by name.
func (am *AppManager) DeleteProvider(name string) error {
	if err := am.appConfig.DeleteProvider(name); err != nil {
		return fmt.Errorf("failed to delete provider: %w", err)
	}
	return nil
}

// ListProviders returns all configured providers.
func (am *AppManager) ListProviders() []*typ.Provider {
	return am.appConfig.ListProviders()
}

// GetProvider returns a provider by name, or nil if not found.
func (am *AppManager) GetProvider(name string) (*typ.Provider, error) {
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

// ExportLine represents a generic line in the export file
type ExportLine struct {
	Type string `json:"type"`
}

// ExportMetadata represents the metadata line
type ExportMetadata struct {
	Type       string `json:"type"`
	Version    string `json:"version"`
	ExportedAt string `json:"exported_at"`
}

// ExportRuleData represents the rule export data
type ExportRuleData struct {
	Type          string                 `json:"type"`
	UUID          string                 `json:"uuid"`
	Scenario      string                 `json:"scenario"`
	RequestModel  string                 `json:"request_model"`
	ResponseModel string                 `json:"response_model"`
	Description   string                 `json:"description"`
	Services      []*loadbalance.Service `json:"services"`
	LBTactic      typ.Tactic             `json:"lb_tactic"`
	Active        bool                   `json:"active"`
	SmartEnabled  bool                   `json:"smart_enabled"`
	SmartRouting  []interface{}          `json:"smart_routing"`
}

// ExportProviderData represents the provider export data
type ExportProviderData struct {
	Type        string           `json:"type"`
	UUID        string           `json:"uuid"`
	Name        string           `json:"name"`
	APIBase     string           `json:"api_base"`
	APIStyle    string           `json:"api_style"`
	AuthType    string           `json:"auth_type"`
	Token       string           `json:"token"`
	OAuthDetail *typ.OAuthDetail `json:"oauth_detail"`
	Enabled     bool             `json:"enabled"`
	ProxyURL    string           `json:"proxy_url"`
	Timeout     int64            `json:"timeout"`
	Tags        []string         `json:"tags"`
	Models      []string         `json:"models"`
}

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

// ImportResult contains the results of an import operation.
type ImportResult struct {
	RuleCreated      bool
	RuleUpdated      bool
	ProvidersCreated int
	ProvidersUsed    int
	ProviderMap      map[string]string // old UUID -> new UUID
}

// ImportRuleFromJSONL imports a rule from JSONL format (either file content or stdin format).
// The data should be line-delimited JSON with:
// - Line 1: metadata (type="metadata")
// - Line 2: rule data (type="rule")
// - Subsequent lines: provider data (type="provider")
func (am *AppManager) ImportRuleFromJSONL(data string, opts ImportOptions) (*ImportResult, error) {
	result := &ImportResult{
		ProviderMap: make(map[string]string),
	}

	// Set defaults
	if opts.OnProviderConflict == "" {
		opts.OnProviderConflict = "use"
	}
	if opts.OnRuleConflict == "" {
		opts.OnRuleConflict = "skip"
	}

	// Parse lines
	scanner := bufio.NewScanner(strings.NewReader(data))
	var metadata *ExportMetadata
	var ruleData *ExportRuleData
	providersData := []*ExportProviderData{}

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue // Skip empty lines
		}

		// Parse line type
		var base ExportLine
		if err := json.Unmarshal([]byte(line), &base); err != nil {
			return nil, fmt.Errorf("line %d: invalid JSON: %w", lineNum, err)
		}

		switch base.Type {
		case "metadata":
			if err := json.Unmarshal([]byte(line), &metadata); err != nil {
				return nil, fmt.Errorf("line %d: invalid metadata: %w", lineNum, err)
			}
			if metadata.Version != "1.0" {
				return nil, fmt.Errorf("unsupported export version: %s", metadata.Version)
			}

		case "rule":
			if err := json.Unmarshal([]byte(line), &ruleData); err != nil {
				return nil, fmt.Errorf("line %d: invalid rule data: %w", lineNum, err)
			}

		case "provider":
			var provider ExportProviderData
			if err := json.Unmarshal([]byte(line), &provider); err != nil {
				return nil, fmt.Errorf("line %d: invalid provider data: %w", lineNum, err)
			}
			providersData = append(providersData, &provider)

		default:
			return nil, fmt.Errorf("line %d: unknown type '%s'", lineNum, base.Type)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading input: %w", err)
	}

	if ruleData == nil {
		return nil, fmt.Errorf("no rule data found in export")
	}

	globalConfig := am.appConfig.GetGlobalConfig()

	// Import providers
	for _, p := range providersData {
		// Check if provider with same name exists
		existingProvider, err := globalConfig.GetProviderByName(p.Name)
		if err == nil && existingProvider != nil {
			switch opts.OnProviderConflict {
			case "use":
				result.ProviderMap[p.UUID] = existingProvider.UUID
				result.ProvidersUsed++
				continue
			case "skip":
				continue
			case "suffix":
				// Create with suffixed name
				suffix := 2
				newName := fmt.Sprintf("%s-%d", p.Name, suffix)
				for {
					_, err := globalConfig.GetProviderByName(newName)
					if err != nil {
						break // Name is available
					}
					suffix++
					newName = fmt.Sprintf("%s-%d", p.Name, suffix)
				}
				p.Name = newName
			default:
				result.ProviderMap[p.UUID] = existingProvider.UUID
				result.ProvidersUsed++
				continue
			}
		}

		// Create new provider
		newProvider := &typ.Provider{
			UUID:        uuid.New().String(),
			Name:        p.Name,
			APIBase:     p.APIBase,
			APIStyle:    protocol.APIStyle(p.APIStyle),
			AuthType:    typ.AuthType(p.AuthType),
			Token:       p.Token,
			OAuthDetail: p.OAuthDetail,
			Enabled:     p.Enabled,
			ProxyURL:    p.ProxyURL,
			Timeout:     p.Timeout,
			Tags:        p.Tags,
			Models:      p.Models,
		}

		if err := globalConfig.AddProvider(newProvider); err != nil {
			return nil, fmt.Errorf("failed to add provider '%s': %w", p.Name, err)
		}

		result.ProviderMap[p.UUID] = newProvider.UUID
		result.ProvidersCreated++
	}

	// Check for existing rule
	existingRule := globalConfig.GetRuleByRequestModelAndScenario(ruleData.RequestModel, typ.RuleScenario(ruleData.Scenario))

	// Remap provider UUIDs in services
	for i := range ruleData.Services {
		if oldUUID, ok := result.ProviderMap[ruleData.Services[i].Provider]; ok {
			ruleData.Services[i].Provider = oldUUID
		}
	}

	// Create rule
	rule := typ.Rule{
		UUID:          uuid.New().String(),
		Scenario:      typ.RuleScenario(ruleData.Scenario),
		RequestModel:  ruleData.RequestModel,
		ResponseModel: ruleData.ResponseModel,
		Description:   ruleData.Description,
		Services:      ruleData.Services,
		LBTactic:      ruleData.LBTactic,
		Active:        ruleData.Active,
	}

	if existingRule != nil {
		switch opts.OnRuleConflict {
		case "skip":
			return result, nil
		case "update":
			rule.UUID = existingRule.UUID
			if err := globalConfig.UpdateRule(existingRule.UUID, rule); err != nil {
				return nil, fmt.Errorf("failed to update rule: %w", err)
			}
			result.RuleUpdated = true
		case "new":
			rule.RequestModel = fmt.Sprintf("%s-imported", ruleData.RequestModel)
			if err := globalConfig.AddRule(rule); err != nil {
				return nil, fmt.Errorf("failed to add rule: %w", err)
			}
			result.RuleCreated = true
		}
	} else {
		if err := globalConfig.AddRule(rule); err != nil {
			return nil, fmt.Errorf("failed to add rule: %w", err)
		}
		result.RuleCreated = true
	}

	return result, nil
}

// ============
// Export
// ============

// ExportRule exports a rule with its providers in the specified format
func (am *AppManager) ExportRule(rule *typ.Rule, format dataexport.Format) (string, error) {
	if rule == nil {
		return "", fmt.Errorf("rule is required for export")
	}

	globalConfig := am.appConfig.GetGlobalConfig()

	// Collect providers referenced in the rule
	providerUUIDs := am.getProviderUUIDsFromRule(rule)
	providers := make([]*typ.Provider, 0, len(providerUUIDs))

	for _, providerUUID := range providerUUIDs {
		provider, err := globalConfig.GetProviderByUUID(providerUUID)
		if err == nil && provider != nil {
			providers = append(providers, provider)
		}
	}

	// Build export request
	req := &exportpkg.ExportRequest{
		Rule:      rule,
		Providers: providers,
	}

	// Perform export
	result, err := exportpkg.Export(req, format)
	if err != nil {
		return "", fmt.Errorf("failed to export rule: %w", err)
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
func (am *AppManager) ImportRule(data string, format importpkg.Format, opts ImportOptions) (*ImportResult, error) {
	globalConfig := am.appConfig.GetGlobalConfig()

	// Convert command.ImportOptions to import.ImportOptions
	importOpts := importpkg.ImportOptions{
		OnProviderConflict: opts.OnProviderConflict,
		OnRuleConflict:     opts.OnRuleConflict,
		Quiet:              opts.Quiet,
	}

	// Perform import
	result, err := importpkg.Import(data, globalConfig, format, importOpts)
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
