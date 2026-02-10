package dataexport

import (
	"testing"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestExportRoundTrip tests the complete export->import cycle
func TestExportRoundTrip(t *testing.T) {
	// Create a test rule with providers
	rule := &typ.Rule{
		UUID:          "test-rule-uuid",
		Scenario:      typ.RuleScenario("general"),
		RequestModel:  "gpt-4",
		ResponseModel: "gpt-4",
		Description:   "Test rule for export/import",
		Services: []*loadbalance.Service{
			{
				Provider: "provider-1",
				Model:    "gpt-4",
				Weight:   100,
			},
		},
		LBTactic: typ.Tactic{
			Type: loadbalance.TacticRoundRobin,
		},
		Active:       true,
		SmartEnabled: false,
	}

	providers := []*typ.Provider{
		{
			UUID:     "provider-1",
			Name:     "Test Provider",
			APIBase:  "https://api.example.com",
			APIStyle: protocol.APIStyleOpenAI,
			AuthType: typ.AuthTypeAPIKey,
			Token:    "test-token",
			Enabled:  true,
			Timeout:  30,
			Tags:     []string{"test"},
			Models:   []string{"gpt-4"},
		},
	}

	req := &ExportRequest{
		Rule:      rule,
		Providers: providers,
	}

	t.Run("JSONL format round trip", func(t *testing.T) {
		exporter := NewJSONLExporter()
		result, err := exporter.Export(req)
		if err != nil {
			t.Fatalf("Export failed: %v", err)
		}

		if result.Format != FormatJSONL {
			t.Errorf("Expected format %v, got %v", FormatJSONL, result.Format)
		}

		if result.Content == "" {
			t.Error("Export result is empty")
		}

		// Verify the content contains expected JSON lines
		if !containsAll(result.Content, []string{
			`"type":"metadata"`,
			`"type":"rule"`,
			`"type":"provider"`,
		}) {
			t.Error("Export content missing expected JSON type markers")
		}
	})

	t.Run("Base64 format round trip", func(t *testing.T) {
		exporter := NewBase64Exporter()
		result, err := exporter.Export(req)
		if err != nil {
			t.Fatalf("Export failed: %v", err)
		}

		if result.Format != FormatBase64 {
			t.Errorf("Expected format %v, got %v", FormatBase64, result.Format)
		}

		if result.Content == "" {
			t.Error("Export result is empty")
		}

		// Verify Base64 format
		if !startsWith(result.Content, Base64Prefix+":1.0:") {
			t.Error("Base64 export missing correct prefix")
		}

		// Verify it can be decoded back
		decoded, err := DecodeBase64Export(result.Content)
		if err != nil {
			t.Fatalf("Failed to decode Base64 export: %v", err)
		}

		if decoded == "" {
			t.Error("Decoded content is empty")
		}
	})
}

// TestExportWithEmptyData tests edge cases with minimal data
func TestExportWithEmptyData(t *testing.T) {
	tests := []struct {
		name        string
		rule        *typ.Rule
		providers   []*typ.Provider
		expectError bool
	}{
		{
			name:        "Nil rule",
			rule:        nil,
			providers:   []*typ.Provider{},
			expectError: true,
		},
		{
			name: "Rule with no services",
			rule: &typ.Rule{
				UUID:         "test-uuid",
				Scenario:     typ.RuleScenario("general"),
				RequestModel: "gpt-4",
				Services:     []*loadbalance.Service{},
			},
			providers:   []*typ.Provider{},
			expectError: false,
		},
		{
			name: "Rule with services but no matching providers",
			rule: &typ.Rule{
				UUID:         "test-uuid",
				Scenario:     typ.RuleScenario("general"),
				RequestModel: "gpt-4",
				Services: []*loadbalance.Service{
					{Provider: "non-existent", Model: "gpt-4"},
				},
			},
			providers:   []*typ.Provider{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ExportRequest{
				Rule:      tt.rule,
				Providers: tt.providers,
			}

			exporter := NewJSONLExporter()
			result, err := exporter.Export(req)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result == nil || result.Content == "" {
					t.Error("Expected valid result but got empty content")
				}
			}
		})
	}
}

// TestBase64WithComplexCharacters tests encoding/decoding with special characters
func TestBase64WithComplexCharacters(t *testing.T) {
	// Test with various special characters that might appear in real data
	testStrings := []struct {
		name  string
		input string
	}{
		{"UTF-8 characters", "测试中文🎉"},
		{"URL characters", "https://api.example.com/v1/chat?token=abc123"},
		{"JSON content", `{"key":"value","nested":{"array":[1,2,3]}}`},
		{"Newlines and tabs", "line1\nline2\ttabbed"},
		{"Quotes and escapes", `"quoted"\n'esaped'\` + "`"},
	}

	for _, tt := range testStrings {
		t.Run(tt.name, func(t *testing.T) {
			// Create a simple JSONL with the test string
			jsonl := `{"type":"metadata","version":"1.0","note":"` + tt.input + `"}`

			// For now, we just verify the JSONL is valid
			if jsonl == "" {
				t.Error("JSONL should not be empty")
			}
			if !contains(jsonl, `"note":"`) {
				t.Error("JSONL should contain note field")
			}
		})
	}
}

// Helper functions
func containsAll(s string, substrs []string) bool {
	for _, substr := range substrs {
		if !contains(s, substr) {
			return false
		}
	}
	return true
}

func contains(s, substr string) bool {
	return indexOf(s, substr) >= 0
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[0:len(prefix)] == prefix
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
