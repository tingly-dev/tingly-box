package project

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// The commands a chat has (.design/remote-into-projects.md, "命令精简"): what
// is not here is not a command. /cd and /dir (which folder) are the host's,
// which moves the chat with ChannelToFolder.
const (
	// ChatNew starts a new session in the chat's folder: now with a task,
	// else with the next message. What is running keeps running.
	ChatNew = "new"
	// ChatStop interrupts the turn of the chat's thread, or of "#N".
	ChatStop = "stop"
	// ChatClear stops the chat's session, and its next message starts a new
	// one.
	ChatClear = "clear"
	// ChatSession lists the sessions of the chat's folder or project, or
	// with "N" moves the chat to #N (#0 is a project's conversation).
	ChatSession = "session"
	// ChatStatus says where the chat is and how its session runs.
	ChatStatus = "status"
	// ChatVerbose turns on ("on") or off ("off") hearing each step of the
	// thread's turns; with no arg it says which.
	ChatVerbose = "verbose"
	// ChatYolo switches the chat's session between running every tool
	// without asking and its usual permission mode.
	ChatYolo = "yolo"
)

// yoloMode is the permission mode /yolo turns on.
const yoloMode = "bypassPermissions"

// ChannelCommand runs a chat command (ChatNew and the rest) for a chat with a
// binding; arg is the text after it. handled is false when the chat has no
// binding or the command is not one of these, and the caller carries on as
// before. reply is what to tell the chat.
func (s *Service) ChannelCommand(platform, chatID, name, arg string) (handled bool, reply string, err error) {
	switch name {
	case ChatNew, ChatStop, ChatClear, ChatSession, ChatStatus, ChatVerbose, ChatYolo:
	default:
		return false, "", nil
	}
	b, err := s.Store.GetChannelBinding(platform, chatID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return false, "", nil
		}
		return false, "", err
	}
	arg = strings.TrimSpace(arg)
	switch name {
	case ChatVerbose:
		reply, err := s.verbose(b, arg)
		return true, reply, err
	case ChatStatus:
		reply, err := s.status(b)
		return true, reply, err
	}
	if b.Folder != "" {
		if handled, reply, err := s.folderCommand(b, name, arg); handled {
			return true, reply, err
		}
	}
	t, p, err := s.boundThread(b)
	if err != nil {
		return true, "", err
	}
	switch name {
	case ChatSession:
		if arg == "" {
			reply, err := s.threadList(t, p)
			return true, reply, err
		}
		reply, err := s.focus(b, p, arg)
		return true, reply, err
	case ChatStop:
		if arg != "" {
			target, err := s.threadByRef(p.ID, arg)
			if err != nil {
				return true, "", err
			}
			t = target
		}
	}
	tag := threadTag(t.Seq, t.Title)
	if t.Agent == db.AgentLLM {
		switch name {
		case ChatStop:
			return true, "The project conversation cannot be stopped. Stop a thread in Projects on the web.", nil
		default:
			return true, "This chat talks to the project conversation; ask it to start a thread, or /session N to talk to one.", nil
		}
	}
	switch name {
	case ChatNew:
		if p.Type != db.ProjectFolder {
			return true, fmt.Sprintf("/new works in a chat bound to a folder; this chat is on [%s] of %s.", tag, p.Name), nil
		}
		if arg != "" {
			nt, err := s.startFromChannel(b, t, t.ProjectID, arg, nil, Source{Kind: db.SourceIM, Platform: platform, ChatID: chatID})
			if err != nil {
				return true, "", err
			}
			return true, fmt.Sprintf("Started [%s] in %s.", threadTag(nt.Seq, nt.Title), p.Name), nil
		}
		if err := s.Store.SetChannelBindingNewThread(b.ID, true); err != nil {
			return true, "", err
		}
		return true, fmt.Sprintf("Your next message starts a new session in %s.", p.Name), nil
	case ChatClear:
		if p.Type != db.ProjectFolder {
			return true, fmt.Sprintf("/clear works in a chat bound to a folder; this chat is on [%s] of %s. Use /stop to stop it.", tag, p.Name), nil
		}
		stopped := s.Threads.Running(t.ID)
		s.Threads.Stop(t.ID)
		if err := s.Store.SetChannelBindingNewThread(b.ID, true); err != nil {
			return true, "", err
		}
		if stopped {
			return true, fmt.Sprintf("Stopped [%s]. Your next message starts a new session in %s.", tag, p.Name), nil
		}
		return true, fmt.Sprintf("Your next message starts a new session in %s.", p.Name), nil
	case ChatYolo:
		reply, err := s.yolo(t, p)
		return true, reply, err
	default:
		if !s.Threads.Running(t.ID) {
			return true, fmt.Sprintf("[%s] is not running.", tag), nil
		}
		s.Threads.Stop(t.ID)
		return true, fmt.Sprintf("Stopped [%s].", tag), nil
	}
}

