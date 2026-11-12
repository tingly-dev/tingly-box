// Package agent is the port between Remote and whatever answers a chat: Remote
// brings IM messages, commands and button presses in; the agent says where the
// chat is and what to do with them. Projects implements it; a test fakes it.
//
// Types are plain data keyed by platform and chat id, with no imbot types, so
// an implementation lives in another module.
package agent

import "slices"

// ChatRef identifies the IM chat an inbound call comes from, and the bot it
// came through.
type ChatRef struct {
	Platform string
	ChatID   string
	// BotUUID is the bot the chat wrote through; replies go back through it.
	BotUUID string
	// Profile is the launch profile a chat's first session runs under (the
	// bot's default agent); empty for the default.
	Profile string
	// AuthorizeAnswer checks the current sender's approval permission at the
	// point a text reply is consumed. Nil means the host has no access policy.
	AuthorizeAnswer func() bool
}

// ChatPlace says where a chat is. The zero value is a chat with no place yet.
type ChatPlace struct {
	// Bound is true when the chat has a session, or a folder its next message
	// starts one in.
	Bound bool
	// Folder is the folder the chat is on or has picked for its next session;
	// empty when it is on a project.
	Folder string
	// ProjectID is the project whose coordinator the chat talks to; empty
	// when it is on a folder.
	ProjectID string
}

// File is a file a chat sent, saved on local disk at Path under its own Name.
type File struct {
	Name string
	Path string
}

// ProjectChoice is a project a chat can switch to.
type ProjectChoice struct {
	ID   string
	Name string
}

// Chat is how Remote hands a chat to an agent.
type Chat interface {
	// Post hands a message to the chat's session. handled is false when the
	// chat has no place with the agent.
	Post(ref ChatRef, text string, files []File) (handled bool, err error)
	// Command runs a session command (see Commands). handled is false when
	// the chat has no place or the command is not one of those.
	Command(ref ChatRef, name, arg string) (handled bool, reply string, err error)
	// Answer answers a session's approval or question with the value a chat
	// chose. reply, when set, is what to tell the chat.
	Answer(ref ChatRef, threadID, requestID, value string) (reply string, err error)
	// Place says where the chat is.
	Place(ref ChatRef) (ChatPlace, error)
	// ToFolder moves the chat to a folder (an absolute path that exists): its
	// next message starts a new session there. With ifNew a chat that already
	// has a place is left alone.
	ToFolder(ref ChatRef, folder string, ifNew bool) error
	// ToProject moves the chat to a project's coordinator.
	ToProject(ref ChatRef, projectID string) error
	// Folders lists the existing working folders.
	Folders() ([]string, error)
	// Projects lists the projects, excluding working folders.
	Projects() ([]ProjectChoice, error)
}

// CommandSpec describes a chat command the agent runs: the platform command
// menu and /help are built from these, so a command is described once.
type CommandSpec struct {
	// Name is the command word, without the slash.
	Name string
	// Usage is how it is typed, e.g. "/verbose on|off".
	Usage string
	// Help is the one-sentence description, capitalised, no final period.
	Help string
	// Order places it among all commands; higher is listed first.
	Order int
}

// The session commands an agent runs for a chat.
const (
	CommandSession = "session"
	CommandNew     = "new"
	CommandStop    = "stop"
	CommandClear   = "clear"
	CommandStatus  = "status"
	CommandVerbose = "verbose"
	CommandYolo    = "yolo"
	// CommandCompact compacts the session's conversation; the result comes
	// back as the session's next reply.
	CommandCompact = "compact"
)

var commands = []CommandSpec{
	{CommandSession, "/session [N]", "List the sessions here, or switch to #N", 80},
	{CommandNew, "/new [message]", "Start a new session", 79},
	{CommandStop, "/stop [#N]", "Stop the running session", 78},
	{CommandClear, "/clear", "Stop, and start a new session with the next message", 77},
	{CommandStatus, "/status", "Where this chat is and how its session runs", 76},
	{CommandVerbose, "/verbose on|off", "Also see each step, not only the reply", 75},
	{CommandYolo, "/yolo", "Use every tool without asking, or back", 74},
	{CommandCompact, "/compact [instructions]", "Shrink the conversation to free up context", 73},
}

// Commands are the session commands, in listing order.
func Commands() []CommandSpec { return slices.Clone(commands) }

// IsCommand reports whether name is one of Commands.
func IsCommand(name string) bool {
	for _, c := range commands {
		if c.Name == name {
			return true
		}
	}
	return false
}
