package config

import (
	"errors"
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func (c *Config) teamStore() (*db.TeamStore, error) {
	sm := c.StoreManager()
	if sm == nil || sm.Team() == nil {
		return nil, errors.New("team store is not available")
	}
	return sm.Team(), nil
}

// DuplicateTeam creates a new team as a copy of sourceID: the same routing
// rules and scenario flags under the new team's own scope. Sharing keys are
// credentials, not routing, and are never copied — the copy starts with none,
// and is always enabled. An empty name picks a free "<source> copy". The
// source is left untouched.
func (c *Config) DuplicateTeam(sourceID, name string) (*db.TeamRecord, error) {
	teams, err := c.teamStore()
	if err != nil {
		return nil, err
	}
	source, err := teams.Get(sourceID)
	if err != nil {
		return nil, err
	}
	if name == "" {
		name = freeTeamCopyName(source.Name, teams.List())
	}
	record, err := teams.Create(name)
	if err != nil {
		return nil, err
	}

	// The team row and its routing live in different stores and cannot be
	// saved atomically: if the routing copy fails, take the row back out so no
	// empty team is left behind.
	c.mu.Lock()
	undo, err := c.cloneScopeLocked(db.TeamScenario(source.ID), db.TeamScenario(record.ID))
	if err == nil {
		if err = c.saveLocked(); err != nil {
			undo()
		}
	}
	c.mu.Unlock()
	if err != nil {
		if delErr := teams.Delete(record.ID); delErr != nil {
			logrus.WithError(delErr).Warn("failed to remove the team of a failed duplicate")
		}
		return nil, err
	}
	return record, nil
}

// freeTeamCopyName returns "<source> copy", then "<source> copy 2", ... — the
// first name no team uses, trimmed to the store's length limit.
func freeTeamCopyName(source string, teams []db.TeamRecord) string {
	return firstFree(
		func(n int) string {
			suffix := " copy"
			if n > 1 {
				suffix = fmt.Sprintf(" copy %d", n)
			}
			stem := source
			for len(stem)+len(suffix) > db.MaxTeamNameBytes {
				_, size := utf8.DecodeLastRuneInString(stem)
				stem = stem[:len(stem)-size]
			}
			return stem + suffix
		},
		func(name string) bool {
			return !slices.ContainsFunc(teams, func(t db.TeamRecord) bool { return t.Name == name })
		},
	)
}

// DeleteTeam deletes a team and the routing that lives under it ("team:<id>"
// rules and scenario config). The team store decides whether the team may go
// (not the default team, no sharing keys left); only then is its routing
// removed, so a refused delete changes nothing.
func (c *Config) DeleteTeam(id string) error {
	teams, err := c.teamStore()
	if err != nil {
		return err
	}
	if err := teams.Delete(id); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.removeScopeLocked(db.TeamScenario(id)) {
		return nil
	}
	return c.saveLocked()
}

// dropOrphanTeamScopesOnce removes routing left behind by teams deleted before
// DeleteTeam cleaned up after itself. It runs once, not on every boot: the team
// store is a different database from the config, and a restored or recreated
// team DB would otherwise make every live team's routing look orphaned. The
// store always holds the default team, so an empty list means it was not read;
// then nothing is touched and the sweep stays pending.
func dropOrphanTeamScopesOnce(c *Config) bool {
	const marker = "20261104"
	if c.hasMigrationCompleted(marker) || c.storeManager == nil || c.storeManager.Team() == nil {
		return false
	}
	records := c.storeManager.Team().List()
	if len(records) == 0 {
		return false
	}
	c.markMigrationCompleted(marker)

	live := map[typ.RuleScenario]bool{}
	for _, team := range records {
		live[db.TeamScenario(team.ID)] = true
	}
	orphans := map[typ.RuleScenario]bool{}
	for _, r := range c.Rules {
		if r.Scenario.Base() == typ.ScenarioTeam && !live[r.Scenario] {
			orphans[r.Scenario] = true
		}
	}
	for _, sc := range c.Scenarios {
		if sc.Scenario.Base() == typ.ScenarioTeam && !live[sc.Scenario] {
			orphans[sc.Scenario] = true
		}
	}
	for scope := range orphans {
		c.removeScopeLocked(scope)
		logrus.WithField("scenario", scope).Info("Dropped routing of a deleted team")
	}
	return true
}
