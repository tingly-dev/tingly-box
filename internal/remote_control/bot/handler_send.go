package bot

import (
	"context"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot/feature"
)

func (h *BotHandler) SendText(hCtx HandlerContext, text string) {
	opts := &imbot.SendMessageOptions{
		Text:      text,
		ParseMode: imbot.ParseModeMarkdown,
	}
	// Forward context_token from incoming message metadata (required by Weixin)
	if hCtx.Message.Metadata != nil {
		if ct, ok := hCtx.Message.Metadata["context_token"].(string); ok {
			if opts.Metadata == nil {
				opts.Metadata = make(map[string]interface{})
			}
			opts.Metadata["context_token"] = ct
		}
	}
	resp, err := hCtx.Bot.SendMessage(context.Background(), hCtx.ChatID, opts)
	_ = resp
	if err != nil {
		logrus.WithError(err).Warn("Failed to send message")
	}
}

// sendTextWithReply sends a text message as a reply to another message
// Note: Platform handles chunking internally via BaseBot.ChunkText()
func (h *BotHandler) sendTextWithReply(hCtx HandlerContext, text string, replyTo string) {
	opts := &imbot.SendMessageOptions{
		Text:      text,
		ParseMode: imbot.ParseModeMarkdown,
		ReplyTo:   replyTo,
	}
	// Forward context_token from incoming message metadata (required by Weixin)
	if hCtx.Message.Metadata != nil {
		if ct, ok := hCtx.Message.Metadata["context_token"].(string); ok {
			if opts.Metadata == nil {
				opts.Metadata = make(map[string]interface{})
			}
			opts.Metadata["context_token"] = ct
		}
	}
	_, err := hCtx.Bot.SendMessage(context.Background(), hCtx.ChatID, opts)
	if err != nil {
		logrus.WithError(err).Warn("Failed to send message")
	}
}

// sendTextWithActionKeyboard sends a text message with Clear/Bind action buttons
// Note: Manual chunking is kept here because the keyboard should only be attached to the LAST chunk.
// The platform's BaseBot.ChunkText() doesn't support "last chunk only" metadata yet.
// TODO: Add platform support for metadata that only applies to the last chunk.
func (h *BotHandler) sendTextWithActionKeyboard(hCtx HandlerContext, text string, replyTo string) {
	kb := feature.BuildActionKeyboard()
	tgKeyboard := imbot.BuildTelegramActionKeyboard(kb.Build())

	// Extract context_token from incoming message metadata (required by Weixin)
	var contextToken string
	if hCtx.Message.Metadata != nil {
		if ct, ok := hCtx.Message.Metadata["context_token"].(string); ok {
			contextToken = ct
		}
	}

	actionCard := feature.BuildActionCard()

	// Use public ChunkText API with smart break-point detection
	chunks := hCtx.Bot.ChunkText(text)
	for i, chunk := range chunks {
		opts := &imbot.SendMessageOptions{
			Text: chunk,
		}
		if replyTo != "" {
			opts.ReplyTo = replyTo
		}
		// Only attach keyboard to the last chunk
		if i == len(chunks)-1 {
			opts.Metadata = h.buildTrackedActionMenuMetadata(hCtx, tgKeyboard, actionCard)
		}
		// Forward context_token for Weixin
		if contextToken != "" {
			if opts.Metadata == nil {
				opts.Metadata = make(map[string]interface{})
			}
			opts.Metadata["context_token"] = contextToken
		}

		result, err := hCtx.Bot.SendMessage(context.Background(), hCtx.ChatID, opts)
		if err != nil {
			logrus.WithError(err).Warn("Failed to send message")
			return
		}

		// Track the action menu message ID for later removal
		if i == len(chunks)-1 && result != nil {
			h.actionMenuMessageIDMu.Lock()
			h.actionMenuMessageID[hCtx.ChatID] = result.MessageID
			h.actionMenuMessageIDMu.Unlock()
		}
	}
}

// formatResponseWithHeader adds project/session/user metadata to the response
// Meta information includes: agent type, project path, chat_id, user_id, session_id
// behavior.Debug controls whether meta information is shown
// formatResponseWithHeader adds project/session/user metadata to the response
// Meta information includes: agent type, project path, chat_id, user_id, session_id
// Set showMeta=true to display meta (e.g., for help), false for regular messages
// behavior.Verbose controls whether processing messages are sent (handled elsewhere)
