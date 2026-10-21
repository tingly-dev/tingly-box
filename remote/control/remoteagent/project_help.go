package remoteagent

import "strings"

// routeProjectHelp answers /help in a chat bound to Projects with what the
// chat can do there, instead of the @cc / @tb commands it no longer reaches.
// It reports whether the message was taken; other chats keep the old help.
func (h *BotHandler) routeProjectHelp(hCtx HandlerContext, text string) bool {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "/help", "/h", "/start":
	default:
		return false
	}
	if !h.chatBoundToProject(hCtx) {
		return false
	}
	h.SendText(hCtx, projectsHelp(hCtx))
	return true
}

// projectsHelp is /help in a chat bound to Projects.
func projectsHelp(hCtx HandlerContext) string {
	return strings.Join([]string{
		"This chat talks to Projects. Send a message to talk to it.",
		"@N <message>: talk to thread #N once",
		"/threads: list the project's threads",
		"/focus N: stay with thread #N; /back: return to the one above",
		"/new [message]: start a new session (in a folder)",
		"/stop [#N]: stop the running turn",
		"/clear: stop, and start a new session with the next message (in a folder)",
		"Bind or unbind this chat in Projects on the web.",
		"",
		"Your ID: " + hCtx.SenderID,
		"Chat ID: " + hCtx.ChatID,
	}, "\n")
}
