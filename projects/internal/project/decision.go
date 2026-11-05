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
	// Chosen is the picked option's index, nil until the user answers.
	Chosen *int `json:"chosen,omitempty"`
}

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
	return nil
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
	if _, err := c.Post(main.ProjectID, d.Options[index].Label, nil, src); err != nil {
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

// optionOfReply reads a chat's reply as one of the options: its number
// ("2", "2.") or its label, in any case. Anything else is not an answer.
func (d Decision) optionOfReply(text string) (int, bool) {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(strings.TrimRight(text, ".。)）"+" ")); err == nil {
		return n - 1, n >= 1 && n <= len(d.Options)
	}
	trimmed := strings.TrimRight(text, ".。!！ ")
	for i, o := range d.Options {
		if l := strings.ToLower(o.Label); l == text || l == trimmed {
			return i, true
		}
	}
	return 0, false
}

// chatText is the card as a chat reads it: the question and what is known,
// each option with what it costs, and the recommendation.
func (d Decision) chatText() string {
	var b strings.Builder
	b.WriteString(d.Question)
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
