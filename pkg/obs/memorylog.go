package obs

import (
	"io"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// MemoryLogHook is a logrus hook that stores log entries in an in-memory ring.
//
// It is bounded on two axes: maxEntries (ring capacity) and maxBytes (total-byte
// budget; <=0 disables). Entries inline large fields (e.g. response_body), so the
// count cap alone doesn't bound memory; when bytes exceed the budget the oldest
// entries are evicted until it fits, always keeping the just-written entry.
type MemoryLogHook struct {
	// Circular buffer storing log entries in chronological order
	entries []*logrus.Entry
	// Parallel array of per-entry byte estimates (same index as entries),
	// so eviction can reclaim bytes without re-walking entry fields.
	sizes []int
	// Current write position (circular)
	writeIdx int
	// Maximum number of entries to store
	maxEntries int
	// Current entry count (less than maxEntries when not full)
	count int
	// Total-byte budget across retained entries (<=0 disables)
	maxBytes int
	// Running sum of sizes for the currently retained entries
	bytes int
	// Output writers for tee functionality
	writers []io.Writer
	mu      sync.RWMutex
}

// NewMemoryLogHook creates a new memory log hook bounded by both a count cap
// (maxEntries) and a total-byte budget (maxBytes; <=0 disables the byte budget).
func NewMemoryLogHook(maxEntries, maxBytes int) *MemoryLogHook {
	return &MemoryLogHook{
		entries:    make([]*logrus.Entry, maxEntries),
		sizes:      make([]int, maxEntries),
		maxEntries: maxEntries,
		maxBytes:   maxBytes,
		writers:    make([]io.Writer, 0),
	}
}

// AddWriter adds a writer for tee output functionality.
func (h *MemoryLogHook) AddWriter(w io.Writer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.writers = append(h.writers, w)
}

// Levels returns the log levels this hook processes.
func (h *MemoryLogHook) Levels() []logrus.Level {
	return logrus.AllLevels[:]
}

// entrySize estimates an entry's in-memory footprint. It is an approximation:
// the message and string/[]byte field values dominate (notably response_body),
// with a small fixed overhead per entry and per field for the struct/map/keys.
func entrySize(e *logrus.Entry) int {
	if e == nil {
		return 0
	}
	n := len(e.Message) + 64 // base overhead (struct + time + map header)
	for k, v := range e.Data {
		n += len(k) + 16
		switch s := v.(type) {
		case string:
			n += len(s)
		case []byte:
			n += len(s)
		default:
			n += 16
		}
	}
	return n
}

// Fire processes each log entry.
func (h *MemoryLogHook) Fire(entry *logrus.Entry) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Tee even when the ring has zero capacity; just don't store.
	if h.maxEntries <= 0 {
		h.tee(entry)
		return nil
	}

	// Deep copy entry to avoid subsequent modifications affecting stored logs
	copied := &logrus.Entry{
		Logger:  entry.Logger,
		Data:    make(logrus.Fields, len(entry.Data)),
		Time:    entry.Time,
		Level:   entry.Level,
		Message: entry.Message,
	}
	for k, v := range entry.Data {
		copied.Data[k] = v
	}
	sz := entrySize(copied)

	// Reclaim bytes of the slot we are about to overwrite (ring full case).
	if h.entries[h.writeIdx] != nil {
		h.bytes -= h.sizes[h.writeIdx]
	}

	// Rotate: circular buffer automatically overwrites oldest entry
	h.entries[h.writeIdx] = copied
	h.sizes[h.writeIdx] = sz
	h.bytes += sz
	h.writeIdx = (h.writeIdx + 1) % h.maxEntries
	if h.count < h.maxEntries {
		h.count++
	}

	// Byte-budget eviction: drop oldest entries until within budget, but always
	// keep at least the entry we just wrote (a single oversized entry is retained
	// rather than producing an empty ring).
	if h.maxBytes > 0 {
		for h.bytes > h.maxBytes && h.count > 1 {
			oldest := (h.writeIdx - h.count + h.maxEntries) % h.maxEntries
			h.bytes -= h.sizes[oldest]
			h.entries[oldest] = nil
			h.sizes[oldest] = 0
			h.count--
		}
	}

	h.tee(entry)
	return nil
}

// tee writes the entry's rendered form to all registered writers. Caller holds h.mu.
func (h *MemoryLogHook) tee(entry *logrus.Entry) {
	if len(h.writers) == 0 {
		return
	}
	msg, err := entry.String()
	if err != nil {
		return
	}
	for _, w := range h.writers {
		w.Write([]byte(msg))
	}
}

// GetEntries returns all log entries in chronological order.
func (h *MemoryLogHook) GetEntries() []*logrus.Entry {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.getOrderedEntries()
}

// GetEntriesSince returns log entries after the specified time.
func (h *MemoryLogHook) GetEntriesSince(since time.Time) []*logrus.Entry {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]*logrus.Entry, 0)
	entries := h.getOrderedEntries()
	for _, e := range entries {
		if e.Time.After(since) {
			result = append(result, e)
		}
	}
	return result
}

// GetEntriesByLevel returns log entries matching the specified level.
func (h *MemoryLogHook) GetEntriesByLevel(level logrus.Level) []*logrus.Entry {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]*logrus.Entry, 0)
	entries := h.getOrderedEntries()
	for _, e := range entries {
		if e.Level == level {
			result = append(result, e)
		}
	}
	return result
}

// GetEntriesByLevelRange returns log entries within the specified level range.
func (h *MemoryLogHook) GetEntriesByLevelRange(minLevel, maxLevel logrus.Level) []*logrus.Entry {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]*logrus.Entry, 0)
	entries := h.getOrderedEntries()
	for _, e := range entries {
		if e.Level >= minLevel && e.Level <= maxLevel {
			result = append(result, e)
		}
	}
	return result
}

// Clear removes all log entries.
func (h *MemoryLogHook) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.writeIdx = 0
	h.count = 0
	h.bytes = 0
	for i := range h.entries {
		h.entries[i] = nil
		h.sizes[i] = 0
	}
}

// Size returns the current number of stored log entries.
func (h *MemoryLogHook) Size() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.count
}

// Bytes returns the estimated total byte footprint of the retained entries.
func (h *MemoryLogHook) Bytes() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.bytes
}

// GetLatest returns the newest N log entries.
func (h *MemoryLogHook) GetLatest(n int) []*logrus.Entry {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.count == 0 || n <= 0 {
		return []*logrus.Entry{}
	}

	if n > h.count {
		n = h.count
	}

	entries := h.getOrderedEntries()
	return entries[h.count-n:]
}

// getOrderedEntries returns all retained log entries in chronological order.
// The retained entries always occupy a contiguous run of `count` slots ending
// just before writeIdx, so the oldest index is derived uniformly — this holds
// both for a not-yet-full ring and after byte-budget eviction trimmed the front.
func (h *MemoryLogHook) getOrderedEntries() []*logrus.Entry {
	if h.count == 0 || h.maxEntries == 0 {
		return []*logrus.Entry{}
	}

	oldest := (h.writeIdx - h.count + h.maxEntries) % h.maxEntries
	result := make([]*logrus.Entry, 0, h.count)
	for i := 0; i < h.count; i++ {
		idx := (oldest + i) % h.maxEntries
		if e := h.entries[idx]; e != nil {
			result = append(result, e)
		}
	}
	return result
}
