package webchat

import (
	"time"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// WebSocketMessage represents a message sent over WebSocket
// It's designed to match core.Message structure for easy conversion
type WebSocketMessage struct {
	ID         string                 `json:"id"`
	Timestamp  int64                  `json:"timestamp,omitempty"`
	SenderID   string                 `json:"senderId,omitempty"`
	SenderName string                 `json:"senderName,omitempty"`
	Text       string                 `json:"text,omitempty"`
	Media      []MediaAttachment      `json:"media,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// MediaAttachment represents media in WebSocket message
type MediaAttachment struct {
	Type      string                 `json:"type"` // "image", "video", "audio", "document"
	URL       string                 `json:"url"`
	MimeType  string                 `json:"mimeType,omitempty"`
	Filename  string                 `json:"filename,omitempty"`
	Size      int64                  `json:"size,omitempty"`
	Thumbnail string                 `json:"thumbnail,omitempty"`
	Width     int                    `json:"width,omitempty"`
	Height    int                    `json:"height,omitempty"`
	Duration  int                    `json:"duration,omitempty"`
	Raw       map[string]interface{} `json:"raw,omitempty"`
}

// WebSocketClientInfo represents client connection info
type WebSocketClientInfo struct {
	UserAgent   string `json:"userAgent"`
	IPAddress   string `json:"ipAddress"`
	ConnectTime int64  `json:"connectTime"`
}

// ToCoreMessage converts WebSocketMessage to core.Message
func (m *WebSocketMessage) ToCoreMessage(sessionID string) *core.Message {
	msg := &core.Message{
		ID:        m.ID,
		Platform:  core.PlatformWebChat,
		Timestamp: m.Timestamp,
		Sender: core.Sender{
			ID:          m.SenderID,
			DisplayName: m.SenderName,
		},
		Recipient: core.Recipient{
			ID: sessionID,
		},
		ChatType: core.ChatTypeDirect,
		Metadata: m.Metadata,
	}

	// Set content
	if m.Text != "" || len(m.Media) > 0 {
		if len(m.Media) > 0 {
			media := make([]core.MediaAttachment, len(m.Media))
			for i, m := range m.Media {
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
			msg.Content = core.NewMediaContent(media, m.Text)
		} else {
			msg.Content = core.NewTextContent(m.Text)
		}
	}

	return msg
}

// FromCoreMessage converts core.Message to WebSocketMessage
func FromCoreMessage(msg *core.Message) *WebSocketMessage {
	wsMsg := &WebSocketMessage{
		ID:         msg.ID,
		Timestamp:  msg.Timestamp,
		SenderID:   msg.Sender.ID,
		SenderName: msg.Sender.DisplayName,
		Metadata:   msg.Metadata,
	}

	// Extract content
	switch c := msg.Content.(type) {
	case *core.TextContent:
		wsMsg.Text = c.Text
	case *core.MediaContent:
		wsMsg.Media = make([]MediaAttachment, len(c.Media))
		for i, m := range c.Media {
			wsMsg.Media[i] = MediaAttachment{
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
		wsMsg.Text = c.Caption
	}

	return wsMsg
}

// NewWebSocketMessage creates a new WebSocket message with generated ID and timestamp
func NewWebSocketMessage(senderID, senderName, text string) *WebSocketMessage {
	return &WebSocketMessage{
		ID:         generateMessageID(),
		Timestamp:  time.Now().Unix(),
		SenderID:   senderID,
		SenderName: senderName,
		Text:       text,
	}
}
