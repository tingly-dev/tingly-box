package dataexport

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/config"
	"github.com/tingly-dev/tingly-box/internal/dataimport"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestEndToEndExportImportJSONL tests the complete export->import cycle with JSONL format
func TestEndToEndExportImportJSONL(t *testing.T) {
	// Create source config with test data
	sourceConfigDir := t.TempDir()
	sourceAppConfig, err := config.NewAppConfig(config.WithConfigDir(sourceConfigDir))
	if err != nil {
		t.Fatalf("Failed to create source config: %v", err)
	}

	sourceConfig := sourceAppConfig.GetGlobalConfig()

	// Create test providers
	provider1 := &typ.Provider{
		UUID:     "prov-1-uuid",
		Name:     "Test Provider 1",
		APIBase:  "https://api.example1.com",
		APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeAPIKey,
		Token:    "sk-test-token-1",
		Enabled:  true,
		Timeout:  30,
		Tags:     []string{"test", "provider1"},
		Models:   []string{"gpt-4", "gpt-3.5-turbo"},
	}

	provider2 := &typ.Provider{
		UUID:     "prov-2-uuid",
		Name:     "Test Provider 2",
		APIBase:  "https://api.example2.com",
		APIStyle: protocol.APIStyleAnthropic,
		AuthType: typ.AuthTypeAPIKey,
		Token:    "sk-test-token-2",
		Enabled:  true,
		Timeout:  60,
		Tags:     []string{"test", "provider2"},
		Models:   []string{"claude-3-opus"},
	}

	if err := sourceConfig.AddProvider(provider1); err != nil {
		t.Fatalf("Failed to add provider1: %v", err)
	}
	if err := sourceConfig.AddProvider(provider2); err != nil {
		t.Fatalf("Failed to add provider2: %v", err)
	}

	// Create test rule
	rule := &typ.Rule{
		UUID:          "test-rule-uuid",
		Scenario:      typ.RuleScenario("general"),
		RequestModel:  "gpt-4",
		ResponseModel: "gpt-4",
		Description:   "Test rule for E2E export/import",
		Services: []*loadbalance.Service{
			{
				Provider: provider1.UUID,
				Model:    "gpt-4",
				Weight:   100,
			},
			{
				Provider: provider2.UUID,
				Model:    "claude-3-opus",
				Weight:   50,
			},
		},
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticRoundRobin,
			Params: typ.DefaultRoundRobinParams(),
		},
		Active:       true,
		SmartEnabled: false,
	}

	if err := sourceConfig.AddRule(*rule); err != nil {
		t.Fatalf("Failed to add rule: %v", err)
	}

	// Export the rule
	exportReq := &ExportRequest{
		Rule:      rule,
		Providers: []*typ.Provider{provider1, provider2},
	}

	exporter := NewJSONLExporter()
	result, err := exporter.Export(exportReq)
	if err != nil {
		t.Fatalf("Failed to export rule: %v", err)
	}

	if result.Format != FormatJSONL {
		t.Errorf("Expected format %v, got %v", FormatJSONL, result.Format)
	}

	// Create destination config (empty)
	destConfigDir := t.TempDir()
	destAppConfig, err := config.NewAppConfig(config.WithConfigDir(destConfigDir))
	if err != nil {
		t.Fatalf("Failed to create destination config: %v", err)
	}
	destConfig := destAppConfig.GetGlobalConfig()

	// Import the exported data
	importOpts := dataimport.ImportOptions{
		OnProviderConflict: "use", // Will create new since none exist
		OnRuleConflict:     "skip",
	}

	importer := dataimport.NewJSONLImporter()
	importResult, err := importer.Import(result.Content, destConfig, importOpts)
	if err != nil {
		t.Fatalf("Failed to import rule: %v", err)
	}

	// Verify import result
	if !importResult.RuleCreated {
		t.Error("Expected RuleCreated to be true")
	}
	if importResult.ProvidersCreated != 2 {
		t.Errorf("Expected 2 providers created, got %d", importResult.ProvidersCreated)
	}

	// Verify the imported rule matches the original
	importedRule := destConfig.GetRuleByRequestModelAndScenario(rule.RequestModel, rule.Scenario)
	if importedRule == nil {
		t.Fatal("Imported rule not found")
	}

	// Verify rule properties
	if importedRule.Scenario != rule.Scenario {
		t.Errorf("Expected scenario %v, got %v", rule.Scenario, importedRule.Scenario)
	}
	if importedRule.RequestModel != rule.RequestModel {
		t.Errorf("Expected request model %s, got %s", rule.RequestModel, importedRule.RequestModel)
	}
	if importedRule.ResponseModel != rule.ResponseModel {
		t.Errorf("Expected response model %s, got %s", rule.ResponseModel, importedRule.ResponseModel)
	}
	if importedRule.Description != rule.Description {
		t.Errorf("Expected description %s, got %s", rule.Description, importedRule.Description)
	}
	if importedRule.Active != rule.Active {
		t.Errorf("Expected active %v, got %v", rule.Active, importedRule.Active)
	}

	// Verify services (number and properties)
	if len(importedRule.Services) != len(rule.Services) {
		t.Fatalf("Expected %d services, got %d", len(rule.Services), len(importedRule.Services))
	}

	for i, service := range importedRule.Services {
		originalService := rule.Services[i]
		if service.Model != originalService.Model {
			t.Errorf("Service %d: expected model %s, got %s", i, originalService.Model, service.Model)
		}
		if service.Weight != originalService.Weight {
			t.Errorf("Service %d: expected weight %d, got %d", i, originalService.Weight, service.Weight)
		}
		// Provider UUID will be different (remapped during import)
		if service.Provider == "" {
			t.Errorf("Service %d: provider UUID is empty", i)
		}
	}

	// Verify providers were imported
	importedProviders := destConfig.Providers
	if len(importedProviders) != 2 {
		t.Errorf("Expected 2 providers in destination config, got %d", len(importedProviders))
	}

	// Verify provider properties
	for _, p := range importedProviders {
		if p.Name != provider1.Name && p.Name != provider2.Name {
			t.Errorf("Unexpected provider name: %s", p.Name)
		}
		if p.APIBase != provider1.APIBase && p.APIBase != provider2.APIBase {
			t.Errorf("Unexpected provider API base: %s", p.APIBase)
		}
		// Token should be preserved
		if p.Token != provider1.Token && p.Token != provider2.Token {
			t.Errorf("Token not preserved for provider %s", p.Name)
		}
	}
}

