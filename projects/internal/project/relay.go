package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// Relay sends a thread's finished replies and folded thread reports to the
// chats bound to that thread (a worker's or folder thread's own turn end
// included), and a descendant's approvals and questions to the chats bound to
// it or to any ancestor: with buttons where the host can (ChannelPrompter),
// restated once the request is settled wherever it was answered, else as one
// line pointing to the web. Messages go out one at a time, in the order
// they finished. Each starts with where it comes from: `[project name]` for a
// project conversation, `[#N title]` for any other thread.
type Relay struct {
	store    *db.Store
	sink     ChannelSink
	prompter ChannelPrompter

	mu       sync.Mutex
	queue    []relayJob
	running  bool
	notified map[string]time.Time
	// sent are the prompts out in chats, by request id, until it settles.
	sent map[string][]sentPrompt
	now  func() time.Time
}

// sentPrompt is a prompt a chat was sent, to restate when it settles.
type sentPrompt struct {
	platform, chatID, ref, text string
}

type relayJob struct {
	// threadIDs are the threads whose chats hear it.
	threadIDs []string
	// tag is the prefix ("#2 Fix login"); empty names the project of the
	// first thread instead.
	tag  string
	text string
	// prompt, when set, is a request chats with buttons can answer; text is
	// then what other chats get, at most once per thread and chat every
	// relayNeedsUserEvery. subject is the thread it is about.
	prompt  *ChannelPrompt
	subject string
	// settle, when set, is a request id: the prompts sent for it are
	// restated with text added, and nothing new is sent.
	settle string
}

// NewRelay builds the relay; sink may be nil, which makes every call a no-op.
func NewRelay(store *db.Store, sink ChannelSink) *Relay {
	prompter, _ := sink.(ChannelPrompter)
	return &Relay{store: store, sink: sink, prompter: prompter, notified: map[string]time.Time{}, sent: map[string][]sentPrompt{}, now: time.Now}
}

// Message relays a finished event of a main thread. User events, which the
// chat or the web already shows, and kinds without text for a chat are skipped.
func (r *Relay) Message(e db.ThreadEvent) {
	if r == nil || r.sink == nil {
		return
	}
	var tag, text string
	switch e.Kind {
	case db.EvAssistant:
		text = strings.TrimSpace(e.Content)
	case db.EvError:
		if c := strings.TrimSpace(e.Content); c != "" {
			text = "Error: " + c
		}
	case db.EvThreadUpdate:
		tag, text = foldThreadUpdate(e)
	}
	if text != "" {
		r.enqueue([]string{e.ThreadID}, tag, text)
	}
}

// ThreadReply relays the end of a turn of a non-main thread (a worker or a
// folder thread) to the chats bound to that very thread. Chats bound to its
// parent already hear it as the coordinator's thread report, so only this
// thread's own chats are addressed.
func (r *Relay) ThreadReply(rep Report) {
	if r == nil || r.sink == nil {
		return
	}
	text := strings.TrimSpace(rep.Text)
	if text == "" {
		return
	}
	if rep.Kind == ReportFailed {
		text = "Error: " + text
	}
	r.enqueue([]string{rep.Thread.ID}, threadTag(rep.Thread.Seq, rep.Thread.Title), text)
}

// NeedsUser asks the chats of a thread, and of its ancestors, to answer the
// approval or question in request: with buttons where the chat can show them,
// else one line pointing to the web.
func (r *Relay) NeedsUser(t db.Thread, request db.ThreadEvent) {
	if r == nil || r.sink == nil {
		return
	}
	lineage, err := r.store.Lineage(t.ID)
	if err != nil || len(lineage) == 0 {
		return
	}
	ids := make([]string, len(lineage))
	for i, a := range lineage {
		ids[i] = a.ID
	}
	text := join("Needs you", oneLine(request.Content)) + ". Open Projects in the web UI to answer."
	r.enqueueJob(relayJob{threadIDs: ids, tag: threadTag(t.Seq, t.Title), text: text, prompt: requestPrompt(t, request), subject: t.ID})
}

// Settled restates the prompts a request was sent as with how it ended, so a
// chat never offers buttons for something already answered elsewhere.
func (r *Relay) Settled(t db.Thread, request db.ThreadEvent) {
	if r == nil || r.prompter == nil {
		return
	}
	var p requestPayload
	_ = json.Unmarshal([]byte(request.Payload), &p)
	if p.RequestID == "" {
		return
	}
	r.enqueueJob(relayJob{settle: p.RequestID, text: settledLine(p)})
}

