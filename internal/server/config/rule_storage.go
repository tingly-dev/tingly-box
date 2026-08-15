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
// Lifecycle:
//   - database has rules  -> database is authoritative; file rules ignored
//   - database empty, JSON has rules -> one-time import into the database
//   - both empty -> nothing to do (fresh install; built-ins arrive via AddRule)
//
// Transition period: unlike migrateProvidersToDB (which nulls the JSON copy),
// Save() keeps writing a live "rules" mirror into config.json so downgrading
// to a pre-database version loses nothing. The mirror is write-only; a later
// release removes it. See .design/rule-storage.md §5.

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
		// Database is authoritative. The file's rules array (Save()'s own
		// mirror, or a hand edit made while the server was down) is not an
		// input anymore; the next Save() rewrites it from the live rules.
		c.Rules = stored
		c.LegacyRules = nil
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

	// Save() persists the rules to the store; subsequent startups find the
	// database populated and take the database-authoritative path. The file
	// keeps its "rules" array (rewritten as a live mirror) for downgrade
	// compatibility during the transition period.
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
