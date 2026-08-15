package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// writeLegacyConfig writes a minimal pre-migration config.json carrying rules
// in the legacy JSON array, the way installs stored them before the database
// became authoritative.
func writeLegacyConfig(t *testing.T, dir string, rules []typ.Rule) {
	t.Helper()
	payload := map[string]interface{}{
		"rules":              rules,
		"default_request_id": 0,
		"user_token":         "test-user-token",
		"model_token":        "test-model-token",
		"jwt_secret":         "test-secret",
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal legacy config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0600); err != nil {
		t.Fatalf("failed to write legacy config: %v", err)
	}
}

func legacyTestRule(uuid, requestModel string) typ.Rule {
	return typ.Rule{
		UUID:          uuid,
		Scenario:      typ.ScenarioOpenAI,
		RequestModel:  requestModel,
		ResponseModel: "resp-model",
		Description:   "legacy rule",
		Active:        true,
		Services: []*loadbalance.Service{
			{Provider: "prov-legacy", Model: "m-legacy", Active: true},
		},
		LBTactic: typ.Tactic{Type: loadbalance.TacticRandom, Params: typ.DefaultRandomParams()},
	}
}

// readConfigJSON parses the on-disk config.json into a generic map.
func readConfigJSON(t *testing.T, dir string) map[string]interface{} {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("failed to read config.json: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("failed to parse config.json: %v", err)
	}
	return m
}

func findRule(rules []typ.Rule, uuid string) *typ.Rule {
	for i := range rules {
		if rules[i].UUID == uuid {
			return &rules[i]
		}
	}
	return nil
}

func findRuleByModel(rules []typ.Rule, requestModel string) *typ.Rule {
	for i := range rules {
		if rules[i].RequestModel == requestModel {
			return &rules[i]
		}
	}
	return nil
}

// seedLegacyProvider registers the provider that legacyTestRule services
// reference; AddRule/UpdateRule validate that referenced providers exist and
// are enabled.
func seedLegacyProvider(t *testing.T, cfg *Config) {
	t.Helper()
	if err := cfg.StoreManager().Provider().Save(&typ.Provider{
		UUID:    "prov-legacy",
		Name:    "legacy-provider",
		APIBase: "https://example.invalid/v1",
		Enabled: true,
	}); err != nil {
		t.Fatalf("failed to seed provider: %v", err)
	}
}

func TestRulesMigrateFromLegacyJSONToStore(t *testing.T) {
	dir := t.TempDir()
	writeLegacyConfig(t, dir, []typ.Rule{
		legacyTestRule("legacy-1", "model-one"),
		legacyTestRule("legacy-2", "model-two"),
	})

	cfg, err := NewConfigWithDir(dir)
	if err != nil {
		t.Fatalf("NewConfigWithDir failed: %v", err)
	}
	defer cfg.CloseStores()

	// Legacy rules must survive into the working set.
	if findRule(cfg.Rules, "legacy-1") == nil || findRule(cfg.Rules, "legacy-2") == nil {
		t.Fatalf("legacy rules missing after migration; have %d rules", len(cfg.Rules))
	}

	// The store must now hold every in-memory rule (legacy + built-ins).
	count, err := cfg.StoreManager().Rules().Count()
	if err != nil {
		t.Fatalf("failed to count stored rules: %v", err)
	}
	if want := int64(len(cfg.Rules)); count != want {
		t.Errorf("store has %d rules, want %d", count, want)
	}

	// Round-trip a fat field through the store.
	stored, err := cfg.StoreManager().Rules().GetByUUID("legacy-1")
	if err != nil {
		t.Fatalf("legacy-1 not in store: %v", err)
	}
	if len(stored.Services) != 1 || stored.Services[0].Provider != "prov-legacy" {
		t.Errorf("services did not survive migration: %+v", stored.Services)
	}

	// Transition period: config.json keeps a live "rules" mirror for
	// downgrade compatibility. It must reflect the full working set
	// (legacy + built-ins), not the pre-migration snapshot.
	mirror, ok := readConfigJSON(t, dir)["rules"].([]interface{})
	if !ok {
		t.Fatalf("config.json rules mirror missing after migration")
	}
	if len(mirror) != len(cfg.Rules) {
		t.Errorf("rules mirror has %d entries, want %d (live mirror of working set)", len(mirror), len(cfg.Rules))
	}
}

