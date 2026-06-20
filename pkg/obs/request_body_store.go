package obs

import (
	"strconv"
	"sync"
)

// RequestBodyStore stores, in an in-memory circular buffer, the request body
// together with the error response it produced. Each entry is indexed by a
// unique ID for retrieval.
//
// This is designed for debugging and troubleshooting: bodies are stored in
// memory only (no disk persistence) and automatically discarded when the
// buffer is full. Keeping bodies here — referenced from the access log by a
// single body_ref ID rather than inlined into the log entry — caps total body
// memory at this store's capacity, independent of the much larger log ring
// buffer that would otherwise retain a copy of every captured body.
type RequestBodyStore struct {
	// Circular buffer storing request bodies
	bodies map[string]*RequestBodyEntry
	// Circular queue of IDs for LRU eviction
	ids      []string
	writeIdx int
	maxSize  int
	entrySeq int64 // sequence counter for generating IDs
	mu       sync.RWMutex
}

// RequestBodyEntry represents a stored request together with the error
// response it produced. Both bodies are captured only for error responses and
// retrieved together by a single ID.
type RequestBodyEntry struct {
	ID                string // Unique identifier (e.g., "req_1234567890")
	Method            string // HTTP method
	Path              string // Request path
	RequestBody       string // Request body (may be truncated)
	RequestTruncated  bool   // True if the request body was truncated due to size limits
	ResponseBody      string // Error response body (may be truncated)
	ResponseTruncated bool   // True if the response body was truncated due to size limits
}

// NewRequestBodyStore creates a new request body store with the specified capacity.
func NewRequestBodyStore(maxSize int) *RequestBodyStore {
	return &RequestBodyStore{
		bodies:   make(map[string]*RequestBodyEntry, maxSize),
		ids:      make([]string, maxSize),
		writeIdx: 0,
		maxSize:  maxSize,
		entrySeq: 0,
	}
}

// Store stores a request/response body pair and returns its unique ID.
// Each body is independently truncated to maxBodySize. If the buffer is full,
// the oldest entry is evicted.
func (s *RequestBodyStore) Store(method, path, requestBody, responseBody string, maxBodySize int) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Generate unique ID
	s.entrySeq++
	id := generateRequestID(s.entrySeq)

	// Truncate each body if too large (keep first N chars)
	requestBody, reqTruncated := truncateBody(requestBody, maxBodySize)
	responseBody, respTruncated := truncateBody(responseBody, maxBodySize)

	entry := &RequestBodyEntry{
		ID:                id,
		Method:            method,
		Path:              path,
		RequestBody:       requestBody,
		RequestTruncated:  reqTruncated,
		ResponseBody:      responseBody,
		ResponseTruncated: respTruncated,
	}

	// Calculate storage index (circular)
	idx := s.writeIdx % s.maxSize

	// Evict oldest entry if buffer is full
	if len(s.ids) >= s.maxSize && idx < len(s.ids) {
		oldID := s.ids[idx]
		delete(s.bodies, oldID)
	}

	// Store ID in circular buffer
	if idx < len(s.ids) {
		s.ids[idx] = id
	} else {
		s.ids = append(s.ids, id)
	}
	s.bodies[id] = entry
	s.writeIdx++

	return id
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

	s.bodies = make(map[string]*RequestBodyEntry, s.maxSize)
	s.ids = make([]string, s.maxSize)
	s.writeIdx = 0
}

// Size returns the current number of stored entries.
func (s *RequestBodyStore) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.bodies)
}

// truncateBody clips body to maxBodySize, reporting whether it was truncated.
func truncateBody(body string, maxBodySize int) (string, bool) {
	if len(body) > maxBodySize {
		return body[:maxBodySize], true
	}
	return body, false
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
