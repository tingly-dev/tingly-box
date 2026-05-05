package command

import (
	"bufio"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestQuickstartProviderLookup tests the fix for the provider lookup bug
// where GetProvider was being used with a name instead of UUID.
func TestQuickstartProviderLookup(t *testing.T) {
	// Create a temporary config directory for testing
	tempDir, err := os.MkdirTemp("", "tingly-test-quickstart-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create app manager with temp config
	appManager, err := NewAppManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create app manager: %v", err)
	}

	// Test 1: GetProviderByName should return nil for non-existent provider
	t.Run("GetProviderByName returns nil for non-existent provider", func(t *testing.T) {
		provider, err := appManager.GetProviderByName("non-existent")
		if err == nil {
			t.Error("Expected error for non-existent provider, got nil")
		}
		if provider != nil {
			t.Error("Expected nil provider for non-existent name")
		}
	})

	// Test 2: Add provider and verify it can be retrieved by UUID
	t.Run("Add and retrieve provider by UUID", func(t *testing.T) {
		providerName := "test-deepseek"
		apiBase := "https://api.deepseek.com"
		token := "test-token-123"

		uuid, err := appManager.AddProvider(providerName, apiBase, token, protocol.APIStyleOpenAI)
		if err != nil {
			t.Fatalf("Failed to add provider: %v", err)
		}
		if uuid == "" {
			t.Fatal("UUID should not be empty")
		}

		// Verify we can retrieve by UUID
		provider, err := appManager.GetProvider(uuid)
		if err != nil {
			t.Errorf("Failed to get provider by UUID: %v", err)
		}
		if provider == nil {
			t.Fatal("Provider is nil")
		}

		// Verify the provider details
		if provider.Name != providerName {
			t.Errorf("Expected name %s, got %s", providerName, provider.Name)
		}
		if provider.APIBase != apiBase {
			t.Errorf("Expected APIBase %s, got %s", apiBase, provider.APIBase)
		}
		if provider.APIStyle != protocol.APIStyleOpenAI {
			t.Errorf("Expected APIStyle OpenAI, got %s", provider.APIStyle)
		}
		if provider.Token != token {
			t.Errorf("Expected token %s, got %s", token, provider.Token)
		}
	})

	// Test 3: Adding multiple providers with the same name should succeed
	// (they are distinguished by UUID, not name)
	t.Run("Adding multiple providers with same name succeeds", func(t *testing.T) {
		providerName := "test-same-name"
		apiBase1 := "https://api.example1.com"
		apiBase2 := "https://api.example2.com"
		token := "test-token"

		// First add should succeed
		_, err := appManager.AddProvider(providerName, apiBase1, token, protocol.APIStyleOpenAI)
		if err != nil {
			t.Fatalf("First add failed: %v", err)
		}

		// Second add with same name but different API base should also succeed
		_, err = appManager.AddProvider(providerName, apiBase2, token, protocol.APIStyleOpenAI)
		if err != nil {
			t.Errorf("Second add should succeed (providers are distinguished by UUID), got: %v", err)
		}

		// Verify we have two providers with the same name
		providers := appManager.ListProviders()
		sameNameCount := 0
		for _, p := range providers {
			if p.Name == providerName {
				sameNameCount++
			}
		}
		if sameNameCount != 2 {
			t.Errorf("Expected 2 providers with name '%s', got %d", providerName, sameNameCount)
		}
	})

	// Test 4: GetProvider (by UUID) should work after AddProvider
	t.Run("GetProvider by UUID works after AddProvider", func(t *testing.T) {
		providerName := "test-uuid-lookup"
		apiBase := "https://api.test.com"
		token := "test-token-uuid"

		uuid, err := appManager.AddProvider(providerName, apiBase, token, protocol.APIStyleAnthropic)
		if err != nil {
			t.Fatalf("Failed to add provider: %v", err)
		}
		if uuid == "" {
			t.Fatal("UUID should not be empty")
		}

		// Get by UUID directly
		providerByUUID, err := appManager.GetProvider(uuid)
		if err != nil {
			t.Errorf("Failed to get provider by UUID: %v", err)
		}
		if providerByUUID == nil {
			t.Fatal("Provider by UUID is nil")
		}

		// Verify it's the correct provider
		if providerByUUID.UUID != uuid {
			t.Error("UUIDs don't match")
		}
		if providerByUUID.Name != providerName {
			t.Errorf("Expected name %s, got %s", providerName, providerByUUID.Name)
		}
	})
}

// TestQuickstartConfigureRules tests the rule configuration logic
func TestQuickstartConfigureRules(t *testing.T) {
	// Create a temporary config directory for testing
	tempDir, err := os.MkdirTemp("", "tingly-test-rules-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	appManager, err := NewAppManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create app manager: %v", err)
	}

	// Add a test provider
	providerName := "test-provider"
	apiBase := "https://api.test.com"
	token := "test-token"

	uuid, err := appManager.AddProvider(providerName, apiBase, token, protocol.APIStyleOpenAI)
	if err != nil {
		t.Fatalf("Failed to add provider: %v", err)
	}

	provider, err := appManager.GetProvider(uuid)
	if err != nil {
		t.Fatalf("Failed to get provider: %v", err)
	}

	model := "test-model"

	// Configure rules (this simulates what quickstartConfigureRules does)
	t.Run("Configure routing rules for provider", func(t *testing.T) {
		cfg := appManager.GetGlobalConfig()

		// Configure tingly/cc rule
		service := &loadbalance.Service{
			Provider: provider.UUID,
			Model:    model,
			Weight:   1,
			Active:   true,
		}

		rule := cfg.GetRuleByRequestModelAndScenario("tingly/cc", typ.ScenarioClaudeCode)
		if rule == nil {
			t.Fatal("Expected tingly/cc rule to exist (it should be pre-configured)")
		}

		// Update the rule
		rule.Services = []*loadbalance.Service{service}
		rule.Active = true

		err = cfg.UpdateRule(rule.UUID, *rule)
		if err != nil {
			t.Fatalf("Failed to update rule: %v", err)
		}

		// Verify the rule was updated
		updatedRule := cfg.GetRuleByRequestModelAndScenario("tingly/cc", typ.ScenarioClaudeCode)
		if updatedRule == nil {
			t.Fatal("Rule not found after update")
		}

		if len(updatedRule.Services) != 1 {
			t.Errorf("Expected 1 service, got %d", len(updatedRule.Services))
		}

		if updatedRule.Services[0].Provider != provider.UUID {
			t.Errorf("Expected provider UUID %s, got %s", provider.UUID, updatedRule.Services[0].Provider)
		}

		if updatedRule.Services[0].Model != model {
			t.Errorf("Expected model %s, got %s", model, updatedRule.Services[0].Model)
		}
	})
}