// folderCommand runs the commands that mean something else while a chat
// waits to start in its Folder: /new starts there, /stop and /clear stop the
// thread it was on (if any). handled is false for a command that acts on that thread
// as usual.
func (s *Service) folderCommand(b *db.ChannelBinding, name, arg string) (handled bool, reply string, err error) {
	where := db.FolderName(b.Folder)
	next := fmt.Sprintf("Your next message starts a new session in %s.", where)
	switch name {
	case ChatNew:
		if arg == "" {
			return true, next, nil
		}
		t, err := s.startInChannelFolder(b, arg, nil, Source{Kind: db.SourceIM, Platform: b.Platform, ChatID: b.ChatID})
		if err != nil {
			return true, "", err
		}
		return true, fmt.Sprintf("Started [%s] in %s.", threadTag(t.Seq, t.Title), where), nil
	case ChatStop, ChatClear:
		if b.ThreadID != "" && arg == "" && s.Threads.Running(b.ThreadID) {
			s.Threads.Stop(b.ThreadID)
			if t, err := s.Store.GetThread(b.ThreadID); err == nil {
				return true, fmt.Sprintf("Stopped [%s]. %s", threadTag(t.Seq, t.Title), next), nil
			}
		}
		if name == ChatClear {
			return true, next, nil
		}
	}
	if b.ThreadID == "" {
		return true, "Nothing has started in this chat yet. " + next, nil
	}
	return false, "", nil
}

// status says where a chat is: its folder or project, its session and how
// that runs, and whether the chat hears each step.
func (s *Service) status(b *db.ChannelBinding) (string, error) {
	var lines []string
	if b.ThreadID != "" {
		t, p, err := s.boundThread(b)
		if err != nil {
			return "", err
		}
		if p.Type == db.ProjectFolder {
			lines = append(lines, "Folder: "+p.Path)
		} else {
			lines = append(lines, "Project: "+p.Name)
		}
		if t.Agent == db.AgentLLM {
			lines = append(lines, "Session: the project conversation")
		} else {
			lines = append(lines,
				fmt.Sprintf("Session: [%s], %s", threadTag(t.Seq, t.Title), threadStatusWord(t.Status)),
				"Model: "+orDefault(t.Model),
				"Permissions: "+threadPermissionMode(p, t))
		}
	}
	if b.Folder != "" {
		lines = append(lines, "Next message: a new session in "+b.Folder)
	}
	verbose := "off"
	if b.Verbose {
		verbose = "on"
	}
	lines = append(lines, "Verbose: "+verbose)
	return strings.Join(lines, "\n"), nil
}

// yolo switches a thread between bypassPermissions and the mode it inherits.
func (s *Service) yolo(t *db.Thread, p *db.Project) (string, error) {
	tag := threadTag(t.Seq, t.Title)
	if threadPermissionMode(p, t) == yoloMode {
		if _, err := s.Threads.SetPermissionMode(t.ID, ""); err != nil {
			return "", err
		}
		return fmt.Sprintf("Yolo is off: [%s] asks before using tools again (%s).", tag, permissionMode(p)), nil
	}
	if _, err := s.Threads.SetPermissionMode(t.ID, yoloMode); err != nil {
		return "", err
	}
	return fmt.Sprintf("Yolo is on: [%s] uses every tool without asking. /yolo again to turn it off.", tag), nil
}

// verbose sets or tells whether the chat hears each step of its thread.
func (s *Service) verbose(b *db.ChannelBinding, arg string) (string, error) {
	on := b.Verbose
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "":
	case "on":
		on = true
	case "off":
		on = false
	default:
		return "Usage: /verbose on|off", nil
	}
	if on != b.Verbose {
		if err := s.Store.SetChannelBindingVerbose(b.ID, on); err != nil {
			return "", err
		}
	}
	if on {
		return "Verbose is on: this chat sees each step a session takes, not only its reply. /verbose off to stop.", nil
	}
	return "Verbose is off: this chat sees each reply. /verbose on to also see each step.", nil
}
