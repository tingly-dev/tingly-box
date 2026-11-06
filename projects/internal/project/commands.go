package project

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// Slash commands in a person's message (.design/slash-commands.md). The text
// a user types is parsed here; what a command does goes through the agent
// session API, never as the command's own text to the agent.

// ErrCommandUnsupported is a slash command the chat knows by name but
// Projects does not run.
type ErrCommandUnsupported struct{ Name string }

func (e *ErrCommandUnsupported) Error() string {
	return fmt.Sprintf("/%s isn't available here", e.Name)
}

// ErrNothingToCompact is /compact on a thread that has not run yet.
var ErrNothingToCompact = errors.New("nothing to compact yet: this conversation has not started")

// commandKind says what a message beginning with "/" is.
type commandKind int

const (
	// notCommand is ordinary text, however it starts.
	notCommand commandKind = iota
	cmdCompact
	// cmdUnsupported is the name of a command people know from Claude Code
	// that Projects does not run: it is refused, not sent on as text.
	cmdUnsupported
)

type command struct {
	kind commandKind
	name string
	args string
}

var commandRe = regexp.MustCompile(`^/([a-z][a-z0-9-]*)(?:\s+([\s\S]*))?$`)

// unsupportedCommands are the commands people know from Claude Code (and the
// ones Projects has its own control for) that are refused with a one-line
// reason. Any other "/word" is ordinary text, so a path or a command-like
// word in a sentence still reaches the agent.
var unsupportedCommands = map[string]bool{
	"clear": true, "reset": true, "new": true, "resume": true, "rewind": true,
	"model": true, "effort": true, "fast": true, "config": true, "settings": true,
	"mcp": true, "memory": true, "permissions": true, "add-dir": true, "agents": true,
	"init": true, "login": true, "logout": true, "doctor": true, "color": true,
	"context": true, "usage": true, "cost": true, "stats": true, "autocompact": true,
	"loop": true, "goal": true, "batch": true, "rename": true, "help": true,
	"status": true, "btw": true, "recap": true, "insights": true, "security-review": true,
}

// parseCommand reads text as a slash command. Only the whole message counts:
// "/compact keep the API notes" is one, "please /compact" is not.
func parseCommand(text string) command {
	m := commandRe.FindStringSubmatch(strings.TrimSpace(text))
	if m == nil {
		return command{}
	}
	switch name := m[1]; {
	case name == "compact":
		return command{kind: cmdCompact, name: name, args: strings.TrimSpace(m[2])}
	case unsupportedCommands[name]:
		return command{kind: cmdUnsupported, name: name}
	}
	return command{}
}

// byPerson reports whether a message was typed by a person on the web or in
// a chat. Only such text is a command: a thread's report, the coordinator's
// relay and system notes are never parsed.
func byPerson(src Source) bool {
	return src.Kind == "" || src.Kind == db.SourceIM
}

// literalPrompt is the text that goes to an agent for a message that is not a
// command: a leading slash would make the agent run the first word as its own
// command (or swallow it as unknown), so it gets a leading space, which makes
// it ordinary text.
func literalPrompt(text string) string {
	if strings.HasPrefix(text, "/") {
		return " " + text
	}
	return text
}

// compactText is the message a person sends to compact: the one place the
// command is spelled, for the web, a chat and the coordinator alike.
func compactText(instructions string) string {
	if instructions = strings.TrimSpace(instructions); instructions != "" {
		return "/compact " + instructions
	}
	return "/compact"
}
