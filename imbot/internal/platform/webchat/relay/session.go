package relay

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tingly-dev/tingly-box/imbot/internal/platform/webchat/protocol"
)

// Session represents a WebSocket client session connected to the relay
type Session struct {
	ID         string
	conn       *websocket.Conn
	send       chan *protocol.MessageData
	relay      *RelayServer
	closed     bool
	mu         sync.RWMutex
	createdAt  int64
	lastActive int64
	senderID   string
	senderName string
	clientInfo *ClientInfo
}

// ClientInfo represents client connection information
type ClientInfo struct {
	UserAgent   string `json:"userAgent"`
	IPAddress   string `json:"ipAddress"`
	ConnectTime int64  `json:"connectTime"`
}

// NewSession creates a new session
func NewSession(id string, conn *websocket.Conn, relay *RelayServer) *Session {
	now := time.Now().Unix()
	return &Session{
		ID:         id,
		conn:       conn,
		send:       make(chan *protocol.MessageData, 256),
		relay:      relay,
		createdAt:  now,
		lastActive: now,
		senderID:   generateUserID(),
		senderName: "User",
	}
}

// ReadLoop reads messages from WebSocket connection
func (s *Session) ReadLoop() {
	defer s.Close()
	log.Printf("[Session %s] ReadLoop started", s.ID)

	s.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	s.conn.SetPongHandler(func(string) error {
		s.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	codec := protocol.NewCodec()

	for {
		_, msgBytes, err := s.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[Session %s] WebSocket error: %v", s.ID, err)
			}
			log.Printf("[Session %s] ReadLoop exiting", s.ID)
			break
		}

		clientMsg, err := codec.DecodeClientMessage(msgBytes)
		if err != nil {
			log.Printf("[Session %s] Failed to decode message: %v", s.ID, err)
			continue
		}

		log.Printf("[Session %s] Received message: %s", s.ID, clientMsg.Text)

		// Update sender info from message
		if clientMsg.SenderID != "" {
			s.senderID = clientMsg.SenderID
		}
		if clientMsg.SenderName != "" {
			s.senderName = clientMsg.SenderName
		}

		// Update last active
		s.mu.Lock()
		s.lastActive = time.Now().Unix()
		s.mu.Unlock()

		// Convert to message data and broadcast to bots
		msgData := clientMsg.ToMessageData()
		if msgData.ID == "" {
			msgData.ID = generateMessageID()
		}
		if msgData.Timestamp == 0 {
			msgData.Timestamp = time.Now().Unix()
		}

		// Persist message
		if s.relay.store != nil {
			go s.relay.store.SaveMessage(s.ID, msgData)
		}

		// Update cache
		if s.relay.cache != nil {
			s.relay.cache.Add(s.ID, msgData)
		}

		// Broadcast to all registered bots
		s.relay.BroadcastMessage(s.ID, msgData)
	}
}

// WriteLoop writes messages to WebSocket connection
func (s *Session) WriteLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		s.Close()
		log.Printf("[Session %s] WriteLoop stopped", s.ID)
	}()
	log.Printf("[Session %s] WriteLoop started", s.ID)

	codec := protocol.NewCodec()

	for {
		select {
		case msgData, ok := <-s.send:
			if !ok {
				log.Printf("[Session %s] Send channel closed, exiting WriteLoop", s.ID)
				s.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			log.Printf("[Session %s] Sending message: %s", s.ID, msgData.Text)

			// Convert message data to client message
			clientMsg := &protocol.ClientMessage{
				ID:         msgData.ID,
				Timestamp:  msgData.Timestamp,
				SenderID:   msgData.SenderID,
				SenderName: msgData.SenderName,
				Text:       msgData.Text,
				Media:      msgData.Media,
				Metadata:   msgData.Metadata,
			}

			msgBytes, err := codec.EncodeClientMessage(clientMsg)
			if err != nil {
				log.Printf("[Session %s] Failed to encode message: %v", s.ID, err)
				continue
			}

			s.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := s.conn.WriteMessage(websocket.TextMessage, msgBytes); err != nil {
				log.Printf("[Session %s] Failed to write message: %v", s.ID, err)
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

// Send sends a message to the session
func (s *Session) Send(data *protocol.MessageData) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return ErrSessionClosed
	}

	select {
	case s.send <- data:
		return nil
	default:
		return fmt.Errorf("send buffer full")
	}
}

// SendHistory sends recent message history to the client
func (s *Session) SendHistory(limit int) error {
	// Check cache first
	if s.relay.cache != nil {
		if cached := s.relay.cache.Get(s.ID, limit); cached != nil {
			for _, msg := range cached {
				if err := s.Send(msg); err != nil {
					return err
				}
			}
			return nil
		}
	}

	// Fall back to store
	if s.relay.store != nil {
		messages, err := s.relay.store.GetMessages(s.ID, limit, 0)
		if err != nil {
			return err
		}
		// Send in reverse order (oldest first)
		for i := len(messages) - 1; i >= 0; i-- {
			if err := s.Send(messages[i]); err != nil {
				return err
			}
		}
	}

	return nil
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

// SetClientInfo sets the client info
func (s *Session) SetClientInfo(info *ClientInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clientInfo = info
}

// MarshalJSON for session info
func (s *Session) MarshalJSON() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return json.Marshal(map[string]interface{}{
		"id":         s.ID,
		"senderId":   s.senderID,
		"senderName": s.senderName,
		"createdAt":  s.createdAt,
		"lastActive": s.lastActive,
		"clientInfo": s.clientInfo,
	})
}

// ID generation utilities

func generateMessageID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}

func generateSessionID() string {
	return fmt.Sprintf("sess_%d", time.Now().UnixNano())
}

func generateUserID() string {
	return fmt.Sprintf("user_%d", time.Now().UnixNano())
}
