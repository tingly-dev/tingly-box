package bot

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-agentscope/pkg/memory"
	"github.com/tingly-dev/tingly-agentscope/pkg/message"
	"github.com/tingly-dev/tingly-agentscope/pkg/types"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/ask"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/session"
	"github.com/tingly-dev/tingly-box/internal/remote_control/smart_guide"
)

// handlePermissionTextResponse handles text-based permission responses
// Returns true if the message was a valid permission response, false otherwise
func (h *BotHandler) handlePermissionTextResponse(hCtx HandlerContext) bool {
	// Check if there are pending permission requests for this chat
	pendingReqs := h.imPrompter.GetPendingRequestsForChat(hCtx.ChatID)
	if len(pendingReqs) == 0 {
		return false
	}

	// Get the most recent pending request for this chat
	// (usually there's only one at a time)
	latestReq := pendingReqs[0]

	// For AskUserQuestion, try to parse as option selection first
	if latestReq.ToolName == "AskUserQuestion" {
		// Try to submit as a text selection
		if err := h.imPrompter.SubmitUserResponse(latestReq.ID, ask.Response{
			Type: "text",
			Data: hCtx.Text,
		}); err == nil {
			h.SendText(hCtx, fmt.Sprintf("✅ Selected: %s", hCtx.Text))
			logrus.WithFields(logrus.Fields{
				"request_id": latestReq.ID,
				"tool_name":  latestReq.ToolName,
				"user_id":    hCtx.SenderID,
				"selection":  hCtx.Text,
			}).Info("User selected option via text")
			return true
		}
	}

	// Try to parse the text as a standard permission response
	approved, remember, isValid := ask.ParseTextResponse(hCtx.Text)
	if !isValid {
		// Not a valid permission response, let other handlers process it
		return false
	}

	// Submit the decision
	if err := h.imPrompter.SubmitDecision(latestReq.ID, approved, remember, ""); err != nil {
		logrus.WithError(err).WithField("request_id", latestReq.ID).Error("Failed to submit permission decision")
		h.SendText(hCtx, fmt.Sprintf("Failed to process permission response: %v", err))
		return true
	}

	// Send feedback to user
	var resultText string
	if remember {
		resultText = "🔄 Always allowed"
	} else if approved {
		resultText = "✅ Permission granted"
	} else {
		resultText = "❌ Permission denied"
	}

	h.SendText(hCtx, fmt.Sprintf("%s for tool: `%s`", resultText, latestReq.ToolName))

	logrus.WithFields(logrus.Fields{
		"request_id": latestReq.ID,
		"tool_name":  latestReq.ToolName,
		"user_id":    hCtx.SenderID,
		"approved":   approved,
		"remember":   remember,
	}).Info("User responded to permission request via text")

	return true
}

type CompletionCallback struct {
	hCtx       HandlerContext
	sessionID  string
	sessionMgr *session.Manager
}

func (c *CompletionCallback) OnComplete(result *agentboot.CompletionResult) {
	// Update session status based on completion result
	if c.sessionMgr != nil && c.sessionID != "" {
		if result.Success {
			c.sessionMgr.SetCompleted(c.sessionID, "")
		} else {
			c.sessionMgr.SetFailed(c.sessionID, result.Error)
		}
	}

	// Build action keyboard
	kb := BuildActionKeyboard()
	tgKeyboard := imbot.BuildTelegramActionKeyboard(kb.Build())

	_, err := c.hCtx.Bot.SendMessage(context.Background(), c.hCtx.ChatID, &imbot.SendMessageOptions{
		Text: "✅ Task done. \nContinue to chat with this session or /help.",
		Metadata: map[string]interface{}{
			"replyMarkup":        tgKeyboard,
			"_trackActionMenuID": true,
		},
	})
	if err != nil {
		logrus.WithError(err).Warn("Failed to send action keyboard")
	}

	// Log completion event
	logrus.WithFields(logrus.Fields{
		"chatID":    c.hCtx.ChatID,
		"sessionID": c.sessionID,
		"success":   result.Success,
		"duration":  result.DurationMS,
		"error":     result.Error,
	}).Info("Agent execution completed via callback")
}

