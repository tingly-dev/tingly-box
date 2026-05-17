package probe

import (
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

func TestValidateE2ERequest(t *testing.T) {
	tests := []struct {
		name    string
		req     E2ERequest
		wantErr string // empty = expect no error; non-empty = expect this Field in the ValidationError
	}{
		{
			name: "rule target ok",
			req: E2ERequest{
				TargetType: E2ETargetRule,
				Scenario:   "anthropic",
				RuleUUID:   "rule-1",
				TestMode:   E2EModeSimple,
			},
		},
		{
			name: "rule target missing scenario",
			req: E2ERequest{
				TargetType: E2ETargetRule,
				RuleUUID:   "rule-1",
				TestMode:   E2EModeSimple,
			},
			wantErr: "scenario",
		},
		{
			name: "rule target missing rule_uuid",
			req: E2ERequest{
				TargetType: E2ETargetRule,
				Scenario:   "anthropic",
				TestMode:   E2EModeSimple,
			},
			wantErr: "rule_uuid",
		},
		{
			name: "provider target ok",
			req: E2ERequest{
				TargetType:   E2ETargetProvider,
				ProviderUUID: "p-1",
				Model:        "gpt-4",
				TestMode:     E2EModeStreaming,
			},
		},
		{
			name: "provider target missing provider_uuid",
			req: E2ERequest{
				TargetType: E2ETargetProvider,
				Model:      "gpt-4",
				TestMode:   E2EModeSimple,
			},
			wantErr: "provider_uuid",
		},
		{
			name: "provider target missing model",
			req: E2ERequest{
				TargetType:   E2ETargetProvider,
				ProviderUUID: "p-1",
				TestMode:     E2EModeSimple,
			},
			wantErr: "model",
		},
		{
			name: "provider_config target ok",
			req: E2ERequest{
				TargetType: E2ETargetProviderConfig,
				APIBase:    "https://api.openai.com/v1",
				APIStyle:   "openai",
				Token:      "sk-x",
				TestMode:   E2EModeTool,
			},
		},
		{
			name: "provider_config missing token",
			req: E2ERequest{
				TargetType: E2ETargetProviderConfig,
				APIBase:    "https://api.openai.com/v1",
				APIStyle:   "openai",
				TestMode:   E2EModeSimple,
			},
			wantErr: "token",
		},
		{
			name: "unknown target type",
			req: E2ERequest{
				TargetType: E2ETarget("nope"),
				TestMode:   E2EModeSimple,
			},
			wantErr: "target_type",
		},
		{
			name: "unknown test mode",
			req: E2ERequest{
				TargetType:   E2ETargetProvider,
				ProviderUUID: "p-1",
				Model:        "m",
				TestMode:     E2EMode("bogus"),
			},
			wantErr: "test_mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateE2ERequest(&tt.req)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateE2ERequest unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateE2ERequest expected error for field %q, got nil", tt.wantErr)
			}
			ve, ok := err.(*ValidationError)
			if !ok {
				t.Fatalf("ValidateE2ERequest returned %T, want *ValidationError", err)
			}
			if ve.Field != tt.wantErr {
				t.Errorf("ValidationError.Field = %q, want %q", ve.Field, tt.wantErr)
			}
		})
	}
}

func TestE2EMessage(t *testing.T) {
	if got := E2EMessage(E2EModeSimple, "custom!"); got != "custom!" {
		t.Errorf("custom override ignored: got %q", got)
	}
	if got := E2EMessage(E2EModeTool, ""); !strings.Contains(got, "bash tool") {
		t.Errorf("tool default should mention bash tool, got %q", got)
	}
	if got := E2EMessage(E2EModeSimple, ""); got == "" || strings.Contains(got, "bash tool") {
		t.Errorf("simple default should be a greeting, got %q", got)
	}
}

func TestScenarioEndpoint(t *testing.T) {
	tests := []struct {
		scenario string
		wantPath string
		wantAPI  protocol.APIStyle
	}{
		{"anthropic", "/tingly/anthropic", protocol.APIStyleAnthropic},
		{"claude_code", "/tingly/claude_code", protocol.APIStyleAnthropic},
		{"opencode", "/tingly/opencode", protocol.APIStyleAnthropic},
		{"openai", "/tingly/openai", protocol.APIStyleOpenAI},
		{"unknown-scenario", "/tingly/unknown-scenario", protocol.APIStyleOpenAI},
	}
	for _, tt := range tests {
		t.Run(tt.scenario, func(t *testing.T) {
			gotPath, gotAPI := ScenarioEndpoint(tt.scenario)
			if gotPath != tt.wantPath {
				t.Errorf("ScenarioEndpoint(%q) path = %q, want %q", tt.scenario, gotPath, tt.wantPath)
			}
			if gotAPI != tt.wantAPI {
				t.Errorf("ScenarioEndpoint(%q) apiStyle = %q, want %q", tt.scenario, gotAPI, tt.wantAPI)
			}
		})
	}
}

func TestValidationErrorMessage(t *testing.T) {
	ve := &ValidationError{Field: "scenario", Message: "scenario is required"}
	if got := ve.Error(); got != "scenario: scenario is required" {
		t.Errorf("ValidationError.Error() = %q", got)
	}
}
