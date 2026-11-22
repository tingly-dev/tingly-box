package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// ErrDecided is returned when a decision card was already answered.
var ErrDecided = errors.New("the decision was already made")

// Decision is the payload of a decision event: a question the user answers
// by picking one option on a card.
type Decision struct {
	Question    string           `json:"question"`
	Context     string           `json:"context,omitempty"`
	Options     []DecisionOption `json:"options"`
	Recommended int              `json:"recommended"`
	Reason      string           `json:"reason,omitempty"`
	// ThreadID and Number say what the card is about: a thread of the
	// project and a PR or issue number. Both are optional; the system checks
	// the thread, fills in the other where the thread's links tell it, and
	// writes ThreadTitle and LinkKind itself.
	ThreadID    string `json:"thread_id,omitempty"`
	Number      int    `json:"number,omitempty"`
	ThreadSeq   int    `json:"thread_seq,omitempty"`
	ThreadTitle string `json:"thread_title,omitempty"`
	// LinkKind is db.LinkPR or db.LinkIssue when a thread's links know the
	// number, empty when not.
	LinkKind string `json:"link_kind,omitempty"`
	// Chosen is the picked option's index, nil until the user answers.
	Chosen *int `json:"chosen,omitempty"`
}

// decisionAnswer is what a user message that answers a card carries in its
// payload: the card's source, copied by the system when the user chose, so
// the coordinator reads which card, thread and PR the pick is for.
type decisionAnswer struct {
	Card        string `json:"card"`
	Question    string `json:"question"`
	ThreadID    string `json:"thread_id,omitempty"`
	ThreadSeq   int    `json:"thread_seq,omitempty"`
	ThreadTitle string `json:"thread_title,omitempty"`
	Number      int    `json:"number,omitempty"`
	LinkKind    string `json:"link_kind,omitempty"`
	// Option is the picked option's number, from 1 as the card shows it.
	Option int `json:"option,omitempty"`
}

// answerOf reads the card a user message answers from its payload.
func answerOf(payload string) *decisionAnswer {
	if payload == "" {
		return nil
	}
	var pl struct {
		Answer *decisionAnswer `json:"answer"`
	}
	if json.Unmarshal([]byte(payload), &pl) != nil || pl.Answer == nil || pl.Answer.Card == "" {
		return nil
	}
	return pl.Answer
}

// attrs are the envelope attributes that name the card an answer is for.
func (a *decisionAnswer) attrs() []string {
	if a == nil {
		return nil
	}
	out := []string{"kind", "answer", "card", short(a.Card), "question", a.Question}
	if a.ThreadID != "" {
		out = append(out, "thread", threadTag(a.ThreadSeq, a.ThreadTitle), "id", short(a.ThreadID))
	}
	if a.Number > 0 {
		key := a.LinkKind
		if key == "" {
			key = "number"
		}
		out = append(out, key, strconv.Itoa(a.Number))
	}
	return out
}

// lead is the line the system puts ahead of the user's pick in the
// coordinator's input, so the card the pick answers is the first thing read,
// not only an attribute. The pick itself follows on its own line, as the
// user's words.
func (a *decisionAnswer) lead() string {
	if a == nil {
		return ""
	}
	where := ""
	if s := about(a.ThreadSeq, a.ThreadTitle, a.Number, a.LinkKind); s != "" {
		where = ", about " + s
	}
	pick := "The user picked:"
	if a.Option > 0 {
		pick = fmt.Sprintf("The user picked option %d:", a.Option)
	}
	return escapeEnvelope(fmt.Sprintf("(System: answer to your decision card %s%s, %q. %s)", short(a.Card), where, oneLine(a.Question), pick))
}

// about names a's card's thread and PR in words.
func (a *decisionAnswer) about() string {
	return about(a.ThreadSeq, a.ThreadTitle, a.Number, a.LinkKind)
}

// about says in words what a card is about: its thread, by the project's
// number for it as chats name threads ("#12 Fix login"), and PR or issue.
func about(threadSeq int, threadTitle string, number int, kind string) string {
	var parts []string
	if threadSeq > 0 || threadTitle != "" {
		// A title is one line wherever a card's source is written.
		parts = append(parts, threadTag(threadSeq, strings.Join(strings.Fields(threadTitle), " ")))
	}
	if number > 0 {
		switch kind {
		case db.LinkPR:
			parts = append(parts, "PR #"+strconv.Itoa(number))
		case db.LinkIssue:
			parts = append(parts, "issue #"+strconv.Itoa(number))
		default:
			parts = append(parts, "#"+strconv.Itoa(number))
		}
	}
	return strings.Join(parts, " · ")
}

func (d Decision) about() string { return about(d.ThreadSeq, d.ThreadTitle, d.Number, d.LinkKind) }

// DecisionOption is one choice of a decision.
type DecisionOption struct {
	Label       string `json:"label"`
	Consequence string `json:"consequence,omitempty"`
}

