package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tingly-dev/tingly-box/imbot/internal/core"
	"github.com/tingly-dev/tingly-box/imbot/internal/platform/webchat/protocol"
)

// RelayClient connects a bot to a relay server
type RelayClient struct {
	relayAddr string
	botID     string
	botToken  string
	conn      *websocket.Conn
	mu        sync.RWMutex
	handler   MessageHandler
	ctx       context.Context
	cancel    context.CancelFunc
	connected bool
	codec     *protocol.Codec
}

// MessageHandler handles incoming messages from the relay server
type MessageHandler interface {
	HandleMessage(sessionID string, msg *core.Message)
	SessionJoined(sessionID string)
	SessionLeft(sessionID string)
}

// Config configures a RelayClient
type Config struct {
	RelayAddr string
	BotID     string
	BotToken  string
	Handler   MessageHandler
}

// NewRelayClient creates a new relay client
func NewRelayClient(cfg Config) *RelayClient {
	return &RelayClient{
		relayAddr: cfg.RelayAddr,
		botID:     cfg.BotID,
		botToken:  cfg.BotToken,
		handler:   cfg.Handler,
		codec:     protocol.NewCodec(),
	}
}

// Connect connects to the relay server
func (c *RelayClient) Connect(ctx context.Context) error {
	c.ctx, c.cancel = context.WithCancel(ctx)

	// Construct WebSocket URL
	wsURL := c.wsURL()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to relay server: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.mu.Unlock()

	// Send register message
	registerMsg := protocol.NewRegisterMessage(c.botID, c.botToken)
	if err := c.send(registerMsg); err != nil {
		c.Close()
		return fmt.Errorf("failed to register: %w", err)
	}

	// Start read loop
	go c.readLoop()

	return nil
}

// Disconnect disconnects from the relay server
func (c *RelayClient) Disconnect(ctx context.Context) error {
	if c.cancel != nil {
		c.cancel()
	}
	return c.Close()
}

// Close closes the connection
func (c *RelayClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.connected = false
		return c.conn.Close()
	}
	return nil
}

// SendMessage sends a message to a session via the relay server
func (c *RelayClient) SendMessage(sessionID string, msg *core.Message) error {
	// Convert core message to protocol message data
	msgData := &protocol.MessageData{
		ID:         msg.ID,
		Timestamp:  msg.Timestamp,
		SenderID:   msg.Sender.ID,
		SenderName: msg.Sender.DisplayName,
		Metadata:   msg.Metadata,
	}

	// Extract content
	switch content := msg.Content.(type) {
	case *core.TextContent:
		msgData.Text = content.Text
	case *core.MediaContent:
		msgData.Media = make([]protocol.MediaAttachment, len(content.Media))
		for i, m := range content.Media {
			msgData.Media[i] = protocol.MediaAttachment{
				Type:      m.Type,
				URL:       m.URL,
				MimeType:  m.MimeType,
				Filename:  m.Filename,
				Size:      m.Size,
				Thumbnail: m.Thumbnail,
				Width:     m.Width,
				Height:    m.Height,
				Duration:  m.Duration,
				Raw:       m.Raw,
			}
		}
		msgData.Text = content.Caption
	}

	// Use HTTP API to send (more reliable than WebSocket for bot->relay)
	return c.sendHTTP(sessionID, msgData)
}

// sendHTTP sends a message via HTTP POST
func (c *RelayClient) sendHTTP(sessionID string, msgData *protocol.MessageData) error {
	url := fmt.Sprintf("http://%s/api/bot/%s/send", c.relayAddr, c.botID)

	payload := map[string]interface{}{
		"sessionId": sessionID,
		"message":   msgData,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// readLoop reads messages from the relay server
func (c *RelayClient) readLoop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()

		if conn == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			if c.handler != nil {
				// Notify handler of disconnect
			}
			c.mu.Lock()
			c.connected = false
			c.mu.Unlock()
			return
		}

		// Parse relay message
		var relayMsg protocol.RelayMessage
		if err := json.Unmarshal(msgBytes, &relayMsg); err != nil {
			continue
		}

		c.handleRelayMessage(&relayMsg)
	}
}

// handleRelayMessage handles a message from the relay server
func (c *RelayClient) handleRelayMessage(msg *protocol.RelayMessage) {
	if c.handler == nil {
		return
	}

	switch msg.Type {
	case protocol.MessageTypeIncoming:
		// Convert to core message
		coreMsg := c.toCoreMessage(msg.Message, msg.SessionID)
		c.handler.HandleMessage(msg.SessionID, coreMsg)

	case protocol.MessageTypeSessionJoin:
		c.handler.SessionJoined(msg.SessionID)

	case protocol.MessageTypeSessionLeave:
		c.handler.SessionLeft(msg.SessionID)

	case protocol.MessageTypeError:
		// Log error
	}
}

// toCoreMessage converts protocol message data to core message
func (c *RelayClient) toCoreMessage(data *protocol.MessageData, sessionID string) *core.Message {
	msg := &core.Message{
		ID:        data.ID,
		Platform:  core.PlatformWebChat,
		Timestamp: data.Timestamp,
		Sender: core.Sender{
			ID:          data.SenderID,
			DisplayName: data.SenderName,
		},
		Recipient: core.Recipient{
			ID: sessionID,
		},
		ChatType: core.ChatTypeDirect,
		Metadata: data.Metadata,
	}

	// Set content
	if data.Text != "" || len(data.Media) > 0 {
		if len(data.Media) > 0 {
			media := make([]core.MediaAttachment, len(data.Media))
			for i, m := range data.Media {
				media[i] = core.MediaAttachment{
					Type:      m.Type,
					URL:       m.URL,
					MimeType:  m.MimeType,
					Filename:  m.Filename,
					Size:      m.Size,
					Thumbnail: m.Thumbnail,
					Width:     m.Width,
					Height:    m.Height,
					Duration:  m.Duration,
					Raw:       m.Raw,
				}
			}
			msg.Content = core.NewMediaContent(media, data.Text)
		} else {
			msg.Content = core.NewTextContent(data.Text)
		}
	}

	return msg
}

// send sends a message via WebSocket
func (c *RelayClient) send(msg *protocol.BotMessage) error {
	c.mu.RLock()
	conn := c.conn
	connected := c.connected
	c.mu.RUnlock()

	if !connected || conn == nil {
		return fmt.Errorf("not connected")
	}

	msgBytes, err := c.codec.EncodeBotMessage(msg)
	if err != nil {
		return err
	}

	c.mu.Lock()
	err = conn.WriteMessage(websocket.TextMessage, msgBytes)
	c.mu.Unlock()

	return err
}

// wsURL returns the WebSocket URL for the relay server
func (c *RelayClient) wsURL() string {
	return fmt.Sprintf("ws://%s/bot/%s/ws", c.relayAddr, c.botID)
}

// IsConnected returns true if connected to the relay server
func (c *RelayClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}