// requestPrompt is a request as buttons: an approval is allow, allow for this
// thread or deny; a question is its options, when it is one single-choice
// question with a few of them. nil when it needs the web.
func requestPrompt(t db.Thread, e db.ThreadEvent) *ChannelPrompt {
	var p requestPayload
	_ = json.Unmarshal([]byte(e.Payload), &p)
	if p.RequestID == "" {
		return nil
	}
	out := &ChannelPrompt{ThreadID: t.ID, RequestID: p.RequestID}
	switch e.Kind {
	case db.EvApproval:
		out.Text = "Approve: " + e.Content
		if reason := strings.TrimSpace(p.Reason); reason != "" {
			out.Text += "\n" + reason
		}
		out.Choices = []ChannelChoice{
			{Value: ChoiceAllow, Label: "Allow"},
			{Value: ChoiceAllowThread, Label: "Allow for this thread"},
			{Value: ChoiceDeny, Label: "Deny"},
		}
	case db.EvAsk:
		q, ok := singleChoice(p.Input)
		if !ok {
			return nil
		}
		out.Text = q.question
		for i, o := range q.options {
			out.Choices = append(out.Choices, ChannelChoice{Value: fmt.Sprintf("o%d", i), Label: o.label})
			if o.description != "" {
				out.Text += fmt.Sprintf("\n- %s: %s", o.label, o.description)
			}
		}
	default:
		return nil
	}
	return out
}

type askOption struct{ label, description string }

type askQuestion struct {
	question string
	options  []askOption
}

// singleChoice reads an AskUserQuestion input that buttons can answer: one
// question, one pick, 1 to relayChoicesMax options.
func singleChoice(input map[string]any) (askQuestion, bool) {
	qs, _ := input["questions"].([]any)
	if len(qs) != 1 {
		return askQuestion{}, false
	}
	m, _ := qs[0].(map[string]any)
	q := askQuestion{}
	q.question, _ = m["question"].(string)
	if multi, _ := m["multiSelect"].(bool); multi || q.question == "" {
		return askQuestion{}, false
	}
	opts, _ := m["options"].([]any)
	for _, o := range opts {
		om, _ := o.(map[string]any)
		label, _ := om["label"].(string)
		if label == "" {
			return askQuestion{}, false
		}
		desc, _ := om["description"].(string)
		q.options = append(q.options, askOption{label: label, description: strings.TrimSpace(desc)})
	}
	if len(q.options) == 0 || len(q.options) > relayChoicesMax {
		return askQuestion{}, false
	}
	return q, true
}

// settledLine says how a request ended, and where it was answered.
func settledLine(p requestPayload) string {
	where := " on the web"
	if p.Via == db.SourceIM {
		where = " in a chat"
	}
	switch p.State {
	case "allowed":
		if p.Scope == "thread" {
			return "Allowed for this thread" + where + "."
		}
		return "Allowed" + where + "."
	case "denied":
		return "Denied" + where + "."
	case "answered":
		if p.Auto {
			return join("Answered with its suggestion", oneLine(p.Answer))
		}
		return join("Answered"+where, oneLine(p.Answer))
	default:
		return "No longer waiting: the turn ended."
	}
}

// nudge reports whether a chat may get a thread's "needs you" line now.
func (r *Relay) nudge(threadID, platform, chatID string) bool {
	key := threadID + "\x00" + platform + "\x00" + chatID
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	if last, seen := r.notified[key]; seen && now.Sub(last) < relayNeedsUserEvery {
		return false
	}
	for k, at := range r.notified {
		if now.Sub(at) >= relayNeedsUserEvery {
			delete(r.notified, k)
		}
	}
	r.notified[key] = now
	return true
}

// threadTag names a thread in a chat: "#2 Fix login", the same number the web
// shows. A thread without a number (a main thread) is named by its title.
func threadTag(seq int, title string) string {
	title = strings.TrimSpace(title)
	switch {
	case seq > 0 && title != "":
		return fmt.Sprintf("#%d %s", seq, title)
	case seq > 0:
		return fmt.Sprintf("#%d", seq)
	case title != "":
		return title
	}
	return "Thread"
}

// foldThreadUpdate is a thread's report as the one line a chat gets, and the
// tag of the thread it is about.
func foldThreadUpdate(m db.ThreadEvent) (tag, text string) {
	var p struct {
		Seq   int    `json:"seq"`
		Title string `json:"title"`
		Kind  string `json:"kind"`
	}
	_ = json.Unmarshal([]byte(m.Payload), &p)
	body := oneLine(m.Content)
	tag = threadTag(p.Seq, p.Title)
	switch p.Kind {
	case ReportFailed:
		return tag, join("Failed", body)
	case ReportNeedsInput:
		return tag, join("Needs you", body) + ". Open Projects in the web UI to answer."
	default:
		return tag, join("Finished", body)
	}
}

