package projects

import (
	"errors"
	"slices"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/internal/project"
)

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
}

// ChatPlace says where a chat is in Projects. The zero value is a chat with
// no place yet.
type ChatPlace struct {
	// Bound is true when the chat has a thread, or a folder its next message
	// starts one in.
	Bound bool
	// Folder is the folder the chat is on or has picked for its next
	// session; empty when it is on a project.
	Folder string
	// ProjectID is the project whose coordinator the chat talks to; empty
	// when it is on a folder.
	ProjectID string
}

// Chat is how an IM host hands a chat to Projects: its messages, commands and
// answers go in, and it is told where the chat is. The outbound half (replies,
// reports, approvals) is the optional ChannelSink family.
type Chat interface {
	// Post hands a message to the chat's session. handled is false when the
	// chat has no place in Projects.
	Post(ref ChatRef, text string, files []ChannelFile) (handled bool, err error)
	// Command runs a session command ("new", "stop", "clear", "session",
	// "status", "verbose", "yolo"). handled is false when the chat has no
	// place or the command is not one of these.
	Command(ref ChatRef, name, arg string) (handled bool, reply string, err error)
	// Answer answers a thread's approval or question with the value a chat
	// chose.
	Answer(ref ChatRef, threadID, requestID, value string) (reply string, err error)
	// Place says where the chat is.
	Place(ref ChatRef) (ChatPlace, error)
	// ToFolder moves the chat to a folder (an absolute path that exists): its
	// next message starts a new session there. With ifNew a chat that already
	// has a place is left alone.
	ToFolder(ref ChatRef, folder string, ifNew bool) error
	// ToProject moves the chat to a project's coordinator.
	ToProject(ref ChatRef, projectID string) error
	// Folders lists the existing, non-archived working folders.
	Folders() ([]string, error)
	// Projects lists the project coordinators, excluding working folders.
	Projects() ([]ProjectChoice, error)
}

var _ Chat = (*App)(nil)

// CommandSpec describes a chat command for the host that lists it; see
// ChatCommands.
type CommandSpec = project.ChatCommand

// ChatCommands are the session commands Command runs, in listing order. The
// host builds its command menu and /help from them and adds its own.
func ChatCommands() []CommandSpec { return slices.Clone(project.ChatCommands) }

// ProjectChoice is a non-archived project offered to an IM chat.
type ProjectChoice struct {
	ID   string
	Name string
}

// Post hands a message typed in an IM chat to the thread the chat is bound to
// (a project's main thread is its coordinator), as Send does; replies come
// back through the host's ChannelSink. handled is false when the chat is not
// bound. files, which the host saved, go with the message as its attachments;
// the host may remove them once this returns.
func (a *App) Post(ref ChatRef, text string, files []ChannelFile) (bool, error) {
	return a.svc.PostFromChannel(ref.Platform, ref.ChatID, text, files)
}

// Command runs a chat command for a bound IM chat; handled is false when the
// chat has no binding or the command is not one of the session commands.
func (a *App) Command(ref ChatRef, name, arg string) (bool, string, error) {
	return a.svc.ChannelCommand(ref.Platform, ref.ChatID, name, arg)
}

// Answer answers a thread's approval or question with the button an IM chat
// pressed (see ChannelPrompter). reply, when set, is what to tell the chat;
// otherwise the prompt is restated with the outcome.
func (a *App) Answer(ref ChatRef, threadID, requestID, value string) (string, error) {
	return a.svc.AnswerFromChannel(ref.Platform, ref.ChatID, threadID, requestID, value)
}

// Place says where a chat is: on a project, or on a folder (the one it is on,
// or has picked for its next session).
func (a *App) Place(ref ChatRef) (ChatPlace, error) {
	b, err := a.store.GetChannelBinding(ref.Platform, ref.ChatID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return ChatPlace{}, nil
		}
		return ChatPlace{}, err
	}
	place := ChatPlace{Bound: true}
	if b.Folder != "" {
		place.Folder = b.Folder
		return place, nil
	}
	if b.ThreadID == "" {
		return place, nil
	}
	t, err := a.store.GetThread(b.ThreadID)
	if err != nil {
		return ChatPlace{}, err
	}
	p, err := a.store.GetProject(t.ProjectID)
	if err != nil {
		return ChatPlace{}, err
	}
	switch p.Type {
	case db.ProjectFolder:
		place.Folder = p.Path
	case db.ProjectProject:
		place.ProjectID = p.ID
	}
	return place, nil
}

// ToFolder moves a chat to a folder on this machine; the chat's first folder
// is how a chat gets a place at all (ifNew).
func (a *App) ToFolder(ref ChatRef, folder string, ifNew bool) error {
	return a.svc.ChannelToFolder(ref.Platform, ref.ChatID, "", folder, ref.Profile, ifNew, ref.BotUUID)
}

// ToProject moves a chat to a project's coordinator.
func (a *App) ToProject(ref ChatRef, projectID string) error {
	_, err := a.svc.BindChannel(ref.Platform, ref.ChatID, "", "", projectID, ref.BotUUID)
	return err
}

// Folders lists existing, non-archived working folders. Projects'
// repositories are not independent folder registrations.
func (a *App) Folders() ([]string, error) {
	projects, err := a.store.ListProjects("", false)
	if err != nil {
		return nil, err
	}
	folders := make([]string, 0)
	for _, p := range projects {
		if p.Type != db.ProjectFolder {
			continue
		}
		path, err := project.CleanFolder(p.Path)
		if err == nil {
			folders = append(folders, path)
		}
	}
	return folders, nil
}

// Projects lists project coordinators, excluding working folders.
func (a *App) Projects() ([]ProjectChoice, error) {
	projects, err := a.store.ListProjects("", false)
	if err != nil {
		return nil, err
	}
	out := make([]ProjectChoice, 0)
	for _, p := range projects {
		if p.Type == db.ProjectProject {
			out = append(out, ProjectChoice{ID: p.ID, Name: p.Name})
		}
	}
	return out, nil
}
