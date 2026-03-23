package server

import (
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
)

const (
	defaultAffinityTTL = 2 * time.Hour
	gcInterval         = 30 * time.Minute
)

// AffinityEntry represents a locked service for a session
type AffinityEntry struct {
	Service   *loadbalance.Service
	MessageID string // Last Anthropic message ID seen for this session
	LockedAt  time.Time
	ExpiresAt time.Time
}

// IsExpired checks if the affinity entry has expired
func (e *AffinityEntry) IsExpired() bool {
	return time.Now().After(e.ExpiresAt)
}

// AffinityStore maps (ruleUUID, sessionID) -> locked Service
type AffinityStore struct {
	mu      sync.RWMutex
	entries map[string]*AffinityEntry // key: ruleUUID+":"+sessionID
	ttl     time.Duration
}

// NewAffinityStore creates a new affinity store with the given TTL
func NewAffinityStore(ttl time.Duration) *AffinityStore {
	if ttl <= 0 {
		ttl = defaultAffinityTTL
	}
	return &AffinityStore{
		entries: make(map[string]*AffinityEntry),
		ttl:     ttl,
	}
}

// makeKey creates the composite key for the store
func (s *AffinityStore) makeKey(ruleUUID, sessionID string) string {
	return ruleUUID + ":" + sessionID
}

// Get retrieves an affinity entry for the given rule and session
func (s *AffinityStore) Get(ruleUUID, sessionID string) (*AffinityEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := s.makeKey(ruleUUID, sessionID)
	entry, ok := s.entries[key]
	if !ok {
		return nil, false
	}

	// Check if expired
	if entry.IsExpired() {
		return nil, false
	}

	return entry, true
}

// Set stores an affinity entry for the given rule and session
func (s *AffinityStore) Set(ruleUUID, sessionID string, entry *AffinityEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.makeKey(ruleUUID, sessionID)
	s.entries[key] = entry
}

// UpdateMessageID updates the message ID for an existing affinity entry
func (s *AffinityStore) UpdateMessageID(ruleUUID, sessionID, messageID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.makeKey(ruleUUID, sessionID)
	if entry, ok := s.entries[key]; ok {
		entry.MessageID = messageID
	}
}

// Delete removes an affinity entry for the given rule and session
func (s *AffinityStore) Delete(ruleUUID, sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := s.makeKey(ruleUUID, sessionID)
	delete(s.entries, key)
}

// DeleteByRule removes all affinity entries for a given rule UUID
func (s *AffinityStore) DeleteByRule(ruleUUID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	prefix := ruleUUID + ":"
	for key := range s.entries {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			delete(s.entries, key)
		}
	}
}

// GC removes expired entries from the store
func (s *AffinityStore) GC() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for key, entry := range s.entries {
		if now.After(entry.ExpiresAt) {
			delete(s.entries, key)
		}
	}
}

// StartGC starts a background goroutine that periodically cleans up expired entries
func (s *AffinityStore) StartGC() {
	go func() {
		ticker := time.NewTicker(gcInterval)
		defer ticker.Stop()
		for range ticker.C {
			s.GC()
		}
	}()
}

// ResolveSessionID returns the best available session identifier from the request.
// Priority: Anthropic metadata.user_id > X-Tingly-Session-ID header > ClientIP
func ResolveSessionID(c *gin.Context, req interface{}) string {
	// 1. Extract from Anthropic request metadata.user_id
	switch r := req.(type) {
	case *anthropic.MessageNewParams:
		if r.Metadata.UserID.Valid() && r.Metadata.UserID.Value != "" {
			return "user:" + r.Metadata.UserID.Value
		}
	case *anthropic.BetaMessageNewParams:
		if r.Metadata.UserID.Valid() && r.Metadata.UserID.Value != "" {
			return "user:" + r.Metadata.UserID.Value
		}
	}

	// 2. X-Tingly-Session-ID header
	if id := c.GetHeader("X-Tingly-Session-ID"); id != "" {
		return "hdr:" + id
	}

	// 3. Fallback: client IP
	return "ip:" + c.ClientIP()
}
