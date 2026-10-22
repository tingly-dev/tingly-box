package remoteagent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/imbot"
)

// cmdJoinPrimary is the join command name (used in HandleMessage for group whitelist message)
const cmdJoinPrimary = "/join"

// handleSlashCommand runs a slash command from the registry; it reports
// false for one the registry does not have.
func (h *BotHandler) handleSlashCommand(hCtx HandlerContext, text string) bool {
	fields := strings.Fields(text)
	if len(fields) == 0 || h.commandRegistry == nil {
		return false
	}
	name := strings.ToLower(strings.TrimPrefix(fields[0], "/"))
	if name == "" {
		name = "help"
	}
	handler, ok := h.commandRegistry.Match(name)
	if !ok {
		return false
	}
	cmdCtx := imbot.NewHandlerContext(hCtx.Bot, hCtx.ChatID, hCtx.SenderID, hCtx.Platform).
		WithText(text).
		WithDirectMessage(hCtx.IsDirect()).
		WithMessageID(hCtx.MessageID)
	if err := handler(cmdCtx, fields[1:]); err != nil {
		logrus.WithError(err).WithField("command", name).Error("Command handler failed")
	}
	return true
}

// buildDirText builds the plain-text body for /dir replies. The same
// string is used by every platform so the information shown is always identical.
func buildDirText(currentPath string, paths []string) string {
	var buf strings.Builder
	if currentPath != "" {
		buf.WriteString(fmt.Sprintf("Current folder:\n📁 %s\n\n", currentPath))
	} else {
		buf.WriteString("This chat has not picked a folder.\n\n")
	}
	if len(paths) > 0 {
		buf.WriteString("Folders this chat used:\n")
		for i, path := range paths {
			marker := ""
			if path == currentPath {
				marker = " ✓"
			}
			buf.WriteString(fmt.Sprintf("  %d. %s%s\n", i+1, path, marker))
		}
		buf.WriteString("\n/dir <number> or /cd <path> switches.")
	} else {
		buf.WriteString("/cd <path> picks one.")
	}
	return buf.String()
}

// buildDirKeyboard builds the buttons of an interactive /dir reply. Button
// labels include the index so they map 1-to-1 to the numbered text list.
func buildDirKeyboard(currentPath string, paths []string) imbot.InlineKeyboardMarkup {
	var rows [][]imbot.InlineKeyboardButton
	for i, path := range paths {
		marker := ""
		if path == currentPath {
			marker = " ✓"
		}
		rows = append(rows, []imbot.InlineKeyboardButton{{
			Text:    fmt.Sprintf("%d. 📁 %s%s", i+1, filepath.Base(path), marker),
			Payload: imbot.NewPayload("project", "switch", path),
		}})
	}
	rows = append(rows, []imbot.InlineKeyboardButton{{
		Text:    "📁 Another folder",
		Payload: imbot.NewPayload("action", "bind"),
	}})
	return imbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// handleDirMenu answers the "project" button of an action menu with the /dir
// list. It always runs from a button, so it always has buttons.
func (h *BotHandler) handleDirMenu(hCtx HandlerContext) {
	currentPath, _, _ := h.chatStore.GetProjectPath(hCtx.ChatID)
	paths, _ := h.chatStore.ListChatProjectPaths(hCtx.ChatID)
	_, err := hCtx.Bot.SendMessage(context.Background(), hCtx.ChatID, &imbot.SendMessageOptions{
		Text:    buildDirText(currentPath, paths),
		Actions: buildDirKeyboard(currentPath, paths).ToActionSet(),
	})
	if err != nil {
		logrus.WithError(err).Error("Failed to send folder list")
	}
}
