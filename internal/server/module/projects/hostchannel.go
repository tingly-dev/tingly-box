package projects

import (
	"context"

	projectsapp "github.com/tingly-dev/tingly-box/projects"
	"github.com/tingly-dev/tingly-box/remote/bridge"
)

var (
	_ projectsapp.ChannelSink     = (*Host)(nil)
	_ projectsapp.ChannelPrompter = (*Host)(nil)
)

// SetChatBridge gives the host the bridge it sends chat messages through.
func (h *Host) SetChatBridge(b *bridge.ChatBridge) { h.bridge = b }

// SendChannelText is the host's half of Projects' optional ChannelSink: the
// coordinator's replies reach the IM chats bound to a project.
func (h *Host) SendChannelText(ctx context.Context, platform, chatID, text string) error {
	if h.bridge == nil {
		return bridge.ErrNoChannel
	}
	return h.bridge.SendChannelText(ctx, platform, chatID, text)
}

// SendChannelFile delivers a published file through the bound bot.
func (h *Host) SendChannelFile(ctx context.Context, platform, chatID, path, name, mediaType, caption string) error {
	if h.bridge == nil {
		return bridge.ErrNoChannel
	}
	return h.bridge.SendChannelFile(ctx, platform, chatID, path, name, mediaType, caption)
}

// SendChannelPrompt is the host's half of Projects' optional ChannelPrompter:
// a thread's approvals and questions reach bound chats with buttons.
func (h *Host) SendChannelPrompt(ctx context.Context, platform, chatID string, p projectsapp.ChannelPrompt) (string, error) {
	if h.bridge == nil {
		return "", bridge.ErrNoChannel
	}
	return h.bridge.SendChannelPrompt(ctx, platform, chatID, p)
}

// RestateChannelPrompt restates a prompt once it is settled.
func (h *Host) RestateChannelPrompt(ctx context.Context, platform, chatID, ref, text string) error {
	if h.bridge == nil {
		return bridge.ErrNoChannel
	}
	return h.bridge.RestateChannelPrompt(ctx, platform, chatID, ref, text)
}
