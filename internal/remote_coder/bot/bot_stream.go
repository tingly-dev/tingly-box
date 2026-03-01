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

	// Try to handle as unified AgentMessage first
	if agentMsg, ok := msg.(agentboot.AgentMessage); ok {
		return h.handleAgentMessage(agentMsg)
	}

	// Handle specific types
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
		}
		logrus.Infof("assistant message from claude agent")
		return h.handleClaudeMessage(m)

	case claude.Message:
		return h.handleClaudeMessage(m)

	case agentboot.Event:
		// Convert Event to AgentMessage and handle
		agentType := agentboot.AgentTypeMockAgent // default
		if at, ok := m.Data["agent_type"].(string); ok {
			agentType = agentboot.AgentType(at)
		}
		agentMsg := agentboot.MessageFromEvent(m, agentType)
		if agentMsg != nil {
			return h.handleAgentMessage(agentMsg)
		}
		return h.handleAgentbootEvent(m)

	case map[string]interface{}:
		// Handle raw map messages (legacy support)
		return h.handleMapMessage(m)

	default:
		// Skip unknown message types
		logrus.WithField("msgType", fmt.Sprintf("%T", msg)).Debug("Skipping unknown message type")
		return nil
	}
}

// handleAgentMessage processes unified agentboot.AgentMessage
func (h *streamingMessageHandler) handleAgentMessage(msg agentboot.AgentMessage) error {
	logrus.WithFields(logrus.Fields{
		"type":      msg.GetType(),
		"agentType": msg.GetAgentType(),
		"chatID":    h.chatID,
	}).Debug("Handling unified agent message")

	switch msg.GetType() {
	case agentboot.EventTypeAssistant:
		// Assistant message - send to user
		if assistantMsg, ok := msg.(*agentboot.AssistantMessage); ok {
			text := assistantMsg.GetText()
			if strings.TrimSpace(text) != "" {
				h.sendMessage(text)
			}
		}
		return nil

	case agentboot.EventTypePermissionRequest:
		// Permission requests are handled by IMPrompter directly
		// Just log for visibility
		if permMsg, ok := msg.(*agentboot.PermissionRequestMessage); ok {
			logrus.WithFields(logrus.Fields{
				"request_id": permMsg.RequestID,
				"tool_name":  permMsg.ToolName,
				"step":       permMsg.Step,
				"total":      permMsg.Total,
			}).Info("Permission request received (handled by IMPrompter)")
		}
		return nil

	case agentboot.EventTypePermissionResult:
		if permResultMsg, ok := msg.(*agentboot.PermissionResultMessage); ok {
			status := "denied"
			if permResultMsg.Approved {
				status = "approved"
			}
			logrus.WithFields(logrus.Fields{
				"request_id": permResultMsg.RequestID,
				"status":     status,
			}).Debug("Permission result received")
		}
		return nil

	case agentboot.EventTypeResult:
		// Result events are handled by OnComplete
		if resultMsg, ok := msg.(*agentboot.ResultMessage); ok {
			logrus.WithFields(logrus.Fields{
				"status":  resultMsg.Status,
				"message": resultMsg.Message,
			}).Info("Agent result event received")
		}
		return nil

	case agentboot.EventTypeInit:
		logrus.WithField("agentType", msg.GetAgentType()).Debug("Agent init event received")
		return nil

	case agentboot.EventTypeStreamDelta:
		if deltaMsg, ok := msg.(*agentboot.StreamDeltaMessage); ok {
			// For streaming, we could accumulate or send directly
			// For now, just log
			logrus.WithField("delta", deltaMsg.Delta).Debug("Stream delta received")
		}
		return nil

	default:
		logrus.WithField("type", msg.GetType()).Debug("Unhandled agent message type")
		return nil
	}
}

// handleClaudeMessage processes claude-specific messages
func (h *streamingMessageHandler) handleClaudeMessage(claudeMsg claude.Message) error {
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

// handleAgentbootEvent processes agentboot.Event messages (fallback for unknown event types)
func (h *streamingMessageHandler) handleAgentbootEvent(event agentboot.Event) error {
	logrus.WithFields(logrus.Fields{
		"eventType": event.Type,
		"chatID":    h.chatID,
	}).Debug("Handling agentboot event")

	switch event.Type {
	case agentboot.EventTypeAssistant:
		// Handle assistant message from event
		if msg, ok := event.Data["message"].(string); ok && strings.TrimSpace(msg) != "" {
			h.sendMessage(msg)
		} else if msg, ok := event.Data["text"].(string); ok && strings.TrimSpace(msg) != "" {
			h.sendMessage(msg)
		}
	case agentboot.EventTypePermissionRequest:
		logrus.WithFields(logrus.Fields{
			"request_id": event.Data["request_id"],
			"tool_name":  event.Data["tool_name"],
		}).Info("Permission request event received (handled by IMPrompter)")
	case agentboot.EventTypePermissionResult:
		logrus.WithField("request_id", event.Data["request_id"]).Debug("Permission result event")
	case agentboot.EventTypeResult:
		status, _ := event.Data["status"].(string)
		logrus.WithField("status", status).Info("Agent result event received")
	case agentboot.EventTypeInit, agentboot.EventTypeSystem:
		logrus.WithField("data", event.Data).Debug("System/init event received")
	default:
		logrus.WithField("eventType", event.Type).Debug("Unhandled event type")
	}
	return nil
}

// handleMapMessage processes raw map messages (legacy support)
func (h *streamingMessageHandler) handleMapMessage(m map[string]interface{}) error {
	msgType, ok := m["type"].(string)
	if !ok {
		logrus.WithField("map", m).Debug("Map message without type field")
		return nil
	}

	logrus.WithFields(logrus.Fields{
		"type":   msgType,
		"chatID": h.chatID,
	}).Debug("Handling map message")

	switch msgType {
	case agentboot.EventTypePermissionRequest:
		// Permission requests come from mock agent before going through IMPrompter
		data, _ := m["data"].(map[string]interface{})
		if data != nil {
			logrus.WithFields(logrus.Fields{
				"request_id": data["request_id"],
				"tool_name":  data["tool_name"],
			}).Info("Permission request received (will be handled by IMPrompter)")
		}
	case agentboot.EventTypeAssistant:
		// Assistant message - check for message in data
		if data, ok := m["data"].(map[string]interface{}); ok {
			if msg, ok := data["message"].(string); ok && strings.TrimSpace(msg) != "" {
				h.sendMessage(msg)
			}
		} else if msg, ok := m["message"].(string); ok && strings.TrimSpace(msg) != "" {
			h.sendMessage(msg)
		} else if msg, ok := m["text"].(string); ok && strings.TrimSpace(msg) != "" {
			h.sendMessage(msg)
		}
	default:
		logrus.WithField("type", msgType).Debug("Unhandled map message type")
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
