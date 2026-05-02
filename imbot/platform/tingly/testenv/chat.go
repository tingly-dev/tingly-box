package testenv

import (
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly"
)

// Chat is a virtual conversation between one or more users and a bot.
// Tests drive inbound traffic via SendText / SendCallback / etc., and
// observe outbound bot actions via the WaitX helpers.
type Chat struct {
	env       *TestEnv
	botUUID   string
	transport *tingly.InProcessTransport

	ChatID   string
	ChatType core.ChatType
	Members  []*User
	primary  *User
	channel  <-chan tingly.Event

	mu  sync.Mutex
	buf []tingly.Event
}

// openChat creates (or returns) a chat handle. Each unique chatID gets a
// dedicated event channel from the transport.
func (e *TestEnv) openChat(botUUID, chatID string, chatType core.ChatType, members []*User, primary *User) *Chat {
	tr := e.Transport(botUUID)
	if tr == nil {
		e.t.Fatalf("openChat: no transport registered for bot %q", botUUID)
	}
	return &Chat{
		env:       e,
		botUUID:   botUUID,
		transport: tr,
		ChatID:    chatID,
		ChatType:  chatType,
		Members:   members,
		primary:   primary,
		channel:   tr.Channel(chatID),
	}
}

// SendText injects an inbound text message from the chat's primary
// sender. Returns the synthetic message id.
func (c *Chat) SendText(text string) string {
	return c.SendTextAs(c.primary, text)
}

// SendTextAs injects an inbound text message from a specific user.
func (c *Chat) SendTextAs(u *User, text string) string {
	if u == nil {
		c.env.t.Fatalf("SendTextAs: nil user")
	}
	id := c.env.nextID("in")
	msg := tingly.NewIncomingTextMessage(id, c.ChatID, u.Sender(), text, c.ChatType)
	c.transport.Inject(msg)
	return id
}

// SendCallback injects an inline-keyboard callback as if the primary user
// pressed a button on a previously sent bot message.
func (c *Chat) SendCallback(messageID, callbackData string) {
	c.SendCallbackAs(c.primary, messageID, callbackData)
}

// SendCallbackAs is like SendCallback but with an explicit user.
func (c *Chat) SendCallbackAs(u *User, messageID, callbackData string) {
	if u == nil {
		c.env.t.Fatalf("SendCallbackAs: nil user")
	}
	cbID := c.env.nextID("cb")
	msg := tingly.NewIncomingCallback(cbID, c.ChatID, u.Sender(), callbackData, c.ChatType)
	// Tag the originating message id so handlers that want to know which
	// message the press was on can look it up.
	msg.Metadata["message_id"] = messageID
	c.transport.Inject(msg)
}

// SendMedia injects an inbound media message.
func (c *Chat) SendMedia(media []core.MediaAttachment) string {
	id := c.env.nextID("in")
	msg := tingly.NewIncomingTextMessage(id, c.ChatID, c.primary.Sender(), "", c.ChatType)
	msg.Content = core.NewMediaContent(media, "")
	c.transport.Inject(msg)
	return id
}

// WaitText returns the next outbound text-bearing send event in this chat.
// Fails the test on timeout.
func (c *Chat) WaitText(d time.Duration) *OutEvent {
	c.env.t.Helper()
	return c.waitOrFatal(d, "text", func(e tingly.Event) bool {
		return e.Kind == tingly.EventSend && e.Text != ""
	})
}

// WaitAnySend returns the next outbound send-or-media event regardless of
// whether it carries text.
func (c *Chat) WaitAnySend(d time.Duration) *OutEvent {
	c.env.t.Helper()
	return c.waitOrFatal(d, "send", func(e tingly.Event) bool {
		return e.Kind == tingly.EventSend || e.Kind == tingly.EventMedia
	})
}

// WaitEdit waits for an edit applied to the given message id.
func (c *Chat) WaitEdit(messageID string, d time.Duration) *OutEvent {
	c.env.t.Helper()
	return c.waitOrFatal(d, "edit on "+messageID, func(e tingly.Event) bool {
		return e.Kind == tingly.EventEdit && e.MessageID == messageID
	})
}

