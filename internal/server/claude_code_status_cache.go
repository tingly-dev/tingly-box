package server

import (
	"container/list"
	"sync"
	"time"
)

// sessionCacheEntry holds cached data for a single session
type sessionCacheEntry struct {
	lastInput   *ClaudeCodeStatusInput
	lastUpdate  time.Time
	listElement *list.Element
}

// ClaudeCodeStatusCache caches Claude Code status inputs per session with LRU eviction
type ClaudeCodeStatusCache struct {
	mu sync.RWMutex

	maxSessions int
	maxAge      time.Duration

	sessions map[string]*sessionCacheEntry
	lruList  *list.List
}

// NewClaudeCodeStatusCache creates a new cache
func NewClaudeCodeStatusCache() *ClaudeCodeStatusCache {
	return &ClaudeCodeStatusCache{
		maxSessions: 100,
		maxAge:      30 * time.Minute,
		sessions:    make(map[string]*sessionCacheEntry),
		lruList:     list.New(),
	}
}

// Update stores input for the session (one entry per session)
func (c *ClaudeCodeStatusCache) Update(input *ClaudeCodeStatusInput) {
	if input == nil || input.SessionID == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	sessionID := input.SessionID

	if entry, exists := c.sessions[sessionID]; exists {
		entry.lastInput = input
		entry.lastUpdate = time.Now()
		c.lruList.MoveToFront(entry.listElement)
		return
	}

	// Evict LRU if at capacity
	if len(c.sessions) >= c.maxSessions {
		if oldest := c.lruList.Back(); oldest != nil {
			c.removeEntry(oldest.Value.(string))
		}
	}

	// Add new entry
	entry := &sessionCacheEntry{
		lastInput:  input,
		lastUpdate: time.Now(),
	}
	entry.listElement = c.lruList.PushFront(sessionID)
	c.sessions[sessionID] = entry
}

// Get returns cached input for the session, merging zero values from cache
func (c *ClaudeCodeStatusCache) Get(input *ClaudeCodeStatusInput) *ClaudeCodeStatusInput {
	if input == nil {
		return nil
	}
	if input.SessionID == "" {
		return input
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.sessions[input.SessionID]
	if !exists || entry == nil || entry.lastInput == nil {
		return input
	}

	// Stale cache - return input as-is
	if time.Since(entry.lastUpdate) > c.maxAge {
		c.removeEntry(input.SessionID)
		return input
	}

	c.lruList.MoveToFront(entry.listElement)

	// Merge zero values from cache
	merged, cached := *input, entry.lastInput

	if merged.Model.DisplayName == "" && cached.Model.DisplayName != "" {
		merged.Model.DisplayName = cached.Model.DisplayName
	}
	if merged.Model.ID == "" && cached.Model.ID != "" {
		merged.Model.ID = cached.Model.ID
	}
	if merged.ContextWindow.UsedPercentage == 0 && cached.ContextWindow.UsedPercentage > 0 {
		merged.ContextWindow.UsedPercentage = cached.ContextWindow.UsedPercentage
	}
	if merged.ContextWindow.ContextWindowSize == 0 && cached.ContextWindow.ContextWindowSize > 0 {
		merged.ContextWindow.ContextWindowSize = cached.ContextWindow.ContextWindowSize
	}
	if merged.ContextWindow.TotalInputTokens == 0 && cached.ContextWindow.TotalInputTokens > 0 {
		merged.ContextWindow.TotalInputTokens = cached.ContextWindow.TotalInputTokens
	}
	if merged.ContextWindow.TotalOutputTokens == 0 && cached.ContextWindow.TotalOutputTokens > 0 {
		merged.ContextWindow.TotalOutputTokens = cached.ContextWindow.TotalOutputTokens
	}
	if merged.Cost.TotalCostUSD == 0 && cached.Cost.TotalCostUSD > 0 {
		merged.Cost.TotalCostUSD = cached.Cost.TotalCostUSD
	}
	if merged.Cost.TotalDurationMs == 0 && cached.Cost.TotalDurationMs > 0 {
		merged.Cost.TotalDurationMs = cached.Cost.TotalDurationMs
	}
	if merged.Cost.TotalAPIDurationMs == 0 && cached.Cost.TotalAPIDurationMs > 0 {
		merged.Cost.TotalAPIDurationMs = cached.Cost.TotalAPIDurationMs
	}
	if merged.Cost.TotalLinesAdded == 0 && cached.Cost.TotalLinesAdded > 0 {
		merged.Cost.TotalLinesAdded = cached.Cost.TotalLinesAdded
	}
	if merged.Cost.TotalLinesRemoved == 0 && cached.Cost.TotalLinesRemoved > 0 {
		merged.Cost.TotalLinesRemoved = cached.Cost.TotalLinesRemoved
	}

	return &merged
}

// removeEntry removes a session entry (must hold lock)
func (c *ClaudeCodeStatusCache) removeEntry(sessionID string) {
	if entry, ok := c.sessions[sessionID]; ok {
		if entry.listElement != nil {
			c.lruList.Remove(entry.listElement)
		}
		delete(c.sessions, sessionID)
	}
}

// globalClaudeCodeStatusCache is the global cache instance
var globalClaudeCodeStatusCache = NewClaudeCodeStatusCache()

// GetGlobalClaudeCodeStatusCache returns the global cache instance
func GetGlobalClaudeCodeStatusCache() *ClaudeCodeStatusCache {
	return globalClaudeCodeStatusCache
}
