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
	// gen is bumped by reset so a summary computed before a delete/rename is
	// not stored after it (the query runs outside this mutex).
	gen uint64
}

func (c *perfCache) generation() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gen
}

// perfCacheKey identifies a request. The window bounds are truncated to the
// TTL: the dashboard's "today" range ends at the current second, so an exact
// key would differ on every refresh and never hit. A hit may therefore answer
// for a window up to one TTL shorter, which is within the staleness already
// accepted.
func perfCacheKey(start, end time.Time, filters map[string]string, ttl time.Duration) string {
	if ttl > 0 {
		start, end = start.Truncate(ttl), end.Truncate(ttl)
	}
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

func (c *perfCache) put(key string, summary PerformanceSummary, ttl time.Duration, gen uint64) {
	if ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if gen != c.gen {
		return
	}
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
	c.gen++
	clear(c.entries)
}
