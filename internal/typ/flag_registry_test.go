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
		"clean_header",
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
		FlagTypeBool:       true,
		FlagTypeString:     true,
		FlagTypeEnum:       true,
		FlagTypeInt:        true,
		FlagTypeServiceRef: true,
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

// TestRuleFlagRegistry_EnumOptions asserts that every FlagTypeEnum spec
// declares at least two options with non-empty Labels. The first option's
// Value may be empty (it acts as the inactive/"By Client" sentinel that
// `omitempty` hides on the wire); subsequent options must carry a value.
func TestRuleFlagRegistry_EnumOptions(t *testing.T) {
	for _, spec := range RuleFlagRegistry() {
		if spec.Type != FlagTypeEnum {
			continue
		}
		if len(spec.Options) < 2 {
			t.Errorf("enum flag %q has %d options, expected at least 2", spec.Key, len(spec.Options))
		}
		seen := map[string]bool{}
		for i, opt := range spec.Options {
			if opt.Value == "" && i != 0 {
				t.Errorf("enum flag %q option %d has empty Value (only the first option may be empty as the inactive sentinel)", spec.Key, i)
			}
			if opt.Label == "" {
				t.Errorf("enum flag %q option %d has empty Label", spec.Key, i)
			}
			if seen[opt.Value] {
				t.Errorf("enum flag %q has duplicate option Value %q", spec.Key, opt.Value)
			}
			seen[opt.Value] = true
		}
	}
}

// TestRuleFlagRegistry_SharedFlagsHaveInheritanceMode verifies that every flag
// marked Shared declares an InheritanceMode and vice versa.
func TestRuleFlagRegistry_SharedFlagsHaveInheritanceMode(t *testing.T) {
	for _, spec := range RuleFlagRegistry() {
		if spec.Shared && spec.InheritanceMode == "" {
			t.Errorf("shared flag %q must declare InheritanceMode", spec.Key)
		}
		if !spec.Shared && spec.InheritanceMode != "" {
			t.Errorf("non-shared flag %q should not declare InheritanceMode", spec.Key)
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

// TestRuleFlagRegistry_VisionProxyServiceRef pins the vision_proxy_service
// flag to the service_ref type so the frontend keeps rendering it as a model
// picker (not a text field).
func TestRuleFlagRegistry_VisionProxyServiceRef(t *testing.T) {
	var found *FlagSpec
	for i, s := range RuleFlagRegistry() {
		if s.Key == "vision_proxy_service" {
			found = &RuleFlagRegistry()[i]
			break
		}
	}
	if found == nil {
		t.Fatal("vision_proxy_service missing from registry")
	}
	if found.Type != FlagTypeServiceRef {
		t.Fatalf("vision_proxy_service type = %q, want %q", found.Type, FlagTypeServiceRef)
	}
}

// TestDefaultUserAgents_NonEmptyAndWellFormed checks that the curated preset
// list is populated and that every entry carries both a friendly label and a
// concrete User-Agent value (so the UI can render a labelled quick-pick).
func TestDefaultUserAgents_NonEmptyAndWellFormed(t *testing.T) {
	uas := DefaultUserAgents()
	if len(uas) == 0 {
		t.Fatal("DefaultUserAgents() returned no presets")
	}
	seen := map[string]bool{}
	for i, ua := range uas {
		if ua.Label == "" {
			t.Errorf("preset %d has empty Label", i)
		}
		if ua.Value == "" {
			t.Errorf("preset %d (%q) has empty Value", i, ua.Label)
		}
		if seen[ua.Value] {
			t.Errorf("duplicate preset User-Agent value %q", ua.Value)
		}
		seen[ua.Value] = true
	}
}

// TestRuleFlagRegistry_CustomUserAgentSuggestions pins the custom_user_agent
// flag to surface the curated presets so the frontend can offer them as a
// quick-pick while still allowing free-form input.
func TestRuleFlagRegistry_CustomUserAgentSuggestions(t *testing.T) {
	var found *FlagSpec
	for i, s := range RuleFlagRegistry() {
		if s.Key == "custom_user_agent" {
			found = &RuleFlagRegistry()[i]
			break
		}
	}
	if found == nil {
		t.Fatal("custom_user_agent missing from registry")
	}
	if found.Type != FlagTypeString {
		t.Fatalf("custom_user_agent type = %q, want %q", found.Type, FlagTypeString)
	}
	if len(found.Suggestions) == 0 {
		t.Error("custom_user_agent should expose User-Agent suggestions")
	}
}

// TestRuleFlags_VisionProxyService_JSONRoundTrip guards the wire shape of the
// rule-level vision proxy service: a configured {provider, model} survives a
// marshal/unmarshal cycle, and an unset pointer stays absent (omitempty).
func TestRuleFlags_VisionProxyService_JSONRoundTrip(t *testing.T) {
	flags := RuleFlags{VisionProxyService: &VisionProxyService{Provider: "p-uuid", Model: "claude-3-5-sonnet"}}
	raw, err := json.Marshal(flags)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"vision_proxy_service"`) ||
		!strings.Contains(string(raw), `"p-uuid"`) ||
		!strings.Contains(string(raw), `"claude-3-5-sonnet"`) {
		t.Fatalf("marshaled flags missing fields: %s", raw)
	}

	var back RuleFlags
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.VisionProxyService == nil {
		t.Fatal("VisionProxyService lost on round-trip")
	}
	if back.VisionProxyService.Provider != "p-uuid" || back.VisionProxyService.Model != "claude-3-5-sonnet" {
		t.Fatalf("round-trip mismatch: %+v", back.VisionProxyService)
	}

	// Unset pointer must be omitted entirely (omitempty), so an empty rule
	// stays "vision proxy off".
	empty, err := json.Marshal(RuleFlags{})
	if err != nil {
		t.Fatalf("marshal empty: %v", err)
	}
	if strings.Contains(string(empty), "vision_proxy_service") {
		t.Fatalf("unset VisionProxyService should be omitted, got: %s", empty)
	}
}
