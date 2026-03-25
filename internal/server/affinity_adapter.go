package server

import (
	"github.com/tingly-dev/tingly-box/internal/server/routing"
)

// affinityStoreAdapter adapts server.AffinityStore to routing.AffinityStore interface
type affinityStoreAdapter struct {
	store *AffinityStore
}

// Get retrieves an affinity entry, converting from server.AffinityEntry to routing.AffinityEntry
func (a *affinityStoreAdapter) Get(ruleUUID, sessionID string) (*routing.AffinityEntry, bool) {
	entry, ok := a.store.Get(ruleUUID, sessionID)
	if !ok {
		return nil, false
	}

	// Convert server.AffinityEntry to routing.AffinityEntry
	return &routing.AffinityEntry{
		Service:   entry.Service,
		MessageID: entry.MessageID,
		LockedAt:  entry.LockedAt,
		ExpiresAt: entry.ExpiresAt,
	}, true
}

// Set stores an affinity entry, converting from routing.AffinityEntry to server.AffinityEntry
func (a *affinityStoreAdapter) Set(ruleUUID, sessionID string, entry *routing.AffinityEntry) {
	a.store.Set(ruleUUID, sessionID, &AffinityEntry{
		Service:   entry.Service,
		MessageID: entry.MessageID,
		LockedAt:  entry.LockedAt,
		ExpiresAt: entry.ExpiresAt,
	})
}

// newAffinityStoreAdapter creates a new adapter
func newAffinityStoreAdapter(store *AffinityStore) routing.AffinityStore {
	return &affinityStoreAdapter{store: store}
}