// TestQuickstartProviderTemplate tests using template ID as provider name
// This is the specific bug that was fixed: template IDs like "deepseek-com"
// should work as provider names.
func TestQuickstartProviderTemplate(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test-template-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	appManager, err := NewAppManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create app manager: %v", err)
	}

	// Simulate what quickstart does: use template ID as provider name
	templateID := "deepseek-com"
	apiBase := "https://api.deepseek.com"
	token := "test-token"

	uuid, err := appManager.AddProvider(templateID, apiBase, token, protocol.APIStyleOpenAI)
	if err != nil {
		t.Fatalf("Failed to add provider with template ID as name: %v", err)
	}
	if uuid == "" {
		t.Fatal("UUID should not be empty")
	}

	// The fix: AddProvider now returns the UUID, use GetProvider to retrieve
	provider, err := appManager.GetProvider(uuid)
	if err != nil {
		t.Errorf("Failed to get provider by UUID: %v", err)
	}
	if provider == nil {
		t.Fatal("Provider is nil")
	}

	// Verify the provider has a proper UUID (not the template ID)
	if provider.UUID == templateID {
		t.Error("Provider UUID should be a generated UUID, not the template ID")
	}
	if provider.UUID == "" {
		t.Error("Provider UUID should not be empty")
	}

	// But the name should be the template ID
	if provider.Name != templateID {
		t.Errorf("Expected name %s, got %s", templateID, provider.Name)
	}
}

// TestPromptHelperFunctions tests the helper functions used by quickstart
func TestPromptHelperFunctions(t *testing.T) {
	// Test that promptForInput works with a simulated input
	t.Run("promptForInput with simulated input", func(t *testing.T) {
		input := "test-input\n"
		stringReader := strings.NewReader(input)
		reader := bufio.NewReader(stringReader)

		// Capture output
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		result, err := promptForInput(reader, "Enter value: ", true)

		// Restore stdout
		w.Close()
		os.Stdout = oldStdout
		io.Copy(io.Discard, r) // Discard output

		if err != nil {
			t.Errorf("promptForInput failed: %v", err)
		}
		if result != "test-input" {
			t.Errorf("Expected 'test-input', got '%s'", result)
		}
	})
}

// TestQuickstartConfigPersistence tests that configuration persists correctly
func TestQuickstartConfigPersistence(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tingly-test-persist-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// First session: add provider and configure
	t.Run("First session: add provider", func(t *testing.T) {
		appManager1, err := NewAppManager(tempDir)
		if err != nil {
			t.Fatalf("Failed to create app manager: %v", err)
		}

		_, err = appManager1.AddProvider("persist-test", "https://api.test.com", "token", protocol.APIStyleOpenAI)
		if err != nil {
			t.Fatalf("Failed to add provider: %v", err)
		}

		err = appManager1.SaveConfig()
		if err != nil {
			t.Fatalf("Failed to save config: %v", err)
		}
	})

	// Second session: verify provider persists
	t.Run("Second session: verify provider persists", func(t *testing.T) {
		appManager2, err := NewAppManager(tempDir)
		if err != nil {
			t.Fatalf("Failed to create app manager: %v", err)
		}

		provider, err := appManager2.GetProviderByName("persist-test")
		if err != nil {
			t.Errorf("Failed to get persisted provider: %v", err)
		}
		if provider == nil {
			t.Fatal("Persisted provider is nil")
		}

		if provider.Name != "persist-test" {
			t.Errorf("Expected name 'persist-test', got '%s'", provider.Name)
		}
		if provider.APIBase != "https://api.test.com" {
			t.Errorf("Expected APIBase 'https://api.test.com', got '%s'", provider.APIBase)
		}
	})
}
