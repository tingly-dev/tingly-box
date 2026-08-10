package server

import (
	"context"
	"errors"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/db"
	notifymodule "github.com/tingly-dev/tingly-box/internal/server/module/notify"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/control/bot"
	"github.com/tingly-dev/tingly-box/remote/interaction"
)

// chatTestChannel is the minimal channel.Channel a lister test needs: an id
// and a platform. Send/Prompt are never reached.
type chatTestChannel struct {
	id       string
	platform string
}

func (c *chatTestChannel) ID() string                         { return c.id }
func (c *chatTestChannel) Platform() string                   { return c.platform }
func (c *chatTestChannel) Capabilities() channel.Capabilities { return channel.Capabilities{} }
func (c *chatTestChannel) Send(context.Context, channel.Target, interaction.Notification) error {
	return nil
}
func (c *chatTestChannel) Prompt(context.Context, channel.Target, interaction.Interaction) (interaction.Reply, error) {
	return interaction.Reply{}, nil
}

// chatTestProvider backs botChatProvider with a real store.
type chatTestProvider struct {
	store bot.ChatStoreInterface
}

func (p *chatTestProvider) ChatStore() (bot.ChatStoreInterface, error) { return p.store, nil }

func newChatWiring(t *testing.T) (*channel.Registry, *chatTestProvider, *db.RemoteChatStore) {
	t.Helper()
	sm, err := db.NewStoreManager(t.TempDir())
	if err != nil {
		t.Fatalf("open store manager: %v", err)
	}
	t.Cleanup(func() { _ = sm.Close() })
	store := sm.RemoteChats()

	reg := channel.NewRegistry()
	reg.Register(&chatTestChannel{id: "bot-1", platform: "telegram"})
	reg.Register(&chatTestChannel{id: "bot-2", platform: "telegram"})

	return reg, &chatTestProvider{store: store}, store
}

// TestChatLister_ExcludesPairedElsewhere is the attribution tightening: a chat
// paired to another bot belongs to that bot's list, while unpaired
// same-platform chats stay visible to every bot of the platform.
func TestChatLister_ExcludesPairedElsewhere(t *testing.T) {
	reg, provider, store := newChatWiring(t)

	if _, err := store.GetOrCreateChat("unpaired", "telegram"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.SetPaired("mine", "telegram", "bot-1", "sender"); err != nil {
		t.Fatalf("pair mine: %v", err)
	}
	if err := store.SetPaired("theirs", "telegram", "bot-2", "sender"); err != nil {
		t.Fatalf("pair theirs: %v", err)
	}

	mgr := newBotChatManager(reg, provider)
	chats, err := mgr.ListChats("bot-1", false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got := map[string]bool{}
	for _, c := range chats {
		got[c.ChatID] = true
	}
	if !got["unpaired"] || !got["mine"] || got["theirs"] || len(got) != 2 {
		t.Errorf("bot-1 list = %v, want {unpaired, mine}", got)
	}
}

// TestChatLister_DisabledHiddenUnlessAsked checks the includeDisabled
// passthrough end to end and that the summary carries the flag.
func TestChatLister_DisabledHiddenUnlessAsked(t *testing.T) {
	reg, provider, store := newChatWiring(t)

	if _, err := store.GetOrCreateChat("open", "telegram"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := store.GetOrCreateChat("blocked", "telegram"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.SetChatDisabled("blocked", true); err != nil {
		t.Fatalf("disable: %v", err)
	}

	mgr := newBotChatManager(reg, provider)
	chats, err := mgr.ListChats("bot-1", false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(chats) != 1 || chats[0].ChatID != "open" {
		t.Errorf("default list = %+v, want just open", chats)
	}

	all, err := mgr.ListChats("bot-1", true)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("includeDisabled list = %+v, want both", all)
	}
	for _, c := range all {
		if c.ChatID == "blocked" && (!c.Disabled || c.DisabledAt == "") {
			t.Errorf("blocked summary missing disabled state: %+v", c)
		}
	}
}

// TestChatDeleter_AllowsReachableAndRefusesForeign verifies that no special
// ChatIDLock survives while bot ownership still prevents cross-bot deletion.
func TestChatDeleter_AllowsReachableAndRefusesForeign(t *testing.T) {
	reg, provider, store := newChatWiring(t)

	if _, err := store.GetOrCreateChat("locked-chat", "telegram"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.SetPaired("theirs", "telegram", "bot-2", "sender"); err != nil {
		t.Fatalf("pair: %v", err)
	}
	if _, err := store.GetOrCreateChat("mine", "telegram"); err != nil {
		t.Fatalf("create: %v", err)
	}

	mgr := newBotChatManager(reg, provider)

	if err := mgr.DeleteChat("bot-1", "locked-chat"); err != nil {
		t.Errorf("deleting a formerly locked chat: %v", err)
	}
	if err := mgr.DeleteChat("bot-1", "theirs"); !errors.Is(err, notifymodule.ErrChatNotFound) {
		t.Errorf("deleting a foreign chat: err = %v, want ErrChatNotFound", err)
	}
	if err := mgr.DeleteChat("bot-1", "missing"); !errors.Is(err, notifymodule.ErrChatNotFound) {
		t.Errorf("deleting a missing chat: err = %v, want ErrChatNotFound", err)
	}

	if err := mgr.DeleteChat("bot-1", "mine"); err != nil {
		t.Fatalf("deleting own chat: %v", err)
	}
	if chat, _ := store.GetChat("mine"); chat != nil {
		t.Errorf("chat survived delete: %+v", chat)
	}
	// The foreign chat is untouched.
	if chat, _ := store.GetChat("theirs"); chat == nil {
		t.Error("foreign chat was deleted")
	}
}

// TestChatDisabler_ScopedToReachable mirrors the deleter's reachability check.
func TestChatDisabler_ScopedToReachable(t *testing.T) {
	reg, provider, store := newChatWiring(t)

	if err := store.SetPaired("theirs", "telegram", "bot-2", "sender"); err != nil {
		t.Fatalf("pair: %v", err)
	}
	if _, err := store.GetOrCreateChat("mine", "telegram"); err != nil {
		t.Fatalf("create: %v", err)
	}

	mgr := newBotChatManager(reg, provider)
	if err := mgr.SetChatDisabled("bot-1", "theirs", true); !errors.Is(err, notifymodule.ErrChatNotFound) {
		t.Errorf("disabling a foreign chat: err = %v, want ErrChatNotFound", err)
	}
	if err := mgr.SetChatDisabled("bot-1", "mine", true); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if !store.IsChatDisabled("mine") {
		t.Error("chat not disabled")
	}

	if !mgr.IsChatDisabled("mine") || mgr.IsChatDisabled("theirs") {
		t.Error("disabled checker disagrees with store state")
	}
}
