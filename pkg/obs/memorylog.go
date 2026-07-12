package obs

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// MemoryLogHook is a logrus hook that stores log entries in an in-memory circular buffer.
type MemoryLogHook struct {
	// Circular buffer storing log entries in chronological order
	entries []*logrus.Entry
	// Current write position (circular)
	writeIdx int
	// Maximum number of entries to store
	maxEntries int
	// Current entry count (less than maxEntries when not full)
	count int
	// Output writers for tee functionality
	writers []io.Writer
	mu      sync.RWMutex
}

// NewMemoryLogHook creates a new memory log hook with the specified maximum capacity.
func NewMemoryLogHook(maxEntries int) *MemoryLogHook {
	return &MemoryLogHook{
		entries:    make([]*logrus.Entry, maxEntries),
		maxEntries: maxEntries,
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

// Fire processes each log entry.
func (h *MemoryLogHook) Fire(entry *logrus.Entry) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Store a DETACHED copy, not the entry's own values. Entries retained in
	// this ring outlive their request, and field values frequently alias
	// request-scoped storage: gjson-parsed strings are substrings of the full
	// raw request body, so one retained model-name field would pin megabytes
	// until the ring rotates (50 entries × multi-MB agent bodies is a real
	// OOM vector — surfaced by the duo memory harness). Context is dropped
	// for the same reason: it chains to the live *http.Request.
	copied := &logrus.Entry{
		Logger:  entry.Logger,
		Data:    make(logrus.Fields, len(entry.Data)),
		Time:    entry.Time,
		Level:   entry.Level,
		Message: strings.Clone(entry.Message),
	}
	for k, v := range entry.Data {
		copied.Data[strings.Clone(k)] = detachValue(v)
	}

	// Rotate: circular buffer automatically overwrites oldest entry
	h.entries[h.writeIdx] = copied
	h.writeIdx = (h.writeIdx + 1) % h.maxEntries
	if h.count < h.maxEntries {
		h.count++
	}

	// Tee: output to all registered writers
	msg, err := entry.String()
	if err == nil {
		for _, w := range h.writers {
			w.Write([]byte(msg))
		}
	}

	return nil
}

// detachValue returns v with its string storage detached from whatever
// backing array it aliases. Primitives pass through; strings are cloned;
// errors are rendered (an error value can pin a whole wrapped chain, and
// most render as {} under JSON anyway); everything composite is re-encoded
// as json.RawMessage — the readers of these entries (the /api/v1 log and
// request-timeline handlers) serialize them to JSON, so the wire shape is
// unchanged while every string inside gets fresh storage.
func detachValue(v any) any {
	switch t := v.(type) {
	case nil, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64,
		time.Time, time.Duration:
		return v
	case string:
		return strings.Clone(t)
	case error:
		return strings.Clone(t.Error())
	default:
		if b, err := json.Marshal(v); err == nil {
			return json.RawMessage(b)
		}
		return fmt.Sprintf("%v", v)
	}
}

// GetEntries returns all log entries in chronological order.
func (h *MemoryLogHook) GetEntries() []*logrus.Entry {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.count == 0 {
		return []*logrus.Entry{}
	}

	result := make([]*logrus.Entry, 0, h.count)
	if h.count < h.maxEntries {
		// Buffer not full, return from 0 to count
		for i := 0; i < h.count; i++ {
			result = append(result, h.entries[i])
		}
	} else {
		// Buffer full, start from writeIdx (oldest entry)
		for i := 0; i < h.maxEntries; i++ {
			idx := (h.writeIdx + i) % h.maxEntries
			result = append(result, h.entries[idx])
		}
	}
	return result
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
	for i := range h.entries {
		h.entries[i] = nil
	}
}

// Size returns the current number of stored log entries.
func (h *MemoryLogHook) Size() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.count
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

// getOrderedEntries returns all log entries in chronological order.
func (h *MemoryLogHook) getOrderedEntries() []*logrus.Entry {
	if h.count == 0 {
		return []*logrus.Entry{}
	}

	result := make([]*logrus.Entry, 0, h.count)
	if h.count < h.maxEntries {
		for i := 0; i < h.count; i++ {
			result = append(result, h.entries[i])
		}
	} else {
		for i := 0; i < h.maxEntries; i++ {
			idx := (h.writeIdx + i) % h.maxEntries
			result = append(result, h.entries[idx])
		}
	}
	return result
}
