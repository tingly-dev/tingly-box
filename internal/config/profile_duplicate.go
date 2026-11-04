package config

import (
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

// ProfileMainID addresses a scenario's main configuration where a profile id
// is expected: DuplicateProfile(base, ProfileMainID, …) creates a profile from
// the main scenario's rules (for claude_code, the main Claude Code config).
// "default" is a reserved profile name, so it never names a real profile.
const ProfileMainID = "default"

// DuplicateProfile creates a new profile of baseScenario as a copy of
// sourceID — or of the main configuration for ProfileMainID: all of its rules
// (routing, smart routing, flags, active state), its own scenario flags and
// its Claude Code overrides. The source is left untouched. An empty name picks
// a free "<source>-copy" name; an explicit name is validated like
// CreateProfile's.
func (c *Config) DuplicateProfile(baseScenario typ.RuleScenario, sourceID, name string) (typ.ProfileMeta, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	base := string(baseScenario)
	profiles := c.Profiles[base]

	var source typ.ProfileMeta
	srcScenario := baseScenario
	if sourceID == ProfileMainID {
		if d, ok := typ.GetScenarioDescriptor(baseScenario); !ok || !d.SupportsProfiles {
			return typ.ProfileMeta{}, fmt.Errorf("scenario '%s' has no profiles", base)
		}
		// The main config has no profile overrides; the copy inherits them
		// from it like any profile.
		source = typ.ProfileMeta{Name: ProfileMainID, Unified: true}
	} else {
		i := slices.IndexFunc(profiles, func(p typ.ProfileMeta) bool { return p.ID == sourceID })
		if i < 0 {
			return typ.ProfileMeta{}, fmt.Errorf("%w: '%s' in scenario '%s'", ErrProfileNotFound, sourceID, base)
		}
		source = profiles[i]
		srcScenario = typ.ProfiledScenarioName(baseScenario, sourceID)
	}

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

	undo, err := c.cloneScopeLocked(srcScenario, typ.ProfiledScenarioName(baseScenario, meta.ID))
	if err != nil {
		return typ.ProfileMeta{}, err
	}

	if c.Profiles == nil {
		c.Profiles = make(map[string][]typ.ProfileMeta)
	}
	c.Profiles[base] = append(slices.Clip(profiles), meta)
	if err := c.saveLocked(); err != nil {
		undo()
		c.Profiles[base] = profiles
		return typ.ProfileMeta{}, err
	}
	return cloneProfileMeta(meta), nil
}

// firstFree returns the first candidate(1), candidate(2), ... that ok accepts.
func firstFree(candidate func(n int) string, ok func(string) bool) string {
	for n := 1; ; n++ {
		if name := candidate(n); ok(name) {
			return name
		}
	}
}

// freeCopyName returns "<source>-copy", then "<source>-copy-2", ... — the
// first one that is a valid profile name and not taken. Characters a profile
// name cannot hold (legacy names) become '-'.
func freeCopyName(source string, profiles []typ.ProfileMeta) string {
	stem := strings.Trim(profileNameUnsafe.ReplaceAllString(source, "-"), "-")
	if stem == "" {
		stem = "profile"
	}
	return firstFree(
		func(n int) string {
			if n == 1 {
				return stem + "-copy"
			}
			return fmt.Sprintf("%s-copy-%d", stem, n)
		},
		func(name string) bool {
			return !slices.ContainsFunc(profiles, func(p typ.ProfileMeta) bool { return p.Name == name }) &&
				typ.ValidateProfileName(name) == nil
		},
	)
}
