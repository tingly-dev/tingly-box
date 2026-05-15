package typ

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestRuleFlagRegistry_NotEmpty guards against accidentally clearing the
// registry — every release should expose at least the historical flags.
func TestRuleFlagRegistry_NotEmpty(t *testing.T) {
	specs := RuleFlagRegistry()
	if len(specs) == 0 {
		t.Fatal("RuleFlagRegistry() returned no specs")
	}
}

// TestRuleFlagRegistry_KnownKeys verifies the catalog still surfaces the
// flags the UI relies on. New flags can be added freely; removing one
// without coordinating with the frontend will break the catalog dialog.
func TestRuleFlagRegistry_KnownKeys(t *testing.T) {
	required := []string{
		"cursor_compat",
		"cursor_compat_auto",
		"skip_usage",
		"use_max_completion_tokens",
		"custom_user_agent",
	}

	present := map[string]bool{}
	for _, s := range RuleFlagRegistry() {
		present[s.Key] = true
	}

	for _, key := range required {
		if !present[key] {
			t.Errorf("RuleFlagRegistry missing required key %q", key)
		}
	}
}

// TestRuleFlagRegistry_KeysMatchStructFields prevents the registry from
// drifting away from RuleFlags. Every registry key must correspond to a
// JSON tag on the struct so the API contract stays consistent.
func TestRuleFlagRegistry_KeysMatchStructFields(t *testing.T) {
	flagsType := reflect.TypeOf(RuleFlags{})

	jsonTags := map[string]bool{}
	for i := 0; i < flagsType.NumField(); i++ {
		tag := flagsType.Field(i).Tag.Get("json")
		name := strings.SplitN(tag, ",", 2)[0]
		if name != "" && name != "-" {
			jsonTags[name] = true
		}
	}

	for _, spec := range RuleFlagRegistry() {
		if !jsonTags[spec.Key] {
			t.Errorf("FlagSpec key %q has no matching json tag on RuleFlags", spec.Key)
		}
	}
}

// TestRuleFlagRegistry_TypesAreValid ensures every flag declares one of the
// supported value types — otherwise the catalog dialog has no idea how to
// render it.
func TestRuleFlagRegistry_TypesAreValid(t *testing.T) {
	allowed := map[FlagValueType]bool{
		FlagTypeBool:   true,
		FlagTypeString: true,
	}
	for _, spec := range RuleFlagRegistry() {
		if !allowed[spec.Type] {
			t.Errorf("flag %q has unsupported value type %q", spec.Key, spec.Type)
		}
		if spec.Label == "" {
			t.Errorf("flag %q has empty label", spec.Key)
		}
		if spec.Description == "" {
			t.Errorf("flag %q has empty description", spec.Key)
		}
	}
}

// TestRuleFlagRegistry_JSONRoundTrip catches accidental json-tag changes
// that would break the wire format.
func TestRuleFlagRegistry_JSONRoundTrip(t *testing.T) {
	specs := RuleFlagRegistry()
	raw, err := json.Marshal(specs)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var decoded []FlagSpec
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(decoded) != len(specs) {
		t.Fatalf("round-trip length mismatch: %d -> %d", len(specs), len(decoded))
	}
	for i := range specs {
		if decoded[i].Key != specs[i].Key {
			t.Errorf("index %d key mismatch: %q != %q", i, decoded[i].Key, specs[i].Key)
		}
	}
}
