package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// Relay sends a thread's finished replies and folded thread reports to the
// chats bound to that thread (a worker's or folder thread's own turn end
// included), and a descendant's approvals and questions to the chats bound to
// it or to any ancestor, to answer by replying (an approval also has buttons
// where the host can show them, ChannelPrompter, restated once the request is
// settled wherever it was answered). Messages go out one at a time, in the order
// they finished. Each starts with where it comes from: `[project name]` for a
// project conversation, `[#N title]` for any other thread.
type Relay struct {
	store    *db.Store
	sink     ChannelSink
	prompter ChannelPrompter

	mu      sync.Mutex
	queue   []relayJob
	running bool
	// sent are the prompts with buttons out in chats, by request id, until
	// it settles.
	sent map[string][]sentPrompt
	// awaiting are the requests each chat was asked and that are not
	// settled, oldest first, by chatKey: the chat's next message answers
	// the first.
	awaiting map[string][]pendingRequest
}

// pendingRequest is a request a chat was asked.
type pendingRequest struct{ threadID, requestID string }

func chatKey(platform, chatID string) string { return platform + "\x00" + chatID }

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
	// request, when set, is the request text asks; chats answer it by
	// replying. prompt, when set, is that request with buttons, for the
	// chats that can show them.
	request *pendingRequest
	prompt  *ChannelPrompt
	// settle, when set, is a request id: the prompts sent for it are
	// restated with text added, and nothing new is sent.
	settle string
}

// NewRelay builds the relay; sink may be nil, which makes every call a no-op.
func NewRelay(store *db.Store, sink ChannelSink) *Relay {
	prompter, _ := sink.(ChannelPrompter)
	return &Relay{store: store, sink: sink, prompter: prompter, sent: map[string][]sentPrompt{}, awaiting: map[string][]pendingRequest{}}
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
// approval or question in request by replying (an approval also has buttons
// where the chat can show them).
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
	job := relayJob{threadIDs: ids, tag: threadTag(t.Seq, t.Title)}
	var p requestPayload
	_ = json.Unmarshal([]byte(request.Payload), &p)
	if p.RequestID == "" {
		// Nothing a chat could answer.
		job.text = join("Needs you", oneLine(request.Content)) + ". Open Projects in the web UI to answer."
		r.enqueueJob(job)
		return
	}
	job.text = requestText(request, p)
	job.request = &pendingRequest{threadID: t.ID, requestID: p.RequestID}
	if request.Kind == db.EvApproval {
		job.prompt = &ChannelPrompt{ThreadID: t.ID, RequestID: p.RequestID, Text: approvalBody(request, p), Choices: []ChannelChoice{
			{Value: ChoiceAllow, Label: "Allow"},
			{Value: ChoiceAllowThread, Label: "Allow for this thread"},
			{Value: ChoiceDeny, Label: "Deny"},
		}}
	}
	r.enqueueJob(job)
}

// Settled restates the prompts a request was sent as with how it ended, so a
// chat never offers buttons for something already answered elsewhere, and
// stops taking the chats' next messages as its answer.
func (r *Relay) Settled(t db.Thread, request db.ThreadEvent) {
	if r == nil || r.sink == nil {
		return
	}
	var p requestPayload
	_ = json.Unmarshal([]byte(request.Payload), &p)
	if p.RequestID == "" {
		return
	}
	r.forget(p.RequestID)
	if r.prompter != nil {
		r.enqueueJob(relayJob{settle: p.RequestID, text: settledLine(p)})
	}
}

// Awaiting returns the oldest request a chat was asked that is not settled:
// the chat's next message answers it.
func (r *Relay) Awaiting(platform, chatID string) (threadID, requestID string, ok bool) {
	if r == nil {
		return "", "", false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	reqs := r.awaiting[chatKey(platform, chatID)]
	if len(reqs) == 0 {
		return "", "", false
	}
	return reqs[0].threadID, reqs[0].requestID, true
}

// forget drops a settled request from every chat that was asked it.
func (r *Relay) forget(requestID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, reqs := range r.awaiting {
		kept := reqs[:0]
		for _, q := range reqs {
			if q.requestID != requestID {
				kept = append(kept, q)
			}
		}
		if len(kept) == 0 {
			delete(r.awaiting, k)
		} else {
			r.awaiting[k] = kept
		}
	}
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
		if p.Seq > 0 {
			return tag, join("Needs you", body) + fmt.Sprintf(". Reply with @%d and your answer.", p.Seq)
		}
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
		if job.request != nil {
			r.await(b, *job.request)
		}
		if job.prompt == nil || !r.sendPrompt(ctx, b, "["+tag+"] ", *job.prompt) {
			if err := r.sink.SendChannelText(ctx, b.Platform, b.ChatID, text); err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{"platform": b.Platform, "chat_id": b.ChatID, "thread_id": b.ThreadID}).Warn("projects: send to IM chat")
			}
		}
		cancel()
	}
}

// await records that a chat was asked a request, before the chat can see it,
// so that its reply is taken as the answer.
func (r *Relay) await(b db.ChannelBinding, q pendingRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := chatKey(b.Platform, b.ChatID)
	r.awaiting[k] = append(r.awaiting[k], q)
}

// sendPrompt sends a request's buttons to a chat and remembers the message
// for restate. It reports false when the chat should get the request as text
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
