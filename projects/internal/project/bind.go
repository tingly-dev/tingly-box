package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// Errors for channel bindings.
var (
	ErrBindingInvalid = errors.New("platform, chat_id and one of thread_id or project_id are required")
	// ErrBindFolder is for binding a folder by project id: a folder has no
	// main thread, so the chat is bound to one of its threads.
	ErrBindFolder = errors.New("a folder has no main thread; bind one of its threads (thread_id)")
	// ErrNotThisChat is for a chat answering a thread it is not bound to,
	// directly or through an ancestor.
	ErrNotThisChat = errors.New("this chat is not bound to that thread")
)

// BindChannel binds an IM chat to a thread; a chat that is already bound
// moves. A project id stands for the project's main thread. title is an
// optional display name for the chat.
func (s *Service) BindChannel(platform, chatID, title, threadID, projectID string) (*db.ChannelBinding, error) {
	platform, chatID = strings.TrimSpace(platform), strings.TrimSpace(chatID)
	threadID, projectID = strings.TrimSpace(threadID), strings.TrimSpace(projectID)
	if platform == "" || chatID == "" || (threadID == "") == (projectID == "") {
		return nil, ErrBindingInvalid
	}
	var t *db.Thread
	if threadID != "" {
		var err error
		if t, err = s.Store.GetThread(threadID); err != nil {
			return nil, err
		}
	} else {
		p, err := s.Store.GetProject(projectID)
		if err != nil {
			return nil, err
		}
		if p.Type == db.ProjectFolder {
			return nil, ErrBindFolder
		}
		if t, err = s.Store.GetThread(p.MainThreadID); err != nil {
			return nil, err
		}
	}
	p, err := s.Store.GetProject(t.ProjectID)
	if err != nil {
		return nil, err
	}
	if p.ArchivedAt != nil {
		return nil, ErrArchived
	}
	b := &db.ChannelBinding{Platform: platform, ChatID: chatID, ChatTitle: strings.TrimSpace(title), ThreadID: t.ID}
	if err := s.Store.UpsertChannelBinding(b); err != nil {
		return nil, err
	}
	return b, nil
}

