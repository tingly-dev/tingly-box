package transform

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// TestScenarioFlags_NotLeakedToJSON verifies that ScenarioFlags are not serialized
// to the JSON request body sent to upstream providers.
func TestScenarioFlags_NotLeakedToJSON(t *testing.T) {
	// Create a sample request
	req := newOpenAIRequest("gpt-4", 1024)
	req.StreamOptions = openai.ChatCompletionStreamOptionsParam{
		IncludeUsage: param.Opt[bool]{Value: true},
	}

	// Create scenario flags
	flags := &typ.ScenarioFlags{
		DisableStreamUsage: true,
		SessionAffinity:    1800,
	}

	// Create transform context
	ctx := NewTransformContext(req,
		WithScenarioFlags(flags),
		WithStreaming(true),
	)

	// Apply consistency transform
	ct := NewConsistencyTransform(protocol.TypeOpenAIChat)
	err := ct.Apply(ctx)
	if err != nil {
		t.Fatalf("Error applying transform: %v", err)
	}

	// Marshal the request to JSON
	jsonBytes, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Error marshaling: %v", err)
	}

	// Check if scenario_flags is in the JSON
	jsonStr := string(jsonBytes)
	if contains(jsonStr, "scenario_flags") {
		t.Errorf("scenario_flags found in JSON (leaking to upstream): %s", jsonStr)
	}

	// Verify flags were applied (DisableStreamUsage should have turned off IncludeUsage)
	if req.StreamOptions.IncludeUsage.Value {
		t.Error("IncludeUsage should be false after applying DisableStreamUsage flag")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
