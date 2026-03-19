package dataimport

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ImportLine is the base type for all import lines
type ImportLine struct {
	Type string `json:"type"`
}

// ImportMetadata represents the metadata line
type ImportMetadata struct {
	Type       string `json:"type"`
	Version    string `json:"version"`
	ExportedAt string `json:"exported_at"`
}

// ImportRuleData represents the rule import data
type ImportRuleData struct {
	Type          string                 `json:"type"`
	UUID          string                 `json:"uuid"`
	Scenario      string                 `json:"scenario"`
	RequestModel  string                 `json:"request_model"`
	ResponseModel string                 `json:"response_model"`
	Description   string                 `json:"description"`
	Services      []*loadbalance.Service `json:"services"`
	Flags         typ.RuleFlags          `json:"flags,omitempty"`
	LBTactic      typ.Tactic             `json:"lb_tactic"`
	Active        bool                   `json:"active"`
	SmartEnabled  bool                   `json:"smart_enabled"`
	SmartRouting  []interface{}          `json:"smart_routing"`
}

// ImportProviderData represents the provider import data
type ImportProviderData struct {
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

// ImportOptions controls how imports are handled when conflicts occur
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

// ImportResult contains the results of an import operation
type ImportResult struct {
	RuleCreated      bool
	RuleUpdated      bool
	ProvidersCreated int
	ProvidersUsed    int
	Providers        []ProviderImportInfo
	ProviderMap      map[string]string // old UUID -> new UUID
}

// Importer defines the interface for import implementations
type Importer interface {
	Import(data string, globalConfig *config.Config, opts ImportOptions) (*ImportResult, error)
	Format() Format
}

// JSONLImporter imports data from JSONL format
type JSONLImporter struct{}

// NewJSONLImporter creates a new JSONL importer
func NewJSONLImporter() *JSONLImporter {
	return &JSONLImporter{}
}

// Format returns the format type
func (i *JSONLImporter) Format() Format {
	return FormatJSONL
}

// Import imports data from JSONL format
func (i *JSONLImporter) Import(data string, globalConfig *config.Config, opts ImportOptions) (*ImportResult, error) {
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
	var metadata *ImportMetadata
	var ruleData *ImportRuleData
	providersData := []*ImportProviderData{}

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue // Skip empty lines
		}

		// Parse line type
		var base ImportLine
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
			var provider ImportProviderData
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

	if ruleData == nil && len(providersData) == 0 {
		return nil, fmt.Errorf("no rule or provider data found in export")
	}

	// Import providers first (before rule processing, so they're available for the rule)
	for _, p := range providersData {
		providerResult, err := i.importProvider(globalConfig, p, opts.OnProviderConflict, result.ProviderMap)
		if err != nil {
			return nil, fmt.Errorf("failed to import provider '%s': %w", p.Name, err)
		}
		if providerResult.created {
			result.ProvidersCreated++
		}
		if providerResult.used {
			result.ProvidersUsed++
		}
		// Add provider info to result
		if providerResult.info != nil {
			result.Providers = append(result.Providers, *providerResult.info)
		}
	}

	// If we don't have rule data, we're done (provider-only import)
	if ruleData == nil {
		return result, nil
	}

	// Collect unique provider UUIDs from services for validation
	requiredProviderUUIDs := make(map[string]bool)
	for _, service := range ruleData.Services {
		if service.Provider != "" {
			requiredProviderUUIDs[service.Provider] = true
		}
	}

	// Validate that all required providers have been mapped
	var missingProviders []string
	for oldUUID := range requiredProviderUUIDs {
		if _, mapped := result.ProviderMap[oldUUID]; !mapped {
			// Check if this UUID exists as a provider in the export data
			found := false
			for _, p := range providersData {
				if p.UUID == oldUUID {
					found = true
					break
				}
			}
			if !found {
				missingProviders = append(missingProviders, oldUUID)
			}
		}
	}

	// If there are missing providers that weren't in the export, fail with a clear error
	if len(missingProviders) > 0 {
		return nil, fmt.Errorf("rule references providers that were not included in the export: %v. Please ensure all providers referenced by the rule are included in the export data", missingProviders)
	}

	// Check for existing rule
	existingRule := globalConfig.GetRuleByRequestModelAndScenario(ruleData.RequestModel, typ.RuleScenario(ruleData.Scenario))

	// Remap provider UUIDs in services
	for i := range ruleData.Services {
		if oldUUID, ok := result.ProviderMap[ruleData.Services[i].Provider]; ok {
			ruleData.Services[i].Provider = oldUUID
		}
	}

	// Create or update rule
	rule := typ.Rule{
		UUID:          uuid.New().String(),
		Scenario:      typ.RuleScenario(ruleData.Scenario),
		RequestModel:  ruleData.RequestModel,
		ResponseModel: ruleData.ResponseModel,
		Description:   ruleData.Description,
		Services:      ruleData.Services,
		Flags:         ruleData.Flags,
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

type providerImportResult struct {
	created bool
	used    bool
	info    *ProviderImportInfo
}

func (i *JSONLImporter) importProvider(globalConfig *config.Config, p *ImportProviderData, onConflict string, providerMap map[string]string) (*providerImportResult, error) {
	result := &providerImportResult{}

	// Check if provider with same UUID already exists (real conflict)
	existingProvider, err := globalConfig.GetProviderByUUID(p.UUID)
	if err == nil && existingProvider != nil {
		// Real UUID conflict - provider was already imported before
		switch onConflict {
		case "skip":
			result.info = &ProviderImportInfo{
				UUID:   p.UUID,
				Name:   p.Name,
				Action: "skipped",
			}
			return result, nil
		case "use":
			// Use the existing provider
			providerMap[p.UUID] = existingProvider.UUID
			result.used = true
			result.info = &ProviderImportInfo{
				UUID:   existingProvider.UUID,
				Name:   existingProvider.Name,
				Action: "used",
			}
			return result, nil
		default:
			// Default to using existing provider for UUID conflicts
			providerMap[p.UUID] = existingProvider.UUID
			result.used = true
			result.info = &ProviderImportInfo{
				UUID:   existingProvider.UUID,
				Name:   existingProvider.Name,
				Action: "used",
			}
			return result, nil
		}
	}

	// Check if provider name already exists (need to avoid duplicate names)
	_, err = globalConfig.GetProviderByName(p.Name)
	nameExists := err == nil

	// Always create a new UUID for imported providers
	// This allows the same provider export to be imported multiple times
	providerUUID := uuid.New().String()

	// If name exists, add suffix to avoid conflicts
	if nameExists {
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
	}

	// Create new provider with new UUID
	newProvider := &typ.Provider{
		UUID:        providerUUID,
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
		return nil, fmt.Errorf("failed to add provider: %w", err)
	}

	// Map old UUID to new UUID
	providerMap[p.UUID] = newProvider.UUID
	result.created = true
	result.info = &ProviderImportInfo{
		UUID:   newProvider.UUID,
		Name:   newProvider.Name,
		Action: "created",
	}
	return result, nil
}
