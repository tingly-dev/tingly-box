package statusline

import "sync"

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

	merged.Model.DisplayName = mergeIfEmpty(merged.Model.DisplayName, cached.Model.DisplayName)
	merged.Model.ID = mergeIfEmpty(merged.Model.ID, cached.Model.ID)

	merged.ContextWindow.UsedPercentage = mergeIfZero(merged.ContextWindow.UsedPercentage, cached.ContextWindow.UsedPercentage)
	merged.ContextWindow.ContextWindowSize = mergeIfZero(merged.ContextWindow.ContextWindowSize, cached.ContextWindow.ContextWindowSize)
	merged.ContextWindow.TotalInputTokens = mergeIfZero(merged.ContextWindow.TotalInputTokens, cached.ContextWindow.TotalInputTokens)
	merged.ContextWindow.TotalOutputTokens = mergeIfZero(merged.ContextWindow.TotalOutputTokens, cached.ContextWindow.TotalOutputTokens)

	merged.Cost.TotalCostUSD = mergeIfZero(merged.Cost.TotalCostUSD, cached.Cost.TotalCostUSD)
	merged.Cost.TotalDurationMs = mergeIfZero(merged.Cost.TotalDurationMs, cached.Cost.TotalDurationMs)
	merged.Cost.TotalAPIDurationMs = mergeIfZero(merged.Cost.TotalAPIDurationMs, cached.Cost.TotalAPIDurationMs)
	merged.Cost.TotalLinesAdded = mergeIfZero(merged.Cost.TotalLinesAdded, cached.Cost.TotalLinesAdded)
	merged.Cost.TotalLinesRemoved = mergeIfZero(merged.Cost.TotalLinesRemoved, cached.Cost.TotalLinesRemoved)

	return &merged
}

func mergeIfEmpty(target, cached string) string {
	if target == "" && cached != "" {
		return cached
	}
	return target
}

func mergeIfZero[T int | int64 | float64](target, cached T) T {
	if target == 0 && cached != 0 {
		return cached
	}
	return target
}
