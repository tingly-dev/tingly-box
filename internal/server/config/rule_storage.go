package config

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/typ"
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

	// Consume the file's rules exactly once, here; the field must never
	// carry state past hydration (see its doc comment).
	legacy := c.LegacyRules
	c.LegacyRules = nil

	store := c.rulesStore()
	if store == nil {
		// No store (lightweight test configs): fall back to whatever the JSON
		// had so behavior degrades to the old in-memory semantics.
		c.Rules = legacy
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
		// A divergent mirror means rule changes were made outside this
		// version's control — most plausibly via an older (pre-database)
		// binary during a downgrade — and those changes are NOT merged back;
		// say so loudly instead of discarding them in silence.
		if len(legacy) > 0 && !rulesEquivalent(legacy, stored) {
			logrus.Warnf("config.json rules differ from the database (%d in file, %d in database); the database wins and the file copy will be overwritten — rule changes made under an older version or by hand are not merged back", len(legacy), len(stored))
		}
		c.Rules = stored
		return nil
	}

	if len(legacy) == 0 {
		// Fresh install: nothing to migrate.
		return nil
	}

	// One-time migration: import legacy JSON rules into the database.
	logrus.Infof("Migrating %d rule(s) from JSON config to database...", len(legacy))

	c.Rules = legacy

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

// syncRulesToStore writes the in-memory rule list through to the database
// and returns the JSON snapshot of the rules, which Save() reuses as the
// file mirror so the rules are marshaled exactly once per Save. Called from
// Save() so every existing rule-mutation path persists without individual
// call sites needing to know about the store. The store write is skipped
// when the rules did not change since the last sync (snapshot compare) or
// when no store is attached (lightweight test configs, closed stores) — the
// snapshot is still returned for the mirror. Returns (nil, nil) before
// hydration. ruleSyncMu serializes concurrent Save() calls (not all of them
// hold c.mu).
func (c *Config) syncRulesToStore() ([]byte, error) {
	c.ruleSyncMu.Lock()
	defer c.ruleSyncMu.Unlock()

	if !c.rulesHydrated {
		return nil, nil
	}

	snapshot, err := json.Marshal(c.Rules)
	if err != nil {
		return nil, fmt.Errorf("failed to snapshot rules: %w", err)
	}

	store := c.rulesStore()
	if store == nil || bytes.Equal(snapshot, c.lastSyncedRules) {
		return snapshot, nil
	}

	if err := store.SyncAll(c.Rules); err != nil {
		return nil, fmt.Errorf("failed to sync rules to store: %w", err)
	}
	c.lastSyncedRules = snapshot
	return snapshot, nil
}

// rulesEquivalent reports whether two rule lists carry the same content,
// compared through their JSON encoding (both sides are already-decoded
// domain values, so encoding differences the decoder normalizes cannot
// produce false negatives here).
func rulesEquivalent(a, b []typ.Rule) bool {
	if len(a) != len(b) {
		return false
	}
	aj, errA := json.Marshal(a)
	bj, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return bytes.Equal(aj, bj)
}
