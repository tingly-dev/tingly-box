// Package chatclient is the client SDK for chat-side participants on the
// tingly platform. A chat client represents a user (or system) talking to a
// bot; it sends inbound messages and receives outbound bot frames in
// response.
//
// This is intentionally a low-ceremony client: it owns one WebSocket, no
// reconnect (a chat session is short-lived in typical use), and exposes
// callbacks for incoming bot frames.
package chatclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/wire"
)

// Config configures a chat client.
type Config struct {
	// URL is the tingly server's WS endpoint, e.g. ws://host:port/tingly/ws.
	URL string
	// BotID is the bot the chat is talking to. Required.
	BotID string
	// ChatID identifies this chat / conversation. Required.
	ChatID string
	// Sender describes the chat participant. Required.
	Sender core.Sender
	// Token is forwarded in the Hello frame if non-empty.
	Token string
	// HistoryLimit asks for up to N recent messages on connect.
	HistoryLimit int
	// HandshakeTimeout caps the dial.
	HandshakeTimeout time.Duration
}

// BotEvent is a forwarded frame from the bot received by the chat. The
// concrete payload depends on Kind; the most common is a text message
// (KindBotSend with non-empty Text).
type BotEvent struct {
	Kind      wire.Kind
	MessageID string
	Text      string
	Media     []core.MediaAttachment
	ParseMode core.ParseMode
	ReplyTo   string
	Emoji     string         // for KindBotReact
	Metadata  map[string]any // for KindBotSend
}

// Client is a connected chat-side WebSocket client.
type Client struct {
	cfg Config

	mu      sync.Mutex
	ws      *websocket.Conn
	pending map[string]chan ackResult
	handler func(BotEvent)

	writeMu sync.Mutex

	idSeq  atomic.Int64
	closed atomic.Bool

	// History is the snapshot returned at connect time. Populated by Connect.
	History []wire.HistoryEntry
}

type ackResult struct {
	messageID string
	timestamp int64
	err       error
}

// New constructs a Client. Call Connect to establish the WebSocket.
func New(cfg Config) (*Client, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("chatclient: URL is required")
	}
	if cfg.BotID == "" || cfg.ChatID == "" {
		return nil, fmt.Errorf("chatclient: BotID and ChatID are required")
	}
	if cfg.HandshakeTimeout == 0 {
		cfg.HandshakeTimeout = 10 * time.Second
	}
	return &Client{cfg: cfg, pending: make(map[string]chan ackResult)}, nil
}

// OnBotEvent registers the handler invoked for every forwarded bot frame.
// Setting it to nil drops events.
func (c *Client) OnBotEvent(h func(BotEvent)) {
	c.mu.Lock()
	c.handler = h
	c.mu.Unlock()
}

// Connect dials the server, performs the Hello/Welcome handshake, and
// starts the read loop. The returned History is the recent-message snapshot
// from the server (only populated when Config.HistoryLimit > 0).
func (c *Client) Connect(ctx context.Context) error {
	dialer := &websocket.Dialer{HandshakeTimeout: c.cfg.HandshakeTimeout}
	ws, _, err := dialer.DialContext(ctx, c.cfg.URL, http.Header{})
	if err != nil {
		return fmt.Errorf("chatclient: dial: %w", err)
	}
	hello := wire.Hello{
		Version:      wire.Version,
		Role:         wire.RoleChat,
		BotID:        c.cfg.BotID,
		ChatID:       c.cfg.ChatID,
		Token:        c.cfg.Token,
		Sender:       &c.cfg.Sender,
		HistoryLimit: c.cfg.HistoryLimit,
	}
	hd, _ := wire.EncodeData(hello)
	if err := ws.WriteJSON(wire.Frame{Kind: wire.KindHello, Bot: c.cfg.BotID, Chat: c.cfg.ChatID, Data: hd}); err != nil {
		ws.Close()
		return fmt.Errorf("chatclient: write hello: %w", err)
	}
	var welcome wire.Frame
	if err := ws.ReadJSON(&welcome); err != nil {
		ws.Close()
		return fmt.Errorf("chatclient: read welcome: %w", err)
	}
	if welcome.Kind == wire.KindError {
		var ep wire.ErrorPayload
		_ = wire.DecodeData(welcome.Data, &ep)
		ws.Close()
		return fmt.Errorf("chatclient: server rejected hello: %s: %s", ep.Code, ep.Message)
	}
	if welcome.Kind != wire.KindWelcome {
		ws.Close()
		return fmt.Errorf("chatclient: expected welcome, got %s", welcome.Kind)
	}
	var w wire.Welcome
	if err := wire.DecodeData(welcome.Data, &w); err == nil {
		c.History = w.History
	}
	c.mu.Lock()
	c.ws = ws
	c.mu.Unlock()
	go c.readLoop(ws)
	return nil
}

