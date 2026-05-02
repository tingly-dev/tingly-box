// Package tingly implements a full-featured imbot platform that doubles as
// an end-to-end test harness for any imbot-based bot.
//
// The platform is built around a Transport seam: the Bot impl never knows
// whether it's talking to an in-process channel (used by tests) or a remote
// tingly server (a future WebSocket transport — not implemented yet).
package tingly

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/core"
)

// EventKind identifies the type of outbound bot action captured by the
// transport.
type EventKind string

const (
	EventSend   EventKind = "send"
	EventEdit   EventKind = "edit"
	EventDelete EventKind = "delete"
	EventReact  EventKind = "react"
	EventMedia  EventKind = "media"
)

// Event records a single outbound action issued by the bot.
//
// The fields populated depend on Kind. Tests should treat zero values as
// "not applicable for this kind."
type Event struct {
	Kind      EventKind
	ChatID    string
	MessageID string
	Text      string
	ParseMode core.ParseMode
	Media     []core.MediaAttachment
	Emoji     string
	ReplyTo   string
	Timestamp time.Time

	// Decoded inline keyboard, if the outbound options carried one.
	Keyboard *Keyboard

	// Raw send options (Send/Media kinds only). Useful when a test wants to
	// assert against fields not lifted into the struct.
	Raw *core.SendMessageOptions
}

// Keyboard is a decoded inline keyboard. It accepts both
// imbot.InlineKeyboardMarkup and the Telegram models.InlineKeyboardMarkup —
// see DecodeReplyMarkup in adapter.go.
type Keyboard struct {
	Rows [][]Button
}

// Button is a decoded inline keyboard button.
type Button struct {
	Label        string
	CallbackData string
	URL          string
}

// MessageHandler receives an inbound message injected via the transport.
type MessageHandler func(core.Message)

// Transport is the seam between tingly.Bot and whatever delivers messages.
//
// The current implementation is InProcessTransport, used by the testenv
// harness. A future WebSocketTransport will satisfy this same interface so
// the Bot impl can be reused unchanged.
type Transport interface {
	// Send records an outbound text/media message and returns the synthetic
	// message id.
	Send(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error)

	// SendMedia records an outbound media-only send.
	SendMedia(ctx context.Context, target string, media []core.MediaAttachment) (*core.SendResult, error)

	// Edit records a message edit.
	Edit(ctx context.Context, messageID, text string) error

	// Delete records a message delete.
	Delete(ctx context.Context, messageID string) error

	// React records a reaction.
	React(ctx context.Context, messageID, emoji string) error

	// Subscribe registers the handler that will receive injected inbound
	// messages. Only one subscriber is supported (the Bot itself).
	Subscribe(handler MessageHandler)

	// Close releases any transport resources.
	Close() error
}

// InProcessTransport implements Transport entirely in memory. It records
// every outbound op into a thread-safe slice; tests inject inbound messages
// via Inject (text) and InjectCallback / InjectMessage (lower-level).
type InProcessTransport struct {
	mu       sync.RWMutex
	events   []Event
	listener MessageHandler

	// Per-chat broadcast: tests typically wait on a single chat at a time,
	// so we fan out events into per-chat channels in addition to the slice.
	chans map[string]chan Event

	// inboundChat maps inbound message id → chat id so that reactions /
	// edits / deletes targeting an inbound message can be attributed to a
	// chat in the per-chat event stream.
	inboundChat map[string]string

	closed atomic.Bool
	idSeq  atomic.Int64
}

// NewInProcessTransport builds a fresh transport with no subscriber.
func NewInProcessTransport() *InProcessTransport {
	return &InProcessTransport{
		chans:       make(map[string]chan Event),
		inboundChat: make(map[string]string),
	}
}

// nextMessageID mints a unique synthetic message id.
func (t *InProcessTransport) nextMessageID(prefix string) string {
	n := t.idSeq.Add(1)
	return prefix + "-" + formatInt(n)
}

func formatInt(n int64) string {
	// Avoid pulling in strconv just for this hot path.
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func (t *InProcessTransport) record(e Event) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed.Load() {
		return
	}
	t.events = append(t.events, e)
	if ch := t.chans[e.ChatID]; ch != nil {
		// Non-blocking send so a slow consumer never deadlocks the bot.
		// Holding mu for the send is fine: consumers don't acquire mu.
		select {
		case ch <- e:
		default:
		}
	}
}

// Send implements Transport.
func (t *InProcessTransport) Send(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	if t.closed.Load() {
		return nil, core.NewBotError(core.ErrConnectionFailed, "tingly transport closed", false)
	}
	id := t.nextMessageID("ty")
	kind := EventSend
	var media []core.MediaAttachment
	var text string
	var parseMode core.ParseMode
	var replyTo string
	var kb *Keyboard
	if opts != nil {
		text = opts.Text
		parseMode = opts.ParseMode
		replyTo = opts.ReplyTo
		media = opts.Media
		if len(media) > 0 && text == "" {
			kind = EventMedia
		}
		kb = decodeReplyMarkup(opts.Metadata)
	}
	t.record(Event{
		Kind:      kind,
		ChatID:    target,
		MessageID: id,
		Text:      text,
		ParseMode: parseMode,
		Media:     media,
		ReplyTo:   replyTo,
		Keyboard:  kb,
		Raw:       opts,
	})
	return &core.SendResult{
		MessageID: id,
		Timestamp: time.Now().Unix(),
	}, nil
}

