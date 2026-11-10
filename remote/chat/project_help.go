package chat

import "strings"

// helpText is /help: the commands this chat can run
// (.design/remote-into-projects.md, "命令精简"), and the ids the notify API
// asks for. privileged is whether the chat has the "Any folder and files"
// permission: without it /cd is not offered and /dir only picks from the
// registered folders; in a group, where every folder and project command
// needs it, none of the three is. The lines come from commandSpecs, the same
// list the platform menu is built from.
func helpText(senderID, chatID string, privileged, group bool) string {
	lines := []string{"Send a message to talk to Claude Code.", "/h: Shortcut for /help"}
	for _, c := range commandSpecs() {
		if usage, ok := c.usageFor(privileged, group); ok {
			lines = append(lines, usage+": "+c.Help)
		}
	}
	lines = append(lines,
		"@N <message>: say one thing to session #N",
		"",
		"Your ID: "+senderID,
	)
	if chatID != "" {
		lines = append(lines, "Chat ID: "+chatID)
	}
	return strings.Join(lines, "\n")
}