// Close closes the WebSocket. Idempotent.
func (c *Client) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	c.mu.Lock()
	ws := c.ws
	c.ws = nil
	for id, ch := range c.pending {
		select {
		case ch <- ackResult{err: fmt.Errorf("chatclient: closed")}:
		default:
		}
		delete(c.pending, id)
	}
	c.mu.Unlock()
	if ws != nil {
		_ = ws.Close()
	}
	return nil
}

func (c *Client) readLoop(ws *websocket.Conn) {
	for {
		var f wire.Frame
		if err := ws.ReadJSON(&f); err != nil {
			return
		}
		c.dispatch(f)
	}
}

func (c *Client) dispatch(f wire.Frame) {
	switch f.Kind {
	case wire.KindAck:
		var ack wire.Ack
		_ = wire.DecodeData(f.Data, &ack)
		c.completeAck(f.ID, ackResult{messageID: ack.MessageID, timestamp: ack.Timestamp})
	case wire.KindError:
		var ep wire.ErrorPayload
		_ = wire.DecodeData(f.Data, &ep)
		c.completeAck(f.ID, ackResult{err: fmt.Errorf("%s: %s", ep.Code, ep.Message)})
	case wire.KindBotSend:
		var bs wire.BotSend
		_ = wire.DecodeData(f.Data, &bs)
		c.fire(BotEvent{
			Kind:      f.Kind,
			MessageID: f.ID,
			Text:      bs.Text,
			Media:     bs.Media,
			ParseMode: bs.ParseMode,
			ReplyTo:   bs.ReplyTo,
			Metadata:  bs.Metadata,
		})
	case wire.KindBotEdit:
		var be wire.BotEdit
		_ = wire.DecodeData(f.Data, &be)
		c.fire(BotEvent{Kind: f.Kind, MessageID: be.MessageID, Text: be.Text})
	case wire.KindBotDelete:
		var bd wire.BotDelete
		_ = wire.DecodeData(f.Data, &bd)
		c.fire(BotEvent{Kind: f.Kind, MessageID: bd.MessageID})
	case wire.KindBotReact:
		var br wire.BotReact
		_ = wire.DecodeData(f.Data, &br)
		c.fire(BotEvent{Kind: f.Kind, MessageID: br.MessageID, Emoji: br.Emoji})
	}
}

func (c *Client) fire(ev BotEvent) {
	c.mu.Lock()
	h := c.handler
	c.mu.Unlock()
	if h != nil {
		h(ev)
	}
}

func (c *Client) completeAck(id string, res ackResult) {
	c.mu.Lock()
	ch, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	c.mu.Unlock()
	if ok {
		select {
		case ch <- res:
		default:
		}
	}
}

func (c *Client) nextID() string {
	return fmt.Sprintf("c%d", c.idSeq.Add(1))
}

// SendText sends a text message from the chat to the bot. It blocks until
// the server acknowledges it (returning the assigned message id) or ctx is
// canceled.
func (c *Client) SendText(ctx context.Context, text string, chatType core.ChatType) (string, error) {
	cs := wire.ChatSend{Text: text, ChatType: chatType, Sender: c.cfg.Sender}
	return c.send(ctx, wire.KindChatSend, cs)
}

// SendCallback sends a button-click style callback from the chat.
func (c *Client) SendCallback(ctx context.Context, data string, chatType core.ChatType) (string, error) {
	cb := wire.ChatCallback{Sender: c.cfg.Sender, CallbackData: data, ChatType: chatType}
	return c.send(ctx, wire.KindChatCallback, cb)
}

func (c *Client) send(ctx context.Context, kind wire.Kind, payload any) (string, error) {
	if c.closed.Load() {
		return "", fmt.Errorf("chatclient: closed")
	}
	c.mu.Lock()
	ws := c.ws
	c.mu.Unlock()
	if ws == nil {
		return "", fmt.Errorf("chatclient: not connected")
	}
	id := c.nextID()
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	ch := make(chan ackResult, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	c.writeMu.Lock()
	err = ws.WriteJSON(wire.Frame{
		Kind: kind,
		ID:   id,
		Bot:  c.cfg.BotID,
		Chat: c.cfg.ChatID,
		Data: data,
	})
	c.writeMu.Unlock()
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return "", err
	}
	select {
	case res := <-ch:
		if res.err != nil {
			return "", res.err
		}
		return res.messageID, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return "", ctx.Err()
	}
}
