package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/imbot"
)

// streamingMessageHandler implements agentboot.MessageHandler for real-time message streaming
type streamingMessageHandler struct {
	bot       imbot.Bot
	chatID    string
	replyTo   string
	mu        sync.Mutex
	formatter *claude.TextFormatter
}

// newStreamingMessageHandler creates a new streaming message handler
func newStreamingMessageHandler(bot imbot.Bot, chatID, replyTo string) *streamingMessageHandler {
	return &streamingMessageHandler{
		bot:       bot,
		chatID:    chatID,
		replyTo:   replyTo,
		formatter: claude.NewTextFormatter(),
	}
}

// OnMessage implements agentboot.MessageHandler
func (h *streamingMessageHandler) OnMessage(msg interface{}) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	logrus.WithFields(logrus.Fields{
		"msgType": fmt.Sprintf("%T", msg),
		"chatID":  h.chatID,
	}).Debug("Received message from agent")

	// Convert to claude.Message if possible
	var claudeMsg claude.Message
	switch m := msg.(type) {
	case *claude.AssistantMessage:
		meaningful := false
		for _, c := range m.Message.Content {
			logrus.Info(c.Content)
			if strings.TrimSpace(c.Text) != "" {
				meaningful = true
			}
		}
		if !meaningful {
			logrus.Debugf("ignoring non-meaningful message from assistant")
			return nil
		} else {
			claudeMsg = m
			logrus.Infof("assistant message from agent")
		}
	case claude.Message:
		claudeMsg = m
	default:
		// Skip non-claude messages
		logrus.WithField("msgType", fmt.Sprintf("%T", msg)).Debug("Skipping non-claude message")
		return nil
	}

	// Format using the formatter
	formatted := h.formatter.Format(claudeMsg)
	d, _ := json.Marshal(claudeMsg.GetRawData())
	logrus.Infof("[bot] Raw: %s", d)
	logrus.Infof("[bot] Formatted: %s", formatted)

	if strings.TrimSpace(formatted) != "" {
		h.sendMessage(formatted)
	} else {
		logrus.WithField("msgType", claudeMsg.GetType()).Debug("Skipping empty formatted message")
	}

	return nil
}

// OnError implements agentboot.MessageHandler
func (h *streamingMessageHandler) OnError(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sendMessage(fmt.Sprintf("[ERROR] %v", err))
}

// OnComplete implements agentboot.MessageHandler - sends action keyboard when complete
func (h *streamingMessageHandler) OnComplete(result *agentboot.CompletionResult) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Build action keyboard
	kb := BuildActionKeyboard()
	tgKeyboard := convertActionKeyboardToTelegram(kb.Build())

	_, err := h.bot.SendMessage(context.Background(), h.chatID, &imbot.SendMessageOptions{
		Text: "/bot tips",
		Metadata: map[string]interface{}{
			"replyMarkup": tgKeyboard,
		},
	})
	if err != nil {
		logrus.WithError(err).Warn("Failed to send action keyboard")
	}
}

// GetOutput returns the accumulated output (for compatibility, returns empty as we stream immediately)
func (h *streamingMessageHandler) GetOutput() string {
	return ""
}

// sendMessage sends a message to the bot
func (h *streamingMessageHandler) sendMessage(text string) {
	for _, chunk := range chunkText(text, imbot.DefaultMessageLimit) {
		_, err := h.bot.SendMessage(context.Background(), h.chatID, &imbot.SendMessageOptions{
			Text:    chunk,
			ReplyTo: h.replyTo,
		})
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"chatID":  h.chatID,
				"replyTo": h.replyTo,
				"error":   err,
				"chunk":   chunk[:minInt(100, len(chunk))],
			}).Error("Failed to send streaming message")
			continue
		}
		logrus.WithField("chatID", h.chatID).WithField("chunkLen", len(chunk)).Debug("Sent streaming message chunk")
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
