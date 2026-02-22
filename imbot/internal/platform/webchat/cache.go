package webchat

import (
	"sync"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// MessageCache provides in-memory caching for recent messages
// This provides fast access for active sessions without hitting the database
type MessageCache struct {
	messages map[string][]*core.Message // session_id -> messages
	mu       sync.RWMutex
	maxSize  int // Max messages per session
}

// NewMessageCache creates a new message cache
func NewMessageCache(maxSize int) *MessageCache {
	if maxSize <= 0 {
		maxSize = 100 // Default cache size
	}

	return &MessageCache{
		messages: make(map[string][]*core.Message),
		maxSize:  maxSize,
	}
}

// Add adds a message to the cache for a session
func (c *MessageCache) Add(sessionID string, msg *core.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()

	msgs := c.messages[sessionID]
	msgs = append(msgs, msg)

	// Keep only recent messages (LRU-style truncation)
	if len(msgs) > c.maxSize {
		msgs = msgs[len(msgs)-c.maxSize:]
	}

	c.messages[sessionID] = msgs
}

// Get retrieves the most recent messages for a session from cache
// Returns nil if no cached messages exist
func (c *MessageCache) Get(sessionID string, limit int) []*core.Message {
	c.mu.RLock()
	defer c.mu.RUnlock()

	msgs := c.messages[sessionID]
	if len(msgs) == 0 {
		return nil
	}

	// Determine how many messages to return
	if limit <= 0 || limit > len(msgs) {
		limit = len(msgs)
	}

	// Return the most recent messages (already in order: oldest -> newest)
	result := make([]*core.Message, limit)
	copy(result, msgs[len(msgs)-limit:])
	return result
}

// GetAll returns all cached messages for a session
func (c *MessageCache) GetAll(sessionID string) []*core.Message {
	c.mu.RLock()
	defer c.mu.RUnlock()

	msgs := c.messages[sessionID]
	if len(msgs) == 0 {
		return nil
	}

	result := make([]*core.Message, len(msgs))
	copy(result, msgs)
	return result
}

// Remove removes all cached messages for a session
func (c *MessageCache) Remove(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.messages, sessionID)
}

// Clear clears all cached messages
func (c *MessageCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.messages = make(map[string][]*core.Message)
}

// Size returns the number of sessions currently cached
func (c *MessageCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.messages)
}

// SessionSize returns the number of messages cached for a session
func (c *MessageCache) SessionSize(sessionID string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	msgs := c.messages[sessionID]
	return len(msgs)
}