// SmartGuideCompletionCallback handles completion events for SmartGuide agent
// It saves messages to session, updates project path if changed, and sends response + action keyboard
type SmartGuideCompletionCallback struct {
	hCtx           HandlerContext
	sessionID      string
	chatStore      ChatStoreInterface
	tbSessionStore *smart_guide.SessionStore
	agent          *smart_guide.TinglyBoxAgent
	projectPath    string
	meta           ResponseMeta
	behavior       OutputBehavior
	botHandler     *BotHandler // Add reference to bot handler for formatting
}

// OnComplete implements agentboot.CompletionCallback
func (c *SmartGuideCompletionCallback) OnComplete(result *agentboot.CompletionResult) {
	// Get response text from the agent's memory
	responseText := ""
	if result.Success {
		// Try to get the last assistant message from agent memory
		if mem := c.agent.GetMemory(); mem != nil {
			if hist, ok := mem.(*memory.History); ok {
				messages := hist.GetMessages()
				for i := len(messages) - 1; i >= 0; i-- {
					if messages[i].Role == "assistant" {
						responseText = messages[i].GetTextContent()
						break
					}
				}
			}
		}
	}

	// Save assistant message to session
	if c.tbSessionStore != nil && responseText != "" {
		assistantMsg := message.NewMsg("assistant", responseText, types.RoleAssistant)
		if err := c.tbSessionStore.AddMessage(c.hCtx.ChatID, assistantMsg); err != nil {
			logrus.WithError(err).Warn("Failed to save assistant message to session")
		}
	}

	// Check if working directory was changed by change_workdir tool
	newProjectPath := c.agent.GetExecutor().GetWorkingDirectory()
	if newProjectPath != c.projectPath {
		logrus.WithFields(logrus.Fields{
			"chatID":         c.hCtx.ChatID,
			"oldProjectPath": c.projectPath,
			"newProjectPath": newProjectPath,
		}).Info("Project path changed, updating chat store")

		// Update chat store with new project path
		chat, err := c.chatStore.GetChat(c.hCtx.ChatID)
		if err != nil {
			logrus.WithError(err).WithField("chatID", c.hCtx.ChatID).Warn("Failed to get chat for update")
		}

		if chat == nil {
			now := time.Now().UTC()
			chat = &Chat{
				ChatID:      c.hCtx.ChatID,
				Platform:    string(c.hCtx.Platform),
				ProjectPath: newProjectPath,
				BashCwd:     newProjectPath,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if err := c.chatStore.UpsertChat(chat); err != nil {
				logrus.WithError(err).WithField("chatID", c.hCtx.ChatID).Warn("Failed to create chat")
			}
		} else {
			if err := c.chatStore.UpdateChat(c.hCtx.ChatID, func(ch *Chat) {
				ch.ProjectPath = newProjectPath
				ch.BashCwd = newProjectPath
			}); err != nil {
				logrus.WithError(err).WithField("chatID", c.hCtx.ChatID).Warn("Failed to update chat")
			}
		}
	}

	// Send the final response with meta header, then action keyboard
	if responseText != "" {
		c.botHandler.sendTextWithReply(c.hCtx, c.botHandler.formatResponseWithMeta(c.meta, responseText, c.behavior), c.hCtx.MessageID)
	}

	// Send action keyboard on completion
	kb := BuildActionKeyboard()
	tgKeyboard := imbot.BuildTelegramActionKeyboard(kb.Build())

	_, err := c.hCtx.Bot.SendMessage(context.Background(), c.hCtx.ChatID, &imbot.SendMessageOptions{
		Text: "✅ Task done. \nContinue to chat with this session or /help.",
		Metadata: map[string]interface{}{
			"replyMarkup":        tgKeyboard,
			"_trackActionMenuID": true,
		},
	})
	if err != nil {
		logrus.WithError(err).Warn("Failed to send action keyboard for SmartGuide")
	}

	// Log completion event
	logrus.WithFields(logrus.Fields{
		"chatID":    c.hCtx.ChatID,
		"sessionID": c.sessionID,
		"success":   result.Success,
		"duration":  result.DurationMS,
	}).Info("SmartGuide execution completed via callback")
}
