package protocol

// ClientMessage represents a message from WebSocket client to relay
type ClientMessage struct {
	ID         string                 `json:"id"`
	Timestamp  int64                  `json:"timestamp,omitempty"`
	SenderID   string                 `json:"senderId,omitempty"`
	SenderName string                 `json:"senderName,omitempty"`
	Text       string                 `json:"text,omitempty"`
	Media      []MediaAttachment      `json:"media,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Type       string                 `json:"type,omitempty"` // "message", "ping", "history_request"
}

// NewClientMessage creates a new client message
func NewClientMessage(senderID, senderName, text string) *ClientMessage {
	return &ClientMessage{
		SenderID:   senderID,
		SenderName: senderName,
		Text:       text,
	}
}

// ToMessageData converts client message to message data
func (m *ClientMessage) ToMessageData() *MessageData {
	return &MessageData{
		ID:         m.ID,
		Timestamp:  m.Timestamp,
		SenderID:   m.SenderID,
		SenderName: m.SenderName,
		Text:       m.Text,
		Media:      m.Media,
		Metadata:   m.Metadata,
	}
}
