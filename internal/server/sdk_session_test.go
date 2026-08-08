package server

import (
	"encoding/json"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestScenarioTransportLabel(t *testing.T) {
	cases := []struct {
		name      string
		transport []typ.ScenarioTransport
		want      string
	}{
		{"both", []typ.ScenarioTransport{typ.TransportOpenAI, typ.TransportAnthropic}, "both"},
		{"anthropic-only", []typ.ScenarioTransport{typ.TransportAnthropic}, "anthropic"},
		{"openai-only", []typ.ScenarioTransport{typ.TransportOpenAI}, "openai"},
		{"embed-falls-to-openai", []typ.ScenarioTransport{typ.TransportEmbed}, "openai"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scenarioTransportLabel(typ.ScenarioDescriptor{SupportedTransport: tc.transport})
			if got != tc.want {
				t.Fatalf("scenarioTransportLabel(%v) = %q, want %q", tc.transport, got, tc.want)
			}
		})
	}
}

func TestBindableScenarioIDsIncludesExperiment(t *testing.T) {
	ids := bindableScenarioIDs()
	found := false
	for _, id := range ids {
		if id == string(typ.ScenarioExperiment) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected experiment scenario in bindable list, got %v", ids)
	}
}

// TestSDKSessionResponseShape freezes the JSON field names the Python SDK
// depends on. If a field is renamed here without updating the SDK, this fails.
func TestSDKSessionResponseShape(t *testing.T) {
	resp := SDKSessionResponse{
		BaseURL:   "http://127.0.0.1:12580/tingly/experiment",
		Token:     "tok",
		Scenario:  "experiment",
		Transport: "both",
		Ready:     true,
		Services:  2,
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"base_url", "token", "scenario", "transport", "ready", "services"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("response JSON missing %q field; have %v", key, m)
		}
	}
	// expires_at is omitempty and absent here
	if _, ok := m["expires_at"]; ok {
		t.Fatalf("expires_at should be omitted when empty")
	}
}
