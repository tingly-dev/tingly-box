package builder

import (
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// MessageBuilder builds core.Message with a fluent API
type MessageBuilder struct {
	platform core.Platform
	msg      core.Message
}

// NewMessageBuilder creates a new message builder for the given platform
func NewMessageBuilder(platform core.Platform) *MessageBuilder {
	return &MessageBuilder{
		platform: platform,
		msg: core.Message{
			Platform: platform,
			Metadata: make(map[string]interface{}),
		},
	}
}

// WithSender sets the sender information
func (b *MessageBuilder) WithSender(id, username, displayName string) *MessageBuilder {
	b.msg.Sender = core.Sender{
		ID:          id,
		Username:    username,
		DisplayName: displayName,
		Raw:         make(map[string]interface{}),
	}
	return b
}

// WithSenderFrom sets the sender from a core.Sender
func (b *MessageBuilder) WithSenderFrom(sender core.Sender) *MessageBuilder {
	b.msg.Sender = sender
	if sender.Raw == nil {
		b.msg.Sender.Raw = make(map[string]interface{})
	}
	return b
}

// WithRecipient sets the recipient information
func (b *MessageBuilder) WithRecipient(id, chatType, displayName string) *MessageBuilder {
	b.msg.Recipient = core.Recipient{
		ID:          id,
		Type:        chatType,
		DisplayName: displayName,
	}
	b.msg.ChatType = core.ChatType(chatType)
	return b
}

// WithRecipientFrom sets the recipient from a core.Recipient
func (b *MessageBuilder) WithRecipientFrom(recipient core.Recipient) *MessageBuilder {
	b.msg.Recipient = recipient
	b.msg.ChatType = core.ChatType(recipient.Type)
	return b
}

// WithTextContent sets text content
func (b *MessageBuilder) WithTextContent(text string, entities []core.Entity) *MessageBuilder {
	b.msg.Content = core.NewTextContent(text, entities...)
	return b
}

// WithMediaContent sets media content
func (b *MessageBuilder) WithMediaContent(media []core.MediaAttachment, caption string) *MessageBuilder {
	b.msg.Content = core.NewMediaContent(media, caption)
	return b
}

// WithPollContent sets poll content
func (b *MessageBuilder) WithPollContent(poll core.Poll) *MessageBuilder {
	b.msg.Content = core.NewPollContent(poll)
	return b
}

// WithReactionContent sets reaction content
func (b *MessageBuilder) WithReactionContent(reaction core.Reaction) *MessageBuilder {
	b.msg.Content = core.NewReactionContent(reaction)
	return b
}

// WithSystemContent sets system content
func (b *MessageBuilder) WithSystemContent(eventType string, data map[string]interface{}) *MessageBuilder {
	b.msg.Content = core.NewSystemContent(eventType, data)
	return b
}

// WithContent sets content directly
func (b *MessageBuilder) WithContent(content core.Content) *MessageBuilder {
	b.msg.Content = content
	return b
}

// WithTimestamp sets the message timestamp
func (b *MessageBuilder) WithTimestamp(ts int64) *MessageBuilder {
	b.msg.Timestamp = ts
	return b
}

// WithReplyTo sets the message as a reply to another message
func (b *MessageBuilder) WithReplyTo(messageID, parentMessageID string) *MessageBuilder {
	b.msg.ThreadContext = &core.ThreadContext{
		ID:              messageID,
		ParentMessageID: parentMessageID,
	}
	return b
}

// WithThreadContext sets the thread context
func (b *MessageBuilder) WithThreadContext(thread *core.ThreadContext) *MessageBuilder {
	b.msg.ThreadContext = thread
	return b
}

// WithID sets the message ID
func (b *MessageBuilder) WithID(id string) *MessageBuilder {
	b.msg.ID = id
	return b
}

// WithMetadata adds metadata to the message
func (b *MessageBuilder) WithMetadata(key string, value interface{}) *MessageBuilder {
	if b.msg.Metadata == nil {
		b.msg.Metadata = make(map[string]interface{})
	}
	b.msg.Metadata[key] = value
	return b
}

// WithMetadataMap sets the entire metadata map
func (b *MessageBuilder) WithMetadataMap(metadata map[string]interface{}) *MessageBuilder {
	b.msg.Metadata = metadata
	return b
}

// WithRawMetadata adds raw platform data to metadata under a namespaced key
func (b *MessageBuilder) WithRawMetadata(key string, value interface{}) *MessageBuilder {
	return b.WithMetadata(string(b.platform)+":"+key, value)
}

// Build returns the constructed message
func (b *MessageBuilder) Build() *core.Message {
	// Validate required fields
	if b.msg.ID == "" {
		// ID is optional for incoming messages (can be set by platform)
	}
	if b.msg.Sender.ID == "" {
		// Sender ID is required
		b.msg.Sender.ID = "unknown"
	}
	if b.msg.Recipient.ID == "" {
		// Recipient ID is required
		b.msg.Recipient.ID = "unknown"
	}
	if b.msg.Content == nil {
		// Content is required
		b.msg.Content = core.NewSystemContent("empty", nil)
	}

	return &b.msg
}

// MustBuild returns the constructed message or panics if validation fails
func (b *MessageBuilder) MustBuild() *core.Message {
	msg := b.Build()
	if msg == nil {
		panic("message builder: build failed")
	}
	return msg
}