// validate checks a decision the coordinator wants to ask.
func (d *Decision) validate() error {
	d.Chosen = nil
	d.Question = strings.TrimSpace(d.Question)
	if d.Question == "" {
		return errors.New("question is required")
	}
	if n := len(d.Options); n < 2 || n > 4 {
		return fmt.Errorf("give two to four options, not %d", n)
	}
	for i := range d.Options {
		d.Options[i].Label = strings.TrimSpace(d.Options[i].Label)
		if d.Options[i].Label == "" {
			return errors.New("every option needs a label")
		}
	}
	if d.Recommended < 0 || d.Recommended >= len(d.Options) {
		return fmt.Errorf("recommended must be an option index from 0 to %d", len(d.Options)-1)
	}
	if d.Number < 0 {
		return errors.New("number must be a PR or issue number")
	}
	return nil
}

// resolveSource checks what the card says it is about against the project's
// threads: a thread must be one of the project's own, and the system fills
// in what their links tell it. A thread named with no number is about its
// one open PR, if it has exactly one; a number named with no thread is about
// the one thread that has it. Nothing is guessed beyond that.
func (c *Coordinator) resolveSource(p *db.Project, d *Decision) error {
	d.ThreadSeq, d.ThreadTitle, d.LinkKind = 0, "", ""
	d.ThreadID = strings.TrimSpace(d.ThreadID)
	var th *db.Thread
	switch {
	case d.ThreadID != "":
		// By its number (#3) or id, as every thread tool takes it; the
		// conversation itself is not a thread a card can be about.
		var err error
		if th, err = c.findThread(p.ID, d.ThreadID); err != nil {
			return err
		}
		if th.ID == p.MainThreadID {
			return fmt.Errorf("no thread %q in this project", d.ThreadID)
		}
		// The card states it as the system's fact: a number the thread
		// never touched would pair it with someone else's PR.
		if d.Number > 0 && linkKind(th.Links, d.Number) == "" {
			return fmt.Errorf("thread %s has no PR or issue #%d; leave number out or name the thread that has it", threadName(th), d.Number)
		}
	case d.Number > 0:
		threads, err := c.store.ListThreads(p.ID)
		if err != nil {
			return err
		}
		for i := range threads {
			if threads[i].ID == p.MainThreadID || linkKind(threads[i].Links, d.Number) == "" {
				continue
			}
			if th != nil {
				return nil // two threads have it: not ours to pick
			}
			th = &threads[i]
		}
		if th == nil {
			return nil
		}
	default:
		return nil
	}
	d.ThreadID, d.ThreadSeq, d.ThreadTitle = th.ID, th.Seq, th.Title
	if d.Number == 0 {
		var open []db.ThreadLink
		for _, l := range th.Links {
			if l.Kind == db.LinkPR && l.State == db.LinkStateOpen {
				open = append(open, l)
			}
		}
		if len(open) == 1 {
			d.Number = open[0].Number
		}
	}
	d.LinkKind = linkKind(th.Links, d.Number)
	return nil
}

// linkKind is what a thread's link with this number is: a PR when any is
// (one thread can touch PRs and issues of several repos), else an issue; ""
// when the thread has none.
func linkKind(links []db.ThreadLink, number int) string {
	kind := ""
	for _, l := range links {
		if l.Number != number || number == 0 {
			continue
		}
		if l.Kind == db.LinkPR {
			return db.LinkPR
		}
		kind = l.Kind
	}
	return kind
}


// ask posts a decision card to the conversation.
func (c *Coordinator) ask(projectID string, d Decision) string {
	// The content is the card in words, for chats that cannot show a card.
	c.final(c.addEvent(projectID, &db.ThreadEvent{Kind: db.EvDecision, Content: d.text(), Payload: jsonString(d)}))
	return fmt.Sprintf("asked the user. Keep working on %q as long as it can be undone; stop before any step that cannot be. The answer arrives as the user's next message.",
		d.Options[d.Recommended].Label)
}