// TestEndToEndExportImportBase64 tests the complete export->import cycle with Base64 format
func TestEndToEndExportImportBase64(t *testing.T) {
	// Create source config with test data
	sourceConfigDir := t.TempDir()
	sourceAppConfig, err := config.NewAppConfig(config.WithConfigDir(sourceConfigDir))
	if err != nil {
		t.Fatalf("Failed to create source config: %v", err)
	}

	sourceConfig := sourceAppConfig.GetGlobalConfig()

	// Create test provider
	provider := &typ.Provider{
		UUID:     "prov-test-uuid",
		Name:     "Base64 Test Provider",
		APIBase:  "https://api.base64-test.com",
		APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeAPIKey,
		Token:    "sk-base64-test-token",
		Enabled:  true,
		Timeout:  45,
		Tags:     []string{"base64", "test"},
		Models:   []string{"gpt-4"},
	}

	if err := sourceConfig.AddProvider(provider); err != nil {
		t.Fatalf("Failed to add provider: %v", err)
	}

	// Create test rule
	rule := &typ.Rule{
		UUID:          "base64-rule-uuid",
		Scenario:      typ.RuleScenario("base64-test"),
		RequestModel:  "gpt-4",
		ResponseModel: "gpt-4",
		Description:   "Base64 format test rule with special chars: 测试🎉",
		Services: []*loadbalance.Service{
			{
				Provider: provider.UUID,
				Model:    "gpt-4",
				Weight:   100,
			},
		},
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticRoundRobin,
			Params: typ.DefaultRoundRobinParams(),
		},
		Active:       true,
		SmartEnabled: false,
	}

	if err := sourceConfig.AddRule(*rule); err != nil {
		t.Fatalf("Failed to add rule: %v", err)
	}

	// Export the rule in Base64 format
	exportReq := &ExportRequest{
		Rule:      rule,
		Providers: []*typ.Provider{provider},
	}

	exporter := NewBase64Exporter()
	result, err := exporter.Export(exportReq)
	if err != nil {
		t.Fatalf("Failed to export rule: %v", err)
	}

	if result.Format != FormatBase64 {
		t.Errorf("Expected format %v, got %v", FormatBase64, result.Format)
	}

	// Verify Base64 format
	base64Content := result.Content
	if len(base64Content) < len(Base64Prefix)+10 {
		t.Error("Base64 export content too short")
	}
	if !e2eStartsWith(base64Content, Base64Prefix+":1.0:") {
		t.Errorf("Base64 export missing correct prefix, got: %s", base64Content[:20])
	}

	// Decode Base64 to verify it's valid
	decoded, err := DecodeBase64Export(base64Content)
	if err != nil {
		t.Fatalf("Failed to decode Base64 export: %v", err)
	}
	if decoded == "" {
		t.Error("Decoded content is empty")
	}

	// Create destination config (empty)
	destConfigDir := t.TempDir()
	destAppConfig, err := config.NewAppConfig(config.WithConfigDir(destConfigDir))
	if err != nil {
		t.Fatalf("Failed to create destination config: %v", err)
	}
	destConfig := destAppConfig.GetGlobalConfig()

	// Import the Base64 export
	importOpts := dataimport.ImportOptions{
		OnProviderConflict: "use",
		OnRuleConflict:     "skip",
	}

	importer := dataimport.NewBase64Importer()
	importResult, err := importer.Import(base64Content, destConfig, importOpts)
	if err != nil {
		t.Fatalf("Failed to import Base64 export: %v", err)
	}

	// Verify import result
	if !importResult.RuleCreated {
		t.Error("Expected RuleCreated to be true")
	}
	if importResult.ProvidersCreated != 1 {
		t.Errorf("Expected 1 provider created, got %d", importResult.ProvidersCreated)
	}

	// Verify the imported rule matches the original
	importedRule := destConfig.GetRuleByRequestModelAndScenario(rule.RequestModel, rule.Scenario)
	if importedRule == nil {
		t.Fatal("Imported rule not found")
	}

	// Verify special characters in description are preserved
	if importedRule.Description != rule.Description {
		t.Errorf("Expected description %s, got %s", rule.Description, importedRule.Description)
	}

	// Verify provider
	importedProviders := destConfig.Providers
	if len(importedProviders) != 1 {
		t.Errorf("Expected 1 provider, got %d", len(importedProviders))
	}

	importedProvider := importedProviders[0]
	if importedProvider.Name != provider.Name {
		t.Errorf("Expected provider name %s, got %s", provider.Name, importedProvider.Name)
	}
	if importedProvider.Token != provider.Token {
		t.Errorf("Expected provider token %s, got %s", provider.Token, importedProvider.Token)
	}
}

