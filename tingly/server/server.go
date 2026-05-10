package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/wire"
)

// Config configures a tingly server.
type Config struct {
	// Addr is the TCP listen address (e.g. ":12581"). Required for ListenAndServe.
	Addr string
	// Path is the WS endpoint path. Defaults to "/tingly/ws".
	Path string
	// StorePath is the jsonstore file path for message persistence. Required.
	StorePath string
	// Token, if non-empty, is required from clients in their Hello frame.
	Token string
	// AllowOrigin, if non-nil, is consulted to allow cross-origin upgrades.
	// When nil, all origins are allowed (suitable for local same-machine
	// deployments where bot and chat client live next to the server).
	AllowOrigin func(r *http.Request) bool
	// HandshakeTimeout caps the WebSocket upgrade. Defaults to 10s.
	HandshakeTimeout time.Duration
	// WriteTimeout caps individual frame writes. Defaults to 10s.
	WriteTimeout time.Duration
	// PingInterval controls server→client ping cadence. Defaults to 30s.
	PingInterval time.Duration
}

// Server is the tingly platform service. It exposes a WebSocket endpoint
// where bots and chat clients connect, and routes frames between them.
type Server struct {
	cfg   Config
	hub   *hub
	store *Store
	up    websocket.Upgrader

	mu       sync.Mutex
	httpSrv  *http.Server
	listener net.Listener
}

// New constructs a Server. Call ListenAndServe (or Serve with a custom
// listener, via Handler) to start.
func New(cfg Config) (*Server, error) {
	if cfg.Path == "" {
		cfg.Path = "/tingly/ws"
	}
	if cfg.HandshakeTimeout == 0 {
		cfg.HandshakeTimeout = 10 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 10 * time.Second
	}
	if cfg.PingInterval == 0 {
		cfg.PingInterval = 30 * time.Second
	}
	store, err := NewStore(cfg.StorePath)
	if err != nil {
		return nil, err
	}
	s := &Server{
		cfg:   cfg,
		hub:   newHub(),
		store: store,
		up: websocket.Upgrader{
			HandshakeTimeout: cfg.HandshakeTimeout,
			CheckOrigin: func(r *http.Request) bool {
				if cfg.AllowOrigin != nil {
					return cfg.AllowOrigin(r)
				}
				return true
			},
		},
	}
	return s, nil
}

// Handler returns an http.Handler that serves the WS endpoint at cfg.Path.
// Useful for embedding into an existing http.ServeMux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(s.cfg.Path, s.serveWS)
	return mux
}

// ListenAndServe starts an HTTP server bound to cfg.Addr. Blocks until the
// server stops; returns http.ErrServerClosed on graceful Shutdown.
func (s *Server) ListenAndServe() error {
	if s.cfg.Addr == "" {
		return errors.New("tingly server: Addr is required for ListenAndServe")
	}
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return err
	}
	return s.Serve(ln)
}

// Serve runs the HTTP server on the given listener.
func (s *Server) Serve(ln net.Listener) error {
	srv := &http.Server{
		Handler:      s.Handler(),
		ReadTimeout:  0, // WS streams must not be capped
		WriteTimeout: 0,
	}
	s.mu.Lock()
	s.httpSrv = srv
	s.listener = ln
	s.mu.Unlock()
	return srv.Serve(ln)
}

// Addr returns the bound listener address (useful when Addr was ":0").
func (s *Server) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	srv := s.httpSrv
	s.mu.Unlock()
	if srv != nil {
		_ = srv.Shutdown(ctx)
	}
	return s.store.Close()
}

func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	wsConn, err := s.up.Upgrade(w, r, nil)
	if err != nil {
		// Upgrader has already written an error response.
		return
	}
	c := newConn(wsConn, s.cfg.WriteTimeout)
	defer c.close()

	// Expect Hello as first frame.
	var f wire.Frame
	if err := wsConn.ReadJSON(&f); err != nil {
		_ = c.writeError("", "BAD_HELLO", "failed to read hello: "+err.Error())
		return
	}
	if f.Kind != wire.KindHello {
		_ = c.writeError(f.ID, "BAD_HELLO", "first frame must be hello")
		return
	}
	var hello wire.Hello
	if err := wire.DecodeData(f.Data, &hello); err != nil {
		_ = c.writeError(f.ID, "BAD_HELLO", "invalid hello payload")
		return
	}
	if hello.Version != wire.Version {
		_ = c.writeError(f.ID, "VERSION", "unsupported wire version")
		return
	}
	if s.cfg.Token != "" && hello.Token != s.cfg.Token {
		_ = c.writeError(f.ID, "AUTH_FAILED", "token rejected")
		return
	}
	if hello.BotID == "" {
		_ = c.writeError(f.ID, "BAD_HELLO", "botId is required")
		return
	}
	c.botID = hello.BotID

	switch hello.Role {
	case wire.RoleBot:
		s.hub.addBot(c)
		defer s.hub.removeBot(c)
	case wire.RoleChat:
		if hello.ChatID == "" {
			_ = c.writeError(f.ID, "BAD_HELLO", "chatId is required for chat role")
			return
		}
		c.chatID = hello.ChatID
		c.role = wire.RoleChat
		s.hub.addChat(c)
		defer s.hub.removeChat(c)
	default:
		_ = c.writeError(f.ID, "BAD_HELLO", "unknown role")
		return
	}
	c.role = hello.Role

	// Send Welcome (with history for chat clients that asked).
	welcome := wire.Welcome{Version: wire.Version, Now: time.Now().Unix()}
	if hello.Role == wire.RoleChat && hello.HistoryLimit > 0 {
		welcome.History = s.store.History(c.botID, c.chatID, hello.HistoryLimit)
	}
	wd, _ := wire.EncodeData(welcome)
	if err := c.write(wire.Frame{Kind: wire.KindWelcome, ID: f.ID, Bot: c.botID, Chat: c.chatID, Data: wd}); err != nil {
		return
	}

	// Start ping loop and read frames.
	stopPing := make(chan struct{})
	go s.pingLoop(c, stopPing)
	defer close(stopPing)

	s.readLoop(c)
}

