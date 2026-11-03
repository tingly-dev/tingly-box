package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ErrProfileNotFound is wrapped by errors for a profile that does not exist.
var ErrProfileNotFound = errors.New("profile not found")

var profileNameUnsafe = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// DuplicateProfile creates a new profile of baseScenario as a copy of
// sourceID: all of its rules (routing, smart routing, flags, active state),
// its own scenario flags and its Claude Code overrides. The source is left
// untouched. An empty name picks a free "<source>-copy" name; an explicit name
// is validated like CreateProfile's.
func (c *Config) DuplicateProfile(baseScenario typ.RuleScenario, sourceID, name string) (typ.ProfileMeta, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	base := string(baseScenario)
	profiles := c.Profiles[base]
	i := slices.IndexFunc(profiles, func(p typ.ProfileMeta) bool { return p.ID == sourceID })
	if i < 0 {
		return typ.ProfileMeta{}, fmt.Errorf("%w: '%s' in scenario '%s'", ErrProfileNotFound, sourceID, base)
	}
	source := profiles[i]

	if name == "" {
		name = freeCopyName(source.Name, profiles)
	} else {
		if err := typ.ValidateProfileName(name); err != nil {
			return typ.ProfileMeta{}, err
		}
		if slices.ContainsFunc(profiles, func(p typ.ProfileMeta) bool { return p.Name == name }) {
			return typ.ProfileMeta{}, fmt.Errorf("profile name '%s' already exists in scenario '%s'", name, base)
		}
	}

	ids := make([]string, len(profiles))
	for i, p := range profiles {
		ids[i] = p.ID
	}
	meta := cloneProfileMeta(source)
	meta.ID = typ.NextFreeNumberedID("p", ids)
	meta.Name = name

	srcScenario := typ.ProfiledScenarioName(baseScenario, sourceID)
	dstScenario := typ.ProfiledScenarioName(baseScenario, meta.ID)

	nRules, nScenarios := len(c.Rules), len(c.Scenarios)
	rollback := func() {
		c.Rules = c.Rules[:nRules]
		c.Scenarios = c.Scenarios[:nScenarios]
	}

	var rules []typ.Rule
	for k := range c.Rules {
		r := &c.Rules[k]
		if r.Scenario != srcScenario {
			continue
		}
		var cp typ.Rule
		if err := deepCopyJSON(r, &cp); err != nil {
			return typ.ProfileMeta{}, err
		}
		cp.Scenario = dstScenario
		// Built-in rules are addressed by UUID (builtin:<scenario>:<slot>);
		// the copy must keep that shape under its own scenario.
		if slot, ok := strings.CutPrefix(r.UUID, BuiltinRuleUUID(srcScenario, "")); ok {
			cp.UUID = BuiltinRuleUUID(dstScenario, slot)
		} else {
			cp.UUID = GenerateUUID()
		}
		rules = append(rules, cp)
	}
	c.Rules = append(c.Rules, rules...)

	// Only the profile's own scenario config is copied: when it has none, the
	// copy keeps inheriting the base scenario's live config like the source.
	if k := slices.IndexFunc(c.Scenarios, func(sc typ.ScenarioConfig) bool { return sc.Scenario == srcScenario }); k >= 0 {
		var cp typ.ScenarioConfig
		if err := deepCopyJSON(c.Scenarios[k], &cp); err != nil {
			rollback()
			return typ.ProfileMeta{}, err
		}
		cp.Scenario = dstScenario
		c.Scenarios = append(c.Scenarios, cp)
	}

	if c.Profiles == nil {
		c.Profiles = make(map[string][]typ.ProfileMeta)
	}
	c.Profiles[base] = append(slices.Clip(profiles), meta)
	if err := c.saveLocked(); err != nil {
		rollback()
		c.Profiles[base] = profiles
		return typ.ProfileMeta{}, err
	}
	return cloneProfileMeta(meta), nil
}

// freeCopyName returns "<source>-copy", then "<source>-copy-2", ... — the
// first one that is a valid profile name and not taken. Characters a profile
// name cannot hold (legacy names) become '-'.
func freeCopyName(source string, profiles []typ.ProfileMeta) string {
	stem := strings.Trim(profileNameUnsafe.ReplaceAllString(source, "-"), "-")
	if stem == "" {
		stem = "profile"
	}
	for n := 1; ; n++ {
		candidate := stem + "-copy"
		if n > 1 {
			candidate = fmt.Sprintf("%s-copy-%d", stem, n)
		}
		taken := slices.ContainsFunc(profiles, func(p typ.ProfileMeta) bool { return p.Name == candidate })
		if !taken && typ.ValidateProfileName(candidate) == nil {
			return candidate
		}
	}
}

func deepCopyJSON(src, dst any) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}
