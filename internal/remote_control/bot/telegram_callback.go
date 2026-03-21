package bot

import (
	"context"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot/ask"
	"github.com/tingly-dev/tingly-box/imbot"
)

// handleCallbackQuery handles callback queries from inline keyboards
func (h *BotHandler) handleCallbackQuery(bot imbot.Bot, chatID string, msg imbot.Message) {
	callbackData, _ := msg.Metadata["callback_data"].(string)
	if callbackData == "" {
		return
	}

	parts := imbot.ParseCallbackData(callbackData)
	if len(parts) == 0 {
		return
	}

	// Create a minimal handler context for callbacks
	hCtx := HandlerContext{
		Bot:       bot,
		ChatID:    chatID,
		SenderID:  msg.Sender.ID,
		MessageID: msg.ID,
		Platform:  msg.Platform,
	}

	action := parts[0]

	switch action {
	case "perm":
		// Handle permission request response
		h.handlePermissionCallback(hCtx, parts)

	case "action":
		if len(parts) < 2 {
			return
		}
		subAction := parts[1]
		switch subAction {
		case "clear":
			// Remove the action keyboard before handling
			h.removeActionKeyboard(bot, chatID)
			h.handleClearCommand(hCtx)
		case "bind":
			// Remove the action keyboard before handling
			h.removeActionKeyboard(bot, chatID)
			// Start interactive bind
			// Start interactive bind
			h.handleBindInteractive(hCtx)
		case "project":
			// Remove the action keyboard before handling
			h.removeActionKeyboard(bot, chatID)
			// Start interactive bind
			// Start interactive bind
			h.handleBotProjectCommand(hCtx)
		}

	case "project":
		// Remove the action keyboard before handling
		h.removeActionKeyboard(bot, chatID)
		// Start interactive bind
		if len(parts) < 3 {
			return
		}
		subAction := parts[1]
		switch subAction {
		case "switch":
			projectID := parts[2]
			h.handleProjectSwitch(hCtx, projectID)
		}

	case "bind":
		// Remove the action keyboard before handling
		h.removeActionKeyboard(bot, chatID)
		// Start interactive bind
		if len(parts) < 2 {
			return
		}
		subAction := parts[1]
		switch subAction {
		case "confirm":
			// Confirm bind to current directory
			h.handleBindConfirm(hCtx)

		case "dir":
			// Navigate to directory by index
			if len(parts) < 3 {
				return
			}
			indexStr := parts[2]
			var index int
			if _, err := fmt.Sscanf(indexStr, "%d", &index); err != nil {
				logrus.WithError(err).Warn("Failed to parse directory index")
				return
			}
			if err := h.directoryBrowser.NavigateByIndex(chatID, index); err != nil {
				logrus.WithError(err).Warn("Failed to navigate directory")
				return
			}
			// Get message ID from metadata for editing
			msgID, _ := msg.Metadata["message_id"].(string)
			if msgID == "" {
				msgID = msg.ID
			}
			_, _ = SendDirectoryBrowser(h.ctx, bot, h.directoryBrowser, chatID, msgID)

		case "up":
			// Navigate to parent directory
			if err := h.directoryBrowser.NavigateUp(chatID); err != nil {
				logrus.WithError(err).Warn("Failed to navigate up")
				return
			}
			msgID, _ := msg.Metadata["message_id"].(string)
			if msgID == "" {
				msgID = msg.ID
			}
			_, _ = SendDirectoryBrowser(h.ctx, bot, h.directoryBrowser, chatID, msgID)

		case "prev":
			if err := h.directoryBrowser.PrevPage(chatID); err != nil {
				logrus.WithError(err).Warn("Failed to go to previous page")
				return
			}
			msgID, _ := msg.Metadata["message_id"].(string)
			if msgID == "" {
				msgID = msg.ID
			}
			_, _ = SendDirectoryBrowser(h.ctx, bot, h.directoryBrowser, chatID, msgID)

		case "next":
			if err := h.directoryBrowser.NextPage(chatID); err != nil {
				logrus.WithError(err).Warn("Failed to go to next page")
				return
			}
			msgID, _ := msg.Metadata["message_id"].(string)
			if msgID == "" {
				msgID = msg.ID
			}
			_, _ = SendDirectoryBrowser(h.ctx, bot, h.directoryBrowser, chatID, msgID)

		case "select":
			// Select current directory (path is in state)
			currentPath := h.directoryBrowser.GetCurrentPath(chatID)
			if currentPath == "" {
				logrus.Warn("No current path in bind flow")
				return
			}
			// Get message ID from state before clearing
			msgID := ""
			if state := h.directoryBrowser.GetState(chatID); state != nil {
				msgID = state.MessageID
			}
			h.completeBind(hCtx, currentPath)
			h.directoryBrowser.Clear(chatID)
			// Edit message to show success and remove keyboard
			if msgID != "" {
				editDirectoryBrowserMessage(h.ctx, bot, chatID, msgID, fmt.Sprintf("✅ Bound to: `%s`", currentPath))
			}

		case "custom":
			// Start custom path input mode
			h.handleCustomPathPrompt(hCtx)

		case "create":
			// Create directory and bind (path from custom input, encoded)
			if len(parts) < 3 {
				return
			}
			encodedPath := parts[2]
			path := imbot.ParseDirPath(encodedPath)
			// Create the directory
			if err := os.MkdirAll(path, 0755); err != nil {
				logrus.WithError(err).Error("Failed to create directory")
				h.SendText(hCtx, fmt.Sprintf("Failed to create directory: %v", err))
				return
			}

		case "cancel":
			h.directoryBrowser.Clear(chatID)
			// Get message ID from metadata for editing
			msgID, _ := msg.Metadata["message_id"].(string)
			if msgID == "" {
				msgID = msg.ID
			}
			// Edit message to show cancel and remove keyboard
			editDirectoryBrowserMessage(h.ctx, bot, chatID, msgID, "❌ Bind cancelled.")
			h.SendText(hCtx, "Bind cancelled.")
		}
	}
}

