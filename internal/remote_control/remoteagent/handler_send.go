package remoteagent

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
)

func (h *BotHandler) SendText(hCtx HandlerContext, text string) {
	b := h.botFromCtx(hCtx)
	if b == nil {
		logrus.WithField("chatID", hCtx.ChatID).Warn("SendText: no bot available")
		return
	}
	opts := &imbot.SendMessageOptions{
		Text:      text,
		ParseMode: imbot.ParseModeMarkdown,
	}
	bot.ForwardReplyContext(opts, hCtx.Message)
	resp, err := b.SendMessage(context.Background(), hCtx.ChatID, opts)
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
	b := h.botFromCtx(hCtx)
	if b == nil {
		logrus.WithField("chatID", hCtx.ChatID).Warn("sendTextWithReply: no bot available")
		return
	}
	opts := &imbot.SendMessageOptions{
		Text:      text,
		ParseMode: imbot.ParseModeMarkdown,
		ReplyTo:   replyTo,
	}
	bot.ForwardReplyContext(opts, hCtx.Message)
	_, err := b.SendMessage(context.Background(), hCtx.ChatID, opts)
	if err != nil {
		logrus.WithError(err).Warn("Failed to send message")
	}
}

// newStreamingMessageHandler creates the per-execution streaming chat writer.
func (h *BotHandler) newStreamingMessageHandler(hCtx HandlerContext) *streamingMessageHandler {
	return newStreamingMessageHandler(hCtx.Bot, hCtx.ChatID, hCtx.MessageID, h.GetVerbose(hCtx.ChatID))
}