func join(head, body string) string {
	if body == "" {
		return head
	}
	return head + ": " + body
}

// oneLine is the first non-empty line of s, cut to relayLineMax runes.
func oneLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if r := []rune(line); len(r) > relayLineMax {
			line = string(r[:relayLineMax]) + "..."
		}
		return line
	}
	return ""
}

func (r *Relay) enqueue(threadIDs []string, tag, text string) {
	r.enqueueJob(relayJob{threadIDs: threadIDs, tag: tag, text: text})
}

func (r *Relay) enqueueJob(job relayJob) {
	r.mu.Lock()
	r.queue = append(r.queue, job)
	start := !r.running
	r.running = true
	r.mu.Unlock()
	if start {
		go r.drain()
	}
}

func (r *Relay) drain() {
	for {
		r.mu.Lock()
		if len(r.queue) == 0 {
			r.running = false
			r.mu.Unlock()
			return
		}
		job := r.queue[0]
		r.queue = r.queue[1:]
		r.mu.Unlock()
		r.deliver(job)
	}
}

// deliver sends one message to every chat bound to the job's threads,
// prefixed with where it comes from so a chat never forgets where it is.
func (r *Relay) deliver(job relayJob) {
	if job.settle != "" {
		r.restate(job.settle, job.text)
		return
	}
	bindings, err := r.store.ListChannelBindingsOf(job.threadIDs)
	if err != nil || len(bindings) == 0 {
		return
	}
	tag := job.tag
	if tag == "" {
		first, err := r.store.GetThread(job.threadIDs[0])
		if err != nil {
			return
		}
		p, err := r.store.GetProject(first.ProjectID)
		if err != nil {
			return
		}
		tag = p.Name
	}
	text := "[" + tag + "] " + job.text
	for _, b := range bindings {
		ctx, cancel := context.WithTimeout(context.Background(), relaySendTimeout)
		if job.subject != "" {
			// A request: buttons where the chat has them, else the line,
			// throttled.
			if (job.prompt != nil && r.sendPrompt(ctx, b, "["+tag+"] ", *job.prompt)) || !r.nudge(job.subject, b.Platform, b.ChatID) {
				cancel()
				continue
			}
		}
		if err := r.sink.SendChannelText(ctx, b.Platform, b.ChatID, text); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{"platform": b.Platform, "chat_id": b.ChatID, "thread_id": b.ThreadID}).Warn("projects: send to IM chat")
		}
		cancel()
	}
}

// sendPrompt sends a request's buttons to a chat and remembers the message
// for restate. It reports false when the chat should get the text line
// instead: no prompter, or no buttons on its platform.
func (r *Relay) sendPrompt(ctx context.Context, b db.ChannelBinding, prefix string, p ChannelPrompt) bool {
	if r.prompter == nil {
		return false
	}
	p.Text = prefix + p.Text
	ref, err := r.prompter.SendChannelPrompt(ctx, b.Platform, b.ChatID, p)
	if err != nil {
		if errors.Is(err, ErrNoButtons) {
			return false
		}
		// The chat is unreachable now; the text line would fail as well.
		logrus.WithError(err).WithFields(logrus.Fields{"platform": b.Platform, "chat_id": b.ChatID, "thread_id": p.ThreadID}).Warn("projects: send a prompt to IM chat")
		return true
	}
	r.mu.Lock()
	r.sent[p.RequestID] = append(r.sent[p.RequestID], sentPrompt{platform: b.Platform, chatID: b.ChatID, ref: ref, text: p.Text})
	r.mu.Unlock()
	return true
}

// restate rewrites every prompt sent for a request with how it ended and
// takes the buttons away.
func (r *Relay) restate(requestID, outcome string) {
	r.mu.Lock()
	sent := r.sent[requestID]
	delete(r.sent, requestID)
	r.mu.Unlock()
	for _, sp := range sent {
		ctx, cancel := context.WithTimeout(context.Background(), relaySendTimeout)
		if err := r.prompter.RestateChannelPrompt(ctx, sp.platform, sp.chatID, sp.ref, sp.text+"\n\n"+outcome); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{"platform": sp.platform, "chat_id": sp.chatID}).Warn("projects: restate a prompt in IM chat")
		}
		cancel()
	}
}
