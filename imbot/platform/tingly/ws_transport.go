package tingly

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/wire"
)

// WSTransportConfig configures a WSTransport.
type WSTransportConfig struct {
	// URL is the tingly server's WS endpoint, e.g. ws://host:port/tingly/ws.
	URL string
	// BotID identifies this bot to the server. Required.
	BotID string
	// Token is an optional auth token forwarded in the Hello frame.
	Token string
	// HandshakeTimeout caps the WebSocket dial.
	HandshakeTimeout time.Duration
	// ReconnectInitial is the first reconnect backoff. Defaults to 1s.
	ReconnectInitial time.Duration
	// ReconnectMax caps reconnect backoff. Defaults to 30s.
	ReconnectMax time.Duration
}

// WSTransport implements Transport by talking to a remote tingly server over
// WebSocket. It is the production sibling of InProcessTransport.
//
// The transport keeps a single connection: on disconnect it reconnects with
// exponential backoff and re-sends the Hello frame. Pending Send calls block
// until the connection is up (bounded by the caller's context).
type WSTransport struct {
	cfg WSTransportConfig

	mu       sync.Mutex
	conn     *websocket.Conn
	listener MessageHandler
	ready    chan struct{} // closed each time a connection is up

	writeMu sync.Mutex // serializes WriteJSON

	pending map[string]chan ackResult // request id → ack waiter

	closed atomic.Bool
	idSeq  atomic.Int64

	// stop signals the read loop / reconnect loop to exit.
	stop chan struct{}
	done chan struct{}
}

type ackResult struct {
	messageID string
	timestamp int64
	err       error
}

// NewWSTransport constructs a WSTransport. Call Start to dial; SendMessage
// etc. block until the first connection is established.
func NewWSTransport(cfg WSTransportConfig) (*WSTransport, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("tingly: WSTransport URL is required")
	}
	if cfg.BotID == "" {
		return nil, fmt.Errorf("tingly: WSTransport BotID is required")
	}
	if cfg.HandshakeTimeout == 0 {
		cfg.HandshakeTimeout = 10 * time.Second
	}
	if cfg.ReconnectInitial == 0 {
		cfg.ReconnectInitial = time.Second
	}
	if cfg.ReconnectMax == 0 {
		cfg.ReconnectMax = 30 * time.Second
	}
	return &WSTransport{
		cfg:     cfg,
		ready:   make(chan struct{}),
		pending: make(map[string]chan ackResult),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}, nil
}

// Start launches the connect/read/reconnect loop in the background. It
// returns immediately; the first successful Hello unblocks waiters.
func (t *WSTransport) Start(ctx context.Context) {
	go t.run(ctx)
}

