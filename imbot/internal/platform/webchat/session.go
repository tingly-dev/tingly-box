package webchat

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// Session represents a WebSocket client session
type Session struct {
	ID         string
	conn       *websocket.Conn
	send       chan *core.Message
	bot        *Bot
	closed     bool
	mu         sync.RWMutex
	createdAt  int64
	lastActive int64
	senderID   string
	senderName string
	clientInfo *WebSocketClientInfo
}

// NewSession creates a new session
func NewSession(id string, conn *websocket.Conn, bot *Bot) *Session {
	now := time.Now().Unix()
	return &Session{
		ID:         id,
		conn:       conn,
		send:       make(chan *core.Message, 256),
		bot:        bot,
		createdAt:  now,
		lastActive: now,
		senderID:   generateUserID(),
		senderName: "User",
	}
}

// ReadLoop reads messages from WebSocket connection
func (s *Session) ReadLoop() {
	defer s.Close()

	s.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	s.conn.SetPongHandler(func(string) error {
		s.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, msgBytes, err := s.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				s.bot.Logger().Error("WebSocket error: %v", err)
			}
			break
		}

		var wsMsg WebSocketMessage
		if err := json.Unmarshal(msgBytes, &wsMsg); err != nil {
			s.bot.Logger().Error("Failed to unmarshal message: %v", err)
			continue
		}

		// Update sender info from message
		if wsMsg.SenderID != "" {
			s.senderID = wsMsg.SenderID
		}
		if wsMsg.SenderName != "" {
			s.senderName = wsMsg.SenderName
		}

		// Update last active
		s.mu.Lock()
		s.lastActive = time.Now().Unix()
		s.mu.Unlock()

		// Forward to bot
		s.bot.HandleIncomingMessage(s.ID, &wsMsg)
	}
}

// WriteLoop writes messages to WebSocket connection
func (s *Session) WriteLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		s.Close()
	}()

	for {
		select {
		case msg, ok := <-s.send:
			if !ok {
				s.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Convert core message to WebSocket message
			wsMsg := FromCoreMessage(msg)
			msgBytes, err := json.Marshal(wsMsg)
			if err != nil {
				s.bot.Logger().Error("Failed to marshal message: %v", err)
				continue
			}

			s.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := s.conn.WriteMessage(websocket.TextMessage, msgBytes); err != nil {
				return
			}

		case <-ticker.C:
			s.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := s.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Send queues a message to be sent to the client
func (s *Session) Send(msg *core.Message) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return core.NewBotError(core.ErrConnectionFailed, "session closed", false)
	}

	select {
	case s.send <- msg:
		return nil
	default:
		return core.NewBotError(core.ErrPlatformError, "send buffer full", false)
	}
}

// Close closes the session
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	close(s.send)
	return s.conn.Close()
}

// IsClosed returns true if the session is closed
func (s *Session) IsClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}

// SenderID returns the sender ID for this session
func (s *Session) SenderID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.senderID
}

// SenderName returns the sender name for this session
func (s *Session) SenderName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.senderName
}

// SetSenderInfo sets the sender info for this session
func (s *Session) SetSenderInfo(id, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.senderID = id
	s.senderName = name
}

// CreatedAt returns the session creation time
func (s *Session) CreatedAt() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.createdAt
}

// LastActive returns the last active time
func (s *Session) LastActive() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastActive
}