// TestEndToEndMultipleProviders tests export/import with multiple providers
func TestEndToEndMultipleProviders(t *testing.T) {
	sourceConfigDir := t.TempDir()
	sourceAppConfig, err := config.NewAppConfig(config.WithConfigDir(sourceConfigDir))
	if err != nil {
		t.Fatalf("Failed to create source config: %v", err)
	}
	sourceConfig := sourceAppConfig.GetGlobalConfig()

	// Create 3 providers
	providers := make([]*typ.Provider, 3)
	for i := 0; i < 3; i++ {
		providers[i] = &typ.Provider{
			UUID:     "prov-" + string(rune('1'+i)) + "-uuid",
			Name:     "Provider " + string(rune('A'+i)),
			APIBase:  "https://api.provider" + string(rune('1'+i)) + ".com",
			APIStyle: protocol.APIStyleOpenAI,
			AuthType: typ.AuthTypeAPIKey,
			Token:    "sk-token-" + string(rune('1'+i)),
			Enabled:  true,
			Timeout:  30,
			Models:   []string{"model-" + string(rune('1'+i))},
		}
		if err := sourceConfig.AddProvider(providers[i]); err != nil {
			t.Fatalf("Failed to add provider %d: %v", i, err)
		}
	}

	// Create rule with services referencing all 3 providers
	services := make([]*loadbalance.Service, 3)
	for i := 0; i < 3; i++ {
		services[i] = &loadbalance.Service{
			Provider: providers[i].UUID,
			Model:    providers[i].Models[0],
			Weight:   100 - i*10,
		}
	}

	rule := &typ.Rule{
		UUID:          "multi-prov-rule-uuid",
		Scenario:      typ.RuleScenario("multi-prov"),
		RequestModel:  "multi-model",
		ResponseModel: "multi-model",
		Description:   "Test with multiple providers",
		Services:      services,
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticRoundRobin,
			Params: typ.DefaultRoundRobinParams(),
		},
		Active: true,
	}

	if err := sourceConfig.AddRule(*rule); err != nil {
		t.Fatalf("Failed to add rule: %v", err)
	}

	// Export and import
	exportReq := &ExportRequest{
		Rule:      rule,
		Providers: providers,
	}

	exporter := NewJSONLExporter()
	result, err := exporter.Export(exportReq)
	if err != nil {
		t.Fatalf("Failed to export: %v", err)
	}

	destConfigDir := t.TempDir()
	destAppConfig, err := config.NewAppConfig(config.WithConfigDir(destConfigDir))
	if err != nil {
		t.Fatalf("Failed to create dest config: %v", err)
	}
	destConfig := destAppConfig.GetGlobalConfig()

	importer := dataimport.NewJSONLImporter()
	importResult, err := importer.Import(result.Content, destConfig, dataimport.ImportOptions{
		OnProviderConflict: "use",
		OnRuleConflict:     "skip",
	})
	if err != nil {
		t.Fatalf("Failed to import: %v", err)
	}

	if importResult.ProvidersCreated != 3 {
		t.Errorf("Expected 3 providers created, got %d", importResult.ProvidersCreated)
	}

	importedRule := destConfig.GetRuleByRequestModelAndScenario(rule.RequestModel, rule.Scenario)
	if importedRule == nil {
		t.Fatal("Imported rule not found")
	}

	if len(importedRule.Services) != 3 {
		t.Errorf("Expected 3 services, got %d", len(importedRule.Services))
	}

	if len(destConfig.Providers) != 3 {
		t.Errorf("Expected 3 providers in config, got %d", len(destConfig.Providers))
	}
}

// Helper function with unique name to avoid conflict with integration_test.go
func e2eStartsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[0:len(prefix)] == prefix
}
