package projects

import (
	"errors"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/projects/internal/project"
	"github.com/tingly-dev/tingly-box/remote/agent"
)

// The IM chat port is Remote's (remote/agent); Projects implements it on App.
// The names stay here so a host talks to Projects in Projects' words.
type (
	// ChatRef identifies the IM chat an inbound call comes from, and the bot it
	// came through.
	ChatRef = agent.ChatRef
	// ChatPlace says where a chat is in Projects. The zero value is a chat with
	// no place yet.
	ChatPlace = agent.ChatPlace
	// ProjectChoice is a non-archived project offered to an IM chat.
	ProjectChoice = agent.ProjectChoice
	// Chat is how an IM host hands a chat to Projects: its messages, commands and
	// answers go in, and it is told where the chat is. The outbound half
	// (replies, reports, approvals) is the agent.Outbox the host offers (OutboxHost).
	Chat = agent.Chat
)

var _ Chat = (*App)(nil)

// MayAnswerText reports whether a message could answer a current request.
func (a *App) MayAnswerText(ref ChatRef, text string) bool {
	b, err := a.store.GetChannelBinding(ref.Platform, ref.ChatID)
	if err != nil || (b.BotUUID != "" && b.BotUUID != ref.BotUUID) {
		return false
	}
	return a.svc.MayAnswerChannelText(ref.Platform, ref.ChatID, text)
}

// Post hands a message typed in an IM chat to the thread the chat is bound to
// (a project's main thread is its coordinator), as Send does; replies come
// back through the host's outbox. handled is false when the chat is not
// bound. files, which the host saved, go with the message as its attachments;
// the host may remove them once this returns.
func (a *App) Post(ref ChatRef, text string, files []ChannelFile) (bool, error) {
	checkAnswer := ref.AuthorizeAnswer
	if checkAnswer != nil {
		checkAnswer = func() bool {
			b, err := a.store.GetChannelBinding(ref.Platform, ref.ChatID)
			if err != nil || (b.BotUUID != "" && b.BotUUID != ref.BotUUID) {
				return false
			}
			return ref.AuthorizeAnswer()
		}
	}
	return a.svc.PostFromChannel(ref.Platform, ref.ChatID, text, files, checkAnswer, ref.AuthorizeStart)
}

// Command runs a chat command for a bound IM chat; handled is false when the
// chat has no binding or the command is not one of the session commands.
func (a *App) Command(ref ChatRef, name, arg string) (bool, string, error) {
	return a.svc.ChannelCommand(ref.Platform, ref.ChatID, name, arg)
}

// Answer answers a thread's approval or question with the button an IM chat
// pressed (see agent.Prompt). reply, when set, is what to tell the chat;
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