// handleCustomPathPrompt sends the custom path input prompt
func (h *BotHandler) handleCustomPathPrompt(hCtx HandlerContext) {
	// Ensure state exists
	state := h.directoryBrowser.GetState(hCtx.ChatID)
	if state == nil {
		// Start a new bind flow if none exists
		var err error
		state, err = h.directoryBrowser.Start(hCtx.ChatID)
		if err != nil {
			h.SendText(hCtx, fmt.Sprintf("Failed to start bind flow: %v", err))
			return

		}
	}

	// Set waiting for input state
	h.directoryBrowser.SetWaitingInput(hCtx.ChatID, true, "")

	// Send prompt with cancel keyboard
	kb := BuildCancelKeyboard()
	tgKeyboard := imbot.BuildTelegramActionKeyboard(kb.Build())

	result, err := hCtx.Bot.SendMessage(context.Background(), hCtx.ChatID, &imbot.SendMessageOptions{
		Text:      BuildCustomPathPrompt(),
		ParseMode: imbot.ParseModeMarkdown,
		Metadata: map[string]interface{}{
			"replyMarkup":        tgKeyboard,
			"_trackActionMenuID": true,
		},
	})
	if err != nil {
		logrus.WithError(err).Error("Failed to send custom path prompt")
		return

	}

	// Store prompt message ID
	h.directoryBrowser.SetWaitingInput(hCtx.ChatID, true, result.MessageID)
}

// handlePermissionCallback handles permission request callback responses
func (h *BotHandler) handlePermissionCallback(hCtx HandlerContext, parts []string) {
	if len(parts) < 3 {
		logrus.WithField("parts", parts).Warn("Invalid permission callback data")
		return
	}

	subAction := parts[1]
	requestID := parts[2]

	// Check if the request exists
	pendingReq, exists := h.imPrompter.GetPendingRequest(requestID)
	if !exists {
		logrus.WithField("request_id", requestID).Warn("Permission request not found or expired")
		h.SendText(hCtx, "⚠️ This permission request has expired or already been answered.")
		return
	}

	var resultText string

	switch subAction {
	case "allow":
		if err := h.imPrompter.SubmitDecision(requestID, true, false, ""); err != nil {
			logrus.WithError(err).WithField("request_id", requestID).Error("Failed to submit permission decision")
			h.SendText(hCtx, fmt.Sprintf("Failed to process permission response: %v", err))
			return
		}
		resultText = "✅ Permission granted"
		logrus.WithFields(logrus.Fields{
			"request_id": requestID,
			"tool_name":  pendingReq.ToolName,
			"user_id":    hCtx.SenderID,
		}).Info("User approved tool permission")

	case "deny":
		if err := h.imPrompter.SubmitDecision(requestID, false, false, ""); err != nil {
			logrus.WithError(err).WithField("request_id", requestID).Error("Failed to submit permission decision")
			h.SendText(hCtx, fmt.Sprintf("Failed to process permission response: %v", err))
			return
		}
		resultText = "❌ Permission denied"
		logrus.WithFields(logrus.Fields{
			"request_id": requestID,
			"tool_name":  pendingReq.ToolName,
			"user_id":    hCtx.SenderID,
		}).Info("User denied tool permission")

	case "always":
		if err := h.imPrompter.SubmitDecision(requestID, true, true, ""); err != nil {
			logrus.WithError(err).WithField("request_id", requestID).Error("Failed to submit permission decision")
			h.SendText(hCtx, fmt.Sprintf("Failed to process permission response: %v", err))
			return
		}
		resultText = "🔄 Always allowed"
		logrus.WithFields(logrus.Fields{
			"request_id": requestID,
			"tool_name":  pendingReq.ToolName,
			"user_id":    hCtx.SenderID,
		}).Info("User approved tool permission (always)")

	case "option":
		// Handle multi-option selection (e.g., AskUserQuestion)
		if len(parts) < 4 {
			logrus.WithField("parts", parts).Warn("Invalid option callback data")
			return
		}
		optionIndex := parts[3]

		// Convert index to label from the pending request
		optionLabel := optionIndex
		if questions, ok := pendingReq.Input["questions"].([]interface{}); ok && len(questions) > 0 {
			if question, ok := questions[0].(map[string]interface{}); ok {
				if options, ok := question["options"].([]interface{}); ok {
					// Parse index
					var idx int
					if _, err := fmt.Sscanf(optionIndex, "%d", &idx); err == nil && idx >= 0 && idx < len(options) {
						if option, ok := options[idx].(map[string]interface{}); ok {
							if label, ok := option["label"].(string); ok {
								optionLabel = label
							}
						}
					}
				}
			}
		}

		// Submit as a structured response with the label
		if err := h.imPrompter.SubmitUserResponse(requestID, ask.Response{
			Type: "selection",
			Data: optionLabel,
		}); err != nil {
			logrus.WithError(err).WithField("request_id", requestID).Error("Failed to submit option selection")
			h.SendText(hCtx, fmt.Sprintf("Failed to process option selection: %v", err))
			return
		}
		resultText = fmt.Sprintf("✅ Selected: %s", optionLabel)
		logrus.WithFields(logrus.Fields{
			"request_id":   requestID,
			"tool_name":    pendingReq.ToolName,
			"option_index": optionIndex,
			"option_label": optionLabel,
			"user_id":      hCtx.SenderID,
		}).Info("User selected option")

	default:
		logrus.WithField("action", subAction).Warn("Unknown permission action")
		return
	}

	// Send feedback to user
	h.SendText(hCtx, fmt.Sprintf("%s for tool: `%s`", resultText, pendingReq.ToolName))
}