func TestRulesReloadFromStoreOnRestart(t *testing.T) {
	dir := t.TempDir()
	writeLegacyConfig(t, dir, []typ.Rule{legacyTestRule("legacy-1", "model-one")})

	cfg, err := NewConfigWithDir(dir)
	if err != nil {
		t.Fatalf("first NewConfigWithDir failed: %v", err)
	}

	seedLegacyProvider(t, cfg)

	// Mutate through the normal API so the change flows through Save().
	updated := *findRule(cfg.Rules, "legacy-1")
	if updated.UUID == "" {
		t.Fatal("legacy-1 missing after first start")
	}
	updated.Description = "edited between restarts"
	if err := cfg.UpdateRule("legacy-1", updated); err != nil {
		t.Fatalf("UpdateRule failed: %v", err)
	}
	firstCount := len(cfg.Rules)
	if err := cfg.CloseStores(); err != nil {
		t.Fatalf("CloseStores failed: %v", err)
	}

	// Restart: rules must come back from the database, not the file.
	cfg2, err := NewConfigWithDir(dir)
	if err != nil {
		t.Fatalf("second NewConfigWithDir failed: %v", err)
	}
	defer cfg2.CloseStores()

	if len(cfg2.Rules) != firstCount {
		t.Errorf("restart changed rule count: %d -> %d", firstCount, len(cfg2.Rules))
	}
	got := findRule(cfg2.Rules, "legacy-1")
	if got == nil {
		t.Fatal("legacy-1 missing after restart")
	}
	if got.Description != "edited between restarts" {
		t.Errorf("edit lost across restart: %q", got.Description)
	}
}

func TestRulesStoreWinsOverStaleJSON(t *testing.T) {
	dir := t.TempDir()
	writeLegacyConfig(t, dir, []typ.Rule{legacyTestRule("legacy-1", "model-one")})

	cfg, err := NewConfigWithDir(dir)
	if err != nil {
		t.Fatalf("first NewConfigWithDir failed: %v", err)
	}
	if err := cfg.CloseStores(); err != nil {
		t.Fatalf("CloseStores failed: %v", err)
	}

	// Simulate a hand edit while the server is down: put a different rule set
	// back into config.json. The database must still win.
	raw := readConfigJSON(t, dir)
	raw["rules"] = []typ.Rule{legacyTestRule("hand-edited", "sneaky-model")}
	data, _ := json.MarshalIndent(raw, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0600); err != nil {
		t.Fatalf("failed to rewrite config.json: %v", err)
	}

	cfg2, err := NewConfigWithDir(dir)
	if err != nil {
		t.Fatalf("second NewConfigWithDir failed: %v", err)
	}
	defer cfg2.CloseStores()

	if findRule(cfg2.Rules, "hand-edited") != nil {
		t.Error("hand-edited JSON rule leaked into the working set; database should be authoritative")
	}
	if findRule(cfg2.Rules, "legacy-1") == nil {
		t.Error("database rule lost when stale JSON was present")
	}

	// The startup Save rewrites the file mirror from the database-backed
	// working set, so the hand edit disappears from the file too.
	raw2, _ := json.Marshal(readConfigJSON(t, dir)["rules"])
	if bytes.Contains(raw2, []byte("hand-edited")) {
		t.Error("hand-edited rule still present in the file mirror after restart")
	}
	if !bytes.Contains(raw2, []byte("legacy-1")) {
		t.Error("file mirror does not reflect the database rules after restart")
	}
}

func TestRuleCRUDWritesThroughToStore(t *testing.T) {
	dir := t.TempDir()
	cfg, err := NewConfigWithDir(dir)
	if err != nil {
		t.Fatalf("NewConfigWithDir failed: %v", err)
	}
	defer cfg.CloseStores()

	store := cfg.StoreManager().Rules()
	seedLegacyProvider(t, cfg)

	rule := legacyTestRule("crud-1", "crud-model")
	if err := cfg.AddRule(rule); err != nil {
		t.Fatalf("AddRule failed: %v", err)
	}
	if _, err := store.GetByUUID("crud-1"); err != nil {
		t.Fatalf("AddRule did not reach the store: %v", err)
	}

	rule.Description = "updated"
	if err := cfg.UpdateRule("crud-1", rule); err != nil {
		t.Fatalf("UpdateRule failed: %v", err)
	}
	stored, err := store.GetByUUID("crud-1")
	if err != nil {
		t.Fatalf("rule missing after update: %v", err)
	}
	if stored.Description != "updated" {
		t.Errorf("update did not reach the store: %q", stored.Description)
	}

	if err := cfg.DeleteRule("crud-1"); err != nil {
		t.Fatalf("DeleteRule failed: %v", err)
	}
	if _, err := store.GetByUUID("crud-1"); err == nil {
		t.Error("DeleteRule did not reach the store")
	}
}

