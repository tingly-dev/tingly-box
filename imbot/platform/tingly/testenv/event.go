package testenv

import (
	"fmt"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly"
)

// OutEvent is the test-facing view of a captured outbound bot action.
// It carries a back-reference to the originating chat so Button.Click()
// can inject a follow-up callback without the test plumbing it manually.
type OutEvent struct {
	Kind      tingly.EventKind
	ChatID    string
	MessageID string
	Text      string
	ParseMode core.ParseMode
	Buttons   [][]Button
	Media     []core.MediaAttachment
	Emoji     string
	ReplyTo   string
	Timestamp time.Time

	Raw *core.SendMessageOptions

	chat *Chat
}

// Button is a clickable inline-keyboard button.
type Button struct {
	Label        string
	CallbackData string
	URL          string

	chat      *Chat
	messageID string
}

// Click simulates the user pressing this button. It injects the callback
// query as the chat's primary user.
func (b *Button) Click() {
	if b.chat == nil {
		return
	}
	b.chat.SendCallback(b.messageID, b.CallbackData)
}

// ClickAs lets a specific (non-primary) user press the button.
func (b *Button) ClickAs(u *User) {
	if b.chat == nil {
		return
	}
	b.chat.SendCallbackAs(u, b.messageID, b.CallbackData)
}

func toOutEvent(chat *Chat, e tingly.Event) OutEvent {
	out := OutEvent{
		Kind:      e.Kind,
		ChatID:    e.ChatID,
		MessageID: e.MessageID,
		Text:      e.Text,
		ParseMode: e.ParseMode,
		Media:     e.Media,
		Emoji:     e.Emoji,
		ReplyTo:   e.ReplyTo,
		Timestamp: e.Timestamp,
		Raw:       e.Raw,
		chat:      chat,
	}
	if e.Keyboard != nil {
		out.Buttons = make([][]Button, 0, len(e.Keyboard.Rows))
		for _, row := range e.Keyboard.Rows {
			rowBtns := make([]Button, 0, len(row))
			for _, b := range row {
				rowBtns = append(rowBtns, Button{
					Label:        b.Label,
					CallbackData: b.CallbackData,
					URL:          b.URL,
					chat:         chat,
					messageID:    e.MessageID,
				})
			}
			out.Buttons = append(out.Buttons, rowBtns)
		}
	}
	return out
}

// AssertContains fails the test if e.Text doesn't contain substr.
func (e *OutEvent) AssertContains(t TestingT, substr string) *OutEvent {
	t.Helper()
	if !strings.Contains(e.Text, substr) {
		t.Fatalf("expected text to contain %q, got %q", substr, e.Text)
	}
	return e
}

// AssertEquals fails the test if e.Text != want.
func (e *OutEvent) AssertEquals(t TestingT, want string) *OutEvent {
	t.Helper()
	if e.Text != want {
		t.Fatalf("expected text %q, got %q", want, e.Text)
	}
	return e
}

// AssertHasButton fails the test if no button labelled exactly `label`
// is present in the event's keyboard.
func (e *OutEvent) AssertHasButton(t TestingT, label string) *OutEvent {
	t.Helper()
	if _, ok := e.ButtonByLabel(label); !ok {
		t.Fatalf("expected button %q in keyboard, got %s", label, formatButtons(e.Buttons))
	}
	return e
}

// AssertParseMode fails if the event's parse mode does not match.
func (e *OutEvent) AssertParseMode(t TestingT, mode core.ParseMode) *OutEvent {
	t.Helper()
	if e.ParseMode != mode {
		t.Fatalf("expected parse mode %q, got %q", mode, e.ParseMode)
	}
	return e
}

// AssertNoKeyboard fails if the event carries an inline keyboard.
func (e *OutEvent) AssertNoKeyboard(t TestingT) *OutEvent {
	t.Helper()
	if len(e.Buttons) > 0 {
		t.Fatalf("expected no keyboard, got %s", formatButtons(e.Buttons))
	}
	return e
}

// AssertReplyTo fails if the event was not a reply to the given message.
func (e *OutEvent) AssertReplyTo(t TestingT, messageID string) *OutEvent {
	t.Helper()
	if e.ReplyTo != messageID {
		t.Fatalf("expected ReplyTo %q, got %q", messageID, e.ReplyTo)
	}
	return e
}

// ButtonByLabel finds a button by exact-match label. Falls back to a
// case-insensitive substring match if no exact hit exists.
func (e *OutEvent) ButtonByLabel(label string) (*Button, bool) {
	for i := range e.Buttons {
		for j := range e.Buttons[i] {
			if e.Buttons[i][j].Label == label {
				return &e.Buttons[i][j], true
			}
		}
	}
	lower := strings.ToLower(label)
	for i := range e.Buttons {
		for j := range e.Buttons[i] {
			if strings.Contains(strings.ToLower(e.Buttons[i][j].Label), lower) {
				return &e.Buttons[i][j], true
			}
		}
	}
	return nil, false
}

func formatButtons(rows [][]Button) string {
	if len(rows) == 0 {
		return "(no buttons)"
	}
	parts := make([]string, 0, len(rows))
	for _, row := range rows {
		labels := make([]string, 0, len(row))
		for _, b := range row {
			labels = append(labels, b.Label)
		}
		parts = append(parts, "["+strings.Join(labels, ", ")+"]")
	}
	return strings.Join(parts, " / ")
}

func summarize(events []OutEvent) string {
	if len(events) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(events))
	for _, e := range events {
		text := e.Text
		if len(text) > 40 {
			text = text[:40] + "..."
		}
		parts = append(parts, fmt.Sprintf("%s(%q)", e.Kind, text))
	}
	return strings.Join(parts, ", ")
}
