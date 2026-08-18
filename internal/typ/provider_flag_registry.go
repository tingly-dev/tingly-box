package typ

// ProviderFlagRegistry returns the catalog of flags configurable on a provider
// and its models — the FlagLevelProvider slice of the one flag definition in
// RuleFlagRegistry, not a second catalog. Deriving it means the same knob
// always renders the same control on both surfaces and the two cannot drift.
//
// The scenario-inheritance fields belong to the rule axis and are cleared, so
// the provider UI never offers a scenario default for a supply-side value.
func ProviderFlagRegistry() []FlagSpec {
	var specs []FlagSpec
	for _, spec := range RuleFlagRegistry() {
		if !spec.HasLevel(FlagLevelProvider) {
			continue
		}
		spec.Shared = false
		spec.InheritanceMode = ""
		specs = append(specs, spec)
	}
	return specs
}
