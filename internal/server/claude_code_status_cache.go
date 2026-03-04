package server

import (
	"sync"
	"time"
)

// sessionCacheEntry holds cached data for a single session
type sessionCacheEntry struct {
	lastInput  *ClaudeCodeStatusInput
	lastUpdate time.Time
}

// ClaudeCodeStatusCache caches Claude Code status inputs per session
type ClaudeCodeStatusCache struct {
	mu       sync.RWMutex
	sessions map[string]*sessionCacheEntry
	maxAge   time.Duration
}

// NewClaudeCodeStatusCache creates a new cache
func NewClaudeCodeStatusCache() *ClaudeCodeStatusCache {
	return &ClaudeCodeStatusCache{
		sessions: make(map[string]*sessionCacheEntry),
		maxAge:   30 * time.Minute,
	}
}

// Update stores input for the session
func (c *ClaudeCodeStatusCache) Update(input *ClaudeCodeStatusInput) {
	if input == nil || input.SessionID == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.sessions[input.SessionID] = &sessionCacheEntry{
		lastInput:  input,
		lastUpdate: time.Now(),
	}
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

	// Stale cache - return input as-is and clean up
	if time.Since(entry.lastUpdate) > c.maxAge {
		delete(c.sessions, input.SessionID)
		return input
	}

	// Merge zero values from cache
	return mergeStatusInput(input, entry.lastInput)
}

// mergeStatusInput merges zero/empty fields from cached into input
func mergeStatusInput(input, cached *ClaudeCodeStatusInput) *ClaudeCodeStatusInput {
	merged := *input

	// Model fields
	merged.Model.DisplayName = mergeIfEmpty(merged.Model.DisplayName, cached.Model.DisplayName)
	merged.Model.ID = mergeIfEmpty(merged.Model.ID, cached.Model.ID)

	// ContextWindow fields
	merged.ContextWindow.UsedPercentage = mergeIfZero(merged.ContextWindow.UsedPercentage, cached.ContextWindow.UsedPercentage)
	merged.ContextWindow.ContextWindowSize = mergeIfZero(merged.ContextWindow.ContextWindowSize, cached.ContextWindow.ContextWindowSize)
	merged.ContextWindow.TotalInputTokens = mergeIfZero(merged.ContextWindow.TotalInputTokens, cached.ContextWindow.TotalInputTokens)
	merged.ContextWindow.TotalOutputTokens = mergeIfZero(merged.ContextWindow.TotalOutputTokens, cached.ContextWindow.TotalOutputTokens)

	// Cost fields
	merged.Cost.TotalCostUSD = mergeIfZero(merged.Cost.TotalCostUSD, cached.Cost.TotalCostUSD)
	merged.Cost.TotalDurationMs = mergeIfZero(merged.Cost.TotalDurationMs, cached.Cost.TotalDurationMs)
	merged.Cost.TotalAPIDurationMs = mergeIfZero(merged.Cost.TotalAPIDurationMs, cached.Cost.TotalAPIDurationMs)
	merged.Cost.TotalLinesAdded = mergeIfZero(merged.Cost.TotalLinesAdded, cached.Cost.TotalLinesAdded)
	merged.Cost.TotalLinesRemoved = mergeIfZero(merged.Cost.TotalLinesRemoved, cached.Cost.TotalLinesRemoved)

	return &merged
}

// mergeIfEmpty returns cached if target is empty
func mergeIfEmpty(target, cached string) string {
	if target == "" && cached != "" {
		return cached
	}
	return target
}

// mergeIfZero returns cached if target is zero (for numeric types where 0 means "not set")
func mergeIfZero[T int | int64 | float64](target, cached T) T {
	if target == 0 && cached != 0 {
		return cached
	}
	return target
}

// globalClaudeCodeStatusCache is the global cache instance
var globalClaudeCodeStatusCache = NewClaudeCodeStatusCache()

// GetGlobalClaudeCodeStatusCache returns the global cache instance
func GetGlobalClaudeCodeStatusCache() *ClaudeCodeStatusCache {
	return globalClaudeCodeStatusCache
}
