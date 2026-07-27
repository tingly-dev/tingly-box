package remoteagent

import (
	"context"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/feature"
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
			h.handleClearCommand(hCtx)
		case "bind":
			h.handleBindInteractive(hCtx)
		case "project":
			h.handleBotProjectCommand(hCtx)
		}

	case "project":
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
		subAction := payload.Arg(1)
		if subAction == "" {
			return
		}
		switch subAction {
		case "dir", "up", "prev", "next":
			// Every browser move is "mutate the flow state, then redraw in
			// place". Keeping that one shape here means a new move cannot
			// forget the redraw.
			if err := h.moveDirectoryBrowser(chatID, subAction, payload.Arg(2)); err != nil {
				logrus.WithError(err).WithField("move", subAction).Warn("Directory browser move failed")
				return
			}
			_, _ = feature.SendDirectoryBrowser(h.ctx, bot, h.directoryBrowser, chatID, browserMessageID(msg))

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
			// Take the confirmation down, as the select path does, so the
			// button cannot be tapped a second time against cleared state.
			editDirectoryBrowserMessage(h.ctx, bot, chatID, browserMessageID(msg),
				fmt.Sprintf("✅ Created and bound: `%s`", path))

		case "cancel":
			h.directoryBrowser.Clear(chatID)
			editDirectoryBrowserMessage(h.ctx, bot, chatID, browserMessageID(msg), "❌ Bind cancelled.")
			h.SendText(hCtx, "Bind cancelled.")
		}
	}
}

// browserMessageID returns the message the browser keyboard is attached to,
// falling back to the callback's own message when the platform did not supply
// one separately.
func browserMessageID(msg imbot.Message) string {
	if id, _ := msg.Metadata["message_id"].(string); id != "" {
		return id
	}
	return msg.ID
}

// moveDirectoryBrowser applies one navigation step to the bind flow. target is
// only read by the "dir" move.
func (h *BotHandler) moveDirectoryBrowser(chatID, move, target string) error {
	switch move {
	case "dir":
		if target == "" {
			return fmt.Errorf("no directory given")
		}
		return h.directoryBrowser.Navigate(chatID, target)
	case "up":
		return h.directoryBrowser.NavigateUp(chatID)
	case "prev":
		return h.directoryBrowser.PrevPage(chatID)
	case "next":
		return h.directoryBrowser.NextPage(chatID)
	default:
		return fmt.Errorf("unknown move %q", move)
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
	bot.HandlePromptCallback(h.imPrompter, func(text string) { h.SendText(hCtx, text) }, hCtx.SenderID, payload, true)
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
	})
	if err != nil {
		logrus.WithError(err).Error("Failed to send create confirmation")
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
