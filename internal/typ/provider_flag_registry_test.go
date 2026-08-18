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

// TestProviderFlagRegistry_IsARuleRegistrySlice pins that the provider catalog
// is derived, not re-declared: every spec is the FlagLevelProvider entry of the
// same key in RuleFlagRegistry, unchanged apart from the cleared scenario axis.
func TestProviderFlagRegistry_IsARuleRegistrySlice(t *testing.T) {
	ruleSpecs := map[string]FlagSpec{}
	want := 0
	for _, spec := range RuleFlagRegistry() {
		ruleSpecs[spec.Key] = spec
		if spec.HasLevel(FlagLevelProvider) {
			want++
		}
	}

	specs := ProviderFlagRegistry()
	if len(specs) != want {
		t.Errorf("ProviderFlagRegistry() returned %d specs, want the %d provider-level rule specs", len(specs), want)
	}
	for _, spec := range specs {
		rule, ok := ruleSpecs[spec.Key]
		if !ok {
			t.Errorf("flag %q is not in RuleFlagRegistry", spec.Key)
			continue
		}
		// The scenario axis is a rule-side concept and must not leak through.
		rule.Shared, rule.InheritanceMode = false, ""
		if !reflect.DeepEqual(spec, rule) {
			t.Errorf("flag %q diverges from its rule spec:\n got %+v\nwant %+v", spec.Key, spec, rule)
		}
	}
}

// TestFlagRegistry_LevelsAreDeclared pins that every spec declares its levels
// and that the rule level is universal — a spec with no levels would vanish
// from every catalog.
func TestFlagRegistry_LevelsAreDeclared(t *testing.T) {
	known := map[FlagLevel]bool{FlagLevelProvider: true, FlagLevelModel: true, FlagLevelRule: true}
	for _, spec := range RuleFlagRegistry() {
		if !spec.HasLevel(FlagLevelRule) {
			t.Errorf("flag %q does not declare the rule level", spec.Key)
		}
		// Provider and model share one storage struct, so a flag settable at
		// one is settable at the other.
		if spec.HasLevel(FlagLevelProvider) != spec.HasLevel(FlagLevelModel) {
			t.Errorf("flag %q declares only one of the two supply levels: %v", spec.Key, spec.Levels)
		}
		for _, level := range spec.Levels {
			if !known[level] {
				t.Errorf("flag %q declares unknown level %q", spec.Key, level)
			}
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
