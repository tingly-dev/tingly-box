package bot

import (
	"context"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot/feature"
)

func (h *BotHandler) SendText(hCtx HandlerContext, text string) {
	bot := h.botFromCtx(hCtx)
	if bot == nil {
		logrus.WithField("chatID", hCtx.ChatID).Warn("SendText: no bot available")
		return
	}
	opts := &imbot.SendMessageOptions{
		Text:      text,
		ParseMode: imbot.ParseModeMarkdown,
	}
	forwardReplyContext(opts, hCtx.Message)
	resp, err := bot.SendMessage(context.Background(), hCtx.ChatID, opts)
	_ = resp
	if err != nil {
		logrus.WithError(err).Warn("Failed to send message")
	}
}

// botFromCtx returns the bot from the handler context, falling back to
// looking it up via the manager when commands routed through the registry
// adapter (command_integration.go) construct a HandlerContext without
// populating Bot.
func (h *BotHandler) botFromCtx(hCtx HandlerContext) imbot.Bot {
	if hCtx.Bot != nil {
		return hCtx.Bot
	}
	if h.manager == nil {
		return nil
	}
	return h.manager.GetBotByUUID(h.botSetting.UUID)
}

// sendTextWithReply sends a text message as a reply to another message
// Note: Platform handles chunking internally via BaseBot.ChunkText()
func (h *BotHandler) sendTextWithReply(hCtx HandlerContext, text string, replyTo string) {
	bot := h.botFromCtx(hCtx)
	if bot == nil {
		logrus.WithField("chatID", hCtx.ChatID).Warn("sendTextWithReply: no bot available")
		return
	}
	opts := &imbot.SendMessageOptions{
		Text:      text,
		ParseMode: imbot.ParseModeMarkdown,
		ReplyTo:   replyTo,
	}
	forwardReplyContext(opts, hCtx.Message)
	_, err := bot.SendMessage(context.Background(), hCtx.ChatID, opts)
	if err != nil {
		logrus.WithError(err).Warn("Failed to send message")
	}
}

// sendTextWithActionKeyboard sends a text message with Clear/Bind action buttons
// Note: Manual chunking is kept here because the keyboard should only be attached to the LAST chunk.
// The platform's BaseBot.ChunkText() doesn't support "last chunk only" metadata yet.
// TODO: Add platform support for metadata that only applies to the last chunk.
func (h *BotHandler) sendTextWithActionKeyboard(hCtx HandlerContext, text string, replyTo string) {
	if strings.TrimSpace(text) == "" {
		// ChunkText("") returns [""] which would otherwise produce an empty
		// outbound bubble. Drop silently — there's nothing to render.
		return
	}
	kb := feature.BuildActionKeyboard()

	bot := h.botFromCtx(hCtx)
	if bot == nil {
		logrus.WithField("chatID", hCtx.ChatID).Warn("sendTextWithActionKeyboard: no bot available")
		return
	}

	// Use public ChunkText API with smart break-point detection
	chunks := bot.ChunkText(text)
	for i, chunk := range chunks {
		opts := &imbot.SendMessageOptions{
			Text: chunk,
		}
		if replyTo != "" {
			opts.ReplyTo = replyTo
		}
		// Only attach the action menu to the last chunk
		if i == len(chunks)-1 {
			opts.Actions = kb.BuildActions()
			opts.Metadata = trackActionMenuMetadata()
		}
		forwardReplyContext(opts, hCtx.Message)

		result, err := bot.SendMessage(context.Background(), hCtx.ChatID, opts)
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
