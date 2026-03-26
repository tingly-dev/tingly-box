// Package command provides a simple, generic command management system for bots.
package command

import (
	"github.com/tingly-dev/tingly-box/imbot/core"
)

// HandlerContext provides context for command execution.
// It encapsulates all information needed to handle a command.
type HandlerContext struct {
	// Bot is the bot instance receiving the command
	Bot core.Bot

	// ChatID is the ID of the chat where the command was sent
	ChatID string

	// SenderID is the ID of the user who sent the command
	SenderID string

	// Platform identifies the messaging platform
	Platform core.Platform

	// Text is the raw message text
	Text string

	// IsDirectMessage indicates if this is a direct/private message
	IsDirectMessage bool

	// MessageID is the ID of the message (for editing/deleting)
	MessageID string
}

// NewHandlerContext creates a new handler context.
func NewHandlerContext(bot core.Bot, chatID, senderID string, platform core.Platform) *HandlerContext {
	return &HandlerContext{
		Bot:      bot,
		ChatID:   chatID,
		SenderID: senderID,
		Platform: platform,
	}
}

// WithText sets the message text.
func (c *HandlerContext) WithText(text string) *HandlerContext {
	c.Text = text
	return c
}

// WithDirectMessage sets whether this is a direct message.
func (c *HandlerContext) WithDirectMessage(isDirect bool) *HandlerContext {
	c.IsDirectMessage = isDirect
	return c
}

// WithMessageID sets the message ID.
func (c *HandlerContext) WithMessageID(msgID string) *HandlerContext {
	c.MessageID = msgID
	return c
}

// SendText sends a text message to the chat.
func (c *HandlerContext) SendText(text string) error {
	_, err := c.Bot.SendText(nil, c.ChatID, text)
	return err
}

// Reply sends a reply message.
func (c *HandlerContext) Reply(text string) error {
	return c.SendText(text)
}

// SendError sends an error message.
func (c *HandlerContext) SendError(err error) error {
	return c.SendText("❌ Error: " + err.Error())
}

// IsPlatform checks if the message is from a specific platform.
func (c *HandlerContext) IsPlatform(platform core.Platform) bool {
	return c.Platform == platform
}

// Clone creates a copy of the context.
func (c *HandlerContext) Clone() *HandlerContext {
	return &HandlerContext{
		Bot:             c.Bot,
		ChatID:          c.ChatID,
		SenderID:        c.SenderID,
		Platform:        c.Platform,
		Text:            c.Text,
		IsDirectMessage: c.IsDirectMessage,
		MessageID:       c.MessageID,
	}
}
