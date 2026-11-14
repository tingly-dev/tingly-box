package db

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"
)

// defaultPerfCacheTTL is how long a GetPerformanceSummary result is reused.
//
// The summary is percentiles over a whole window (30 days of successful
// requests read row by row), so it is the dashboard's most expensive query,
// and the dashboard re-asks it with identical parameters on every reload,
// filter toggle and auto-refresh tick. Percentiles over thousands of requests
// don't move visibly in seconds, so a short TTL trades nothing the user can
// see for skipping that scan. Counts and sums (GetAggregatedStats,
// GetTimeSeries) are deliberately NOT cached: they are cheap after the
// usage_daily rollup and the live "today" view should tick with traffic.
const defaultPerfCacheTTL = 15 * time.Second

// perfCacheMax bounds the entry count; the key space is the handful of
// (window, filter) combinations the dashboard produces.
const perfCacheMax = 64

type perfCacheEntry struct {
	summary PerformanceSummary
	at      time.Time
}

// perfCache is a small TTL cache for performance summaries. The zero value is
// ready to use; a non-positive ttl disables it.
type perfCache struct {
	mu      sync.Mutex
	entries map[string]perfCacheEntry
}

func perfCacheKey(start, end time.Time, filters map[string]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d|%d", start.UnixNano(), end.UnixNano())
	for _, k := range slices.Sorted(maps.Keys(filters)) {
		fmt.Fprintf(&b, "|%s=%s", k, filters[k])
	}
	return b.String()
}

func (c *perfCache) get(key string, ttl time.Duration) (PerformanceSummary, bool) {
	if ttl <= 0 {
		return PerformanceSummary{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || time.Since(e.at) > ttl {
		return PerformanceSummary{}, false
	}
	return e.summary, true
}

func (c *perfCache) put(key string, summary PerformanceSummary, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]perfCacheEntry)
	}
	if len(c.entries) >= perfCacheMax {
		for k, e := range c.entries {
			if time.Since(e.at) > ttl {
				delete(c.entries, k)
			}
		}
		if len(c.entries) >= perfCacheMax {
			clear(c.entries)
		}
	}
	c.entries[key] = perfCacheEntry{summary: summary, at: time.Now()}
}

func (c *perfCache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.entries)
}