// SendMedia implements Transport.
func (t *InProcessTransport) SendMedia(ctx context.Context, target string, media []core.MediaAttachment) (*core.SendResult, error) {
	return t.Send(ctx, target, &core.SendMessageOptions{Media: media})
}

// Edit implements Transport.
func (t *InProcessTransport) Edit(ctx context.Context, messageID, text string) error {
	if t.closed.Load() {
		return core.NewBotError(core.ErrConnectionFailed, "tingly transport closed", false)
	}
	chatID := t.chatIDForMessage(messageID)
	t.record(Event{
		Kind:      EventEdit,
		ChatID:    chatID,
		MessageID: messageID,
		Text:      text,
	})
	return nil
}

// Delete implements Transport.
func (t *InProcessTransport) Delete(ctx context.Context, messageID string) error {
	if t.closed.Load() {
		return core.NewBotError(core.ErrConnectionFailed, "tingly transport closed", false)
	}
	chatID := t.chatIDForMessage(messageID)
	t.record(Event{
		Kind:      EventDelete,
		ChatID:    chatID,
		MessageID: messageID,
	})
	return nil
}

// React implements Transport.
func (t *InProcessTransport) React(ctx context.Context, messageID, emoji string) error {
	if t.closed.Load() {
		return core.NewBotError(core.ErrConnectionFailed, "tingly transport closed", false)
	}
	chatID := t.chatIDForMessage(messageID)
	t.record(Event{
		Kind:      EventReact,
		ChatID:    chatID,
		MessageID: messageID,
		Emoji:     emoji,
	})
	return nil
}

// chatIDForMessage finds the chat id that owns a message id. It first
// checks the inbound-message map (for reactions on user messages), then
// scans recorded outbound events (for edits/deletes/reactions on bot
// messages).
func (t *InProcessTransport) chatIDForMessage(messageID string) string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if id, ok := t.inboundChat[messageID]; ok {
		return id
	}
	for i := len(t.events) - 1; i >= 0; i-- {
		if t.events[i].MessageID == messageID && t.events[i].Kind != EventDelete {
			return t.events[i].ChatID
		}
	}
	return ""
}

// Subscribe registers the inbound message handler. The bot calls this from
// Connect().
func (t *InProcessTransport) Subscribe(handler MessageHandler) {
	t.mu.Lock()
	t.listener = handler
	t.mu.Unlock()
}

// Close marks the transport closed and closes all per-chat channels.
func (t *InProcessTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.closed.CompareAndSwap(false, true) {
		return nil
	}
	for _, ch := range t.chans {
		close(ch)
	}
	t.chans = nil
	t.listener = nil
	return nil
}

// Inject delivers an inbound message to the subscribed handler. Tests call
// this directly when constructing arbitrary Message values; the testenv
// helpers wrap it with chat-aware sugar.
func (t *InProcessTransport) Inject(msg core.Message) {
	t.mu.Lock()
	listener := t.listener
	if msg.ID != "" && msg.Recipient.ID != "" {
		t.inboundChat[msg.ID] = msg.Recipient.ID
	}
	t.mu.Unlock()
	if listener != nil {
		listener(msg)
	}
}

// Events returns a snapshot of all recorded events. Useful for after-the-
// fact assertions.
func (t *InProcessTransport) Events() []Event {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]Event, len(t.events))
	copy(out, t.events)
	return out
}

// EventsForChat returns recorded events filtered to a single chat id.
func (t *InProcessTransport) EventsForChat(chatID string) []Event {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var out []Event
	for _, e := range t.events {
		if e.ChatID == chatID {
			out = append(out, e)
		}
	}
	return out
}

// ClearEvents drops the recorded event log. Per-chat channels are kept.
func (t *InProcessTransport) ClearEvents() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = nil
}

// Channel returns (creating if needed) a per-chat event channel. Each
// outbound op for the chat is delivered to this channel, and tests use it
// to wait for specific event kinds.
//
// The channel is buffered. If the buffer fills (slow test), events are
// still recorded to the slice and remain available via EventsForChat.
func (t *InProcessTransport) Channel(chatID string) <-chan Event {
	t.mu.Lock()
	defer t.mu.Unlock()
	if ch, ok := t.chans[chatID]; ok {
		return ch
	}
	ch := make(chan Event, 256)
	t.chans[chatID] = ch
	return ch
}

// transportRegistry lets the platform creator find a pre-registered
// transport by bot UUID, so tests can wire the bot to their harness without
// touching the Manager.
var (
	registryMu sync.RWMutex
	registry   = map[string]Transport{}
)

// Register associates a transport with a bot UUID. testenv calls this
// before adding the bot to imbot.Manager.
func Register(uuid string, t Transport) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[uuid] = t
}

// Unregister removes the association.
func Unregister(uuid string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	delete(registry, uuid)
}

// lookup returns the transport associated with a bot UUID, or nil.
func lookup(uuid string) Transport {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registry[uuid]
}
