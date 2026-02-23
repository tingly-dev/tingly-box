package protocol

import (
	"encoding/json"
)

// Codec handles encoding and decoding of protocol messages
type Codec struct{}

// NewCodec creates a new codec
func NewCodec() *Codec {
	return &Codec{}
}

// EncodeBotMessage encodes a bot message to JSON
func (c *Codec) EncodeBotMessage(msg *BotMessage) ([]byte, error) {
	return json.Marshal(msg)
}

// DecodeBotMessage decodes a bot message from JSON
func (c *Codec) DecodeBotMessage(data []byte) (*BotMessage, error) {
	var msg BotMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// EncodeRelayMessage encodes a relay message to JSON
func (c *Codec) EncodeRelayMessage(msg *RelayMessage) ([]byte, error) {
	return json.Marshal(msg)
}

// DecodeRelayMessage decodes a relay message from JSON
func (c *Codec) DecodeRelayMessage(data []byte) (*RelayMessage, error) {
	var msg RelayMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// EncodeClientMessage encodes a client message (for WebSocket clients)
func (c *Codec) EncodeClientMessage(msg *ClientMessage) ([]byte, error) {
	return json.Marshal(msg)
}

// DecodeClientMessage decodes a client message from WebSocket
func (c *Codec) DecodeClientMessage(data []byte) (*ClientMessage, error) {
	var msg ClientMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}
