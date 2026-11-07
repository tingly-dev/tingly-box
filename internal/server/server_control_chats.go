package server

import (
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/remote/bot"

	notifymodule "github.com/tingly-dev/tingly-box/internal/server/module/notify"
	"github.com/tingly-dev/tingly-box/remote/channel"
)

// formatChatTime renders a chat's UpdatedAt as RFC3339; empty when zero.
func formatChatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// botChatManager is the single BotChatManager implementation: it scopes the
// shared chat store to a bot via one reachability check instead of one
// per-operation closure. The store is global across bots (keyed by chat_id
// with no platform dimension), so a chat is attributed to a bot when its
// Platform matches the bot's channel platform and it is not paired to a
// different bot. Explicit DirectChat/Group access is evaluated separately;
// the retired ChatIDLock does not narrow this resource list.
type botChatManager struct {
	reg      *channel.Registry
	provider botChatProvider
}

// newBotChatManager wires BotChatManager to the shared chat store. Returns
// nil when no provider is available, in which case the bot interaction API's
// chat lifecycle endpoints report unavailable (same behavior as before this
// refactor: stock setups without an IM handler have no chat store to drive).
func newBotChatManager(reg *channel.Registry, provider botChatProvider) notifymodule.BotChatManager {
	if provider == nil {
		return nil
	}
	return &botChatManager{reg: reg, provider: provider}
}

// resolveReachableChat centralizes the "does this chat belong to this bot"
// check the mutations must pass: the bot is running, its platform matches the
// chat's platform, and the chat is not paired to a different bot. Returns
// notifymodule.ErrChatNotFound on any miss — one body for all causes, so a
// caller cannot probe which chats exist on other platforms.
func (m *botChatManager) resolveReachableChat(botUUID, chatID string) (bot.ChatStoreInterface, error) {
	ch, ok := m.reg.Get(botUUID)
	if !ok {
		return nil, notifymodule.ErrChatNotFound
	}
	store, err := m.provider.ChatStore()
	if err != nil {
		return nil, err
	}
	c, err := store.GetChat(chatID)
	if err != nil {
		return nil, err
	}
	if c == nil || c.Platform != ch.Platform() {
		return nil, notifymodule.ErrChatNotFound
	}
	if c.IsPaired && c.PairedBotUUID != "" && c.PairedBotUUID != botUUID {
		return nil, notifymodule.ErrChatNotFound
	}
	return store, nil
}

// ListChats backs GET /bots/:bot/chats. It scopes the shared store to this
// bot's platform, then drops chats paired to another bot. This is what makes
// the chat_id required by /notify and
// /interact discoverable — see ux-principles #5 (show the concrete value) and
// #11 (hand over the artifact for the next action).
func (m *botChatManager) ListChats(botUUID string, includeDisabled bool) ([]notifymodule.ChatSummary, error) {
	// Resolve the bot's platform from its registered channel. If the bot
	// isn't running, the route layer already returned the empty/404 result
	// before calling List — but defend in depth anyway.
	ch, ok := m.reg.Get(botUUID)
	if !ok {
		return nil, nil
	}
	platform := ch.Platform()

	// Shared store, owned by the StoreManager — not ours to close.
	store, err := m.provider.ChatStore()
	if err != nil {
		return nil, err
	}

	// ListChats scopes at the source: only records whose Platform field
	// equals this bot's channel platform are returned, so unattributed or
	// cross-platform chats never reach the API surface.
	all, err := store.ListChats(platform, includeDisabled)
	if err != nil {
		return nil, err
	}

	out := make([]notifymodule.ChatSummary, 0, len(all))
	for _, c := range all {
		if c.IsPaired && c.PairedBotUUID != "" && c.PairedBotUUID != botUUID {
			// Paired to another bot — it belongs to that bot's list, not
			// this one. Unpaired same-platform chats stay visible to every
			// bot of the platform: there is nothing else to attribute them
			// by, and hiding them would make fresh chats undiscoverable.
			continue
		}
		out = append(out, notifymodule.ChatSummary{
			ChatID:        c.ChatID,
			Platform:      c.Platform,
			IsPaired:      c.IsPaired,
			IsWhitelisted: c.IsWhitelisted,
			ProjectPath:   c.ProjectPath,
			Disabled:      c.Disabled,
			DisabledAt:    formatChatTime(c.DisabledAt),
			UpdatedAt:     formatChatTime(c.UpdatedAt),
		})
	}
	logrus.Debugf("bot chats list: bot=%s platform=%s count=%d", botUUID, platform, len(out))
	return out, nil
}

// DeleteChat backs DELETE /bots/:bot/chats/:chat_id.
func (m *botChatManager) DeleteChat(botUUID, chatID string) error {
	store, err := m.resolveReachableChat(botUUID, chatID)
	if err != nil {
		return err
	}
	return store.DeleteChat(chatID)
}

// SetChatDisabled backs PUT /bots/:bot/chats/:chat_id/disabled.
func (m *botChatManager) SetChatDisabled(botUUID, chatID string, disabled bool) error {
	store, err := m.resolveReachableChat(botUUID, chatID)
	if err != nil {
		return err
	}
	return store.SetChatDisabled(chatID, disabled)
}

// IsChatDisabled backs the outbound blocklist check used by POST
// /bots/:bot/{notify,interact}. Unknown chats report false so pushes to fresh
// chat ids keep working.
func (m *botChatManager) IsChatDisabled(chatID string) bool {
	store, err := m.provider.ChatStore()
	if err != nil {
		return false
	}
	return store.IsChatDisabled(chatID)
}

// botChatProvider is the narrow surface botChatManager needs from the imbot
// handler to reach the shared chat store. An interface keeps the helper
// testable without the full imbot.Handler.
type botChatProvider interface {
	ChatStore() (bot.ChatStoreInterface, error)
}
