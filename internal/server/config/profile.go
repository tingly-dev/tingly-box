package config

import (
	"fmt"
	"maps"
	"slices"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// --- Profile CRUD ---

// GetProfiles returns all profiles for a base scenario.
func (c *Config) GetProfiles(baseScenario typ.RuleScenario) []typ.ProfileMeta {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.Profiles == nil {
		return nil
	}
	profiles := c.Profiles[string(baseScenario)]
	if profiles == nil {
		return nil
	}
	result := make([]typ.ProfileMeta, len(profiles))
	for i := range profiles {
		result[i] = cloneProfileMeta(profiles[i])
	}
	return result
}

// GetProfile returns a single profile by base scenario and profile ID.
func (c *Config) GetProfile(baseScenario typ.RuleScenario, profileID string) (typ.ProfileMeta, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.Profiles == nil {
		return typ.ProfileMeta{}, false
	}
	profiles := c.Profiles[string(baseScenario)]
	for _, p := range profiles {
		if p.ID == profileID {
			return cloneProfileMeta(p), true
		}
	}
	return typ.ProfileMeta{}, false
}

func cloneProfileMeta(profile typ.ProfileMeta) typ.ProfileMeta {
	profile.ClaudeCode = cloneClaudeCodeProfileConfig(profile.ClaudeCode)
	return profile
}

func cloneClaudeCodeProfileConfig(profileConfig *typ.ClaudeCodeProfileConfig) *typ.ClaudeCodeProfileConfig {
	if profileConfig == nil {
		return nil
	}
	cloned := *profileConfig
	cloned.Env = maps.Clone(profileConfig.Env)
	cloned.UnsetEnv = slices.Clone(profileConfig.UnsetEnv)
	return &cloned
}

func hasClaudeCodeProfileOverrides(profileConfig *typ.ClaudeCodeProfileConfig) bool {
	return profileConfig != nil && (len(profileConfig.Env) > 0 ||
		len(profileConfig.UnsetEnv) > 0 || profileConfig.DefaultMode != "")
}

// UpdateClaudeCodeProfileConfig replaces the persistent Claude Code override
// delta for a profile. Passing nil clears all overrides and restores pure
// inheritance from the main Claude Code settings and profile routing rules.
func (c *Config) UpdateClaudeCodeProfileConfig(baseScenario typ.RuleScenario, profileID string, profileConfig *typ.ClaudeCodeProfileConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	profiles := c.Profiles[string(baseScenario)]
	for i := range profiles {
		if profiles[i].ID != profileID {
			continue
		}
		profiles[i].ClaudeCode = nil
		if hasClaudeCodeProfileOverrides(profileConfig) {
			profiles[i].ClaudeCode = cloneClaudeCodeProfileConfig(profileConfig)
		}
		c.Profiles[string(baseScenario)] = profiles
		return c.Save()
	}
	return fmt.Errorf("profile '%s' not found in scenario '%s'", profileID, baseScenario)
}

// newCCProfileRules builds fresh rules for a claude_code profile.
// unified=true → one rule "cc"; unified=false → five rules (default/haiku/sonnet/opus/subagent).
// Rules are empty (no services, no smart routing) for users to configure.
// UUIDs follow the modern built-in convention "builtin:<scenario>:<model>"
// (e.g. "builtin:claude_code:p1:haiku"), so profile rules stay addressable by
// a deterministic identity just like the main scenario's built-ins.
func newCCProfileRules(profiledScenario typ.RuleScenario, unified bool) []typ.Rule {
	newRule := func(requestModel, description string) typ.Rule {
		return typ.Rule{
			UUID:         BuiltinRuleUUID(profiledScenario, requestModel),
			Scenario:     profiledScenario,
			RequestModel: requestModel,
			Description:  description,
			LBTactic: typ.Tactic{
				Type:   loadbalance.TacticRandom,
				Params: typ.DefaultRandomParams(),
			},
			// Claude Code profiles inherit the built-in CC defaults: normalize the
			// mid-conversation system role (ClaudeCodeCompat) so third-party
			// providers accept the request, and strip the billing header
			// (CleanHeader) so it never leaks to external providers.
			Flags:  typ.RuleFlags{ClaudeCodeCompat: true, CleanHeader: true, SessionAffinity: defaultSessionAffinitySeconds},
			Active: true,
		}
	}

	if unified {
		return []typ.Rule{
			newRule("cc", "Claude Code profile - unified mode"),
		}
	}
	return []typ.Rule{
		newRule("default", "Claude Code profile - default model"),
		newRule("haiku", "Claude Code profile - haiku model"),
		newRule("sonnet", "Claude Code profile - sonnet model"),
		newRule("opus", "Claude Code profile - opus model"),
		newRule("subagent", "Claude Code profile - subagent model"),
	}
}

// CreateProfile adds a new profile to a base scenario. Returns the created ProfileMeta.
// The unified parameter determines whether to use unified mode (single model) or separate mode (individual models).
func (c *Config) CreateProfile(baseScenario typ.RuleScenario, name string, unified bool) (typ.ProfileMeta, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	base := string(baseScenario)
	id, err := c.nextProfileIDLocked(baseScenario, name)
	if err != nil {
		return typ.ProfileMeta{}, err
	}

	meta := typ.ProfileMeta{
		ID:      id,
		Name:    name,
		Unified: unified,
	}

	c.Profiles[base] = append(c.Profiles[base], meta)

	// Create fresh profile rules from the DefaultRules templates. For claude_code:
	// unified mode → one "cc" rule; separate mode → five individual model rules.
	// Rules start with empty Services/SmartRouting so the user configures the
	// upstream providers for the new profile explicitly.
	profiledScenario := typ.ProfiledScenarioName(baseScenario, meta.ID)
	if baseScenario == typ.ScenarioClaudeCode {
		c.Rules = append(c.Rules, newCCProfileRules(profiledScenario, unified)...)
	}

	return meta, c.Save()
}

// nextProfileIDLocked validates a new profile name for baseScenario and returns
// the ID the profile will get. Callers must hold c.mu.
func (c *Config) nextProfileIDLocked(baseScenario typ.RuleScenario, name string) (string, error) {
	base := string(baseScenario)

	// Constrain the name to a URL-friendly alias up front, so the profile is
	// always addressable as "/tingly/<base>:<name>" — not just by its ID.
	if err := typ.ValidateProfileName(name); err != nil {
		return "", err
	}

	if c.Profiles == nil {
		c.Profiles = make(map[string][]typ.ProfileMeta)
	}

	profiles := c.Profiles[base]

	// Validate name uniqueness within this scenario
	for _, p := range profiles {
		if p.Name == name {
			return "", fmt.Errorf("profile name '%s' already exists in scenario '%s'", name, base)
		}
	}

	// Generate next profile ID: find the first unused ID starting from 1.
	// This reuses IDs from deleted profiles instead of always incrementing the max.
	existingIDs := make([]string, len(profiles))
	for i, p := range profiles {
		existingIDs[i] = p.ID
	}
	return typ.NextFreeNumberedID("p", existingIDs), nil
}

// UpdateProfile updates the name of an existing profile.
// The unified parameter is accepted for API compatibility but ignored — mode is
// fixed at creation time. To switch modes, delete and recreate the profile.
func (c *Config) UpdateProfile(baseScenario typ.RuleScenario, profileID string, name string, unified *bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	base := string(baseScenario)
	if c.Profiles == nil {
		return fmt.Errorf("no profiles found for scenario '%s'", base)
	}

	profiles := c.Profiles[base]
	idx := -1
	for i, p := range profiles {
		if p.ID == profileID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("profile '%s' not found in scenario '%s'", profileID, base)
	}

	// Apply the URL-friendly constraint only when the name actually changes.
	// Editing a legacy profile (e.g. changing nothing but the mode) replays its
	// existing name through here; if that name predates the constraint we must
	// not reject the edit. A genuine rename, on the other hand, is a fresh write
	// and must not introduce a new non-routable name.
	if name != profiles[idx].Name {
		if err := typ.ValidateProfileName(name); err != nil {
			return err
		}
	}

	// Validate name uniqueness (excluding current profile)
	for i, p := range profiles {
		if i != idx && p.Name == name {
			return fmt.Errorf("profile name '%s' already exists in scenario '%s'", name, base)
		}
	}

	// Update fields
	profiles[idx].Name = name
	// Note: unified/separate mode is intentionally not updated here.
	// Mode is fixed at profile creation time; to switch, delete and recreate.
	// Accepting a unified flag change here would silently diverge the stored
	// metadata from the actual rules, which are not rebuilt by this function.

	return c.Save()
}

// DeleteProfile removes a profile by ID and cleans up all associated rules.
func (c *Config) DeleteProfile(baseScenario typ.RuleScenario, profileID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	base := string(baseScenario)
	if c.Profiles == nil {
		return fmt.Errorf("no profiles found for scenario '%s'", base)
	}

	profiles := c.Profiles[base]
	idx := -1
	for i, p := range profiles {
		if p.ID == profileID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("profile '%s' not found in scenario '%s'", profileID, base)
	}

	// Remove profile metadata
	c.Profiles[base] = append(profiles[:idx], profiles[idx+1:]...)
	if len(c.Profiles[base]) == 0 {
		delete(c.Profiles, base)
	}

	c.removeScenarioLocked(typ.ProfiledScenarioName(baseScenario, profileID))
	return c.Save()
}

// removeScenarioLocked drops every rule of scenario and its own scenario
// config — what a deleted profile or team leaves behind. Callers must hold
// c.mu. Reports whether anything was removed.
func (c *Config) removeScenarioLocked(scenario typ.RuleScenario) bool {
	nRules, nScenarios := len(c.Rules), len(c.Scenarios)
	c.Rules = slices.DeleteFunc(c.Rules, func(r typ.Rule) bool { return r.Scenario == scenario })
	c.Scenarios = slices.DeleteFunc(c.Scenarios, func(sc typ.ScenarioConfig) bool { return sc.Scenario == scenario })
	return len(c.Rules) != nRules || len(c.Scenarios) != nScenarios
}

// ResolveProfileNameOrID resolves a profile identifier to a profile ID.
// If the input matches an existing ID (e.g. "p1"), returns it directly.
// If the input matches an existing name, returns the corresponding ID.
func (c *Config) ResolveProfileNameOrID(baseScenario typ.RuleScenario, input string) (string, error) {
	if input == "" {
		return "", nil
	}

	// Direct ID match
	if _, ok := c.GetProfile(baseScenario, input); ok {
		return input, nil
	}

	// Name match
	profiles := c.GetProfiles(baseScenario)
	for _, p := range profiles {
		if p.Name == input {
			return p.ID, nil
		}
	}

	return "", fmt.Errorf("profile '%s' not found in scenario '%s'", input, baseScenario)
}

// ResolveProfileAlias resolves a URL-friendly profile alias to its canonical
// profile ID. The alias may be:
//   - the profile ID itself (e.g. "p1") — returned as-is, and
//   - a profile's human-readable name (e.g. "mine") — but only when that name
//     is a simple identifier safe to embed in a URL path segment.
//
// Profiles whose names contain spaces or other characters that don't parse
// cleanly out of a URL are intentionally not routable by name; callers must
// address those by ID. Returns ("", false) when the alias matches neither a
// known ID nor a simple, unique profile name.
func (c *Config) ResolveProfileAlias(baseScenario typ.RuleScenario, alias string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	profiles := c.Profiles[string(baseScenario)]

	// Direct ID match — already canonical, nothing to rewrite.
	for _, p := range profiles {
		if p.ID == alias {
			return alias, true
		}
	}

	// Otherwise resolve by name, but only for URL-friendly aliases (this also
	// rejects "", which IsSimpleProfileAlias reports false for). A stored name
	// equal to a simple alias is itself simple, so no per-name re-check needed.
	if !typ.IsSimpleProfileAlias(alias) {
		return "", false
	}
	for _, p := range profiles {
		if p.Name == alias {
			return p.ID, true
		}
	}

	return "", false
}
