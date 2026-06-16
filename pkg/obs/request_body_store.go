package obs

import (
	"strconv"
	"sync"
)

// RequestBodyStore stores request bodies in an in-memory FIFO buffer.
// Each entry is indexed by a unique ID for retrieval.
//
// This is designed for debugging and troubleshooting: request bodies are
// stored in memory only (no disk persistence) and automatically discarded
// when either bound is exceeded.
//
// The store is bounded on TWO axes (mirroring MemoryLogHook):
//   - maxCount: how many request bodies to retain.
//   - maxBytes: a total-byte budget across retained bodies.
//
// A single body is truncated only when it alone exceeds the whole byte budget
// (the "capacity is still insufficient" fallback); otherwise bodies are kept
// whole and the oldest are evicted to stay within both bounds.
type RequestBodyStore struct {
	// Map from ID to stored entry.
	bodies map[string]*RequestBodyEntry
	// FIFO order of IDs (front = oldest), used for eviction.
	order []string
	// maxCount bounds the number of retained entries (<=0 = unlimited count).
	maxCount int
	// maxBytes bounds the total bytes of retained entries (<=0 = unlimited bytes).
	maxBytes int
	// Running sum of retained entry sizes.
	bytes    int
	entrySeq int64 // sequence counter for generating IDs
	mu       sync.RWMutex
}

// RequestBodyEntry represents a stored request body with metadata
type RequestBodyEntry struct {
	ID        string // Unique identifier (e.g., "req_1234567890")
	Method    string // HTTP method
	Path      string // Request path
	Body      string // Request body (may be truncated)
	Truncated bool   // True if body was truncated due to size limits
}

func (e *RequestBodyEntry) size() int {
	if e == nil {
		return 0
	}
	return len(e.Body) + len(e.Method) + len(e.Path) + len(e.ID)
}

// NewRequestBodyStore creates a new request body store bounded by a count cap
// (maxCount) and a total-byte budget (maxBytes; <=0 disables the byte budget).
func NewRequestBodyStore(maxCount, maxBytes int) *RequestBodyStore {
	return &RequestBodyStore{
		bodies:   make(map[string]*RequestBodyEntry),
		maxCount: maxCount,
		maxBytes: maxBytes,
	}
}

// Store stores a request body and returns its unique ID. Oldest entries are
// evicted to keep the store within both the count and byte bounds. If the body
// alone exceeds the whole byte budget it is truncated (Truncated=true).
func (s *RequestBodyStore) Store(method, path, body string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Generate unique ID
	s.entrySeq++
	id := generateRequestID(s.entrySeq)

	// Last-resort truncation: only when the body alone can't fit the budget.
	truncated := false
	if s.maxBytes > 0 && len(body) > s.maxBytes {
		body = body[:s.maxBytes]
		truncated = true
	}

	entry := &RequestBodyEntry{
		ID:        id,
		Method:    method,
		Path:      path,
		Body:      body,
		Truncated: truncated,
	}

	s.bodies[id] = entry
	s.order = append(s.order, id)
	s.bytes += entry.size()

	// Evict oldest until within both bounds, always keeping the new entry.
	for len(s.order) > 1 && s.overBudget() {
		oldID := s.order[0]
		s.order = s.order[1:]
		if old, ok := s.bodies[oldID]; ok {
			s.bytes -= old.size()
			delete(s.bodies, oldID)
		}
	}

	return id
}

// overBudget reports whether either bound is currently exceeded.
func (s *RequestBodyStore) overBudget() bool {
	if s.maxCount > 0 && len(s.order) > s.maxCount {
		return true
	}
	if s.maxBytes > 0 && s.bytes > s.maxBytes {
		return true
	}
	return false
}

// Get retrieves a request body by ID.
// Returns nil if the ID is not found (may have been evicted).
func (s *RequestBodyStore) Get(id string) *RequestBodyEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.bodies[id]
}

// Clear removes all entries from the store.
func (s *RequestBodyStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.bodies = make(map[string]*RequestBodyEntry)
	s.order = s.order[:0]
	s.bytes = 0
}

// Size returns the current number of stored entries.
func (s *RequestBodyStore) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.bodies)
}

// Bytes returns the estimated total byte footprint of the retained bodies.
func (s *RequestBodyStore) Bytes() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bytes
}

// generateRequestID generates a unique request ID from a sequence number.
func generateRequestID(seq int64) string {
	// Use a simple prefix + decimal sequence for readability
	// Format: req_<seq>
	return "req_" + formatSeq(seq)
}

// formatSeq formats a sequence number as a compact string.
func formatSeq(seq int64) string {
	return strconv.FormatInt(seq, 10)
}
