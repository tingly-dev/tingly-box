package projects

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

func (a *App) Post(ref ChatRef, text string, files []ChannelFile) (bool, error) {
	return a.PostFromChannel(ref.Platform, ref.ChatID, text, files)
}

func (a *App) Command(ref ChatRef, name, arg string) (bool, string, error) {
	return a.ChannelCommand(ref.Platform, ref.ChatID, name, arg)
}

func (a *App) Answer(ref ChatRef, threadID, requestID, value string) (string, error) {
	return a.AnswerFromChannel(ref.Platform, ref.ChatID, threadID, requestID, value)
}

func (a *App) Place(ref ChatRef) (ChatPlace, error) {
	if !a.ChannelBound(ref.Platform, ref.ChatID) {
		return ChatPlace{}, nil
	}
	folder, err := a.ChannelFolder(ref.Platform, ref.ChatID)
	if err != nil {
		return ChatPlace{}, err
	}
	projectID, err := a.ChannelProject(ref.Platform, ref.ChatID)
	if err != nil {
		return ChatPlace{}, err
	}
	return ChatPlace{Bound: true, Folder: folder, ProjectID: projectID}, nil
}

func (a *App) ToFolder(ref ChatRef, folder string, ifNew bool) error {
	return a.ChannelToFolder(ref.Platform, ref.ChatID, "", folder, ref.Profile, ifNew, ref.BotUUID)
}

func (a *App) ToProject(ref ChatRef, projectID string) error {
	return a.ChannelToProject(ref.Platform, ref.ChatID, ref.BotUUID, projectID)
}

func (a *App) Folders() ([]string, error) { return a.RegisteredFolders() }

func (a *App) Projects() ([]ProjectChoice, error) { return a.RegisteredProjects() }