func TestRulesDuplicateUUIDsSurviveLegacyMigration(t *testing.T) {
	dir := t.TempDir()
	first := legacyTestRule("dup-uuid", "model-first")
	second := legacyTestRule("dup-uuid", "model-second")
	writeLegacyConfig(t, dir, []typ.Rule{first, second})

	cfg, err := NewConfigWithDir(dir)
	if err != nil {
		t.Fatalf("NewConfigWithDir failed: %v", err)
	}
	defer cfg.CloseStores()

	gotFirst := findRuleByModel(cfg.Rules, "model-first")
	gotSecond := findRuleByModel(cfg.Rules, "model-second")
	if gotFirst == nil || gotSecond == nil {
		t.Fatalf("a duplicate-UUID rule was dropped during migration (first=%v second=%v)", gotFirst != nil, gotSecond != nil)
	}
	if gotFirst.UUID == gotSecond.UUID {
		t.Fatalf("duplicate UUIDs were not disambiguated: both %q", gotFirst.UUID)
	}

	store := cfg.StoreManager().Rules()
	for _, r := range []*typ.Rule{gotFirst, gotSecond} {
		if _, err := store.GetByUUID(r.UUID); err != nil {
			t.Errorf("rule %s (%s) missing from store: %v", r.RequestModel, r.UUID, err)
		}
	}
}

func TestSaveAfterCloseStoresStillWritesFile(t *testing.T) {
	dir := t.TempDir()
	cfg, err := NewConfigWithDir(dir)
	if err != nil {
		t.Fatalf("NewConfigWithDir failed: %v", err)
	}
	if err := cfg.CloseStores(); err != nil {
		t.Fatalf("CloseStores failed: %v", err)
	}

	// Unrelated config edits must still reach config.json after the stores
	// are gone; the rule sync silently detaches instead of erroring.
	if err := cfg.SetVerbose(true); err != nil {
		t.Fatalf("Save after CloseStores failed: %v", err)
	}
	if got, _ := readConfigJSON(t, dir)["verbose"].(bool); !got {
		t.Error("verbose=true did not reach config.json after CloseStores")
	}
}

func TestRuleMutationsKeepFileMirrorFresh(t *testing.T) {
	dir := t.TempDir()
	cfg, err := NewConfigWithDir(dir)
	if err != nil {
		t.Fatalf("NewConfigWithDir failed: %v", err)
	}
	defer cfg.CloseStores()
	seedLegacyProvider(t, cfg)

	rule := legacyTestRule("mirror-1", "mirror-model")
	if err := cfg.AddRule(rule); err != nil {
		t.Fatalf("AddRule failed: %v", err)
	}

	// The downgrade-compat mirror must parse as the pre-database format:
	// a typ.Rule array under "rules", containing the new rule.
	var onDisk struct {
		Rules []typ.Rule `json:"rules"`
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("failed to read config.json: %v", err)
	}
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatalf("file mirror is not old-format parseable: %v", err)
	}
	if findRule(onDisk.Rules, "mirror-1") == nil {
		t.Error("AddRule did not refresh the file mirror")
	}

	if err := cfg.DeleteRule("mirror-1"); err != nil {
		t.Fatalf("DeleteRule failed: %v", err)
	}
	raw, _ = os.ReadFile(filepath.Join(dir, "config.json"))
	if bytes.Contains(raw, []byte("mirror-1")) {
		t.Error("DeleteRule did not remove the rule from the file mirror")
	}
}

func TestRulesUUIDAssignedDuringLegacyMigration(t *testing.T) {
	dir := t.TempDir()
	noUUID := legacyTestRule("", "uuidless-model")
	writeLegacyConfig(t, dir, []typ.Rule{noUUID})

	cfg, err := NewConfigWithDir(dir)
	if err != nil {
		t.Fatalf("NewConfigWithDir failed: %v", err)
	}
	defer cfg.CloseStores()

	migrated := findRuleByModel(cfg.Rules, "uuidless-model")
	if migrated == nil {
		t.Fatal("uuidless rule lost during migration")
	}
	if migrated.UUID == "" {
		t.Fatal("rule was not assigned a UUID during migration")
	}
	if _, err := cfg.StoreManager().Rules().GetByUUID(migrated.UUID); err != nil {
		t.Errorf("uuidless rule not persisted to store: %v", err)
	}
}
