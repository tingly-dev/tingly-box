package discord

import (
	"context"
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/tingly-dev/tingly-box/imbot/internal/adapter"
	"github.com/tingly-dev/tingly-box/imbot/internal/builder"
	"github.com/tingly-dev/tingly-box/imbot/internal/content"
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// Adapter adapts Discord events to core.Message
type Adapter struct {
	*adapter.BaseAdapter
	session *discordgo.Session
}

// NewAdapter creates a new Discord adapter
func NewAdapter(config *core.Config, session *discordgo.Session) *Adapter {
	return &Adapter{
		BaseAdapter: adapter.NewBaseAdapter(config),
		session:     session,
	}
}

// Platform returns core.PlatformDiscord
func (a *Adapter) Platform() core.Platform {
	return core.PlatformDiscord
}

// AdaptMessage converts a Discord message create event to core.Message
func (a *Adapter) AdaptMessage(ctx context.Context, m *discordgo.MessageCreate) (*core.Message, error) {
	if m == nil || m.Message == nil {
		return nil, fmt.Errorf("nil message")
	}

	msg := m.Message

	// Ignore messages from bots
	if msg.Author.Bot {
		return nil, fmt.Errorf("ignoring bot message")
	}

	// Determine chat type
	chatType := a.getChatType(msg.ChannelID)

	// Build message using fluent builder
	messageBuilder := builder.NewMessageBuilder(core.PlatformDiscord).
		WithID(msg.ID).
		WithTimestamp(msg.Timestamp.Unix()).
		WithRecipient(msg.ChannelID, string(chatType), a.getChannelName(msg.ChannelID)).
		WithSenderFrom(a.extractSender(msg.Author)).
		WithContent(a.extractContent(msg)).
		WithMetadata("raw_message", msg) // Store raw for platform-specific access

	// Add thread context if reply
	if msg.Reference != nil {
		ref := msg.Reference()
		if ref != nil && ref.MessageID != "" {
			messageBuilder.WithReplyTo(ref.MessageID, ref.MessageID)
		}
	}

	return messageBuilder.Build(), nil
}

// AdaptReaction converts a Discord reaction to core.Message
func (a *Adapter) AdaptReaction(ctx context.Context, emoji *discordgo.MessageReactionAdd) (*core.Message, error) {
	if emoji == nil {
		return nil, fmt.Errorf("nil reaction")
	}

	// Get user info
	var sender core.Sender
	if emoji.Member != nil {
		sender = a.extractSender(emoji.Member.User)
	}

	// Get message info
	channelID := emoji.ChannelID
	messageID := emoji.MessageID

	messageBuilder := builder.NewMessageBuilder(core.PlatformDiscord).
		WithID(messageID).
		WithTimestamp(time.Now().Unix()).
		WithRecipient(channelID, "direct", "").
		WithSenderFrom(sender).
		WithReactionContent(core.Reaction{
			MessageID: messageID,
			Emoji:     emoji.Emoji.Name,
			Action:    "add",
			UserID:    sender.ID,
		})

	return messageBuilder.Build(), nil
}

// extractSender extracts sender info from Discord User
func (a *Adapter) extractSender(user *discordgo.User) core.Sender {
	if user == nil {
		return core.Sender{ID: "unknown"}
	}

	displayName := user.GlobalName
	if displayName == "" {
		displayName = user.Username
	}
	if user.Discriminator != "" && user.Discriminator != "0000" {
		displayName += "#" + user.Discriminator
	}

	return core.Sender{
		ID:          user.ID,
		Username:    user.Username,
		DisplayName: displayName,
		Avatar:      user.AvatarURL(""),
		Raw:         make(map[string]interface{}),
	}
}

// extractContent extracts content from a Discord message
func (a *Adapter) extractContent(msg *discordgo.Message) core.Content {
	// Create content registry
	registry := content.NewRegistry[*discordgo.Message]()

	// Register handlers in priority order
	registry.Register(content.NewTextHandler(func(m *discordgo.Message) (string, []core.Entity, bool) {
		if m.Content != "" {
			// Discord uses markdown by default, extract it as text
			return m.Content, nil, true
		}
		return "", nil, false
	}))

	registry.Register(content.NewMediaHandler("embed", func(m *discordgo.Message) ([]core.MediaAttachment, string, bool) {
		if len(m.Embeds) > 0 {
			// Convert embeds to text representation
			text := a.formatEmbeds(m.Embeds)
			return []core.MediaAttachment{{
				Type:  "embed",
				URL:   "",
				Title: m.Embeds[0].Title,
				Raw:   map[string]interface{}{"embeds": m.Embeds},
			}}, text, true
		}
		return nil, "", false
	}))

	registry.Register(content.NewMediaHandler("attachment", func(m *discordgo.Message) ([]core.MediaAttachment, string, bool) {
		if len(m.Attachments) > 0 {
			media := make([]core.MediaAttachment, len(m.Attachments))
			for i, att := range m.Attachments {
				media[i] = a.convertAttachment(att)
			}
			// Use first attachment filename as caption
			caption := ""
			if len(m.Attachments) == 1 && m.Attachments[0].Filename != "" {
				caption = m.Attachments[0].Filename
			}
			return media, caption, true
		}
		return nil, "", false
	}))

	// Set default for unknown content
	registry.SetDefault(content.NewSystemHandler("unknown", func(m *discordgo.Message) (string, map[string]interface{}, bool) {
		return "unknown", map[string]interface{}{"message_type": "unsupported"}, true
	}))

	// Handle content
	result, err := registry.Handle(context.Background(), msg)
	if err != nil {
		a.Logger().Error("Failed to extract content: %v", err)
		return core.NewSystemContent("error", map[string]interface{}{"error": err.Error()})
	}

	return result
}

// convertAttachment converts a Discord attachment to core MediaAttachment
func (a *Adapter) convertAttachment(att *discordgo.MessageAttachment) core.MediaAttachment {
	mediaType := "document"

	switch att.ContentType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		mediaType = "image"
	case "video/mp4", "video/webm", "video/quicktime":
		mediaType = "video"
	case "audio/mpeg", "audio/ogg":
		mediaType = "audio"
	}

	return core.MediaAttachment{
		Type:     mediaType,
		URL:      att.URL,
		MimeType: att.ContentType,
		Filename: att.Filename,
		Size:     int64(att.Size),
		Width:    att.Width,
		Height:   att.Height,
		Raw:      make(map[string]interface{}),
	}
}