// WaitAnyEdit waits for the next edit in this chat regardless of which
// message id it targets.
func (c *Chat) WaitAnyEdit(d time.Duration) *OutEvent {
	c.env.t.Helper()
	return c.waitOrFatal(d, "any edit", func(e tingly.Event) bool {
		return e.Kind == tingly.EventEdit
	})
}

// WaitReaction waits for a reaction applied to the given message id.
func (c *Chat) WaitReaction(messageID string, d time.Duration) *OutEvent {
	c.env.t.Helper()
	return c.waitOrFatal(d, "react on "+messageID, func(e tingly.Event) bool {
		return e.Kind == tingly.EventReact && e.MessageID == messageID
	})
}

// WaitDelete waits for a delete of the given message id.
func (c *Chat) WaitDelete(messageID string, d time.Duration) *OutEvent {
	c.env.t.Helper()
	return c.waitOrFatal(d, "delete on "+messageID, func(e tingly.Event) bool {
		return e.Kind == tingly.EventDelete && e.MessageID == messageID
	})
}

// WaitMedia waits for the next media-bearing send event.
func (c *Chat) WaitMedia(d time.Duration) *OutEvent {
	c.env.t.Helper()
	return c.waitOrFatal(d, "media", func(e tingly.Event) bool {
		return (e.Kind == tingly.EventSend || e.Kind == tingly.EventMedia) && len(e.Media) > 0
	})
}

// ExpectNoEvent fails the test if any matching event arrives within d.
func (c *Chat) ExpectNoEvent(d time.Duration, kinds ...tingly.EventKind) {
	c.env.t.Helper()
	if len(kinds) == 0 {
		kinds = []tingly.EventKind{
			tingly.EventSend, tingly.EventEdit, tingly.EventReact, tingly.EventDelete, tingly.EventMedia,
		}
	}
	match := func(e tingly.Event) bool {
		for _, k := range kinds {
			if e.Kind == k {
				return true
			}
		}
		return false
	}
	if e, ok := c.tryReceive(d, match); ok {
		c.env.t.Fatalf("[chat=%s] expected no event but got %s: %+v", c.ChatID, e.Kind, e)
	}
}

// Drain returns and clears the buffered (out-of-order) events for this
// chat. It does not consume from the live channel.
func (c *Chat) Drain() []OutEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]OutEvent, 0, len(c.buf))
	for _, e := range c.buf {
		out = append(out, toOutEvent(c, e))
	}
	c.buf = nil
	return out
}

// AllEvents returns every recorded event for this chat (including ones
// already consumed by Wait calls). Useful for debugging assertions.
func (c *Chat) AllEvents() []OutEvent {
	events := c.transport.EventsForChat(c.ChatID)
	out := make([]OutEvent, 0, len(events))
	for _, e := range events {
		out = append(out, toOutEvent(c, e))
	}
	return out
}

// waitOrFatal waits for a matching event with a deadline; failure to find
// one is a test fatal. The label is included in the failure message.
func (c *Chat) waitOrFatal(d time.Duration, label string, match func(tingly.Event) bool) *OutEvent {
	if e, ok := c.tryReceive(d, match); ok {
		out := toOutEvent(c, e)
		return &out
	}
	c.env.t.Fatalf("[chat=%s] timed out after %s waiting for %s; events seen: %s",
		c.ChatID, d, label, summarize(c.AllEvents()))
	return nil // unreachable
}

func (c *Chat) tryReceive(d time.Duration, match func(tingly.Event) bool) (tingly.Event, bool) {
	// First scan the out-of-order buffer.
	c.mu.Lock()
	for i, e := range c.buf {
		if match(e) {
			c.buf = append(c.buf[:i], c.buf[i+1:]...)
			c.mu.Unlock()
			return e, true
		}
	}
	c.mu.Unlock()

	deadline := time.Now().Add(d)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return tingly.Event{}, false
		}
		select {
		case e, ok := <-c.channel:
			if !ok {
				return tingly.Event{}, false
			}
			if match(e) {
				return e, true
			}
			c.mu.Lock()
			c.buf = append(c.buf, e)
			c.mu.Unlock()
		case <-time.After(remaining):
			return tingly.Event{}, false
		}
	}
}
