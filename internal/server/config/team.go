package config

import (
	"errors"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// DeleteTeam deletes a team and the routing that lives under it
// ("team:<id>" rules and scenario config). The team store decides whether the
// team may go (not the default team, no sharing keys left); only then is its
// routing removed, so a refused delete changes nothing.
func (c *Config) DeleteTeam(id string) error {
	sm := c.StoreManager()
	if sm == nil || sm.Team() == nil {
		return errors.New("team store is not available")
	}
	if err := sm.Team().Delete(id); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.removeScenarioLocked(db.TeamScenario(id)) {
		return nil
	}
	return c.Save()
}

// dropOrphanTeamScopes removes routing left behind by teams that no longer
// exist — deleted before DeleteTeam cleaned up after itself, or whose cleanup
// save failed. The team store is authoritative and loaded before migrations.
// Baseline: runs every boot and is a no-op once nothing is orphaned.
func dropOrphanTeamScopes(c *Config) bool {
	sm := c.storeManager
	if sm == nil || sm.Team() == nil {
		return false
	}
	live := map[typ.RuleScenario]bool{}
	for _, team := range sm.Team().List() {
		live[db.TeamScenario(team.ID)] = true
	}
	orphans := map[typ.RuleScenario]bool{}
	for _, r := range c.Rules {
		if r.Scenario.Base() == typ.ScenarioTeam && r.Scenario != typ.ScenarioTeam && !live[r.Scenario] {
			orphans[r.Scenario] = true
		}
	}
	for _, sc := range c.Scenarios {
		if sc.Scenario.Base() == typ.ScenarioTeam && sc.Scenario != typ.ScenarioTeam && !live[sc.Scenario] {
			orphans[sc.Scenario] = true
		}
	}
	for scenario := range orphans {
		c.removeScenarioLocked(scenario)
		logrus.WithField("scenario", scenario).Info("Dropped routing of a deleted team")
	}
	return len(orphans) > 0
}
