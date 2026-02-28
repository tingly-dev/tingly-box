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

// ImportResult contains the results of an import operation
type ImportResult struct {
	RuleCreated      bool
	RuleUpdated      bool
	ProvidersCreated int
	ProvidersUsed    int
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

	if ruleData == nil {
		return nil, fmt.Errorf("no rule data found in export")
	}

	// Import providers
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
}

func (i *JSONLImporter) importProvider(globalConfig *config.Config, p *ImportProviderData, onConflict string, providerMap map[string]string) (*providerImportResult, error) {
	result := &providerImportResult{}

	// Check if provider with same name exists
	existingProvider, err := globalConfig.GetProviderByName(p.Name)
	if err == nil && existingProvider != nil {
		switch onConflict {
		case "use":
			providerMap[p.UUID] = existingProvider.UUID
			result.used = true
			return result, nil
		case "skip":
			return result, nil
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
			providerMap[p.UUID] = existingProvider.UUID
			result.used = true
			return result, nil
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
		return nil, fmt.Errorf("failed to add provider: %w", err)
	}

	providerMap[p.UUID] = newProvider.UUID
	result.created = true
	return result, nil
}