func (s *Server) pingLoop(c *conn, stop <-chan struct{}) {
	t := time.NewTicker(s.cfg.PingInterval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			if err := c.ping(); err != nil {
				return
			}
		}
	}
}

func (s *Server) readLoop(c *conn) {
	for {
		var f wire.Frame
		if err := c.ws.ReadJSON(&f); err != nil {
			return
		}
		s.handleFrame(c, f)
	}
}

func (s *Server) handleFrame(c *conn, f wire.Frame) {
	// Force the routing fields to match the connection identity to prevent
	// a misbehaving client from forging frames for other bots/chats.
	f.Bot = c.botID
	switch c.role {
	case wire.RoleBot:
		s.handleBotFrame(c, f)
	case wire.RoleChat:
		f.Chat = c.chatID
		s.handleChatFrame(c, f)
	default:
		_ = c.writeError(f.ID, "BAD_STATE", "no role")
	}
}

func (s *Server) handleBotFrame(c *conn, f wire.Frame) {
	switch f.Kind {
	case wire.KindBotSend:
		if f.Chat == "" {
			_ = c.writeError(f.ID, "BAD_FRAME", "bot.send requires chat")
			return
		}
		messageID := s.store.nextMessageID()
		// Stamp the assigned message id back into the frame before persisting
		// so history entries carry a stable id.
		stored := f
		stored.ID = messageID
		_ = s.store.Append(stored)
		_ = c.writeAck(f.ID, messageID)
		// Forward to chat clients.
		fanout(s.hub.chatConns(c.botID, f.Chat), stored)

	case wire.KindBotEdit, wire.KindBotDelete, wire.KindBotReact:
		// These reference an existing message id inside their payload. We
		// can't easily learn the chat from the payload without decoding,
		// but the frame must specify it (clients always set it via
		// chatIDForMessage on their side, but the tingly bot transport
		// today doesn't track chat id for outbound ops — see
		// imbot/platform/tingly/transport.go chatIDForMessage). The server
		// resolves the chat by scanning store entries.
		chatID := f.Chat
		if chatID == "" {
			chatID = s.resolveChatForOp(c.botID, f)
		}
		if chatID == "" {
			_ = c.writeError(f.ID, "NOT_FOUND", "message not found for op")
			return
		}
		f.Chat = chatID
		_ = s.store.Append(f)
		_ = c.writeAck(f.ID, "")
		fanout(s.hub.chatConns(c.botID, chatID), f)

	default:
		_ = c.writeError(f.ID, "BAD_FRAME", "unsupported bot kind: "+string(f.Kind))
	}
}

func (s *Server) handleChatFrame(c *conn, f wire.Frame) {
	switch f.Kind {
	case wire.KindChatSend, wire.KindChatCallback:
		messageID := s.store.nextMessageID()
		stored := f
		stored.ID = messageID
		_ = s.store.Append(stored)
		_ = c.writeAck(f.ID, messageID)
		fanout(s.hub.botConns(c.botID), stored)
	default:
		_ = c.writeError(f.ID, "BAD_FRAME", "unsupported chat kind: "+string(f.Kind))
	}
}

// resolveChatForOp finds the chat that owns the message id referenced by
// edit/delete/react. It scans persisted history newest-first.
func (s *Server) resolveChatForOp(botID string, f wire.Frame) string {
	var msgID string
	switch f.Kind {
	case wire.KindBotEdit:
		var p wire.BotEdit
		_ = wire.DecodeData(f.Data, &p)
		msgID = p.MessageID
	case wire.KindBotDelete:
		var p wire.BotDelete
		_ = wire.DecodeData(f.Data, &p)
		msgID = p.MessageID
	case wire.KindBotReact:
		var p wire.BotReact
		_ = wire.DecodeData(f.Data, &p)
		msgID = p.MessageID
	}
	if msgID == "" {
		return ""
	}
	for _, key := range s.store.AllChats() {
		// key is "{botID}/{chatID}"
		if len(key) <= len(botID)+1 || key[:len(botID)] != botID || key[len(botID)] != '/' {
			continue
		}
		chatID := key[len(botID)+1:]
		for _, e := range s.store.History(botID, chatID, 0) {
			if e.Frame.ID == msgID {
				return chatID
			}
		}
	}
	return ""
}