// PostFromChannel delivers a message from an IM chat to the thread the chat is
// bound to, as a message with source im, exactly as the web would send it
// (Send). handled is false when the chat has no binding, and the caller
// carries on as before. The reply arrives in the chat through the Relay.
//
// A folder chat that asked for a new session (/new), or whose thread is done
// (resolved by hand or after idling), starts a new thread in the folder with
// this message instead, and the chat moves to it.
func (s *Service) PostFromChannel(platform, chatID, text string) (handled bool, err error) {
	b, err := s.Store.GetChannelBinding(platform, chatID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	src := Source{Kind: db.SourceIM, Platform: platform, ChatID: chatID}
	t, p, err := s.boundThread(b)
	if err != nil {
		return true, err
	}
	// "@2 ..." talks to thread #2 of the project for this message only.
	if n, rest, ok := parseMention(text); ok {
		target, err := s.threadBySeq(p.ID, n)
		if err != nil {
			return true, err
		}
		_, err = s.Send(context.Background(), target.ID, rest, nil, src)
		return true, err
	}
	// A chat that was asked something answers it with its next message
	// (other than an @N one).
	if threadID, requestID, ok := s.Relay.Awaiting(platform, chatID); ok {
		if done, err := s.answerFromText(threadID, requestID, text); done {
			return true, err
		}
	}
	if p.Type == db.ProjectFolder && (b.NewThread || t.Status == db.ThreadResolved) {
		_, err = s.startFromChannel(b, t, text, src)
		return true, err
	}
	_, err = s.Send(context.Background(), b.ThreadID, text, nil, src)
	return true, err
}

func (s *Service) boundThread(b *db.ChannelBinding) (*db.Thread, *db.Project, error) {
	t, err := s.Store.GetThread(b.ThreadID)
	if err != nil {
		return nil, nil, err
	}
	p, err := s.Store.GetProject(t.ProjectID)
	if err != nil {
		return nil, nil, err
	}
	return t, p, nil
}

// startFromChannel starts a thread in the folder of prev, with prev's model,
// profile and permission mode, and moves the chat to it.
func (s *Service) startFromChannel(b *db.ChannelBinding, prev *db.Thread, task string, src Source) (*db.Thread, error) {
	t, err := s.Threads.StartFolderThread(FolderThreadSpec{
		ProjectID: prev.ProjectID, Task: task, Source: src,
		Model: prev.Model, PermissionMode: prev.PermissionMode, Profile: prev.Profile,
	})
	if err != nil {
		return nil, err
	}
	nb := &db.ChannelBinding{Platform: b.Platform, ChatID: b.ChatID, ThreadID: t.ID}
	if err := s.Store.UpsertChannelBinding(nb); err != nil {
		return nil, err
	}
	return t, nil
}

// Chat commands a bound chat sends instead of a message.
const (
	ChatNew   = "new"   // a new session in the chat's folder: now with a task, else on the next message
	ChatStop  = "stop"  // interrupt the bound thread's turn
	ChatClear = "clear" // stop, and the next message starts a new session
	// ChatFocus binds the chat to thread #N of its project (arg "N" or "#N").
	ChatFocus = "focus"
	// ChatBack binds the chat to the parent of the thread it is on.
	ChatBack = "back"
	// ChatThreads lists the project's threads.
	ChatThreads = "threads"
	// ChatVerbose turns on ("on") or off ("off") hearing each step of the
	// thread's turns; with no arg it says which.
	ChatVerbose = "verbose"
)

// ChannelCommand runs a chat command (ChatNew, ChatStop, ChatClear, ChatFocus,
// ChatBack, ChatThreads, ChatVerbose) for a bound chat; arg is the text after it (for
// ChatStop, an optional "#N" naming another thread of the project). handled is false when the chat has no
// binding or the command is not one of these, and the caller carries on as
// before. reply is what to tell the chat.
func (s *Service) ChannelCommand(platform, chatID, name, arg string) (handled bool, reply string, err error) {
	switch name {
	case ChatNew, ChatStop, ChatClear, ChatFocus, ChatBack, ChatThreads, ChatVerbose:
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
	if name == ChatVerbose {
		reply, err := s.verbose(b, arg)
		return true, reply, err
	}
	t, p, err := s.boundThread(b)
	if err != nil {
		return true, "", err
	}
	switch name {
	case ChatFocus:
		reply, err := s.focus(b, p, arg)
		return true, reply, err
	case ChatBack:
		reply, err := s.back(b, t, p)
		return true, reply, err
	case ChatThreads:
		reply, err := s.threadList(t, p)
		return true, reply, err
	case ChatStop:
		if strings.TrimSpace(arg) != "" {
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
			return true, "This chat talks to the project conversation; ask it to start a thread.", nil
		}
	}
	folder := p.Type == db.ProjectFolder
	switch name {
	case ChatNew:
		if !folder {
			return true, fmt.Sprintf("/new works in a chat bound to a folder; this chat is on [%s] of %s.", tag, p.Name), nil
		}
		if task := strings.TrimSpace(arg); task != "" {
			nt, err := s.startFromChannel(b, t, task, Source{Kind: db.SourceIM, Platform: platform, ChatID: chatID})
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
		if !folder {
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
	default:
		if !s.Threads.Running(t.ID) {
			return true, fmt.Sprintf("[%s] is not running.", tag), nil
		}
		s.Threads.Stop(t.ID)
		return true, fmt.Sprintf("Stopped [%s].", tag), nil
	}
}

// answerFromText answers a request a chat was asked with the chat's reply
// (chat_answer.go). done is false when the request is no longer waiting, and
// the message is then an ordinary one.
func (s *Service) answerFromText(threadID, requestID, text string) (done bool, err error) {
	ev, ok := s.Threads.PendingRequest(threadID, requestID)
	if !ok {
		s.Relay.forget(requestID)
		return false, nil
	}
	var ans Answer
	switch ev.Kind {
	case db.EvApproval:
		ans = approvalReply(text)
	case db.EvAsk:
		var p requestPayload
		_ = json.Unmarshal([]byte(ev.Payload), &p)
		ans = askReply(askItems(p.Input, ev.Content), text)
	default:
		return false, nil
	}
	if err := s.Threads.Respond(threadID, requestID, ans); err != nil {
		if errors.Is(err, ErrNoPrompt) {
			return false, nil
		}
		return true, err
	}
	return true, nil
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

// AnswerFromChannel answers a thread's pending approval with the button a
// chat pressed (a ChannelChoice value). The chat must be bound to
// the thread or to one of its ancestors, the chats the request was sent to.
// reply is what to tell the chat when there is nothing to answer any more;
// on success the prompt is restated through the Relay instead.
func (s *Service) AnswerFromChannel(platform, chatID, threadID, requestID, value string) (reply string, err error) {
	b, err := s.Store.GetChannelBinding(platform, chatID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return "", ErrNotThisChat
		}
		return "", err
	}
	lineage, err := s.Store.Lineage(threadID)
	if err != nil {
		return "", err
	}
	bound := false
	for _, t := range lineage {
		bound = bound || t.ID == b.ThreadID
	}
	if !bound {
		return "", ErrNotThisChat
	}
	ev, ok := s.Threads.PendingRequest(threadID, requestID)
	if !ok {
		return "This was already answered.", nil
	}
	ans := Answer{Via: db.SourceIM}
	switch {
	case value == ChoiceAllow && ev.Kind == db.EvApproval:
		ans.Allow = true
	case value == ChoiceAllowThread && ev.Kind == db.EvApproval:
		ans.Allow, ans.Scope = true, "thread"
	case value == ChoiceDeny && ev.Kind == db.EvApproval:
	default:
		return "", fmt.Errorf("unknown choice %q", value)
	}
	if err := s.Threads.Respond(threadID, requestID, ans); err != nil {
		if errors.Is(err, ErrNoPrompt) {
			return "This was already answered.", nil
		}
		return "", err
	}
	return "", nil
}
