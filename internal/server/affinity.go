package server

import (
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/internal/server/routing"
)

const (
	defaultAffinityTTL = 2 * time.Hour
	gcInterval         = 30 * time.Minute
)

// AffinityStore maps (ruleUUID, sessionID) -> locked Service.
// It directly implements routing.AffinityStore interface.
type AffinityStore struct {
	mu      sync.RWMutex
	entries map[string]*routing.AffinityEntry // key: ruleUUID+":"+sessionID
	ttl     time.Duration
}

// NewAffinityStore creates a new affinity store with the given TTL
func NewAffinityStore(ttl time.Duration) *AffinityStore {
	if ttl <= 0 {
		ttl = defaultAffinityTTL
	}
	return &AffinityStore{
		entries: make(map[string]*routing.AffinityEntry),
		ttl:     ttl,
	}
}

// makeKey creates the composite key for the store
func (s *AffinityStore) makeKey(ruleUUID, sessionID string) string {
	return ruleUUID + ":" + sessionID
}

// Get retrieves an affinity entry for the given rule and session
func (s *AffinityStore) Get(ruleUUID, sessionID string) (*routing.AffinityEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := s.makeKey(ruleUUID, sessionID)
	entry, ok := s.entries[key]
	if !ok {
		return nil, false
	}

	// Check if expired
	if time.Now().After(entry.ExpiresAt) {
		return nil, false
	}

	return entry, true
}

// Set stores an affinity entry for the given rule and session
func (s *AffinityStore) Set(ruleUUID, sessionID string, entry *routing.AffinityEntry) {
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
