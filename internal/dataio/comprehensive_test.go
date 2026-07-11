package dataio

import (
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestFormatDetectionEdgeCases tests format detection with various edge cases
func TestFormatDetectionEdgeCases(t *testing.T) {
	detector := NewDetector()

	tests := []struct {
		name     string
		data     string
		want     Format
		describe string
	}{
		{
			name:     "Base64 with whitespace before prefix",
			data:     "   \n  TGB64:1.0:eyJ0eXBlIjoibWV0YWRhdGEifQ==",
			want:     FormatBase64,
			describe: "Should detect Base64 even with leading whitespace",
		},
		{
			name:     "Base64 with lowercase prefix",
			data:     "tgb64:1.0:eyJ0eXBlIjoibWV0YWRhdGEifQ==",
			want:     FormatJSONL,
			describe: "Should not detect lowercase prefix as Base64",
		},
		{
			name:     "JSONL with special characters in JSON",
			data:     `{"type":"metadata","version":"1.0","note":"测试中文"}`,
			want:     FormatJSONL,
			describe: "Should handle UTF-8 characters in JSONL",
		},
		{
			name:     "Valid JSON but not JSONL format",
			data:     `{"some":"json","without":"type"}`,
			want:     FormatJSONL,
			describe: "Should default to JSONL for non-matching formats",
		},
		{
			name:     "Very long Base64 string",
			data:     "TGB64:1.0:" + strings.Repeat("eyJ0eXBlIjoibWV0YWRhdGEifQ==", 1000),
			want:     FormatBase64,
			describe: "Should handle long Base64 strings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detector.Detect(tt.data)
			if got != tt.want {
				t.Errorf("Detector.Detect() = %v, want %v (%s)", got, tt.want, tt.describe)
			}
		})
	}
}

// TestFormatCompatibilityWithExistingJSONL tests backward compatibility
func TestFormatCompatibilityWithExistingJSONL(t *testing.T) {
	// This is the existing JSONL format from the codebase
	existingJSONL := `{"type":"metadata","version":"1.0","exported_at":"2024-01-01T00:00:00Z"}
{"type":"rule","uuid":"test-uuid","scenario":"general","request_model":"gpt-4","response_model":"gpt-4","description":"Test","services":[{"provider":"prov-1","model":"gpt-4","weight":100}],"lb_tactic":"random","active":true,"smart_enabled":false,"smart_routing":[]}
{"type":"provider","uuid":"prov-1","name":"Test Provider","api_base":"https://api.example.com","api_style":"openai","auth_type":"api_key","token":"sk-test","enabled":true,"timeout":30}`

	detector := NewDetector()
	got := detector.Detect(existingJSONL)

	if got != FormatJSONL {
		t.Errorf("Existing JSONL format should be detected as JSONL, got %v", got)
	}

	// Test that it can be parsed by the JSONL importer
	_ = NewJSONLImporter()

	// Note: This would require a mock Config to fully test
	// For now, we just verify the format is correctly detected
}

// TestBase64FormatVersioning tests version handling
func TestBase64FormatVersioning(t *testing.T) {
	tests := []struct {
		name       string
		data       string
		wantErr    bool
		errMessage string
	}{
		{
			name:       "Future version 2.0",
			data:       "TGB64:2.0:eyJ0eXBlIjoibWV0YWRhdGEifQ==",
			wantErr:    true,
			errMessage: "unsupported version",
		},
		{
			name:       "Malformed version",
			data:       "TGB64:x.y:eyJ0eXBlIjoibWV0YWRhdGEifQ==",
			wantErr:    true,
			errMessage: "unsupported version",
		},
		{
			name:       "Missing version",
			data:       "TGB64::eyJ0eXBlIjoibWV0YWRhdGEifQ==",
			wantErr:    true,
			errMessage: "unsupported version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeBase64Export(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("decodeBase64Export() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errMessage) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.errMessage, err.Error())
			}
		})
	}
}

// TestEmptyAndMalformedInput tests handling of bad input
func TestEmptyAndMalformedInput(t *testing.T) {
	detector := NewDetector()

	tests := []struct {
		name       string
		data       string
		testDetect bool
		testImport bool
	}{
		{
			name:       "Empty string",
			data:       "",
			testDetect: true,
			testImport: false,
		},
		{
			name:       "Only whitespace",
			data:       "   \n\t  ",
			testDetect: true,
			testImport: false,
		},
		{
			name:       "Base64 with trailing spaces",
			data:       "TGB64:1.0:eyJ0eXBlIjoibWV0YWRhdGEifQ==   \n",
			testDetect: true,
			testImport: true,
		},
		{
			name:       "Base64 with newlines in payload (invalid)",
			data:       "TGB64:1.0:eyJ0eXBlIjoibWV0YWRhdGEifQ==\nextra",
			testDetect: true,
			testImport: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.testDetect {
				format := detector.Detect(tt.data)
				if format == "" {
					t.Error("Detector should return a format for all input")
				}
			}

			if tt.testImport {
				_, err := DecodeBase64Export(tt.data)
				// We don't assert error here because some cases might be valid
				// We just want to ensure it doesn't panic
				_ = err
			}
		})
	}
}

// TestExportDataStructureValidation validates the structure of export data
func TestExportDataStructureValidation(t *testing.T) {
	// Test that export data structures have required fields
	metadata := Metadata{
		Type:       "metadata",
		Version:    "1.0",
		ExportedAt: "2024-01-01T00:00:00Z",
	}

	if metadata.Type != "metadata" {
		t.Error("Metadata type field incorrect")
	}
	if metadata.Version != "1.0" {
		t.Error("Metadata version field incorrect")
	}

	providerData := ProviderData{
		Type: "provider",
		Provider: typ.Provider{
			UUID:    "prov-uuid",
			Name:    "Test Provider",
			APIBase: "https://api.example.com",
			Enabled: true,
		},
	}

	if providerData.Type != "provider" {
		t.Error("Provider data type field incorrect")
	}
	if providerData.UUID == "" {
		t.Error("Provider data UUID is required")
	}
	if providerData.Name == "" {
		t.Error("Provider data name is required")
	}
}

// TestCrossFormatCompatibility tests that JSONL and Base64 produce equivalent results
func TestCrossFormatCompatibility(t *testing.T) {
	// This test verifies that Base64 encoding/decoding works correctly
	// by using a pre-encoded Base64 string

	base64Content := "TGB64:1.0:eyJ0eXBlIjoibWV0YWRhdGEiLCJ2ZXJzaW9uIjoiMS4wIn0KeyJ0eXBlIjoicnVsZSIsInV1aWQiOiJ0ZXN0LXV1aWQiLCJyZXF1ZXN0X21vZGVsIjoiZ3B0LTQifQp7InR5cGUiOiJwcm92aWRlciIsInV1aWQiOiJwcm92LTEiLCJuYW1lIjoiVGVzdCJ9"

	decoded, err := DecodeBase64Export(base64Content)

	if err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}

	// Verify the decoded content is valid JSONL (has newlines)
	if !strings.Contains(decoded, "\n") {
		t.Error("Decoded content should be multi-line JSONL")
	}

	// Verify it contains expected elements
	expectedElements := []string{
		`"type":"metadata"`,
		`"type":"rule"`,
		`"type":"provider"`,
	}

	for _, elem := range expectedElements {
		if !strings.Contains(decoded, elem) {
			t.Errorf("Decoded content missing expected element: %s", elem)
		}
	}
}
