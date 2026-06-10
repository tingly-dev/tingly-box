package db

import (
	"testing"
)

// newTestRuleStateStore creates a rule state store backed by a temp database
// via the store manager (which handles db file/directory creation).
func newTestRuleStateStore(t *testing.T) *RuleStateStore {
	t.Helper()
	sm, err := NewStoreManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewStoreManager error: %v", err)
	}
	t.Cleanup(func() { sm.Close() })
	return sm.RuleState()
}

func TestRuleStateStore_RenameRuleUUID(t *testing.T) {
	store := newTestRuleStateStore(t)

	if err := store.SetServiceID("old-uuid", "provider:model"); err != nil {
		t.Fatalf("SetServiceID error: %v", err)
	}
	// A stale row under the target identity must be replaced, not merged.
	if err := store.SetServiceID("builtin:claude_code:p1:haiku", "stale:model"); err != nil {
		t.Fatalf("SetServiceID error: %v", err)
	}

	if err := store.RenameRuleUUID("old-uuid", "builtin:claude_code:p1:haiku"); err != nil {
		t.Fatalf("RenameRuleUUID error: %v", err)
	}

	got, err := store.GetServiceID("builtin:claude_code:p1:haiku")
	if err != nil {
		t.Fatalf("GetServiceID error: %v", err)
	}
	if got != "provider:model" {
		t.Errorf("renamed state = %q, want %q", got, "provider:model")
	}
	if old, _ := store.GetServiceID("old-uuid"); old != "" {
		t.Errorf("old key should be gone, got %q", old)
	}
}

func TestRuleStateStore_DeleteRules(t *testing.T) {
	store := newTestRuleStateStore(t)

	if err := store.SetServiceID("builtin:claude_code:p1:cc", "a:b"); err != nil {
		t.Fatalf("SetServiceID error: %v", err)
	}
	if err := store.SetServiceID("keep", "c:d"); err != nil {
		t.Fatalf("SetServiceID error: %v", err)
	}

	if err := store.DeleteRules([]string{"builtin:claude_code:p1:cc"}); err != nil {
		t.Fatalf("DeleteRules error: %v", err)
	}
	if got, _ := store.GetServiceID("builtin:claude_code:p1:cc"); got != "" {
		t.Errorf("deleted key should be gone, got %q", got)
	}
	if got, _ := store.GetServiceID("keep"); got != "c:d" {
		t.Errorf("unrelated key lost: got %q", got)
	}

	// Empty input is a no-op.
	if err := store.DeleteRules(nil); err != nil {
		t.Errorf("DeleteRules(nil) error: %v", err)
	}
}
