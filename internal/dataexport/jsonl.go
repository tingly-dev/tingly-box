package dataexport

import (
	"encoding/json"
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ExportLine is the base type for all export lines
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

// ExportRequest contains the data needed for export
type ExportRequest struct {
	Rule      *typ.Rule
	Providers []*typ.Provider
}

// JSONLExporter exports data in JSONL format
type JSONLExporter struct{}

// NewJSONLExporter creates a new JSONL exporter
func NewJSONLExporter() *JSONLExporter {
	return &JSONLExporter{}
}

// Export performs the export in JSONL format
func (e *JSONLExporter) Export(req *ExportRequest) (*ExportResult, error) {
	if req.Rule == nil {
		return nil, fmt.Errorf("rule is required for export")
	}

	lines, err := e.buildJSONLLines(req)
	if err != nil {
		return nil, fmt.Errorf("failed to build JSONL: %w", err)
	}

	return &ExportResult{
		Format:  FormatJSONL,
		Content: lines,
	}, nil
}

// Format returns the format type
func (e *JSONLExporter) Format() Format {
	return FormatJSONL
}

// buildJSONLLines constructs the JSONL content from rule and providers
func (e *JSONLExporter) buildJSONLLines(req *ExportRequest) (string, error) {
	lines := make([]string, 0, 2+len(req.Providers))

	// Line 1: Metadata
	metadata := ExportMetadata{
		Type:       "metadata",
		Version:    CurrentVersion,
		ExportedAt: timestamp(),
	}
	metadataLine, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("failed to marshal metadata: %w", err)
	}
	lines = append(lines, string(metadataLine))

	// Line 2: Rule
	ruleData := e.buildRuleData(req.Rule)
	ruleLine, err := json.Marshal(ruleData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal rule: %w", err)
	}
	lines = append(lines, string(ruleLine))

	// Subsequent lines: Providers
	providerUUIDs := e.getProviderUUIDs(req.Rule)
	for _, provider := range req.Providers {
		// Only export providers that are referenced in the rule
		if !e.contains(providerUUIDs, provider.UUID) {
			continue
		}

		providerData := e.buildProviderData(provider)
		providerLine, err := json.Marshal(providerData)
		if err != nil {
			return "", fmt.Errorf("failed to marshal provider: %w", err)
		}
		lines = append(lines, string(providerLine))
	}

	return joinLines(lines), nil
}

// buildRuleData converts a Rule to ExportRuleData
func (e *JSONLExporter) buildRuleData(rule *typ.Rule) ExportRuleData {
	// Convert SmartRouting to []interface{} for JSON marshaling
	smartRouting := make([]interface{}, len(rule.SmartRouting))
	for i, sr := range rule.SmartRouting {
		smartRouting[i] = sr
	}

	return ExportRuleData{
		Type:          "rule",
		UUID:          rule.UUID,
		Scenario:      string(rule.Scenario),
		RequestModel:  rule.RequestModel,
		ResponseModel: rule.ResponseModel,
		Description:   rule.Description,
		Services:      rule.Services,
		LBTactic:      rule.LBTactic,
		Active:        rule.Active,
		SmartEnabled:  rule.SmartEnabled,
		SmartRouting:  smartRouting,
	}
}

// buildProviderData converts a Provider to ExportProviderData
func (e *JSONLExporter) buildProviderData(provider *typ.Provider) ExportProviderData {
	return ExportProviderData{
		Type:        "provider",
		UUID:        provider.UUID,
		Name:        provider.Name,
		APIBase:     provider.APIBase,
		APIStyle:    string(provider.APIStyle),
		AuthType:    string(provider.AuthType),
		Token:       provider.Token,
		OAuthDetail: provider.OAuthDetail,
		Enabled:     provider.Enabled,
		ProxyURL:    provider.ProxyURL,
		Timeout:     provider.Timeout,
		Tags:        provider.Tags,
		Models:      provider.Models,
	}
}

// getProviderUUIDs extracts all provider UUIDs from the rule's services
func (e *JSONLExporter) getProviderUUIDs(rule *typ.Rule) []string {
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

// contains checks if a string slice contains a specific string
func (e *JSONLExporter) contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
