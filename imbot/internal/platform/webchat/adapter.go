package webchat

import (
	"context"
	"fmt"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// Adapter handles conversion between WebSocket messages and core messages
// For WebChat, this is straightforward since we control both sides
type Adapter struct {
	config *core.Config
}

// NewAdapter creates a new WebChat adapter
func NewAdapter(config *core.Config) *Adapter {
	return &Adapter{
		config: config,
	}
}

// AdaptIncomingMessage converts a WebSocket message to core.Message
func (a *Adapter) AdaptIncomingMessage(ctx context.Context, wsMsg *WebSocketMessage, sessionID string) (*core.Message, error) {
	if wsMsg == nil {
		return nil, fmt.Errorf("nil message")
	}

	return wsMsg.ToCoreMessage(sessionID), nil
}

// AdaptOutgoingMessage converts a core.Message to WebSocket message
func (a *Adapter) AdaptOutgoingMessage(ctx context.Context, msg *core.Message) (*WebSocketMessage, error) {
	if msg == nil {
		return nil, fmt.Errorf("nil message")
	}

	return FromCoreMessage(msg), nil
}

// Platform returns core.PlatformWebChat
func (a *Adapter) Platform() core.Platform {
	return core.PlatformWebChat
}
