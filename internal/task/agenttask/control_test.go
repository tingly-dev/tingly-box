package agenttask

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/task"
)

func TestSafeControlInputRedactsSensitiveFields(t *testing.T) {
	input := safeControlInput(map[string]any{
		"command": "go test ./...",
		"headers": map[string]any{
			"Authorization": "Bearer private",
			"x-api-token":   "private",
		},
		"password": "private",
	})
	text := string(input)
	if strings.Contains(text, "private") || !strings.Contains(text, "go test ./...") {
		t.Fatalf("sanitized input = %s", text)
	}
	var decoded map[string]any
	if err := json.Unmarshal(input, &decoded); err != nil {
		t.Fatalf("sanitized input is invalid JSON: %v", err)
	}
}

func TestValidateDecisionUsesTypedValidationError(t *testing.T) {
	err := validateDecision(task.ControlKindApproval, ControlDecision{Action: "always_allow"})
	if !errors.Is(err, ErrInvalidControlDecision) {
		t.Fatalf("error = %v", err)
	}
	if err := validateDecision(task.ControlKindQuestion, ControlDecision{Action: "answer"}); !errors.Is(err, ErrInvalidControlDecision) {
		t.Fatalf("empty answer error = %v", err)
	}
}
