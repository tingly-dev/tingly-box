package bot

import (
	"context"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot/feature"
)

// handleCallbackQuery handles callback queries from inline keyboards
func (h *BotHandler) handleCallbackQuery(bot imbot.Bot, chatID string, msg imbot.Message) {
	// The payload arrives as segments, already decoded by the platform. What
	// those segments looked like on the wire — a colon-joined string, a token
	// standing in for an oversized one, a JSON array in a card button value —
	// is the platform's business, not this dispatcher's.
	payload := msg.Payload
	if payload.IsEmpty() {
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

	switch payload.Name() {
	case "perm":
		// Handle permission request response
		h.handlePermissionCallback(hCtx, payload)

	case "action":
		subAction := payload.Arg(1)
		if subAction == "" {
			return
		}
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
		if payload.Arg(1) == "switch" {
			if projectID := payload.Arg(2); projectID != "" {
				h.handleProjectSwitch(hCtx, projectID)
			}
		}

	case "resume":
		subAction := payload.Arg(1)
		if subAction == "" {
			return
		}
		switch subAction {
		case "pick":
			sessionID := payload.Arg(2)
			if sessionID == "" {
				return
			}
			h.handleResumePick(hCtx, sessionID, msg)
		case "cancel":
			h.handleResumeCancel(hCtx, msg)
		}

	case "bind":
		// Remove the action keyboard before handling
		h.removeActionKeyboard(bot, chatID)
		// Start interactive bind
		subAction := payload.Arg(1)
		if subAction == "" {
			return
		}
		switch subAction {
		case "confirm":
			// Confirm bind to current directory
			h.handleBindConfirm(hCtx)

		case "dir":
			// Navigate to the directory the button names.
			target := payload.Arg(2)
			if target == "" {
				return
			}
			if err := h.directoryBrowser.Navigate(chatID, target); err != nil {
				logrus.WithError(err).Warn("Failed to navigate directory")
				return
			}
			// Get message ID from metadata for editing
			msgID, _ := msg.Metadata["message_id"].(string)
			if msgID == "" {
				msgID = msg.ID
			}
			_, _ = feature.SendDirectoryBrowser(h.ctx, bot, h.directoryBrowser, chatID, msgID)

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
			_, _ = feature.SendDirectoryBrowser(h.ctx, bot, h.directoryBrowser, chatID, msgID)

		case "prev":
			if err := h.directoryBrowser.PrevPage(chatID); err != nil {
				logrus.WithError(err).Warn("Failed to go to previous page")
				return
			}
			msgID, _ := msg.Metadata["message_id"].(string)
			if msgID == "" {
				msgID = msg.ID
			}
			_, _ = feature.SendDirectoryBrowser(h.ctx, bot, h.directoryBrowser, chatID, msgID)

		case "next":
			if err := h.directoryBrowser.NextPage(chatID); err != nil {
				logrus.WithError(err).Warn("Failed to go to next page")
				return
			}
			msgID, _ := msg.Metadata["message_id"].(string)
			if msgID == "" {
				msgID = msg.ID
			}
			_, _ = feature.SendDirectoryBrowser(h.ctx, bot, h.directoryBrowser, chatID, msgID)

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
			// Create the directory the confirmation button carries and bind it.
			path := payload.Arg(2)
			if path == "" {
				return
			}
			if err := os.MkdirAll(path, 0755); err != nil {
				logrus.WithError(err).Error("Failed to create directory")
				h.SendText(hCtx, fmt.Sprintf("Failed to create directory: %v", err))
				return
			}
			// Creating the directory was only half of what the button offered.
			// Without this the flow ended in silence: the directory appeared
			// on disk and the chat said nothing, so the user had no way to
			// tell whether the bind had happened.
			h.completeBind(hCtx, path)
			h.directoryBrowser.Clear(chatID)

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
		currentPath, _, _ := h.chatStore.GetProjectPath(hCtx.ChatID)
		var err error
		state, err = h.directoryBrowser.StartAt(hCtx.ChatID, currentPath)
		if err != nil {
			h.SendText(hCtx, fmt.Sprintf("Failed to start bind flow: %v", err))
			return

		}
	}

	// Set waiting for input state
	h.directoryBrowser.SetWaitingInput(hCtx.ChatID, true, "")

	// Send prompt with cancel keyboard
	kb := feature.BuildCancelKeyboard()

	result, err := hCtx.Bot.SendMessage(context.Background(), hCtx.ChatID, &imbot.SendMessageOptions{
		Text:      BuildCustomPathPrompt(),
		ParseMode: imbot.ParseModeMarkdown,
		Actions:   kb.BuildActions(),
		Metadata:  trackActionMenuMetadata(),
	})
	if err != nil {
		logrus.WithError(err).Error("Failed to send custom path prompt")
		return

	}

	// Store prompt message ID
	h.directoryBrowser.SetWaitingInput(hCtx.ChatID, true, result.MessageID)
}

// handlePermissionCallback handles permission request callback responses.
// Only reachable in standalone (host-less) mode: the managed path's host
// router claims "perm" callbacks first. Mechanics shared via prompt_reply.go;
// as the terminal handler it claims unknown request IDs and reports expired.
func (h *BotHandler) handlePermissionCallback(hCtx HandlerContext, payload imbot.Payload) {
	handlePromptCallback(h.imPrompter, func(text string) { h.SendText(hCtx, text) }, hCtx.SenderID, payload, true)
}

// handleCreateConfirm sends a confirmation prompt for creating a directory
func (h *BotHandler) handleCreateConfirm(hCtx HandlerContext, path string) {
	// Reset waiting input state (no longer waiting for text input)
	h.directoryBrowser.SetWaitingInput(hCtx.ChatID, false, "")

	kb, text := feature.BuildCreateConfirmKeyboard(path)

	_, err := hCtx.Bot.SendMessage(context.Background(), hCtx.ChatID, &imbot.SendMessageOptions{
		Text:      text,
		ParseMode: imbot.ParseModeMarkdown,
		Actions:   kb.BuildActions(),
		Metadata:  trackActionMenuMetadata(),
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

	// Take the menu down wherever the platform supports it. Nil actions mean
	// "no controls"; empty text leaves the message body alone.
	ref := imbot.MessageRef{ChatID: chatID, MessageID: msgID}
	if imbot.RestateOrIgnore(context.Background(), bot, ref, imbot.RestateOptions{}) {
		h.actionMenuMessageIDMu.Lock()
		delete(h.actionMenuMessageID, chatID)
		h.actionMenuMessageIDMu.Unlock()
	} else {
		logrus.WithField("chatID", chatID).WithField("messageID", msgID).Debug("Could not remove action keyboard")
	}
}

// editDirectoryBrowserMessage restates the directory browser message: new
// status text, no controls.
func editDirectoryBrowserMessage(ctx context.Context, bot imbot.Bot, chatID string, msgID string, text string) {
	ref := imbot.MessageRef{ChatID: chatID, MessageID: msgID}
	if !imbot.RestateOrIgnore(ctx, bot, ref, imbot.RestateOptions{Text: text}) {
		logrus.WithField("chatID", chatID).WithField("messageID", msgID).
			Debug("Could not restate directory browser message")
	}
}
