package config

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// Rule storage: rules live in SQLite (db.RuleStore) with Config.Rules as the
// in-memory working set. See .design/rule-storage.md for the full rationale.
//
// The lifecycle mirrors migrateProvidersToDB:
//   - database has rules  -> database is authoritative, stale JSON is cleared
//   - database empty, JSON has rules -> one-time import, then JSON cleared
//   - both empty -> nothing to do (fresh install; built-ins arrive via AddRule)

// hydrateRulesFromStore populates Config.Rules at startup, migrating legacy
// config.json rules into the database the first time it runs on an upgraded
// install. It must run before Migrate() so date migrations see the real rules.
func (c *Config) hydrateRulesFromStore() error {
	if c.ruleStore == nil {
		// No store (lightweight test configs): fall back to whatever the JSON
		// had so behavior degrades to the old in-memory semantics.
		c.Rules = c.LegacyRules
		c.LegacyRules = nil
		c.rulesHydrated = true
		return nil
	}

	count, err := c.ruleStore.Count()
	if err != nil {
		return fmt.Errorf("failed to check rule count: %w", err)
	}

	if count > 0 {
		// Database is authoritative.
		rules, err := c.ruleStore.List()
		if err != nil {
			return fmt.Errorf("failed to load rules from store: %w", err)
		}
		c.Rules = rules
		c.rememberSyncedRules()
		c.rulesHydrated = true

		if len(c.LegacyRules) > 0 {
			// Stale JSON backup left over from an earlier version (or a hand
			// edit while the server was down). The database wins; clear the
			// file copy so the two cannot diverge silently.
			logrus.Infof("Clearing stale rule JSON data (%d rule(s)); database is authoritative", len(c.LegacyRules))
			c.LegacyRules = nil
			if err := c.Save(); err != nil {
				return fmt.Errorf("failed to save config after clearing rule JSON: %w", err)
			}
		}
		c.LegacyRules = nil
		return nil
	}

	if len(c.LegacyRules) == 0 {
		// Fresh install: nothing to migrate.
		c.rulesHydrated = true
		return nil
	}

	// One-time migration: import legacy JSON rules into the database.
	logrus.Infof("Migrating %d rule(s) from JSON config to database...", len(c.LegacyRules))

	c.Rules = c.LegacyRules
	c.LegacyRules = nil

	// Rules need a UUID to be keyed in the store. Very old configs may carry
	// rules without one; assign here exactly like normalizeRuleBasics would
	// (random UUID), so migration order does not change the outcome.
	for i := range c.Rules {
		if c.Rules[i].UUID != "" {
			continue
		}
		if uid, err := uuid.NewUUID(); err == nil {
			c.Rules[i].UUID = uid.String()
		}
	}

	c.rulesHydrated = true

	// Save() persists the rules to the store and rewrites config.json with
	// "rules": null so subsequent startups take the database path.
	if err := c.Save(); err != nil {
		return fmt.Errorf("failed to migrate rules to database: %w", err)
	}

	logrus.Infof("Successfully migrated %d rule(s) to database", len(c.Rules))
	return nil
}

// syncRulesToStore writes the in-memory rule list through to the database.
// Called from Save() so every existing rule-mutation path persists without
// individual call sites needing to know about the store. No-ops when the
// rules did not change since the last sync (cheap JSON snapshot compare) or
// when no store is attached (lightweight test configs).
func (c *Config) syncRulesToStore() error {
	if c.ruleStore == nil || !c.rulesHydrated {
		return nil
	}

	snapshot, err := json.Marshal(c.Rules)
	if err != nil {
		return fmt.Errorf("failed to snapshot rules: %w", err)
	}
	if bytes.Equal(snapshot, c.lastSyncedRules) {
		return nil
	}

	if err := c.ruleStore.SyncAll(c.Rules); err != nil {
		return fmt.Errorf("failed to sync rules to store: %w", err)
	}
	c.lastSyncedRules = snapshot
	return nil
}

// rememberSyncedRules primes the change-detection snapshot after hydrating
// from the store, so the first Save() doesn't re-write identical rows.
func (c *Config) rememberSyncedRules() {
	if snapshot, err := json.Marshal(c.Rules); err == nil {
		c.lastSyncedRules = snapshot
	}
}

// RuleStoreCount is a small test/diagnostic helper reporting how many rules
// the database currently holds. Returns 0 when no store is attached.
func (c *Config) RuleStoreCount() int64 {
	if c.ruleStore == nil {
		return 0
	}
	count, err := c.ruleStore.Count()
	if err != nil {
		return 0
	}
	return count
}
