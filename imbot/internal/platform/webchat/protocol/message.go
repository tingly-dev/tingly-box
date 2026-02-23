package protocol

import "time"

// MessageType represents the type of protocol message
type MessageType string

const (
	// Bot to Relay message types
	MessageTypeRegister    MessageType = "register" // Bot registers with relay
	MessageTypeSendMessage MessageType = "send"     // Bot sends message to session
	MessageTypeAcknowledge MessageType = "ack"      // Acknowledge message receipt

	// Relay to Bot message types
	MessageTypeIncoming     MessageType = "message"        // Incoming message from session
	MessageTypeSessionJoin  MessageType = "session_joined" // Session joined relay
	MessageTypeSessionLeave MessageType = "session_left"   // Session left relay
	MessageTypeError        MessageType = "error"          // Error occurred
)

// BotMessage represents a message from Bot to Relay
type BotMessage struct {
	Type      MessageType    `json:"type"`
	BotID     string         `json:"botId"`
	SessionID string         `json:"sessionId,omitempty"`
	Message   *MessageData   `json:"message,omitempty"`
	Timestamp int64          `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// RelayMessage represents a message from Relay to Bot
type RelayMessage struct {
	Type      MessageType    `json:"type"`
	BotID     string         `json:"botId,omitempty"`
	SessionID string         `json:"sessionId"`
	Message   *MessageData   `json:"message,omitempty"`
	Timestamp int64          `json:"timestamp"`
	Error     string         `json:"error,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// MessageData represents message content shared between bot and relay
type MessageData struct {
	ID         string                 `json:"id"`
	Timestamp  int64                  `json:"timestamp"`
	SenderID   string                 `json:"senderId"`
	SenderName string                 `json:"senderName"`
	Text       string                 `json:"text,omitempty"`
	Media      []MediaAttachment      `json:"media,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// MediaAttachment represents media in a message
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

// NewBotMessage creates a new bot message
func NewBotMessage(msgType MessageType, botID string) *BotMessage {
	return &BotMessage{
		Type:      msgType,
		BotID:     botID,
		Timestamp: time.Now().Unix(),
	}
}

// NewRegisterMessage creates a register message for bot to connect to relay
func NewRegisterMessage(botID, botToken string) *BotMessage {
	msg := NewBotMessage(MessageTypeRegister, botID)
	if botToken != "" {
		msg.Metadata = make(map[string]any)
		msg.Metadata["token"] = botToken
	}
	return msg
}

// NewSendMessage creates a send message from bot to relay
func NewSendMessage(botID, sessionID string, data *MessageData) *BotMessage {
	msg := NewBotMessage(MessageTypeSendMessage, botID)
	msg.SessionID = sessionID
	msg.Message = data
	return msg
}

// NewAckMessage creates an acknowledge message
func NewAckMessage(botID, messageID string) *BotMessage {
	msg := NewBotMessage(MessageTypeAcknowledge, botID)
	if msg.Metadata == nil {
		msg.Metadata = make(map[string]any)
	}
	msg.Metadata["messageId"] = messageID
	return msg
}

// NewRelayMessage creates a new relay message
func NewRelayMessage(msgType MessageType, sessionID string) *RelayMessage {
	return &RelayMessage{
		Type:      msgType,
		SessionID: sessionID,
		Timestamp: time.Now().Unix(),
	}
}

// NewIncomingMessage creates an incoming message from relay to bot
func NewIncomingMessage(botID, sessionID string, data *MessageData) *RelayMessage {
	msg := NewRelayMessage(MessageTypeIncoming, sessionID)
	msg.BotID = botID
	msg.Message = data
	return msg
}

// NewSessionJoinMessage creates a session joined message
func NewSessionJoinMessage(sessionID string) *RelayMessage {
	return NewRelayMessage(MessageTypeSessionJoin, sessionID)
}

// NewSessionLeaveMessage creates a session left message
func NewSessionLeaveMessage(sessionID string) *RelayMessage {
	return NewRelayMessage(MessageTypeSessionLeave, sessionID)
}

// NewErrorMessage creates an error message
func NewErrorMessage(sessionID string, err string) *RelayMessage {
	msg := NewRelayMessage(MessageTypeError, sessionID)
	msg.Error = err
	return msg
}