// Choose records the user's pick on a decision card of a main thread and
// tells the coordinator, as the user's own message: the label alone, so
// the coordinator's wording of the question is never quoted as the user's.
// The message's payload names the card and what it is about, which the
// coordinator reads ahead of the label in the system's words.
func (c *Coordinator) Choose(ctx context.Context, mainThreadID, eventID string, index int, src Source) (*db.ThreadEvent, error) {
	c.choosing.Lock()
	defer c.choosing.Unlock()
	main, err := c.store.GetThread(mainThreadID)
	if err != nil {
		return nil, err
	}
	m, err := c.store.GetThreadEvent(eventID)
	if err != nil {
		return nil, err
	}
	if m.ThreadID != main.ID || m.Kind != db.EvDecision {
		return nil, db.ErrNotFound
	}
	var d Decision
	if err := json.Unmarshal([]byte(m.Payload), &d); err != nil {
		return nil, err
	}
	if d.Chosen != nil {
		return m, ErrDecided
	}
	if index < 0 || index >= len(d.Options) {
		return nil, fmt.Errorf("choose an option from 0 to %d", len(d.Options)-1)
	}
	answer := decisionAnswer{Card: m.ID, Question: d.Question, ThreadID: d.ThreadID, ThreadSeq: d.ThreadSeq, ThreadTitle: d.ThreadTitle, Number: d.Number, LinkKind: d.LinkKind, Option: index + 1}
	if _, err := c.post(main.ProjectID, d.Options[index].Label, jsonString(map[string]any{"answer": answer}), src); err != nil {
		return nil, err
	}
	d.Chosen = &index
	m.Payload = jsonString(d)
	if err := c.store.UpdateThreadEvent(m); err != nil {
		return nil, err
	}
	c.publishEvent(main.ProjectID, m)
	if c.OnDecided != nil {
		c.OnDecided(*m, src)
	}
	return m, nil
}

// pendingDecision returns a decision card of a main thread that is still
// waiting for its answer.
func (c *Coordinator) pendingDecision(threadID, eventID string) (db.ThreadEvent, Decision, bool) {
	m, err := c.store.GetThreadEvent(eventID)
	if err != nil || m.ThreadID != threadID || m.Kind != db.EvDecision {
		return db.ThreadEvent{}, Decision{}, false
	}
	var d Decision
	if json.Unmarshal([]byte(m.Payload), &d) != nil || d.Chosen != nil {
		return db.ThreadEvent{}, Decision{}, false
	}
	return *m, d, true
}

// choiceValue is the value of the button of option i of a decision.
func choiceValue(i int) string { return "o" + strconv.Itoa(i) }

// optionOfChoice reads a button value made by choiceValue.
func optionOfChoice(value string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimPrefix(value, "o"))
	return n, err == nil && strings.HasPrefix(value, "o") && n >= 0
}

// optionOfReply reads a chat's reply as one of the options: its label, in
// any case, else its number ("2", "2."). Anything else is not an answer.
func (d Decision) optionOfReply(text string) (int, bool) {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return 0, false
	}
	trimmed := strings.TrimRight(text, ".。!！ ")
	for i, o := range d.Options {
		if l := strings.ToLower(o.Label); l == text || l == trimmed {
			return i, true
		}
	}
	if n, err := strconv.Atoi(strings.TrimRight(text, ".。)） ")); err == nil && n >= 1 && n <= len(d.Options) {
		return n - 1, true
	}
	return 0, false
}

// chatText is the card as a chat reads it: the question and what is known,
// each option with what it costs, and the recommendation.
func (d Decision) chatText() string {
	var b strings.Builder
	b.WriteString(d.Question)
	if a := d.about(); a != "" {
		b.WriteString("\nAbout " + a)
	}
	if ctx := strings.TrimSpace(d.Context); ctx != "" {
		b.WriteString("\n" + ctx)
	}
	for i, o := range d.Options {
		fmt.Fprintf(&b, "\n%d. %s", i+1, o.Label)
		if o.Consequence != "" {
			b.WriteString(" — " + o.Consequence)
		}
		if i == d.Recommended {
			b.WriteString(" (recommended)")
		}
	}
	if reason := strings.TrimSpace(d.Reason); reason != "" {
		b.WriteString("\nWhy " + fmt.Sprint(d.Recommended+1) + ": " + reason)
	}
	return b.String()
}

// replyHint tells a chat without buttons how to answer.
func (d Decision) replyHint() string {
	nums := make([]string, len(d.Options))
	for i := range d.Options {
		nums[i] = strconv.Itoa(i + 1)
	}
	return "Reply " + strings.Join(nums[:len(nums)-1], ", ") + " or " + nums[len(nums)-1] + " to choose."
}

// describe renders a decision for the coordinator's own history.
func (d Decision) describe() string {
	var parts []string
	for i, o := range d.Options {
		mark := ""
		switch {
		case d.Chosen != nil && *d.Chosen == i:
			mark = ", chosen"
		case d.Chosen == nil && d.Recommended == i:
			mark = ", recommended"
		}
		parts = append(parts, fmt.Sprintf("%d. %s%s", i+1, o.Label, mark))
	}
	return strings.Join(parts, "; ")
}

// text renders the question and its options as a message.
func (d Decision) text() string {
	var b strings.Builder
	b.WriteString(d.Question)
	if a := d.about(); a != "" {
		b.WriteString("\nAbout " + a)
	}
	for i, o := range d.Options {
		fmt.Fprintf(&b, "\n%d. %s", i+1, o.Label)
		if o.Consequence != "" {
			b.WriteString(" — " + o.Consequence)
		}
		if i == d.Recommended {
			b.WriteString(" (recommended)")
		}
	}
	return b.String()
}
