package config

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/db"
)

// Rule storage: rules live in SQLite (db.RuleStore) with Config.Rules as the
// in-memory working set. See .design/rule-storage.md for the full rationale.
//
// The lifecycle mirrors migrateProvidersToDB:
//   - database has rules  -> database is authoritative, stale JSON is cleared
//   - database empty, JSON has rules -> one-time import, then JSON cleared
//   - both empty -> nothing to do (fresh install; built-ins arrive via AddRule)

// rulesStore resolves the rule store at call time from the StoreManager, so
// store liveness has a single owner: after StoreManager.Close() the accessor
// returns nil and rule syncing degrades to a no-op instead of failing on a
// closed connection. Reads the field directly (not via c.StoreManager()) —
// callers may already hold c.mu, and the field is only ever set during
// construction.
func (c *Config) rulesStore() *db.RuleStore {
	if c.storeManager == nil {
		return nil
	}
	return c.storeManager.Rules()
}

// hydrateRulesFromStore populates Config.Rules at startup, migrating legacy
// config.json rules into the database the first time it runs on an upgraded
// install. It must run before Migrate() so migration steps see the real rules.
func (c *Config) hydrateRulesFromStore() error {
	c.rulesHydrated = true

	store := c.rulesStore()
	if store == nil {
		// No store (lightweight test configs): fall back to whatever the JSON
		// had so behavior degrades to the old in-memory semantics.
		c.Rules = c.LegacyRules
		c.LegacyRules = nil
		return nil
	}

	stored, err := store.List()
	if err != nil {
		return fmt.Errorf("failed to load rules from store: %w", err)
	}

	if len(stored) > 0 {
		// Database is authoritative.
		c.Rules = stored
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
		return nil
	}

	if len(c.LegacyRules) == 0 {
		// Fresh install: nothing to migrate.
		return nil
	}

	// One-time migration: import legacy JSON rules into the database.
	logrus.Infof("Migrating %d rule(s) from JSON config to database...", len(c.LegacyRules))

	c.Rules = c.LegacyRules
	c.LegacyRules = nil

	// The store keys rules by UUID; repair empty/duplicate UUIDs before the
	// first sync so no legacy rule is dropped (same policy normalizeRuleBasics
	// applies on every startup).
	ensureRuleUUIDs(c.Rules)

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
// when no store is attached (lightweight test configs, closed stores).
// ruleSyncMu serializes concurrent Save() calls (not all of them hold c.mu).
func (c *Config) syncRulesToStore() error {
	c.ruleSyncMu.Lock()
	defer c.ruleSyncMu.Unlock()

	store := c.rulesStore()
	if store == nil || !c.rulesHydrated {
		return nil
	}

	snapshot, err := json.Marshal(c.Rules)
	if err != nil {
		return fmt.Errorf("failed to snapshot rules: %w", err)
	}
	if bytes.Equal(snapshot, c.lastSyncedRules) {
		return nil
	}

	if err := store.SyncAll(c.Rules); err != nil {
		return fmt.Errorf("failed to sync rules to store: %w", err)
	}
	c.lastSyncedRules = snapshot
	return nil
}
