// Package wire defines the JSON-over-WebSocket protocol used between tingly
// platform participants: the tingly server, bots, and chat clients.
//
// The protocol is intentionally small. Every transmission is a Frame whose
// Kind discriminates the payload, plus a few routing fields lifted out of the
// payload for cheap dispatch (Bot, Chat).
//
// Direction matrix:
//
//	bot   → server: bot.send / bot.edit / bot.delete / bot.react
//	chat  → server: chat.send / chat.callback
//	server→ bot:    chat.send / chat.callback (forwarded from chats)
//	server→ chat:   bot.send / bot.edit / bot.delete / bot.react (forwarded)
//	server→ either: welcome / ack / error
//
// All clients open with a Hello frame; the server replies with Welcome.
//
// Wire types live in imbot rather than the top-level tingly module because
// the bot transport must construct them and imbot is its own Go module.
package wire

import (
	"encoding/json"

	"github.com/tingly-dev/tingly-box/imbot/core"
)

// Version is bumped on incompatible wire changes.
const Version = 1

// Role identifies the connecting client's role in the Hello frame.
type Role string

const (
	RoleBot  Role = "bot"
	RoleChat Role = "chat"
)

// Kind discriminates the payload carried in a Frame.
type Kind string

const (
	KindHello   Kind = "hello"
	KindWelcome Kind = "welcome"
	KindAck     Kind = "ack"
	KindError   Kind = "error"

	KindBotSend   Kind = "bot.send"
	KindBotEdit   Kind = "bot.edit"
	KindBotDelete Kind = "bot.delete"
	KindBotReact  Kind = "bot.react"

	KindChatSend     Kind = "chat.send"
	KindChatCallback Kind = "chat.callback"
)

// Frame is the on-wire envelope. Data is decoded according to Kind.
type Frame struct {
	Kind Kind            `json:"kind"`
	ID   string          `json:"id,omitempty"`   // optional client request id; echoed in ack
	Bot  string          `json:"bot,omitempty"`  // bot UUID (always present after Hello)
	Chat string          `json:"chat,omitempty"` // chat ID (present on chat-scoped frames)
	Data json.RawMessage `json:"data,omitempty"`
}

// Hello is the first frame sent by every client.
type Hello struct {
	Version int          `json:"version"`
	Role    Role         `json:"role"`
	BotID   string       `json:"botId"`
	ChatID  string       `json:"chatId,omitempty"` // chat clients only
	Token   string       `json:"token,omitempty"`
	Sender  *core.Sender `json:"sender,omitempty"` // chat clients only
	// HistoryLimit asks the server to include up to N recent messages in
	// Welcome. 0 means none.
	HistoryLimit int `json:"historyLimit,omitempty"`
}

// Welcome is the server's response to Hello.
type Welcome struct {
	Version  int               `json:"version"`
	ServerID string            `json:"serverId,omitempty"`
	History  []HistoryEntry    `json:"history,omitempty"`
	Now      int64             `json:"now,omitempty"` // server unix seconds
}

// HistoryEntry is a stored event in chronological order. Frame is the
// rebuilt frame as it would appear on the wire (chat.send or bot.send/edit/
// delete/react).
type HistoryEntry struct {
	Frame Frame `json:"frame"`
}

// Ack is sent by the server once a client frame is accepted. For bot/chat
// sends it carries the assigned MessageID and timestamp. For edits/deletes/
// reactions only the ID echo and timestamp are meaningful.
type Ack struct {
	MessageID string `json:"messageId,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// ErrorPayload is the payload of a KindError frame.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// BotSend is the payload for KindBotSend. Mirrors core.SendMessageOptions
// but kept independent so the wire shape is stable across imbot versions.
type BotSend struct {
	Text      string                 `json:"text,omitempty"`
	Media     []core.MediaAttachment `json:"media,omitempty"`
	ParseMode core.ParseMode         `json:"parseMode,omitempty"`
	ReplyTo   string                 `json:"replyTo,omitempty"`
	Metadata  map[string]any         `json:"metadata,omitempty"`
}

// BotEdit payload.
type BotEdit struct {
	MessageID string `json:"messageId"`
	Text      string `json:"text"`
}

// BotDelete payload.
type BotDelete struct {
	MessageID string `json:"messageId"`
}

// BotReact payload.
type BotReact struct {
	MessageID string `json:"messageId"`
	Emoji     string `json:"emoji"`
}

// ChatSend is the payload for KindChatSend (text or media from a chat).
type ChatSend struct {
	Text     string                 `json:"text,omitempty"`
	Media    []core.MediaAttachment `json:"media,omitempty"`
	ChatType core.ChatType          `json:"chatType,omitempty"`
	Sender   core.Sender            `json:"sender"`
	Metadata map[string]any         `json:"metadata,omitempty"`
}

// ChatCallback is the payload for KindChatCallback (button click etc.).
type ChatCallback struct {
	Sender       core.Sender   `json:"sender"`
	CallbackData string        `json:"callbackData"`
	ChatType     core.ChatType `json:"chatType,omitempty"`
}

// EncodeData JSON-encodes a payload struct into a Frame.Data slice. Errors
// are surfaced because malformed payloads here always indicate a bug.
func EncodeData(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

// DecodeData JSON-decodes Frame.Data into the provided pointer.
func DecodeData(raw json.RawMessage, out any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}
