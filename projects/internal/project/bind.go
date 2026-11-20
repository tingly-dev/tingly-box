package project

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
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

// ChannelAuthorization checks the current sender at the point of use.
type ChannelAuthorization struct {
	Answer func() bool
	Start  func() bool
}

// PostFromChannel delivers a message from an IM chat to the thread the chat is
// bound to, as a message with source im, exactly as the web would send it
// (Send). handled is false when the chat has no binding, and the caller
// carries on as before. The reply arrives in the chat through the Relay.
//
// A folder chat that asked for a new session (/new), or whose thread is done
// (resolved by hand or after idling), starts a new thread in the folder with
// this message instead, and the chat moves to it.
func (s *Service) PostFromChannel(platform, chatID, text string, files []ChannelFile, policies ...ChannelAuthorization) (handled bool, err error) {
	var policy ChannelAuthorization
	if len(policies) > 0 {
		policy = policies[0]
	}
	b, err := s.Store.GetChannelBinding(platform, chatID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	authorizeStart := func() bool {
		return policy.Start == nil || policy.Start()
	}
	denyStart := func() (bool, error) {
		return true, errors.New("you are not authorized to send messages to this session")
	}
	src := Source{Kind: db.SourceIM, Platform: platform, ChatID: chatID}
	// A chat that moved to a folder, or talks for the first time, starts a
	// thread there with this message.
	if b.Folder != "" {
		if !authorizeStart() {
			return denyStart()
		}
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
		if !authorizeStart() {
			return denyStart()
		}
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
		if done, err := s.answerWaiting(b, text, src, policy.Answer); done {
			return true, err
		}
	}
	if !authorizeStart() {
		return denyStart()
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

// MayAnswerChannelText is a conservative classification for the inbound gate.
// The actual request and permissions are checked again when consuming the text.
func (s *Service) MayAnswerChannelText(platform, chatID, text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	if _, _, mention := parseMention(text); mention {
		return false
	}
	b, err := s.Store.GetChannelBinding(platform, chatID)
	if err != nil || b.Folder != "" {
		return false
	}
	return len(s.matchReplies(b, text)) > 0
}

// replyMatch is a request a chat's message reads as an answer to.
type replyMatch struct {
	pendingRequest
	// card is set when the request is a decision card.
	card *Decision
	// text is the answer in the message: the message less the request's code.
	text string
	// explicit is set when the message started with the request's code, which
	// names it whatever else is open.
	explicit bool
}

// matchReplies lists the open requests a chat may answer that its message
// answers: a message that starts with a request's code ("a3f9 not on Friday")
// answers that request in any words; otherwise it must be one of the request's
// own answers (an option's number or label, or y, always, n for an approval).
// Anything else answers nothing and is an ordinary message: the one it is sent
// to decides what it means, not a guess about which request it was for.
func (s *Service) matchReplies(b *db.ChannelBinding, text string) []replyMatch {
	var out []replyMatch
	for _, q := range s.Relay.awaitingAll(b.Platform, b.ChatID) {
		if s.mayAnswer(b, q.threadID) != nil {
			continue
		}
		if _, d, ok := s.Coordinator.pendingDecision(q.threadID, q.requestID); ok {
			if reply, explicit, fits := decisionReply(d, q.requestID, text); fits {
				out = append(out, replyMatch{pendingRequest: q, card: &d, text: reply, explicit: explicit})
			}
			continue
		}
		ev, ok := s.Threads.PendingRequest(q.threadID, q.requestID)
		if !ok {
			continue
		}
		if rest, named := cutCode(q.requestID, text); named {
			if rest != "" && (ev.Kind == db.EvApproval || ev.Kind == db.EvAsk) {
				out = append(out, replyMatch{pendingRequest: q, text: rest, explicit: true})
			}
			continue
		}
		if requestTakes(ev, text) {
			out = append(out, replyMatch{pendingRequest: q, text: text})
		}
	}
	return out
}

// answerWaiting gives a chat's message to the request it answers (matchReplies).
// A decision card comes first among the requests a bare reply fits, so "1" is
// never read as an approval's words. When a bare reply fits more than one open
// request the chat is asked which it means instead of being guessed at: "2"
// would answer the wrong question. done is false when the message answers
// nothing, and the caller sends it on as an ordinary message.
func (s *Service) answerWaiting(b *db.ChannelBinding, text string, src Source, authorize func() bool) (done bool, err error) {
	matches := s.matchReplies(b, text)
	var chosen *replyMatch
	for i := range matches {
		if matches[i].explicit {
			chosen = &matches[i]
			break
		}
	}
	if chosen == nil {
		var cards, others []replyMatch
		for _, m := range matches {
			if m.card != nil {
				cards = append(cards, m)
			} else {
				others = append(others, m)
			}
		}
		group := cards
		if len(group) == 0 {
			group = others
		}
		switch len(group) {
		case 0:
			return false, nil
		case 1:
			chosen = &group[0]
		default:
			s.tellWhichRequest(b, group)
			return true, nil
		}
	}
	if !s.authorizeChannelAnswer(b, authorize) {
		return true, nil
	}
	if chosen.card != nil {
		return s.chooseFromText(b, chosen.pendingRequest, *chosen.card, chosen.text, src)
	}
	return s.answerFromText(chosen.threadID, chosen.requestID, chosen.text)
}

// tellWhichRequest asks a chat whose reply fits several open requests which
// one it answers, with the code to start the reply with.
func (s *Service) tellWhichRequest(b *db.ChannelBinding, group []replyMatch) {
	var lines []string
	for _, m := range group {
		label := ""
		if m.card != nil {
			label = m.card.Question
		} else if ev, ok := s.Threads.PendingRequest(m.threadID, m.requestID); ok {
			label = ev.Content
		}
		lines = append(lines, "- "+decisionCode(m.requestID)+" · "+oneLine(label))
	}
	what := "request"
	if group[0].card != nil {
		what = "question"
	}
	s.Relay.tell(b.BotUUID, b.Platform, b.ChatID, "More than one "+what+" is waiting here, so I can't tell which one that answers. Start your reply with the "+what+"'s code, like \""+
		decisionCode(group[0].requestID)+" "+firstWord(group[0].text)+"\":\n"+strings.Join(lines, "\n"))
}

func firstWord(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}

func (s *Service) authorizeChannelAnswer(b *db.ChannelBinding, authorize func() bool) bool {
	if authorize != nil && !authorize() {
		s.Relay.tell(b.BotUUID, b.Platform, b.ChatID, "⚠️ You are not authorized to answer this request. Ask the bot owner to grant the approve permission for this chat.")
		return false
	}
	// Recheck after the potentially slow access-policy check. A rebind can
	// still race the later response; the check only narrows that window.
	current, err := s.Store.GetChannelBinding(b.Platform, b.ChatID)
	return err == nil && current.UpdatedAt.Equal(b.UpdatedAt)
}

func decisionReply(d Decision, requestID, text string) (reply string, explicit, fits bool) {
	if rest, named := cutCode(requestID, text); named {
		if _, fits := d.optionOfReply(rest); fits {
			return rest, true, true
		}
	}
	_, fits = d.optionOfReply(text)
	return text, false, fits
}

// decisionCode is the short code a chat can start a reply with to say which
// open request it answers: four hex digits of the request's id (the ids of
// the requests a thread's agent makes share their first characters).
func decisionCode(requestID string) string {
	sum := sha1.Sum([]byte(requestID))
	return hex.EncodeToString(sum[:])[:4]
}

// cutCode reads a message that starts with the request's code, followed by
// nothing or a separator, and returns the rest of it.
func cutCode(requestID, text string) (rest string, ok bool) {
	text = strings.TrimSpace(text)
	code := decisionCode(requestID)
	if len(text) < len(code) || !strings.EqualFold(text[:len(code)], code) {
		return "", false
	}
	rest = text[len(code):]
	if rest != "" && !strings.ContainsAny(rest[:1], " \t:,.-") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimLeft(rest, " \t:,.-")), true
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
	return mayAnswer(s.Store, b, threadID)
}

func mayAnswer(store *db.Store, b *db.ChannelBinding, threadID string) error {
	if b.Folder != "" {
		return ErrNotThisChat
	}
	lineage, err := store.Lineage(threadID)
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
		if t, err := store.GetThread(b.ThreadID); err == nil {
			project = t.ProjectID
		}
	}
	if project != "" && project == lineage[0].ProjectID && b.TargetThreadID == "" && b.Folder == "" {
		p, err := store.GetProject(project)
		if err != nil {
			return err
		}
		if p.Type == db.ProjectProject {
			return nil
		}
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
