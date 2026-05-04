package testenv

import (
	"fmt"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/imbot"
)

// ApprovalPrompt is a strongly-typed view of an outbound permission prompt
// (Approve / Deny / Always Allow keyboard). Tests use it to click a specific
// option without remembering the button label or callback wire format.
type ApprovalPrompt struct {
	Event     *OutEvent
	RequestID string
}

// Approve clicks the configured "approve" button (default: "✅ Allow"). Fails
// the test if the button is missing.
func (p *ApprovalPrompt) Approve() { p.clickAction("allow") }

// Deny clicks the configured "deny" button. Fails the test if missing.
func (p *ApprovalPrompt) Deny() { p.clickAction("deny") }

// AlwaysAllow clicks the configured "always" button. Fails the test if
// missing.
func (p *ApprovalPrompt) AlwaysAllow() { p.clickAction("always") }

// Text returns the prompt's body text (passthrough convenience).
func (p *ApprovalPrompt) Text() string { return p.Event.Text }

func (p *ApprovalPrompt) clickAction(action string) {
	t := p.Event.chat.env.t
	t.Helper()

	want := imbot.FormatCallbackData("perm", action, p.RequestID)
	if btn, ok := findButtonByCallback(p.Event, want); ok {
		btn.Click()
		return
	}
	t.Fatalf("ApprovalPrompt: no button for action %q (callback=%q); have %s",
		action, want, formatButtons(p.Event.Buttons))
}

// AskPrompt is a strongly-typed view of an outbound AskUserQuestion prompt.
type AskPrompt struct {
	Event     *OutEvent
	RequestID string
}

// SelectOption clicks the (qIdx, optIdx) button. Fails the test if missing.
func (p *AskPrompt) SelectOption(qIdx, optIdx int) {
	t := p.Event.chat.env.t
	t.Helper()
	want := imbot.FormatCallbackData("perm", "option", p.RequestID,
		fmt.Sprintf("%d", qIdx), fmt.Sprintf("%d", optIdx))
	if btn, ok := findButtonByCallback(p.Event, want); ok {
		btn.Click()
		return
	}
	t.Fatalf("AskPrompt: no option button q=%d opt=%d (callback=%q); have %s",
		qIdx, optIdx, want, formatButtons(p.Event.Buttons))
}

// Cancel clicks the AskUserQuestion cancel button.
func (p *AskPrompt) Cancel() {
	t := p.Event.chat.env.t
	t.Helper()
	want := imbot.FormatCallbackData("perm", "deny", p.RequestID)
	if btn, ok := findButtonByCallback(p.Event, want); ok {
		btn.Click()
		return
	}
	if btn, ok := p.Event.ButtonByLabel("❌ Cancel"); ok {
		btn.Click()
		return
	}
	t.Fatalf("AskPrompt: no cancel button (callback=%q); have %s",
		want, formatButtons(p.Event.Buttons))
}

// Text returns the prompt's body text (passthrough convenience).
func (p *AskPrompt) Text() string { return p.Event.Text }

// WaitApprovalPrompt blocks until the next outbound message in this chat is a
// permission keyboard (perm:allow / perm:deny / perm:always callback buttons).
// Fails the test on timeout or if a non-prompt message arrives first.
func (c *Chat) WaitApprovalPrompt(d time.Duration) *ApprovalPrompt {
	t := c.env.t
	t.Helper()
	evt := c.WaitText(d)
	id, ok := extractPermRequestID(evt, "allow", "deny", "always")
	if !ok {
		t.Fatalf("WaitApprovalPrompt: message does not look like a permission prompt: text=%q buttons=%s",
			evt.Text, formatButtons(evt.Buttons))
	}
	return &ApprovalPrompt{Event: evt, RequestID: id}
}

// WaitAskQuestionPrompt blocks until the next outbound message carries an
// AskUserQuestion keyboard (perm:option callback buttons). Fails the test on
// timeout or non-prompt arrival.
func (c *Chat) WaitAskQuestionPrompt(d time.Duration) *AskPrompt {
	t := c.env.t
	t.Helper()
	evt := c.WaitText(d)
	id, ok := extractPermRequestID(evt, "option")
	if !ok {
		t.Fatalf("WaitAskQuestionPrompt: message does not look like an ask prompt: text=%q buttons=%s",
			evt.Text, formatButtons(evt.Buttons))
	}
	return &AskPrompt{Event: evt, RequestID: id}
}

func findButtonByCallback(e *OutEvent, want string) (*Button, bool) {
	for i := range e.Buttons {
		for j := range e.Buttons[i] {
			if e.Buttons[i][j].CallbackData == want {
				return &e.Buttons[i][j], true
			}
		}
	}
	return nil, false
}

func extractPermRequestID(e *OutEvent, actions ...string) (string, bool) {
	want := make(map[string]struct{}, len(actions))
	for _, a := range actions {
		want[a] = struct{}{}
	}
	for i := range e.Buttons {
		for j := range e.Buttons[i] {
			parts := strings.Split(e.Buttons[i][j].CallbackData, ":")
			if len(parts) < 3 || parts[0] != "perm" {
				continue
			}
			if _, ok := want[parts[1]]; !ok {
				continue
			}
			return parts[2], true
		}
	}
	return "", false
}
