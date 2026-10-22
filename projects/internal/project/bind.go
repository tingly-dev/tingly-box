package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
func (s *Service) PostFromChannel(platform, chatID, text string, files []ChannelFile) (handled bool, err error) {
	b, err := s.Store.GetChannelBinding(platform, chatID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	src := Source{Kind: db.SourceIM, Platform: platform, ChatID: chatID}
	// A chat that moved to a folder, or talks for the first time, starts a
	// thread there with this message.
	if b.Folder != "" {
		atts, err := s.adoptChannelFiles(DraftOwner, files)
		if err != nil {
			return true, err
		}
		_, err = s.startInChannelFolder(b, text, atts, src)
		return true, err
	}
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
		atts, err := s.adoptChannelFiles(UploadOwner(p, target), files)
		if err != nil {
			return true, err
		}
		_, err = s.Send(context.Background(), target.ID, rest, atts, src)
		return true, err
	}
	// A chat that was asked something answers it with its next message
	// (other than an @N one, or one with files).
	if threadID, requestID, ok := s.Relay.Awaiting(platform, chatID); ok && len(files) == 0 {
		if done, err := s.answerFromText(threadID, requestID, text); done {
			return true, err
		}
	}
	if p.Type == db.ProjectFolder && (b.NewThread || t.Status == db.ThreadResolved) {
		atts, err := s.adoptChannelFiles(DraftOwner, files)
		if err != nil {
			return true, err
		}
		_, err = s.startFromChannel(b, t, t.ProjectID, text, atts, src)
		return true, err
	}
	atts, err := s.adoptChannelFiles(UploadOwner(p, t), files)
	if err != nil {
		return true, err
	}
	_, err = s.Send(context.Background(), b.ThreadID, text, atts, src)
	return true, err
}

// ChannelFile is a file a chat sent with a message, on local disk until
// PostFromChannel returns.
type ChannelFile struct {
	// Name is the file's name as the chat gave it.
	Name string
	// Path is where the host saved it.
	Path string
}

// maxChannelFile caps one file a chat sends, as the web caps an upload.
const maxChannelFile = 200 << 20

// adoptChannelFiles copies a chat's files into owner's uploads, where the
// thread reads them as it reads files attached on the web.
func (s *Service) adoptChannelFiles(owner string, files []ChannelFile) ([]Attachment, error) {
	if len(files) > MaxAttachments {
		return nil, fmt.Errorf("at most %d files per message", MaxAttachments)
	}
	var atts []Attachment
	for _, f := range files {
		src, err := os.Open(f.Path)
		if err != nil {
			return nil, err
		}
		name := f.Name
		if strings.TrimSpace(name) == "" {
			name = filepath.Base(f.Path)
		}
		a, err := s.Layout.SaveAttachment(owner, name, src, maxChannelFile)
		src.Close()
		if err != nil {
			return nil, err
		}
		atts = append(atts, a)
	}
	return atts, nil
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

// startInChannelFolder starts a thread in the chat's Folder (opening the
// folder in Projects when it is new) and moves the chat to it.
func (s *Service) startInChannelFolder(b *db.ChannelBinding, task string, atts []Attachment, src Source) (*db.Thread, error) {
	p, err := s.Store.OpenFolder(b.Folder, "")
	if err != nil {
		return nil, err
	}
	var prev *db.Thread
	if b.ThreadID != "" {
		prev, _ = s.Store.GetThread(b.ThreadID)
	}
	return s.startFromChannel(b, prev, p.ID, task, atts, src)
}

// startFromChannel starts a thread in the folder projectID, with the model,
// profile and permission mode of prev (the thread the chat was on; nil for
// a chat that never talked), and moves the chat to it.
func (s *Service) startFromChannel(b *db.ChannelBinding, prev *db.Thread, projectID, task string, atts []Attachment, src Source) (*db.Thread, error) {
	spec := FolderThreadSpec{ProjectID: projectID, Task: task, Source: src, Attachments: atts}
	if prev != nil {
		spec.Model, spec.PermissionMode, spec.Profile = prev.Model, prev.PermissionMode, prev.Profile
	}
	t, err := s.Threads.StartFolderThread(spec)
	if err != nil {
		return nil, err
	}
	nb := &db.ChannelBinding{Platform: b.Platform, ChatID: b.ChatID, ThreadID: t.ID}
	if err := s.Store.UpsertChannelBinding(nb); err != nil {
		return nil, err
	}
	return t, nil
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

// ChannelToFolder moves a chat to a folder (an absolute path, which must
// exist): its next message starts a new thread there. With ifNew, a chat
// that already has a binding is left as it is, which is how a chat that
// never talked gets its first folder.
func (s *Service) ChannelToFolder(platform, chatID, title, folder string, ifNew bool) error {
	path, err := CleanFolder(folder)
	if err != nil {
		return err
	}
	if ifNew {
		if _, err := s.Store.GetChannelBinding(platform, chatID); err == nil {
			return nil
		} else if !errors.Is(err, db.ErrNotFound) {
			return err
		}
	}
	return s.Store.SetChannelBindingFolder(platform, chatID, title, path)
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
