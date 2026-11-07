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
	"github.com/tingly-dev/tingly-box/remote/agent"
)

// Errors for channel bindings.
var (
	ErrBindingInvalid = errors.New("platform, chat_id and one of thread_id or project_id are required")
	// ErrNotThisChat is for a chat answering a thread it is not bound to,
	// directly, through an ancestor or through the thread's project.
	ErrNotThisChat = errors.New("this chat is not bound to that thread")
)

// BindChannel binds an IM chat to a thread; a chat that is already bound
// moves. A project id stands for the project's main thread. title is an
// optional display name for the chat.
func (s *Service) BindChannel(platform, chatID, title, threadID, projectID string, botUUID ...string) (*db.ChannelBinding, error) {
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
			if p.ArchivedAt != nil {
				return nil, ErrArchived
			}
			if err := s.ChannelToFolder(platform, chatID, title, p.Path, "", false, botUUID...); err != nil {
				return nil, err
			}
			return s.Store.GetChannelBinding(platform, chatID)
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
	b.TargetProjectID = p.ID
	if threadID != "" && p.Type == db.ProjectFolder {
		b.TargetThreadID = t.ID
	}
	if len(botUUID) > 0 {
		b.BotUUID = strings.TrimSpace(botUUID[0])
	}
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
		p, err := s.Store.OpenFolder(b.Folder, "")
		if err != nil {
			return true, err
		}
		atts, err := s.adoptChannelFiles(p.ID, files)
		if err != nil {
			return true, err
		}
		_, err = s.startInChannelFolder(b, p, text, atts, src)
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
	if len(files) == 0 {
		if done, err := s.answerWaiting(b, text, src); done {
			return true, err
		}
	}
	if p.Type == db.ProjectFolder && (b.NewThread || t.Status == db.ThreadResolved || t.ArchivedAt != nil) {
		atts, err := s.adoptChannelFiles(t.ProjectID, files)
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
// Post returns.
type ChannelFile = agent.File

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
func (s *Service) startInChannelFolder(b *db.ChannelBinding, p *db.Project, task string, atts []Attachment, src Source) (*db.Thread, error) {
	var prev *db.Thread
	if b.ThreadID != "" {
		prev, _ = s.Store.GetThread(b.ThreadID)
	}
	return s.startFromChannel(b, prev, p.ID, task, atts, src)
}

// startFromChannel starts a thread in the folder projectID, with the model,
// profile and permission mode of prev (the thread the chat was on), or the
// chat's profile when it never talked (the default when that profile is
// gone), and moves the chat to it.
func (s *Service) startFromChannel(b *db.ChannelBinding, prev *db.Thread, projectID, task string, atts []Attachment, src Source) (*db.Thread, error) {
	spec := FolderThreadSpec{ProjectID: projectID, Task: task, Source: src, Attachments: atts}
	if prev != nil {
		spec.Model, spec.PermissionMode, spec.Profile = prev.Model, prev.PermissionMode, prev.Profile
	} else if s.Threads.Launchable(b.Profile) {
		spec.Profile = b.Profile
	}
	return s.Threads.startFolderThread(spec, func(t *db.Thread) error {
		nb := &db.ChannelBinding{Platform: b.Platform, ChatID: b.ChatID, ThreadID: t.ID, TargetProjectID: t.ProjectID, TargetThreadID: b.TargetThreadID}
		return s.Store.UpsertChannelBinding(nb)
	})
}

// answerWaiting gives a chat's message to the request it answers. A decision
// card takes only an option's number or label, and the card comes first so a
// bare "1" is never read as an approval's words. A reply that starts with a
// card's code ("a3f9 2") answers that card whatever else is open. When a bare
// reply fits more than one open card the chat is asked which it means instead
// of being guessed at: "2" would answer the wrong question. Otherwise the
// oldest request that takes the reply gets it, and an open card never keeps a
// later approval or question from being answered.
func (s *Service) answerWaiting(b *db.ChannelBinding, text string, src Source) (done bool, err error) {
	type card struct {
		pendingRequest
		d Decision
	}
	var cards []card
	var others []pendingRequest
	for _, q := range s.Relay.awaitingAll(b.Platform, b.ChatID) {
		if s.mayAnswer(b, q.threadID) != nil {
			continue
		}
		if _, d, ok := s.Coordinator.pendingDecision(q.threadID, q.requestID); ok {
			cards = append(cards, card{q, d})
			continue
		}
		others = append(others, q)
	}
	var named []card
	for _, c := range cards {
		if rest, ok := strings.CutPrefix(strings.ToLower(strings.TrimSpace(text)), decisionCode(c.requestID)); ok && (rest == "" || strings.ContainsAny(rest[:1], " \t:,.-")) {
			if _, fits := c.d.optionOfReply(rest); fits {
				return s.chooseFromText(b, c.pendingRequest, c.d, rest, src)
			}
		}
		if _, fits := c.d.optionOfReply(text); fits {
			named = append(named, c)
		}
	}
	switch {
	case len(named) > 1:
		var lines []string
		for _, c := range named {
			lines = append(lines, "- "+decisionCode(c.requestID)+" · "+oneLine(c.d.Question))
		}
		s.Relay.tell(b.BotUUID, b.Platform, b.ChatID, "More than one question is waiting here, so I can't tell which one that answers. Start your reply with the question's code, like \""+
			decisionCode(named[0].requestID)+" 1\":\n"+strings.Join(lines, "\n"))
		return true, nil
	case len(named) == 1:
		return s.chooseFromText(b, named[0].pendingRequest, named[0].d, text, src)
	}
	for _, q := range others {
		if done, err := s.answerFromText(q.threadID, q.requestID, text); done {
			return true, err
		}
	}
	return false, nil
}

// decisionCode is the short code a chat can start a reply with to say which
// open card it answers: the first four characters of the card's id.
func decisionCode(requestID string) string {
	if len(requestID) > 4 {
		return strings.ToLower(requestID[:4])
	}
	return strings.ToLower(requestID)
}

// chooseFromText picks the option a chat's reply names on a decision card. A
// card answered a moment earlier elsewhere is said to be, not an error.
func (s *Service) chooseFromText(b *db.ChannelBinding, q pendingRequest, d Decision, text string, src Source) (bool, error) {
	i, ok := d.optionOfReply(text)
	if !ok {
		return false, nil
	}
	_, err := s.Coordinator.Choose(context.Background(), q.threadID, q.requestID, i, src)
	if errors.Is(err, ErrDecided) {
		s.Relay.tell(b.BotUUID, b.Platform, b.ChatID, "This was already answered.")
		return true, nil
	}
	return true, err
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
// never talked gets its first folder. profile is the launch profile a chat
// that never talked starts its first thread under.
func (s *Service) ChannelToFolder(platform, chatID, title, folder, profile string, ifNew bool, botUUID ...string) error {
	path, err := CleanFolder(folder)
	if err != nil {
		return err
	}
	// Use the same spelling as a registered folder, including symlinks and
	// case-insensitive filesystems, so pending bindings remain visible there.
	path = db.CanonicalPath(path)
	if p, err := s.Store.FindFolder(path); err == nil {
		path = p.Path
	} else if !errors.Is(err, db.ErrNotFound) {
		return err
	}
	if ifNew {
		if _, err := s.Store.GetChannelBinding(platform, chatID); err == nil {
			return nil
		} else if !errors.Is(err, db.ErrNotFound) {
			return err
		}
	}
	return s.Store.SetChannelBindingFolder(platform, chatID, title, path, strings.TrimSpace(profile), botUUID...)
}

// mayAnswer reports whether the chat of b may answer a request of a thread
// (ErrNotThisChat when not): a chat bound to the thread or to one of its
// ancestors, or a chat bound to the thread's project as a whole, which also
// answers for threads it does not follow (it follows one at a time, /session).
// A chat bound to one session of a folder (TargetThreadID) keeps to that
// session's lineage, and a chat of another project never answers.
func (s *Service) mayAnswer(b *db.ChannelBinding, threadID string) error {
	lineage, err := s.Store.Lineage(threadID)
	if err != nil {
		return err
	}
	if len(lineage) == 0 {
		return ErrNotThisChat
	}
	for _, t := range lineage {
		if t.ID == b.ThreadID {
			return nil
		}
	}
	project := b.TargetProjectID
	if project == "" && b.ThreadID != "" {
		if t, err := s.Store.GetThread(b.ThreadID); err == nil {
			project = t.ProjectID
		}
	}
	if project != "" && project == lineage[0].ProjectID && b.TargetThreadID == "" && b.Folder == "" {
		return nil
	}
	return ErrNotThisChat
}

// AnswerFromChannel answers a thread's pending approval with the button a
// chat pressed (a ChannelChoice value). The chat must be allowed to answer
// the thread (mayAnswer).
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
	if err := s.mayAnswer(b, threadID); err != nil {
		return "", err
	}
	if _, _, isDecision := s.Coordinator.pendingDecision(threadID, requestID); isDecision {
		i, ok := optionOfChoice(value)
		if !ok {
			return "", fmt.Errorf("unknown choice %q", value)
		}
		_, err := s.Coordinator.Choose(context.Background(), threadID, requestID, i, Source{Kind: db.SourceIM, Platform: platform, ChatID: chatID})
		switch {
		case errors.Is(err, ErrDecided):
			return "This was already answered.", nil
		case err != nil:
			return "", err
		}
		return "", nil
	}
	if m, err := s.Store.GetThreadEvent(requestID); err == nil && m.Kind == db.EvDecision && m.ThreadID == threadID {
		return "This was already answered.", nil
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