// handleCreateConfirm sends a confirmation prompt for creating a directory
func (h *BotHandler) handleCreateConfirm(hCtx HandlerContext, path string) {
	// Reset waiting input state (no longer waiting for text input)
	h.directoryBrowser.SetWaitingInput(hCtx.ChatID, false, "")

	kb, text := BuildCreateConfirmKeyboard(path)
	tgKeyboard := imbot.BuildTelegramActionKeyboard(kb.Build())

	_, err := hCtx.Bot.SendMessage(context.Background(), hCtx.ChatID, &imbot.SendMessageOptions{
		Text:      text,
		ParseMode: imbot.ParseModeMarkdown,
		Metadata: map[string]interface{}{
			"replyMarkup":        tgKeyboard,
			"_trackActionMenuID": true,
		},
	})
	if err != nil {
		logrus.WithError(err).Error("Failed to send create confirmation")
	}
}

// removeActionKeyboard removes the action keyboard menu from the chat
func (h *BotHandler) removeActionKeyboard(bot imbot.Bot, chatID string) {
	h.actionMenuMessageIDMu.RLock()
	msgID, ok := h.actionMenuMessageID[chatID]
	h.actionMenuMessageIDMu.RUnlock()

	if !ok || msgID == "" {
		return
	}

	// Try to cast to TelegramBot and remove the keyboard
	if tgBot, ok := imbot.AsTelegramBot(bot); ok {
		if err := tgBot.RemoveMessageKeyboard(context.Background(), chatID, msgID); err != nil {
			logrus.WithError(err).WithField("chatID", chatID).WithField("messageID", msgID).Debug("Failed to remove action keyboard")
		} else {
			// Successfully removed, clear the tracking
			h.actionMenuMessageIDMu.Lock()
			delete(h.actionMenuMessageID, chatID)
			h.actionMenuMessageIDMu.Unlock()
		}
	}
}

// editDirectoryBrowserMessage edits the directory browser message to show status and remove keyboard
func editDirectoryBrowserMessage(ctx context.Context, bot imbot.Bot, chatID string, msgID string, text string) {
	if tgBot, ok := imbot.AsTelegramBot(bot); ok {
		// Remove the keyboard first
		if err := tgBot.RemoveMessageKeyboard(ctx, chatID, msgID); err != nil {
			logrus.WithError(err).WithField("chatID", chatID).WithField("messageID", msgID).Debug("Failed to remove directory browser keyboard")
		} else {
			// Successfully removed keyboard, now edit the text
			if err := tgBot.EditMessageWithKeyboard(ctx, chatID, msgID, text, nil); err != nil {
				logrus.WithError(err).WithField("chatID", chatID).WithField("messageID", msgID).Debug("Failed to edit directory browser text")
			}
		}
	}
}
