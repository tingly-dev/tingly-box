package testenv

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/platform/tingly"
)

// Matcher describes a single expected outbound event. Zero-valued fields are
// not enforced; only the fields the caller sets are checked.
//
// Kind is required. Use the constants from package tingly (EventSend,
// EventEdit, EventReact, EventDelete, EventMedia).
type Matcher struct {
	Kind tingly.EventKind

	// Optional: require the event target a specific message id (mainly
	// useful for EventEdit / EventReact / EventDelete).
	MessageID string

	// At most one of TextEquals / TextContains / TextRegexp may be set.
	TextEquals   string
	TextContains string
	TextRegexp   *regexp.Regexp

	// HasButton, if non-empty, requires the event's keyboard contains a
	// button with this exact label. HasCallback (also optional) requires
	// the keyboard contains a button whose CallbackData matches.
	HasButton   string
	HasCallback string

	// NoKeyboard, if true, fails when the event carries any inline keyboard.
	NoKeyboard bool

	// Name is shown in failure messages. Falls back to the Matcher's index.
	Name string
}

// String returns a short human description of the matcher.
func (m Matcher) String() string {
	parts := []string{string(m.Kind)}
	if m.Name != "" {
		parts = append([]string{m.Name + ":"}, parts...)
	}
	if m.MessageID != "" {
		parts = append(parts, "msgID="+m.MessageID)
	}
	if m.TextEquals != "" {
		parts = append(parts, fmt.Sprintf("text==%q", m.TextEquals))
	}
	if m.TextContains != "" {
		parts = append(parts, fmt.Sprintf("text~=%q", m.TextContains))
	}
	if m.TextRegexp != nil {
		parts = append(parts, "text=~/"+m.TextRegexp.String()+"/")
	}
	if m.HasButton != "" {
		parts = append(parts, "button="+m.HasButton)
	}
	if m.HasCallback != "" {
		parts = append(parts, "callback="+m.HasCallback)
	}
	if m.NoKeyboard {
		parts = append(parts, "noKeyboard")
	}
	return strings.Join(parts, " ")
}

func (m Matcher) match(e OutEvent) (bool, string) {
	if e.Kind != m.Kind {
		return false, fmt.Sprintf("kind=%q want %q", e.Kind, m.Kind)
	}
	if m.MessageID != "" && e.MessageID != m.MessageID {
		return false, fmt.Sprintf("messageID=%q want %q", e.MessageID, m.MessageID)
	}
	if m.TextEquals != "" && e.Text != m.TextEquals {
		return false, fmt.Sprintf("text=%q want exactly %q", e.Text, m.TextEquals)
	}
	if m.TextContains != "" && !strings.Contains(e.Text, m.TextContains) {
		return false, fmt.Sprintf("text=%q does not contain %q", e.Text, m.TextContains)
	}
	if m.TextRegexp != nil && !m.TextRegexp.MatchString(e.Text) {
		return false, fmt.Sprintf("text=%q does not match /%s/", e.Text, m.TextRegexp.String())
	}
	if m.HasButton != "" {
		if _, ok := e.ButtonByLabel(m.HasButton); !ok {
			return false, fmt.Sprintf("no button %q in %s", m.HasButton, formatButtons(e.Buttons))
		}
	}
	if m.HasCallback != "" {
		if _, ok := findButtonByCallback(&e, m.HasCallback); !ok {
			return false, fmt.Sprintf("no callback %q in %s", m.HasCallback, formatButtons(e.Buttons))
		}
	}
	if m.NoKeyboard && len(e.Buttons) > 0 {
		return false, fmt.Sprintf("expected no keyboard, got %s", formatButtons(e.Buttons))
	}
	return true, ""
}

// Expect consumes the next len(matchers) events from this chat in order.
// Each event must match the corresponding Matcher exactly. Extra/skipped
// events between matchers are not allowed — use ExpectInOrderLoose for that.
//
// The total wait deadline applies across all matchers (not per-matcher).
// On failure the test is fatal'd with the full event history dumped.
func (c *Chat) Expect(d time.Duration, matchers ...Matcher) []OutEvent {
	c.env.t.Helper()
	deadline := time.Now().Add(d)
	got := make([]OutEvent, 0, len(matchers))
	for i, m := range matchers {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			c.failExpect(matchers, got, i, "deadline reached before matcher", "")
			return got
		}
		// Always advance to the very next event — strict mode.
		e, ok := c.tryReceive(remaining, func(tingly.Event) bool { return true })
		if !ok {
			c.failExpect(matchers, got, i, "no more events", "")
			return got
		}
		out := toOutEvent(c, e)
		got = append(got, out)
		if ok, why := m.match(out); !ok {
			c.failExpect(matchers, got, i, "matcher failed", why)
			return got
		}
	}
	return got
}

