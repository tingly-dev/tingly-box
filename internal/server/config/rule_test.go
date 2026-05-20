package config

import (
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

func TestAddRule_DuplicateNameSameScenario(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	rule1 := typ.Rule{
		UUID:         "uuid-1",
		Scenario:     "openai",
		RequestModel: "gpt-4",
	}
	if err := cfg.AddRule(rule1); err != nil {
		t.Fatalf("first AddRule failed: %v", err)
	}

	rule2 := typ.Rule{
		UUID:         "uuid-2",
		Scenario:     "openai",
		RequestModel: "gpt-4",
	}
	err = cfg.AddRule(rule2)
	if err == nil {
		t.Fatal("expected error for duplicate name in same scenario, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestAddRule_DuplicateNameDifferentScenario(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	rule1 := typ.Rule{
		UUID:         "uuid-1",
		Scenario:     "openai",
		RequestModel: "gpt-4",
	}
	if err := cfg.AddRule(rule1); err != nil {
		t.Fatalf("first AddRule failed: %v", err)
	}

	// Same request_model but different scenario — must succeed
	rule2 := typ.Rule{
		UUID:         "uuid-2",
		Scenario:     "anthropic",
		RequestModel: "gpt-4",
	}
	if err := cfg.AddRule(rule2); err != nil {
		t.Errorf("AddRule with same name in different scenario should succeed, got: %v", err)
	}
}

func TestAddRule_DuplicateUUID(t *testing.T) {
	cfg, err := NewConfig(WithConfigDir(t.TempDir()))
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	rule1 := typ.Rule{
		UUID:         "uuid-1",
		Scenario:     "openai",
		RequestModel: "gpt-4",
	}
	if err := cfg.AddRule(rule1); err != nil {
		t.Fatalf("first AddRule failed: %v", err)
	}

	rule2 := typ.Rule{
		UUID:         "uuid-1", // same UUID, different model
		Scenario:     "openai",
		RequestModel: "gpt-3.5-turbo",
	}
	if err := cfg.AddRule(rule2); err == nil {
		t.Fatal("expected error for duplicate UUID, got nil")
	}
}
