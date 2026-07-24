package statusline

import (
	"cmp"
	"sync"
)

const maxSize = 500

type Cache struct {
	mu       sync.RWMutex
	sessions map[string]*StatusInput
	lru      []string
}

func NewCache() *Cache {
	return &Cache{
		sessions: make(map[string]*StatusInput),
		lru:      make([]string, 0, 100),
	}
}

func (c *Cache) Update(input *StatusInput) {
	if input == nil || input.SessionID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.touchLRU(input.SessionID)
	c.sessions[input.SessionID] = input
	c.evict()
}

func (c *Cache) Get(input *StatusInput) *StatusInput {
	if input == nil || input.SessionID == "" {
		return input
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	cached, ok := c.sessions[input.SessionID]
	if !ok {
		return input
	}

	c.touchLRU(input.SessionID)
	return mergeStatusInput(input, cached)
}

func (c *Cache) touchLRU(id string) {
	for i, v := range c.lru {
		if v == id {
			c.lru = append(c.lru[:i], c.lru[i+1:]...)
			break
		}
	}
	c.lru = append(c.lru, id)
}

func (c *Cache) evict() {
	for len(c.sessions) > maxSize && len(c.lru) > 0 {
		delete(c.sessions, c.lru[0])
		c.lru = c.lru[1:]
	}
}

func mergeStatusInput(input, cached *StatusInput) *StatusInput {
	merged := *input

	merged.Model.DisplayName = cmp.Or(merged.Model.DisplayName, cached.Model.DisplayName)
	merged.Model.ID = cmp.Or(merged.Model.ID, cached.Model.ID)

	merged.ContextWindow.UsedPercentage = cmp.Or(merged.ContextWindow.UsedPercentage, cached.ContextWindow.UsedPercentage)
	merged.ContextWindow.ContextWindowSize = cmp.Or(merged.ContextWindow.ContextWindowSize, cached.ContextWindow.ContextWindowSize)
	merged.ContextWindow.TotalInputTokens = cmp.Or(merged.ContextWindow.TotalInputTokens, cached.ContextWindow.TotalInputTokens)
	merged.ContextWindow.TotalOutputTokens = cmp.Or(merged.ContextWindow.TotalOutputTokens, cached.ContextWindow.TotalOutputTokens)
	merged.ContextWindow.CurrentUsage.InputTokens = cmp.Or(merged.ContextWindow.CurrentUsage.InputTokens, cached.ContextWindow.CurrentUsage.InputTokens)
	merged.ContextWindow.CurrentUsage.OutputTokens = cmp.Or(merged.ContextWindow.CurrentUsage.OutputTokens, cached.ContextWindow.CurrentUsage.OutputTokens)
	merged.ContextWindow.CurrentUsage.CacheRead = cmp.Or(merged.ContextWindow.CurrentUsage.CacheRead, cached.ContextWindow.CurrentUsage.CacheRead)
	merged.ContextWindow.CurrentUsage.CacheWrite = cmp.Or(merged.ContextWindow.CurrentUsage.CacheWrite, cached.ContextWindow.CurrentUsage.CacheWrite)

	merged.Cost.TotalCostUSD = cmp.Or(merged.Cost.TotalCostUSD, cached.Cost.TotalCostUSD)
	merged.Cost.TotalDurationMs = cmp.Or(merged.Cost.TotalDurationMs, cached.Cost.TotalDurationMs)
	merged.Cost.TotalAPIDurationMs = cmp.Or(merged.Cost.TotalAPIDurationMs, cached.Cost.TotalAPIDurationMs)
	merged.Cost.TotalLinesAdded = cmp.Or(merged.Cost.TotalLinesAdded, cached.Cost.TotalLinesAdded)
	merged.Cost.TotalLinesRemoved = cmp.Or(merged.Cost.TotalLinesRemoved, cached.Cost.TotalLinesRemoved)

	merged.CWD = cmp.Or(merged.CWD, cached.CWD)
	merged.SessionName = cmp.Or(merged.SessionName, cached.SessionName)

	return &merged
}