// ExpectInOrderLoose is like Expect but allows extra events between matchers.
// Each matcher is satisfied by the next event that matches it; non-matching
// events are silently consumed.
//
// Use this when the surrounding events are flaky in number/order, but the
// presence and order of the listed matchers still matters.
func (c *Chat) ExpectInOrderLoose(d time.Duration, matchers ...Matcher) []OutEvent {
	c.env.t.Helper()
	deadline := time.Now().Add(d)
	got := make([]OutEvent, 0, len(matchers))
	for i, m := range matchers {
		var matched OutEvent
		found := false
		for !found {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				c.failExpect(matchers, got, i, "deadline reached before matcher", "")
				return got
			}
			e, ok := c.tryReceive(remaining, func(tingly.Event) bool { return true })
			if !ok {
				c.failExpect(matchers, got, i, "no more events", "")
				return got
			}
			out := toOutEvent(c, e)
			if ok, _ := m.match(out); ok {
				matched = out
				found = true
			}
		}
		got = append(got, matched)
	}
	return got
}

// ExpectUnordered waits until every matcher has been satisfied, in any
// order. Each event can satisfy at most one still-unmet matcher; events
// matching no unmet matcher are silently consumed. Returned events are in
// matcher order, not arrival order.
//
// Use this when the listed events are produced by independent goroutines
// with no ordering guarantee between them (e.g. an ack sent by a callback
// handler racing an executor's failure message).
func (c *Chat) ExpectUnordered(d time.Duration, matchers ...Matcher) []OutEvent {
	c.env.t.Helper()
	deadline := time.Now().Add(d)
	matched := make([]OutEvent, len(matchers))
	met := make([]bool, len(matchers))
	unmet := len(matchers)
	for unmet > 0 {
		firstUnmet := 0
		for firstUnmet < len(met) && met[firstUnmet] {
			firstUnmet++
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			c.failExpect(matchers, compactMatched(matched, met), firstUnmet, "deadline reached before matcher", "")
			return matched
		}
		e, ok := c.tryReceive(remaining, func(tingly.Event) bool { return true })
		if !ok {
			c.failExpect(matchers, compactMatched(matched, met), firstUnmet, "no more events", "")
			return matched
		}
		out := toOutEvent(c, e)
		for i, m := range matchers {
			if met[i] {
				continue
			}
			if ok, _ := m.match(out); ok {
				matched[i] = out
				met[i] = true
				unmet--
				break
			}
		}
	}
	return matched
}

// compactMatched returns the already-matched events for failure reporting.
func compactMatched(matched []OutEvent, met []bool) []OutEvent {
	var out []OutEvent
	for i, ok := range met {
		if ok {
			out = append(out, matched[i])
		}
	}
	return out
}

// ExpectIdle fails the test if any outbound event arrives within d.
// Equivalent to ExpectNoEvent with all kinds.
func (c *Chat) ExpectIdle(d time.Duration) {
	c.env.t.Helper()
	c.ExpectNoEvent(d)
}

// CountText returns how many recorded events for this chat have a Text field
// containing the substring substr. Useful for "exactly once" assertions.
func (c *Chat) CountText(substr string) int {
	n := 0
	for _, e := range c.AllEvents() {
		if strings.Contains(e.Text, substr) {
			n++
		}
	}
	return n
}

// AssertTextOccurrences fails the test unless exactly want events with
// substring substr have been observed in this chat so far.
func (c *Chat) AssertTextOccurrences(substr string, want int) {
	c.env.t.Helper()
	got := c.CountText(substr)
	if got != want {
		c.env.t.Fatalf("[chat=%s] expected %d events containing %q, got %d; events seen: %s",
			c.ChatID, want, substr, got, summarize(c.AllEvents()))
	}
}

func (c *Chat) failExpect(matchers []Matcher, got []OutEvent, idx int, reason, detail string) {
	c.env.t.Helper()
	want := matchers[idx]
	var msg strings.Builder
	fmt.Fprintf(&msg, "[chat=%s] Expect: matcher #%d (%s) — %s",
		c.ChatID, idx, want.String(), reason)
	if detail != "" {
		fmt.Fprintf(&msg, ": %s", detail)
	}
	msg.WriteString("\n  matched so far:\n")
	for i := 0; i < idx; i++ {
		fmt.Fprintf(&msg, "    [%d] %s OK <- %s\n", i, matchers[i].String(), brief(got[i]))
	}
	if idx < len(got) {
		fmt.Fprintf(&msg, "    [%d] %s GOT <- %s\n", idx, want.String(), brief(got[idx]))
	}
	msg.WriteString("  full event history (this chat): " + summarize(c.AllEvents()))
	c.env.t.Fatalf("%s", msg.String())
}

func brief(e OutEvent) string {
	text := e.Text
	if len(text) > 60 {
		text = text[:60] + "..."
	}
	parts := []string{string(e.Kind)}
	if e.MessageID != "" {
		parts = append(parts, "msg="+e.MessageID)
	}
	if text != "" {
		parts = append(parts, fmt.Sprintf("%q", text))
	}
	if len(e.Buttons) > 0 {
		parts = append(parts, "kb="+formatButtons(e.Buttons))
	}
	return strings.Join(parts, " ")
}
