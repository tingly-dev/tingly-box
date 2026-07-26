package server

import (
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
	notifymodule "github.com/tingly-dev/tingly-box/internal/server/module/notify"
	"github.com/tingly-dev/tingly-box/remote/channel"
)

// buildBotChatLister wires the GET /bots/:bot/chats endpoint to the shared
// chat store. The store is global across bots, so a chat is attributed to a
// bot when its platform matches the bot's channel platform; when the bot has
// a chat-id lock set, only that single chat is returned (a locked bot can
// reach no other). This is what makes the chat_id required by /notify and
// /interact discoverable — see ux-principles #5 (show the concrete value)
// and #11 (hand over the artifact for the next action).
func buildBotChatLister(reg *channel.Registry, provider botChatProvider) notifymodule.ChatLister {
	return func(botUUID string) ([]notifymodule.ChatSummary, error) {
		// Resolve the bot's platform from its registered channel. If the bot
		// isn't running, the route layer already returned 404 before calling
		// the lister — but defend in depth anyway.
		ch, ok := reg.Get(botUUID)
		if !ok {
			return nil, nil
		}
		platform := ch.Platform()

		// A chat-id lock collapses the reachable set to one chat id.
		lock := provider.ChatIDLock(botUUID)

		store, err := provider.ChatStore()
		if err != nil {
			return nil, err
		}
		defer store.Close()

		// ListChats scopes at the source: only records whose Platform field
		// equals this bot's channel platform are returned, so unattributed or
		// cross-platform chats never reach the API surface.
		all, err := store.ListChats(platform)
		if err != nil {
			return nil, err
		}

		out := make([]notifymodule.ChatSummary, 0, len(all))
		for _, c := range all {
			if lock != "" && c.ChatID != lock {
				// A chat-id lock collapses the reachable set to one chat id.
				continue
			}
			summary := notifymodule.ChatSummary{
				ChatID:        c.ChatID,
				Platform:      c.Platform,
				IsPaired:      c.IsPaired,
				IsWhitelisted: c.IsWhitelisted,
				ProjectPath:   c.ProjectPath,
				UpdatedAt:     formatChatTime(c.UpdatedAt),
			}
			out = append(out, summary)
		}
		logrus.Debugf("bot chats list: bot=%s platform=%s lock=%q count=%d", botUUID, platform, lock, len(out))
		return out, nil
	}
}

// formatChatTime renders a chat's UpdatedAt as RFC3339; empty when zero.
func formatChatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// botChatProvider is the narrow surface buildBotChatLister needs from the
// imbot handler: open the shared chat store and look up a bot's chat-id
// lock. An interface so the helper is testable without the full imbot.Handler.
type botChatProvider interface {
	ChatStore() (bot.ChatStoreInterface, error)
	ChatIDLock(botUUID string) string
}
