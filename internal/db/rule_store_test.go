package db

import (
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// newTestRuleStore builds the store through NewStoreManager so tests run
// under the exact production connection settings (DSN pragmas included).
func newTestRuleStore(t *testing.T) *RuleStore {
	t.Helper()
	sm, err := NewStoreManager(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create store manager: %v", err)
	}
	t.Cleanup(func() { sm.Close() })
	return sm.Rules()
}

func testRule(uuid, requestModel string) typ.Rule {
	return typ.Rule{
		UUID:          uuid,
		Scenario:      typ.RuleScenario("openai"),
		RequestModel:  requestModel,
		ResponseModel: "gpt-x",
		Description:   "test rule " + uuid,
		Active:        true,
		Services: []*loadbalance.Service{
			{Provider: "prov-1", Model: "m-1", Weight: 2, Active: true, Tier: 1},
		},
		Flags: typ.RuleFlags{CleanHeader: true, SessionAffinity: 1800},
	}
}

func TestRuleStoreSyncAllAndList(t *testing.T) {
	store := newTestRuleStore(t)

	rules := []typ.Rule{testRule("r1", "model-a"), testRule("r2", "model-b"), testRule("r3", "model-c")}
	if err := store.SyncAll(rules); err != nil {
		t.Fatalf("SyncAll failed: %v", err)
	}

	got, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("List returned %d rules, want 3", len(got))
	}
	for i, uuid := range []string{"r1", "r2", "r3"} {
		if got[i].UUID != uuid {
			t.Errorf("position %d: got UUID %s, want %s (order must follow sync order)", i, got[i].UUID, uuid)
		}
	}

	// Fat-field round trip.
	if len(got[0].Services) != 1 || got[0].Services[0].Provider != "prov-1" || got[0].Services[0].Weight != 2 || got[0].Services[0].Tier != 1 {
		t.Errorf("services did not round-trip: %+v", got[0].Services)
	}
	if !got[0].Flags.CleanHeader || got[0].Flags.SessionAffinity != 1800 {
		t.Errorf("flags did not round-trip: %+v", got[0].Flags)
	}
}

func TestRuleStoreSyncAllUpdatesDeletesReorders(t *testing.T) {
	store := newTestRuleStore(t)

	rules := []typ.Rule{testRule("r1", "model-a"), testRule("r2", "model-b"), testRule("r3", "model-c")}
	if err := store.SyncAll(rules); err != nil {
		t.Fatalf("initial SyncAll failed: %v", err)
	}

	// Delete r2, reorder r3 before r1, edit r1, add r4.
	edited := testRule("r1", "model-a-renamed")
	edited.Active = false
	next := []typ.Rule{testRule("r3", "model-c"), edited, testRule("r4", "model-d")}
	if err := store.SyncAll(next); err != nil {
		t.Fatalf("second SyncAll failed: %v", err)
	}

	got, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("List returned %d rules, want 3", len(got))
	}
	wantOrder := []string{"r3", "r1", "r4"}
	for i, uuid := range wantOrder {
		if got[i].UUID != uuid {
			t.Errorf("position %d: got %s, want %s", i, got[i].UUID, uuid)
		}
	}
	if got[1].RequestModel != "model-a-renamed" || got[1].Active {
		t.Errorf("r1 edit not persisted: %+v", got[1])
	}
	if _, err := store.GetByUUID("r2"); err == nil {
		t.Error("r2 should have been deleted")
	}
}

func TestRuleStoreSyncAllSkipsUnchangedRows(t *testing.T) {
	store := newTestRuleStore(t)

	rules := []typ.Rule{testRule("r1", "model-a")}
	if err := store.SyncAll(rules); err != nil {
		t.Fatalf("SyncAll failed: %v", err)
	}

	var before RuleRecord
	if err := store.GetDB().Where("uuid = ?", "r1").First(&before).Error; err != nil {
		t.Fatalf("failed to read record: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	if err := store.SyncAll(rules); err != nil {
		t.Fatalf("second SyncAll failed: %v", err)
	}

	var after RuleRecord
	if err := store.GetDB().Where("uuid = ?", "r1").First(&after).Error; err != nil {
		t.Fatalf("failed to re-read record: %v", err)
	}
	if !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Errorf("updated_at moved on a no-op sync: %v -> %v", before.UpdatedAt, after.UpdatedAt)
	}
}

func TestRuleStoreSkipsEmptyAndDuplicateUUIDs(t *testing.T) {
	store := newTestRuleStore(t)

	rules := []typ.Rule{
		testRule("", "no-uuid"),
		testRule("dup", "first"),
		testRule("dup", "second"),
	}
	if err := store.SyncAll(rules); err != nil {
		t.Fatalf("SyncAll failed: %v", err)
	}

	got, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("List returned %d rules, want 1 (empty + duplicate UUIDs skipped)", len(got))
	}
	if got[0].RequestModel != "first" {
		t.Errorf("first occurrence should win, got %s", got[0].RequestModel)
	}
}

func TestRuleStoreSmartRoutingRoundTrip(t *testing.T) {
	store := newTestRuleStore(t)

	rule := testRule("smart", "model-s")
	rule.SmartEnabled = true
	rule.SmartRouting = []smartrouting.SmartRouting{
		{
			Description: "long context to big model",
			Services: []*loadbalance.Service{
				{Provider: "prov-2", Model: "m-big", Active: true},
			},
		},
	}
	rule.LBTactic = typ.Tactic{Type: loadbalance.TacticRandom, Params: typ.DefaultRandomParams()}

	if err := store.SyncAll([]typ.Rule{rule}); err != nil {
		t.Fatalf("SyncAll failed: %v", err)
	}

	got, err := store.GetByUUID("smart")
	if err != nil {
		t.Fatalf("GetByUUID failed: %v", err)
	}
	if !got.SmartEnabled || len(got.SmartRouting) != 1 {
		t.Fatalf("smart routing did not round-trip: %+v", got)
	}
	if got.SmartRouting[0].Description != "long context to big model" {
		t.Errorf("smart routing description lost: %+v", got.SmartRouting[0])
	}
	if len(got.SmartRouting[0].Services) != 1 || got.SmartRouting[0].Services[0].Model != "m-big" {
		t.Errorf("smart routing services lost: %+v", got.SmartRouting[0].Services)
	}
	if got.LBTactic.Type != loadbalance.TacticRandom {
		t.Errorf("lb tactic lost: %+v", got.LBTactic)
	}
}

func TestRuleStoreCount(t *testing.T) {
	store := newTestRuleStore(t)

	count, err := store.Count()
	if err != nil || count != 0 {
		t.Fatalf("Count on empty store = %d, %v; want 0, nil", count, err)
	}

	if err := store.SyncAll([]typ.Rule{testRule("r1", "a"), testRule("r2", "b")}); err != nil {
		t.Fatalf("SyncAll failed: %v", err)
	}
	count, err = store.Count()
	if err != nil || count != 2 {
		t.Fatalf("Count = %d, %v; want 2, nil", count, err)
	}
}
