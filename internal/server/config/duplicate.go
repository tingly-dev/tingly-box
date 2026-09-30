package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// MainScopeID addresses a scenario's main scope where a team/profile id is
// expected: the Default team for "team", the main configuration for
// "claude_code". See .design/duplicate.md.
const MainScopeID = "default"

// DuplicateResult identifies the copy Duplicate created.
type DuplicateResult struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Scenario typ.RuleScenario `json:"scenario"`
}

// ErrDuplicateSourceNotFound: the team or profile to copy does not exist.
type ErrDuplicateSourceNotFound struct{ Scenario, ID string }

func (e ErrDuplicateSourceNotFound) Error() string {
	return fmt.Sprintf("%s '%s' not found", e.Scenario, e.ID)
}

// Duplicate creates a new team ("team") or profile (scenarios with profiles)
// named name, as a copy of sub-scope id's rules and flags. Sharing keys are
// never copied.
func (c *Config) Duplicate(scenario typ.RuleScenario, id, name string) (DuplicateResult, error) {
	if scenario == typ.ScenarioTeam {
		return c.duplicateTeam(id, name)
	}
	if d, ok := typ.GetScenarioDescriptor(scenario); ok && d.SupportsProfiles {
		return c.duplicateProfile(scenario, id, name)
	}
	return DuplicateResult{}, fmt.Errorf("scenario '%s' has no teams or profiles to duplicate", scenario)
}

func (c *Config) duplicateTeam(sourceID, name string) (DuplicateResult, error) {
	if sourceID == MainScopeID {
		sourceID = db.DefaultTeamID
	}
	sm := c.StoreManager()
	if sm == nil || sm.Team() == nil {
		return DuplicateResult{}, errors.New("team store is not available")
	}
	teams := sm.Team()
	if _, err := teams.Get(sourceID); err != nil {
		return DuplicateResult{}, ErrDuplicateSourceNotFound{Scenario: string(typ.ScenarioTeam), ID: sourceID}
	}
	record, err := teams.Create(name)
	if err != nil {
		return DuplicateResult{}, err
	}

	dst := db.TeamScenario(record.ID)
	c.mu.Lock()
	undo, err := c.cloneScenarioLocked(db.TeamScenario(sourceID), dst, nil)
	if err == nil {
		if err = c.Save(); err != nil {
			undo()
		}
	}
	c.mu.Unlock()
	if err != nil {
		_ = teams.Delete(record.ID) // don't leave an empty team behind
		return DuplicateResult{}, err
	}
	return DuplicateResult{ID: record.ID, Name: record.Name, Scenario: dst}, nil
}

// duplicateProfile copies a profile — or, for MainScopeID, the main
// configuration, which keeps both Claude Code rule sets and toggles them by
// mode, so only the current mode's set (plus custom rules) is copied.
func (c *Config) duplicateProfile(base typ.RuleScenario, sourceID, name string) (DuplicateResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	src := base
	meta := typ.ProfileMeta{Name: name, Unified: true}
	var keep func(*typ.Rule) bool
	if sourceID == MainScopeID {
		if sc := c.scenarioConfigLocked(base); sc != nil {
			meta.Unified = !sc.GetDefaultFlags().Separate
		}
		if base == typ.ScenarioClaudeCode {
			inactive := claudeCodeSeparateRuleUUIDs
			if !meta.Unified {
				inactive = claudeCodeUnifiedRuleUUIDs
			}
			keep = func(r *typ.Rule) bool { return !inactive[r.UUID] }
		}
	} else {
		i := slices.IndexFunc(c.Profiles[string(base)], func(p typ.ProfileMeta) bool { return p.ID == sourceID })
		if i < 0 {
			return DuplicateResult{}, ErrDuplicateSourceNotFound{Scenario: string(base), ID: sourceID}
		}
		source := c.Profiles[string(base)][i]
		src = typ.ProfiledScenarioName(base, sourceID)
		meta.Unified = source.Unified
		meta.ClaudeCode = cloneClaudeCodeProfileConfig(source.ClaudeCode)
	}

	id, err := c.nextProfileIDLocked(base, name)
	if err != nil {
		return DuplicateResult{}, err
	}
	meta.ID = id
	dst := typ.ProfiledScenarioName(base, id)
	undo, err := c.cloneScenarioLocked(src, dst, keep)
	if err != nil {
		return DuplicateResult{}, err
	}
	profiles := c.Profiles[string(base)]
	c.Profiles[string(base)] = append(slices.Clip(profiles), meta)
	if err := c.Save(); err != nil {
		undo()
		c.Profiles[string(base)] = profiles
		return DuplicateResult{}, err
	}
	return DuplicateResult{ID: id, Name: name, Scenario: dst}, nil
}

// cloneScenarioLocked deep-copies src's rules (those keep accepts; all when
// nil) and src's own scenario config onto the empty scenario dst. Callers
// must hold c.mu, and call undo if the following Save fails — otherwise the
// next unrelated save would persist the half-made copy.
func (c *Config) cloneScenarioLocked(src, dst typ.RuleScenario, keep func(*typ.Rule) bool) (undo func(), err error) {
	if slices.ContainsFunc(c.Rules, func(r typ.Rule) bool { return r.Scenario == dst }) ||
		slices.ContainsFunc(c.Scenarios, func(sc typ.ScenarioConfig) bool { return sc.Scenario == dst }) {
		return nil, fmt.Errorf("scenario %q is not empty", dst)
	}

	var rules []typ.Rule
	for i := range c.Rules {
		r := &c.Rules[i]
		if r.Scenario != src || (keep != nil && !keep(r)) {
			continue
		}
		var cp typ.Rule
		if err := deepCopyJSON(r, &cp); err != nil {
			return nil, err
		}
		cp.Scenario = dst
		// Built-in rules are addressed by UUID — a profile's settings look
		// each tier up by builtin:<scenario>:<tier> — so keep that identity.
		if tier, ok := strings.CutPrefix(r.UUID, BuiltinRuleUUID(src, "")); ok {
			cp.UUID = BuiltinRuleUUID(dst, tier)
		} else {
			cp.UUID = GenerateUUID()
		}
		rules = append(rules, cp)
	}
	nRules, nScenarios := len(c.Rules), len(c.Scenarios)
	c.Rules = append(c.Rules, rules...)

	// When src is dst's base (Default team, main config), dst already
	// inherits its config live; a snapshot would freeze it.
	if dst.Base() != src {
		if i := slices.IndexFunc(c.Scenarios, func(sc typ.ScenarioConfig) bool { return sc.Scenario == src }); i >= 0 {
			var cp typ.ScenarioConfig
			if err := deepCopyJSON(c.Scenarios[i], &cp); err != nil {
				c.Rules = c.Rules[:nRules]
				return nil, err
			}
			cp.Scenario = dst
			c.Scenarios = append(c.Scenarios, cp)
		}
	}
	return func() {
		c.Rules = c.Rules[:nRules]
		c.Scenarios = c.Scenarios[:nScenarios]
	}, nil
}

func deepCopyJSON(src, dst any) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}
