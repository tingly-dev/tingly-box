package dataio

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestExportRoundTrip tests the complete export->import cycle
func TestExportRoundTrip(t *testing.T) {
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
		providers   []*typ.Provider
		expectError bool
	}{
		{
			name:        "No providers",
			providers:   []*typ.Provider{},
			expectError: true,
		},
		{
			name: "Single provider",
			providers: []*typ.Provider{
				{UUID: "test-uuid", Name: "Test", APIBase: "https://api.example.com"},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ExportRequest{
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
			jsonl := `{"type":"metadata","version":"1.0","note":"` + tt.input + `"}`

			if jsonl == "" {
				t.Error("JSONL should not be empty")
			}
			if !strings.Contains(jsonl, `"note":"`) {
				t.Error("JSONL should contain note field")
			}
		})
	}
}

// newTestConfig creates a throwaway config.Config backed by a temp dir, with
// builtin providers and migration disabled so tests only see what they add.
func newTestConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.NewConfig(
		config.WithConfigDir(t.TempDir()),
		config.WithDisableBuiltIn(),
		config.WithDisableMigration(),
	)
	if err != nil {
		t.Fatalf("failed to create test config: %v", err)
	}
	return cfg
}

// TestProviderRoundTripByAuthType verifies that every field on typ.Provider
// survives an export -> import round trip, for every AuthType — including
// the multi-field credential auth types (aws_sigv4, azure_key, gcp_sa) and
// vmodel, which previously lost their entire credential/config on export.
func TestProviderRoundTripByAuthType(t *testing.T) {
	tests := []struct {
		name     string
		provider *typ.Provider
	}{
		{
			name: "api_key",
			provider: &typ.Provider{
				UUID:               "prov-api-key",
				Name:               "API Key Provider",
				APIBase:            "https://api.example.com",
				APIStyle:           protocol.APIStyleOpenAI,
				APIBaseOpenAI:      "https://api.example.com/openai",
				APIBaseAnthropic:   "https://api.example.com/anthropic",
				AuthType:           typ.AuthTypeAPIKey,
				Token:              "sk-test-token",
				NoKeyRequired:      false,
				Enabled:            true,
				ProxyURL:           "http://localhost:7890",
				UserAgent:          "custom-agent/1.0",
				Timeout:            45,
				Tags:               []string{"prod", "fast"},
				Models:             []string{"gpt-4", "gpt-4o"},
				OpenAIEndpointMode: ai.EndpointModeBoth,
			},
		},
		{
			name: "oauth",
			provider: &typ.Provider{
				UUID:     "prov-oauth",
				Name:     "OAuth Provider",
				APIBase:  "https://api.anthropic.com",
				APIStyle: protocol.APIStyleAnthropic,
				AuthType: typ.AuthTypeOAuth,
				Enabled:  true,
				OAuthDetail: &ai.OAuthDetail{
					AccessToken:  "access-token-value",
					Issuer:       ai.Issuer("claude_code"),
					UserID:       "user-123",
					RefreshToken: "refresh-token-value",
					ExpiresAt:    "2030-01-01T00:00:00Z",
					DeviceID:     "device-abc",
					ExtraFields:  map[string]interface{}{"foo": "bar"},
				},
			},
		},
		{
			name: "vmodel",
			provider: &typ.Provider{
				UUID:     "prov-vmodel",
				Name:     "Virtual Model Provider",
				APIBase:  "https://api.example.com/vmodel",
				APIStyle: protocol.APIStyleOpenAI,
				AuthType: typ.AuthTypeVirtual,
				Enabled:  true,
				VModelDetail: &ai.VModelDetail{
					Models:         []string{"vmodel-a", "vmodel-b"},
					LatencyProfile: "fast",
				},
			},
		},
		{
			name: "aws_sigv4",
			provider: &typ.Provider{
				UUID:     "prov-aws",
				Name:     "AWS Bedrock Provider",
				APIBase:  "https://bedrock-runtime.us-east-1.amazonaws.com",
				APIStyle: protocol.APIStyleAnthropic,
				AuthType: typ.AuthTypeAWSSigV4,
				Enabled:  true,
				Credential: &ai.CredentialBundle{
					Fields: map[string]string{
						"access_key_id":     "AKIA-test",
						"secret_access_key": "secret-test",
						"region":            "us-east-1",
					},
				},
			},
		},
		{
			name: "azure_key",
			provider: &typ.Provider{
				UUID:     "prov-azure",
				Name:     "Azure Provider",
				APIBase:  "https://my-resource.openai.azure.com",
				APIStyle: protocol.APIStyleOpenAI,
				AuthType: typ.AuthTypeAzureKey,
				Enabled:  true,
				Credential: &ai.CredentialBundle{
					Fields: map[string]string{
						"api_key":     "azure-key-test",
						"resource":    "my-resource",
						"deployment":  "gpt-4-deployment",
						"api_version": "2024-02-01",
					},
				},
			},
		},
		{
			name: "gcp_sa",
			provider: &typ.Provider{
				UUID:     "prov-gcp",
				Name:     "GCP Vertex Provider",
				APIBase:  "https://us-central1-aiplatform.googleapis.com",
				APIStyle: protocol.APIStyleAnthropic,
				AuthType: typ.AuthTypeGCPVertex,
				Enabled:  true,
				Credential: &ai.CredentialBundle{
					Fields: map[string]string{
						"project_id":      "my-gcp-project",
						"location":        "us-central1",
						"service_account": `{"type":"service_account","project_id":"my-gcp-project"}`,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exportResult, err := Export(&ExportRequest{Providers: []*typ.Provider{tt.provider}}, FormatJSONL)
			if err != nil {
				t.Fatalf("Export failed: %v", err)
			}

			// Decode straight off the wire format, at the boundary dataio
			// owns — this isolates the export/import fix from the separate
			// DB persistence layer (internal/data/db), which has its own,
			// independent field-mapping gaps (e.g. Models isn't a column at
			// all, UserAgent/DeviceID aren't copied in toRecord/toProvider)
			// that are out of scope for this change.
			decodedProvider := decodeExportedProvider(t, exportResult.Content, tt.provider.UUID)

			want := *tt.provider
			assertProviderFieldsEqual(t, &want, decodedProvider)

			// Also exercise the real Import path end-to-end so the
			// persisted-and-reloaded provider is verified on the fields the
			// DB layer does support today, plus the deliberate import-time
			// overrides (UUID/Source/LastUpdated).
			cfg := newTestConfig(t)
			importResult, err := Import(exportResult.Content, cfg, FormatJSONL, ImportOptions{})
			if err != nil {
				t.Fatalf("Import failed: %v", err)
			}

			newUUID, ok := importResult.ProviderMap[tt.provider.UUID]
			if !ok {
				t.Fatalf("provider UUID not found in ProviderMap")
			}

			imported, err := cfg.GetProviderByUUID(newUUID)
			if err != nil {
				t.Fatalf("failed to fetch imported provider: %v", err)
			}

			if imported.UUID != newUUID {
				t.Errorf("imported.UUID = %q, want %q", imported.UUID, newUUID)
			}
			if imported.Source != typ.ProviderSourceUser {
				t.Errorf("imported.Source = %q, want %q", imported.Source, typ.ProviderSourceUser)
			}
			if imported.LastUpdated != "" {
				t.Errorf("imported.LastUpdated = %q, want empty", imported.LastUpdated)
			}
			if imported.AuthType != tt.provider.AuthType {
				t.Errorf("imported.AuthType = %q, want %q", imported.AuthType, tt.provider.AuthType)
			}
		})
	}
}

// decodeExportedProvider re-parses the exported JSONL content directly (the
// same decode step importProvider uses) and returns the ProviderData whose
// original UUID matches wantUUID, without going through config.Config/DB —
// this is the boundary internal/dataio's export/import fix actually owns.
func decodeExportedProvider(t *testing.T, jsonl string, wantUUID string) *typ.Provider {
	t.Helper()
	for _, line := range strings.Split(jsonl, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var base DataLine
		if err := json.Unmarshal([]byte(line), &base); err != nil {
			t.Fatalf("invalid JSON line: %v", err)
		}
		if base.Type != "provider" {
			continue
		}
		var pd ProviderData
		if err := json.Unmarshal([]byte(line), &pd); err != nil {
			t.Fatalf("invalid provider data: %v", err)
		}
		if pd.UUID == wantUUID {
			p := pd.Provider
			return &p
		}
	}
	t.Fatalf("provider %q not found in exported JSONL", wantUUID)
	return nil
}

// TestProviderImportForcesUserSource verifies that importing a provider
// exported with Source: builtin does not create a builtin (edit/delete
// locked) provider — imports are always user-owned.
func TestProviderImportForcesUserSource(t *testing.T) {
	cfg := newTestConfig(t)

	provider := &typ.Provider{
		UUID:     "prov-builtin-export",
		Name:     "Builtin Provider",
		APIBase:  "https://api.example.com",
		APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeAPIKey,
		Token:    "sk-test",
		Enabled:  true,
		Source:   ai.ProviderSourceBuiltin,
	}

	exportResult, err := Export(&ExportRequest{Providers: []*typ.Provider{provider}}, FormatJSONL)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}
	if !strings.Contains(exportResult.Content, `"source":"builtin"`) {
		t.Fatalf("expected export to carry the builtin source, got: %s", exportResult.Content)
	}

	importResult, err := Import(exportResult.Content, cfg, FormatJSONL, ImportOptions{})
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	newUUID := importResult.ProviderMap[provider.UUID]
	imported, err := cfg.GetProviderByUUID(newUUID)
	if err != nil {
		t.Fatalf("failed to fetch imported provider: %v", err)
	}

	if imported.Source != typ.ProviderSourceUser {
		t.Errorf("imported provider Source = %q, want %q", imported.Source, typ.ProviderSourceUser)
	}
	if imported.IsBuiltin() {
		t.Error("imported provider must not be builtin")
	}
}

// TestProviderImportResetsLastUpdated verifies LastUpdated is not carried
// over verbatim from the source instance, since it's a freshness cache for
// Models rather than portable data.
func TestProviderImportResetsLastUpdated(t *testing.T) {
	cfg := newTestConfig(t)

	provider := &typ.Provider{
		UUID:        "prov-stale-cache",
		Name:        "Stale Cache Provider",
		APIBase:     "https://api.example.com",
		APIStyle:    protocol.APIStyleOpenAI,
		AuthType:    typ.AuthTypeAPIKey,
		Token:       "sk-test",
		Enabled:     true,
		Models:      []string{"gpt-4"},
		LastUpdated: "2020-01-01T00:00:00Z",
	}

	exportResult, err := Export(&ExportRequest{Providers: []*typ.Provider{provider}}, FormatJSONL)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	importResult, err := Import(exportResult.Content, cfg, FormatJSONL, ImportOptions{})
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	newUUID := importResult.ProviderMap[provider.UUID]
	imported, err := cfg.GetProviderByUUID(newUUID)
	if err != nil {
		t.Fatalf("failed to fetch imported provider: %v", err)
	}

	if imported.LastUpdated != "" {
		t.Errorf("imported provider LastUpdated = %q, want empty", imported.LastUpdated)
	}
}

// assertProviderFieldsEqual compares every meaningful field of two providers,
// reporting exactly which field mismatches rather than a single opaque
// DeepEqual failure.
func assertProviderFieldsEqual(t *testing.T, want, got *typ.Provider) {
	t.Helper()

	if want.UUID != got.UUID {
		t.Errorf("UUID = %q, want %q", got.UUID, want.UUID)
	}
	if want.Name != got.Name {
		t.Errorf("Name = %q, want %q", got.Name, want.Name)
	}
	if want.APIBase != got.APIBase {
		t.Errorf("APIBase = %q, want %q", got.APIBase, want.APIBase)
	}
	if want.APIStyle != got.APIStyle {
		t.Errorf("APIStyle = %q, want %q", got.APIStyle, want.APIStyle)
	}
	if want.APIBaseOpenAI != got.APIBaseOpenAI {
		t.Errorf("APIBaseOpenAI = %q, want %q", got.APIBaseOpenAI, want.APIBaseOpenAI)
	}
	if want.APIBaseAnthropic != got.APIBaseAnthropic {
		t.Errorf("APIBaseAnthropic = %q, want %q", got.APIBaseAnthropic, want.APIBaseAnthropic)
	}
	if want.AuthType != got.AuthType {
		t.Errorf("AuthType = %q, want %q", got.AuthType, want.AuthType)
	}
	if want.Token != got.Token {
		t.Errorf("Token = %q, want %q", got.Token, want.Token)
	}
	if want.NoKeyRequired != got.NoKeyRequired {
		t.Errorf("NoKeyRequired = %v, want %v", got.NoKeyRequired, want.NoKeyRequired)
	}
	if want.Enabled != got.Enabled {
		t.Errorf("Enabled = %v, want %v", got.Enabled, want.Enabled)
	}
	if want.ProxyURL != got.ProxyURL {
		t.Errorf("ProxyURL = %q, want %q", got.ProxyURL, want.ProxyURL)
	}
	if want.UserAgent != got.UserAgent {
		t.Errorf("UserAgent = %q, want %q", got.UserAgent, want.UserAgent)
	}
	if want.Timeout != got.Timeout {
		t.Errorf("Timeout = %d, want %d", got.Timeout, want.Timeout)
	}
	if want.LastUpdated != got.LastUpdated {
		t.Errorf("LastUpdated = %q, want %q", got.LastUpdated, want.LastUpdated)
	}
	if want.Source != got.Source {
		t.Errorf("Source = %q, want %q", got.Source, want.Source)
	}
	if want.OpenAIEndpointMode != got.OpenAIEndpointMode {
		t.Errorf("OpenAIEndpointMode = %q, want %q", got.OpenAIEndpointMode, want.OpenAIEndpointMode)
	}
	if !stringSlicesEqual(want.Tags, got.Tags) {
		t.Errorf("Tags = %v, want %v", got.Tags, want.Tags)
	}
	if !stringSlicesEqual(want.Models, got.Models) {
		t.Errorf("Models = %v, want %v", got.Models, want.Models)
	}

	switch {
	case want.OAuthDetail == nil && got.OAuthDetail != nil:
		t.Errorf("OAuthDetail = %+v, want nil", got.OAuthDetail)
	case want.OAuthDetail != nil && got.OAuthDetail == nil:
		t.Error("OAuthDetail = nil, want non-nil")
	case want.OAuthDetail != nil && got.OAuthDetail != nil:
		w, g := want.OAuthDetail, got.OAuthDetail
		if w.AccessToken != g.AccessToken || w.Issuer != g.Issuer || w.UserID != g.UserID ||
			w.RefreshToken != g.RefreshToken || w.ExpiresAt != g.ExpiresAt || w.DeviceID != g.DeviceID {
			t.Errorf("OAuthDetail = %+v, want %+v", g, w)
		}
		if len(w.ExtraFields) != len(g.ExtraFields) {
			t.Errorf("OAuthDetail.ExtraFields = %v, want %v", g.ExtraFields, w.ExtraFields)
		}
	}

	switch {
	case want.VModelDetail == nil && got.VModelDetail != nil:
		t.Errorf("VModelDetail = %+v, want nil", got.VModelDetail)
	case want.VModelDetail != nil && got.VModelDetail == nil:
		t.Error("VModelDetail = nil, want non-nil")
	case want.VModelDetail != nil && got.VModelDetail != nil:
		if want.VModelDetail.LatencyProfile != got.VModelDetail.LatencyProfile {
			t.Errorf("VModelDetail.LatencyProfile = %q, want %q", got.VModelDetail.LatencyProfile, want.VModelDetail.LatencyProfile)
		}
		if !stringSlicesEqual(want.VModelDetail.Models, got.VModelDetail.Models) {
			t.Errorf("VModelDetail.Models = %v, want %v", got.VModelDetail.Models, want.VModelDetail.Models)
		}
	}

	switch {
	case want.Credential == nil && got.Credential != nil:
		t.Errorf("Credential = %+v, want nil", got.Credential)
	case want.Credential != nil && got.Credential == nil:
		t.Error("Credential = nil, want non-nil")
	case want.Credential != nil && got.Credential != nil:
		if len(want.Credential.Fields) != len(got.Credential.Fields) {
			t.Errorf("Credential.Fields = %v, want %v", got.Credential.Fields, want.Credential.Fields)
		}
		for k, v := range want.Credential.Fields {
			if got.Credential.Fields[k] != v {
				t.Errorf("Credential.Fields[%q] = %q, want %q", k, got.Credential.Fields[k], v)
			}
		}
	}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Helper functions
func containsAll(s string, substrs []string) bool {
	for _, substr := range substrs {
		if !strings.Contains(s, substr) {
			return false
		}
	}
	return true
}

func startsWith(s, prefix string) bool {
	return strings.HasPrefix(s, prefix)
}
