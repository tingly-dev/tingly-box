package remoteagent

import "strings"

// routeProjectHelp answers /help in a chat bound to Projects with what the
// chat can do there, instead of the @cc / @tb commands it no longer reaches.
// It reports whether the message was taken; other chats keep the old help.
func (h *BotHandler) routeProjectHelp(hCtx HandlerContext, text string) bool {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "/help", "/start":
	default:
		return false
	}
	if !h.chatBoundToProject(hCtx) {
		return false
	}
	h.SendText(hCtx, projectsHelp(hCtx))
	return true
}

// projectsHelp is /help in a chat that talks to Projects: every command it
// has (.design/remote-into-projects.md, "命令精简").
func projectsHelp(hCtx HandlerContext) string {
	return strings.Join([]string{
		"Send a message to talk to Claude Code.",
		"/cd <path>: work in another folder",
		"/dir [N]: list the folders this chat used, or switch to one",
		"/session [N]: list the sessions here, or switch to #N",
		"/new [message]: start a new session",
		"/stop: stop the running turn",
		"/clear: stop, and start a new session with the next message",
		"/status: where this chat is and how its session runs",
		"/verbose on|off: also see each step, not only the reply",
		"/yolo: run every tool without asking, or back",
		"@N <message>: say one thing to session #N",
		"",
		"Your ID: " + hCtx.SenderID,
		"Chat ID: " + hCtx.ChatID,
	}, "\n")
}
