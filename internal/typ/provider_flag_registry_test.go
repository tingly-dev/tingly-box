package typ

import (
	"reflect"
	"strings"
	"testing"
)

// TestProviderFlagRegistry_KeysMatchStructFields prevents the provider flag
// registry from drifting away from ProviderFlags — the same safety net
// TestRuleFlagRegistry_KeysMatchStructFields provides for rule flags.
func TestProviderFlagRegistry_KeysMatchStructFields(t *testing.T) {
	flagsType := reflect.TypeOf(ProviderFlags{})

	jsonTags := map[string]bool{}
	for i := 0; i < flagsType.NumField(); i++ {
		tag := flagsType.Field(i).Tag.Get("json")
		name := strings.SplitN(tag, ",", 2)[0]
		if name != "" && name != "-" {
			jsonTags[name] = true
		}
	}

	for _, spec := range ProviderFlagRegistry() {
		if !jsonTags[spec.Key] {
			t.Errorf("FlagSpec key %q has no matching json tag on ProviderFlags", spec.Key)
		}
	}
}

// TestProviderFlagRegistry_SpecsAreValid checks the metadata every spec must
// carry — the same contract RuleFlagRegistry specs are held to.
func TestProviderFlagRegistry_SpecsAreValid(t *testing.T) {
	allowedTypes := map[FlagValueType]bool{
		FlagTypeBool:       true,
		FlagTypeString:     true,
		FlagTypeEnum:       true,
		FlagTypeInt:        true,
		FlagTypeServiceRef: true,
		FlagTypeHeaders:    true,
	}
	for _, spec := range ProviderFlagRegistry() {
		if !allowedTypes[spec.Type] {
			t.Errorf("flag %q has unsupported value type %q", spec.Key, spec.Type)
		}
		if spec.Label == "" {
			t.Errorf("flag %q has empty label", spec.Key)
		}
		if spec.Description == "" {
			t.Errorf("flag %q has empty description", spec.Key)
		}
		// Provider flags have no scenario inheritance axis.
		if spec.Shared || spec.InheritanceMode != "" {
			t.Errorf("flag %q must not use the scenario-axis Shared/InheritanceMode fields", spec.Key)
		}
		// The headers control renders free-form rows — enum/suggestion
		// metadata has no meaning for it.
		if spec.Type == FlagTypeHeaders && (len(spec.Options) > 0 || len(spec.Suggestions) > 0) {
			t.Errorf("headers flag %q must not declare Options/Suggestions", spec.Key)
		}
	}
}

// TestProviderFlagRegistry_ExtraHeaders pins the first provider flag: present
// and rendered as the headers control.
func TestProviderFlagRegistry_ExtraHeaders(t *testing.T) {
	var found *FlagSpec
	specs := ProviderFlagRegistry()
	for i := range specs {
		if specs[i].Key == "extra_headers" {
			found = &specs[i]
			break
		}
	}
	if found == nil {
		t.Fatal("extra_headers missing from ProviderFlagRegistry")
	}
	if found.Type != FlagTypeHeaders {
		t.Errorf("extra_headers type = %q, want %q", found.Type, FlagTypeHeaders)
	}
}

// TestProviderFlagRegistry_KeysExistInRuleRegistry guards the shared
// vocabulary: the provider registry derives every spec but the description
// from the rule spec of the same key, so a description entry naming a key the
// rule registry does not have would be silently dropped.
func TestProviderFlagRegistry_KeysExistInRuleRegistry(t *testing.T) {
	ruleKeys := map[string]bool{}
	for _, spec := range RuleFlagRegistry() {
		ruleKeys[spec.Key] = true
	}
	for _, entry := range providerFlagDescriptions {
		if !ruleKeys[entry.Key] {
			t.Errorf("provider flag %q has no rule spec to derive its control from", entry.Key)
		}
	}
	if got, want := len(ProviderFlagRegistry()), len(providerFlagDescriptions); got != want {
		t.Errorf("ProviderFlagRegistry() returned %d specs, want %d", got, want)
	}
}

// TestProviderFlagRegistry_SharesRuleControlShape pins that the two surfaces
// render the same control for the same key — only the wording differs.
func TestProviderFlagRegistry_SharesRuleControlShape(t *testing.T) {
	ruleSpecs := map[string]FlagSpec{}
	for _, spec := range RuleFlagRegistry() {
		ruleSpecs[spec.Key] = spec
	}
	for _, spec := range ProviderFlagRegistry() {
		rule := ruleSpecs[spec.Key]
		if spec.Type != rule.Type {
			t.Errorf("flag %q type = %q, want the rule spec's %q", spec.Key, spec.Type, rule.Type)
		}
		if spec.Label != rule.Label {
			t.Errorf("flag %q label = %q, want the rule spec's %q", spec.Key, spec.Label, rule.Label)
		}
		if !reflect.DeepEqual(spec.Options, rule.Options) {
			t.Errorf("flag %q options = %v, want the rule spec's %v", spec.Key, spec.Options, rule.Options)
		}
		if spec.Description == rule.Description {
			t.Errorf("flag %q reuses the rule wording; provider flags need supply-side wording", spec.Key)
		}
	}
}

// TestProviderFlagRegistry_ExcludesRequestOnlyFlags documents the flags that
// deliberately have no supply-side level, so re-adding one is a conscious act.
func TestProviderFlagRegistry_ExcludesRequestOnlyFlags(t *testing.T) {
	// session_affinity / vision_proxy_service are resolved before an upstream
	// is picked; openai_endpoint_override duplicates Provider.OpenAIEndpointMode;
	// claude_org_id is Claude OAuth only while provider flags are api_key only.
	excluded := []string{"session_affinity", "vision_proxy_service", "openai_endpoint_override", "claude_org_id"}
	present := map[string]bool{}
	for _, spec := range ProviderFlagRegistry() {
		present[spec.Key] = true
	}
	for _, key := range excluded {
		if present[key] {
			t.Errorf("flag %q is exposed at the provider level but has no supply-side meaning", key)
		}
	}
}
