package relay

import (
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/internal/platform/webchat/protocol"
)

// MessageCache provides in-memory caching for recent messages
type MessageCache struct {
	maxSize int
	mu      sync.RWMutex
	// Map of sessionID -> list of messages (most recent first)
	cache map[string][]*protocol.MessageData
}

// NewMessageCache creates a new message cache
func NewMessageCache(maxSize int) *MessageCache {
	return &MessageCache{
		maxSize: maxSize,
		cache:   make(map[string][]*protocol.MessageData),
	}
}

// Add adds a message to the cache for a session
func (c *MessageCache) Add(sessionID string, msg *protocol.MessageData) {
	if c.maxSize <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Initialize session slice if not exists
	if c.cache[sessionID] == nil {
		c.cache[sessionID] = make([]*protocol.MessageData, 0, c.maxSize)
	}

	// Add message to front (most recent)
	messages := c.cache[sessionID]
	messages = append([]*protocol.MessageData{msg}, messages...)

	// Trim to max size
	if len(messages) > c.maxSize {
		messages = messages[:c.maxSize]
	}

	c.cache[sessionID] = messages
}

// Get retrieves recent messages for a session
func (c *MessageCache) Get(sessionID string, limit int) []*protocol.MessageData {
	if c.maxSize <= 0 {
		return nil
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	messages := c.cache[sessionID]
	if messages == nil {
		return nil
	}

	// Apply limit
	if limit > 0 && len(messages) > limit {
		messages = messages[:limit]
	}

	// Return a copy to avoid race conditions
	result := make([]*protocol.MessageData, len(messages))
	copy(result, messages)

	return result
}

// Clear removes all cached messages for a session
func (c *MessageCache) Clear(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.cache, sessionID)
}

// ClearAll removes all cached messages
func (c *MessageCache) ClearAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string][]*protocol.MessageData)
}

// Size returns the number of cached messages for a session
func (c *MessageCache) Size(sessionID string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	messages := c.cache[sessionID]
	if messages == nil {
		return 0
	}
	return len(messages)
}

// TotalSize returns the total number of cached messages across all sessions
func (c *MessageCache) TotalSize() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := 0
	for _, messages := range c.cache {
		total += len(messages)
	}
	return total
}

// SessionCount returns the number of sessions with cached messages
func (c *MessageCache) SessionCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.cache)
}

// PruneOld removes cached messages older than the given duration
func (c *MessageCache) PruneOld(maxAge time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cutoff := time.Now().Add(-maxAge).Unix()

	for sessionID, messages := range c.cache {
		// Filter out old messages
		filtered := make([]*protocol.MessageData, 0)
		for _, msg := range messages {
			if msg.Timestamp >= cutoff {
				filtered = append(filtered, msg)
			}
		}

		if len(filtered) == 0 {
			delete(c.cache, sessionID)
		} else {
			c.cache[sessionID] = filtered
		}
	}
}
