package statusline

import (
	"sync"
	"time"
)

// Cache provides session-based caching for Claude Code status inputs
type Cache struct {
	mu       sync.RWMutex
	sessions map[string]*sessionCacheEntry
	maxAge   time.Duration
}

// sessionCacheEntry holds cached data for a single session
type sessionCacheEntry struct {
	lastInput  *StatusInput
	lastUpdate time.Time
}

// NewCache creates a new cache
func NewCache() *Cache {
	return &Cache{
		sessions: make(map[string]*sessionCacheEntry),
		maxAge:   30 * time.Minute,
	}
}

// Update stores input for the session
func (c *Cache) Update(input *StatusInput) {
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
func (c *Cache) Get(input *StatusInput) *StatusInput {
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
func mergeStatusInput(input, cached *StatusInput) *StatusInput {
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
