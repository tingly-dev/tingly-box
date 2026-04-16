package core

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPolicyEnabledDefaultsToFalseWhenOmitted(t *testing.T) {
	var policy Policy
	if err := yaml.Unmarshal([]byte(`
id: p1
kind: content
match:
  patterns: ["secret"]
`), &policy); err != nil {
		t.Fatalf("yaml unmarshal policy: %v", err)
	}
	if policy.Enabled {
		t.Fatalf("expected YAML policy enabled=false when omitted")
	}

	policy = Policy{}
	if err := json.Unmarshal([]byte(`{"id":"p1","kind":"content","match":{"patterns":["secret"]}}`), &policy); err != nil {
		t.Fatalf("json unmarshal policy: %v", err)
	}
	if policy.Enabled {
		t.Fatalf("expected JSON policy enabled=false when omitted")
	}
}

func TestPolicyGroupEnabledDefaultsToFalseWhenOmitted(t *testing.T) {
	var group PolicyGroup
	if err := yaml.Unmarshal([]byte(`
id: default
name: Default
`), &group); err != nil {
		t.Fatalf("yaml unmarshal group: %v", err)
	}
	if group.Enabled {
		t.Fatalf("expected YAML group enabled=false when omitted")
	}

	group = PolicyGroup{}
	if err := json.Unmarshal([]byte(`{"id":"default","name":"Default"}`), &group); err != nil {
		t.Fatalf("json unmarshal group: %v", err)
	}
	if group.Enabled {
		t.Fatalf("expected JSON group enabled=false when omitted")
	}
}
