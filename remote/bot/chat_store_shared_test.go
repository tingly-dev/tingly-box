package bot_test

import (
	"context"
	"testing"

	"github.com/tingly-dev/tingly-box/remote/bot"
)

// TestManagerChatStoreIsShared pins the invariant that every bot a manager
// runs talks to the same chat store. It used to open one per bot over a shared
// JSON file, and concurrent bots erased each other's chats; the store is now
// injected, so sharing is structural rather than something to remember.
func TestManagerChatStoreIsShared(t *testing.T) {
	store := openStore(t, t.TempDir())

	m := bot.NewManager(nil)
	m.SetChatStore(store)

	first, err := m.ChatStore()
	if err != nil {
		t.Fatalf("first ChatStore: %v", err)
	}
	second, err := m.ChatStore()
	if err != nil {
		t.Fatalf("second ChatStore: %v", err)
	}
	if first != second {
		t.Fatal("ChatStore returned distinct instances; bots would not share state")
	}

	if err := first.BindProject("chat-1", "telegram", "/proj", "owner"); err != nil {
		t.Fatalf("bind: %v", err)
	}
	path, ok, err := second.GetProjectPath("chat-1")
	if err != nil {
		t.Fatalf("get project path: %v", err)
	}
	if !ok || path != "/proj" {
		t.Errorf("GetProjectPath = (%q, %v), want (%q, true)", path, ok, "/proj")
	}
}

// TestManagerRequiresChatStore keeps the failure loud: a bot must not run
// without persistence, and Start is the last place to catch that.
func TestManagerRequiresChatStore(t *testing.T) {
	m := bot.NewManager(nil)

	if _, err := m.ChatStore(); err == nil {
		t.Error("expected an error when no chat store is configured")
	}
	if err := m.Start(context.Background(), "some-uuid"); err == nil {
		t.Error("expected Start to fail without a chat store")
	}
}
