// Package logfields names the structured log fields for bots and IM chats so
// a log line can be grepped by the same key everywhere (design:
// .design/bot-im-alias-display.md §3.5).
//
// Rules: the bot's uuid and name are separate fields; chat_id is always the
// EXTERNAL platform id (what the user sees and pastes), never an internal
// access uuid — those are access_target_id / access_actor_id.
package logfields

import "github.com/sirupsen/logrus"

const (
	BotUUID  = "bot_uuid"
	BotName  = "bot_name"
	Platform = "platform"
	// ChatID is the external platform chat id.
	ChatID    = "chat_id"
	ChatAlias = "chat_alias"
	// ActorID is the external platform user id.
	ActorID = "actor_id"
	// AccessTargetID and AccessActorID are the internal access-store uuids.
	AccessTargetID = "access_target_id"
	AccessActorID  = "access_actor_id"
	// IlinkBotID is Weixin's own bot account id, a different thing from BotUUID.
	IlinkBotID = "ilink_bot_id"
)

// Chat returns the identity fields of a chat on a bot.
func Chat(botUUID, chatID, alias string) logrus.Fields {
	f := logrus.Fields{BotUUID: botUUID, ChatID: chatID}
	if alias != "" {
		f[ChatAlias] = alias
	}
	return f
}
