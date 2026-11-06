package remoteagent

import "strings"

// helpText is /help: the commands this chat can run
// (.design/remote-into-projects.md, "命令精简"), and the ids the notify API
// asks for. privileged is whether the chat has the "Any folder and files"
// permission: without it /cd is not offered and /dir only picks from the
// registered folders; in a group, where every folder and project command
// needs it, none of the three is.
func helpText(senderID, chatID string, privileged, group bool) string {
	lines := []string{"Send a message to talk to Claude Code."}
	switch {
	case privileged:
		lines = append(lines,
			"/cd <path>: work in another folder",
			"/dir [N|path]: list registered folders, or switch to one",
			"/project [N]: list projects, or switch to a project's coordinator",
		)
	case !group:
		lines = append(lines,
			"/dir [N]: list registered folders, or switch to one",
			"/project [N]: list projects, or switch to a project's coordinator",
		)
	}
	lines = append(lines,
		"/session [N]: list the sessions here, or switch to #N",
		"/new [message]: start a new session",
		"/stop: stop the running session",
		"/clear: stop, and start a new session with the next message",
		"/compact [instructions]: shrink the conversation to free up context",
		"/status: where this chat is and how its session runs",
		"/verbose on|off: also see each step, not only the reply",
		"/yolo: use every tool without asking, or back",
		"@N <message>: say one thing to session #N",
		"",
		"Your ID: "+senderID,
	)
	if chatID != "" {
		lines = append(lines, "Chat ID: "+chatID)
	}
	return strings.Join(lines, "\n")
}
