package toolloop

import (
	"errors"
	"testing"
)

func TestWrapErrorPreservesCommittedSideEffects(t *testing.T) {
	providerErr := errors.New("provider failed")

	if got := WrapError(providerErr, false); !errors.Is(got, providerErr) || HasCommittedSideEffects(got) {
		t.Fatalf("uncommitted error = %#v", got)
	}

	got := WrapError(providerErr, true)
	if !errors.Is(got, providerErr) {
		t.Fatalf("wrapped error does not preserve cause: %v", got)
	}
	if !HasCommittedSideEffects(got) {
		t.Fatal("wrapped error lost committed side-effect state")
	}
}

func TestValidateDefinitionsRejectsEmptyAndDuplicateNames(t *testing.T) {
	if err := validateDefinitions([]ToolDefinition{{}}); err == nil {
		t.Fatal("empty tool name was accepted")
	}
	if err := validateDefinitions([]ToolDefinition{{Name: "lookup"}, {Name: "lookup"}}); err == nil {
		t.Fatal("duplicate tool name was accepted")
	}
	if err := validateDefinitions([]ToolDefinition{{Name: "lookup"}, {Name: "calculate"}}); err != nil {
		t.Fatalf("valid definitions rejected: %v", err)
	}
}
