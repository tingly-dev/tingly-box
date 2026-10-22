package remoteagent

import "strings"

// helpText is /help: every command a chat has
// (.design/remote-into-projects.md, "命令精简"), and the ids the notify API
// asks for.
func helpText(senderID, chatID string) string {
	lines := []string{
		"Send a message to talk to Claude Code.",
		"/cd <path>: work in another folder",
		"/dir [N]: list the folders this chat used, or switch to one",
		"/session [N]: list the sessions here, or switch to #N",
		"/new [message]: start a new session",
		"/stop: stop the running session",
		"/clear: stop, and start a new session with the next message",
		"/status: where this chat is and how its session runs",
		"/verbose on|off: also see each step, not only the reply",
		"/yolo: use every tool without asking, or back",
		"@N <message>: say one thing to session #N",
		"",
		"Your ID: " + senderID,
	}
	if chatID != "" {
		lines = append(lines, "Chat ID: "+chatID)
	}
	return strings.Join(lines, "\n")
}