func (t *WSTransport) run(ctx context.Context) {
	defer close(t.done)
	backoff := t.cfg.ReconnectInitial
	for {
		select {
		case <-t.stop:
			return
		case <-ctx.Done():
			return
		default:
		}

		err := t.connectAndServe(ctx)
		if t.closed.Load() {
			return
		}
		if err == nil {
			backoff = t.cfg.ReconnectInitial
			continue
		}

		// Backoff and retry.
		timer := time.NewTimer(backoff)
		select {
		case <-t.stop:
			timer.Stop()
			return
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		backoff *= 2
		if backoff > t.cfg.ReconnectMax {
			backoff = t.cfg.ReconnectMax
		}
	}
}

func (t *WSTransport) connectAndServe(ctx context.Context) error {
	u, err := url.Parse(t.cfg.URL)
	if err != nil {
		return fmt.Errorf("tingly: invalid URL: %w", err)
	}
	dialer := &websocket.Dialer{HandshakeTimeout: t.cfg.HandshakeTimeout}
	conn, _, err := dialer.DialContext(ctx, u.String(), http.Header{})
	if err != nil {
		return fmt.Errorf("tingly: dial: %w", err)
	}

	// Send Hello.
	hello := wire.Hello{
		Version: wire.Version,
		Role:    wire.RoleBot,
		BotID:   t.cfg.BotID,
		Token:   t.cfg.Token,
	}
	helloData, _ := wire.EncodeData(hello)
	if err := conn.WriteJSON(wire.Frame{Kind: wire.KindHello, Bot: t.cfg.BotID, Data: helloData}); err != nil {
		conn.Close()
		return fmt.Errorf("tingly: write hello: %w", err)
	}

	// Read Welcome.
	var welcome wire.Frame
	if err := conn.ReadJSON(&welcome); err != nil {
		conn.Close()
		return fmt.Errorf("tingly: read welcome: %w", err)
	}
	if welcome.Kind != wire.KindWelcome {
		conn.Close()
		return fmt.Errorf("tingly: expected welcome, got %s", welcome.Kind)
	}

	// Mark ready and serve. Closing the current ready channel wakes any
	// waiters in waitReady; they re-check t.conn under the mutex.
	t.mu.Lock()
	t.conn = conn
	prev := t.ready
	t.mu.Unlock()
	close(prev)

	readErr := t.readLoop(conn)

	// Tear down: fail any pending acks and reset readiness.
	t.mu.Lock()
	if t.conn == conn {
		t.conn = nil
	}
	t.ready = make(chan struct{})
	for id, ch := range t.pending {
		select {
		case ch <- ackResult{err: fmt.Errorf("tingly: connection lost")}:
		default:
		}
		delete(t.pending, id)
	}
	t.mu.Unlock()
	conn.Close()
	return readErr
}

func (t *WSTransport) readLoop(conn *websocket.Conn) error {
	for {
		var f wire.Frame
		if err := conn.ReadJSON(&f); err != nil {
			return err
		}
		t.dispatch(f)
	}
}

func (t *WSTransport) dispatch(f wire.Frame) {
	switch f.Kind {
	case wire.KindAck:
		var ack wire.Ack
		_ = wire.DecodeData(f.Data, &ack)
		t.mu.Lock()
		ch, ok := t.pending[f.ID]
		if ok {
			delete(t.pending, f.ID)
		}
		t.mu.Unlock()
		if ok {
			select {
			case ch <- ackResult{messageID: ack.MessageID, timestamp: ack.Timestamp}:
			default:
			}
		}
	case wire.KindError:
		var ep wire.ErrorPayload
		_ = wire.DecodeData(f.Data, &ep)
		t.mu.Lock()
		ch, ok := t.pending[f.ID]
		if ok {
			delete(t.pending, f.ID)
		}
		listener := t.listener
		t.mu.Unlock()
		if ok {
			select {
			case ch <- ackResult{err: fmt.Errorf("tingly: %s: %s", ep.Code, ep.Message)}:
			default:
			}
		} else if listener != nil {
			// orphan error: surface as a system message (best effort).
		}
	case wire.KindChatSend:
		var cs wire.ChatSend
		if err := wire.DecodeData(f.Data, &cs); err != nil {
			return
		}
		msg := chatSendToMessage(f, cs)
		t.deliver(msg)
	case wire.KindChatCallback:
		var cb wire.ChatCallback
		if err := wire.DecodeData(f.Data, &cb); err != nil {
			return
		}
		msg := chatCallbackToMessage(f, cb)
		t.deliver(msg)
	}
}

func (t *WSTransport) deliver(msg core.Message) {
	t.mu.Lock()
	listener := t.listener
	t.mu.Unlock()
	if listener != nil {
		listener(msg)
	}
}

func (t *WSTransport) waitReady(ctx context.Context) (*websocket.Conn, error) {
	for {
		t.mu.Lock()
		conn := t.conn
		ready := t.ready
		t.mu.Unlock()
		if conn != nil {
			return conn, nil
		}
		select {
		case <-ready:
			// loop and recheck
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-t.stop:
			return nil, fmt.Errorf("tingly: transport closed")
		}
	}
}

func (t *WSTransport) nextID() string {
	return fmt.Sprintf("r%d", t.idSeq.Add(1))
}

func (t *WSTransport) sendFrame(ctx context.Context, f wire.Frame) (wire.Ack, error) {
	if t.closed.Load() {
		return wire.Ack{}, core.NewBotError(core.ErrConnectionFailed, "tingly transport closed", false)
	}
	conn, err := t.waitReady(ctx)
	if err != nil {
		return wire.Ack{}, err
	}
	if f.ID == "" {
		f.ID = t.nextID()
	}
	if f.Bot == "" {
		f.Bot = t.cfg.BotID
	}
	ch := make(chan ackResult, 1)
	t.mu.Lock()
	t.pending[f.ID] = ch
	t.mu.Unlock()

	t.writeMu.Lock()
	err = conn.WriteJSON(f)
	t.writeMu.Unlock()
	if err != nil {
		t.mu.Lock()
		delete(t.pending, f.ID)
		t.mu.Unlock()
		return wire.Ack{}, fmt.Errorf("tingly: write: %w", err)
	}

	select {
	case res := <-ch:
		if res.err != nil {
			return wire.Ack{}, res.err
		}
		return wire.Ack{MessageID: res.messageID, Timestamp: res.timestamp}, nil
	case <-ctx.Done():
		t.mu.Lock()
		delete(t.pending, f.ID)
		t.mu.Unlock()
		return wire.Ack{}, ctx.Err()
	}
}

// Send implements Transport.
func (t *WSTransport) Send(ctx context.Context, target string, opts *core.SendMessageOptions) (*core.SendResult, error) {
	bs := wire.BotSend{}
	if opts != nil {
		bs.Text = opts.Text
		bs.Media = opts.Media
		bs.ParseMode = opts.ParseMode
		bs.ReplyTo = opts.ReplyTo
		bs.Metadata = sanitizeMetadata(opts.Metadata)
	}
	data, err := wire.EncodeData(bs)
	if err != nil {
		return nil, err
	}
	ack, err := t.sendFrame(ctx, wire.Frame{Kind: wire.KindBotSend, Chat: target, Data: data})
	if err != nil {
		return nil, err
	}
	return &core.SendResult{MessageID: ack.MessageID, Timestamp: ack.Timestamp}, nil
}

// SendMedia implements Transport.
func (t *WSTransport) SendMedia(ctx context.Context, target string, media []core.MediaAttachment) (*core.SendResult, error) {
	return t.Send(ctx, target, &core.SendMessageOptions{Media: media})
}

// Edit implements Transport.
func (t *WSTransport) Edit(ctx context.Context, messageID, text string) error {
	data, _ := wire.EncodeData(wire.BotEdit{MessageID: messageID, Text: text})
	_, err := t.sendFrame(ctx, wire.Frame{Kind: wire.KindBotEdit, Data: data})
	return err
}

// Delete implements Transport.
func (t *WSTransport) Delete(ctx context.Context, messageID string) error {
	data, _ := wire.EncodeData(wire.BotDelete{MessageID: messageID})
	_, err := t.sendFrame(ctx, wire.Frame{Kind: wire.KindBotDelete, Data: data})
	return err
}

// React implements Transport.
func (t *WSTransport) React(ctx context.Context, messageID, emoji string) error {
	data, _ := wire.EncodeData(wire.BotReact{MessageID: messageID, Emoji: emoji})
	_, err := t.sendFrame(ctx, wire.Frame{Kind: wire.KindBotReact, Data: data})
	return err
}

// Subscribe implements Transport.
func (t *WSTransport) Subscribe(handler MessageHandler) {
	t.mu.Lock()
	t.listener = handler
	t.mu.Unlock()
}

// Close implements Transport. Idempotent.
func (t *WSTransport) Close() error {
	if !t.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(t.stop)
	t.mu.Lock()
	conn := t.conn
	t.conn = nil
	t.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
	return nil
}

// chatSendToMessage rebuilds a core.Message from a chat.send frame.
func chatSendToMessage(f wire.Frame, cs wire.ChatSend) core.Message {
	msg := core.Message{
		ID:        f.ID,
		Platform:  core.PlatformTingly,
		Timestamp: time.Now().Unix(),
		Sender:    cs.Sender,
		Recipient: core.Recipient{
			ID:   f.Chat,
			Type: recipientTypeFromChat(cs.ChatType),
		},
		ChatType: cs.ChatType,
		Metadata: copyMetadata(cs.Metadata),
	}
	if len(cs.Media) > 0 {
		msg.Content = core.NewMediaContent(cs.Media, cs.Text)
	} else {
		msg.Content = core.NewTextContent(cs.Text)
	}
	return msg
}

// chatCallbackToMessage rebuilds a callback core.Message from a frame.
func chatCallbackToMessage(f wire.Frame, cb wire.ChatCallback) core.Message {
	return core.Message{
		ID:        f.ID,
		Platform:  core.PlatformTingly,
		Timestamp: time.Now().Unix(),
		Sender:    cb.Sender,
		Recipient: core.Recipient{
			ID:   f.Chat,
			Type: recipientTypeFromChat(cb.ChatType),
		},
		Content:  core.NewTextContent(""),
		ChatType: cb.ChatType,
		Metadata: map[string]interface{}{
			"is_callback":       true,
			"callback_data":     cb.CallbackData,
			"callback_query_id": f.ID,
		},
	}
}

// sanitizeMetadata copies opts.Metadata while dropping non-JSON-encodable
// values (e.g. *itx.InlineKeyboardMarkup retained by reference). Keyboard
// payloads are reduced to their JSON shape via Marshal/Unmarshal so the wire
// stays compact.
func sanitizeMetadata(meta map[string]interface{}) map[string]any {
	if meta == nil {
		return nil
	}
	out := make(map[string]any, len(meta))
	for k, v := range meta {
		if v == nil {
			continue
		}
		// Try a roundtrip; drop if the value can't be JSON-encoded.
		b, err := json.Marshal(v)
		if err != nil {
			continue
		}
		var any2 any
		if err := json.Unmarshal(b, &any2); err != nil {
			continue
		}
		out[k] = any2
	}
	return out
}

func copyMetadata(in map[string]any) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