// formatEmbeds converts Discord embeds to text representation
func (a *Adapter) formatEmbeds(embeds []*discordgo.MessageEmbed) string {
	if len(embeds) == 0 {
		return ""
	}

	var text string
	embed := embeds[0]

	if embed.Title != "" {
		text += "**" + embed.Title + "**\n"
	}

	if embed.Description != "" {
		text += embed.Description + "\n"
	}

	for _, field := range embed.Fields {
		text += "\n**" + field.Name + "**\n" + field.Value + "\n"
	}

	if embed.Footer != nil && embed.Footer.Text != "" {
		text += "_" + embed.Footer.Text + "_"
	}

	return text
}

// getChannelName gets the channel name from ID (for display)
func (a *Adapter) getChannelName(channelID string) string {
	// Try to get channel from session state
	if a.session == nil {
		return ""
	}

	channel, err := a.session.State.Channel(channelID)
	if err != nil {
		return ""
	}

	if channel != nil && channel.Name != "" {
		return channel.Name
	}

	return ""
}

// getChatType maps Discord channel type to core ChatType
func (a *Adapter) getChatType(channelID string) core.ChatType {
	if a.session == nil {
		return core.ChatTypeDirect
	}

	channel, err := a.session.State.Channel(channelID)
	if err != nil {
		return core.ChatTypeDirect
	}

	switch channel.Type {
	case discordgo.ChannelTypeDM, discordgo.ChannelTypeGroupDM:
		return core.ChatTypeDirect
	case discordgo.ChannelTypeGuildCategory:
		return core.ChatTypeChannel
	default:
		if channel.IsThread() {
			return core.ChatTypeThread
		}
		return core.ChatTypeGroup
	}
}
