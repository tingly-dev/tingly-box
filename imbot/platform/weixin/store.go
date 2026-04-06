// Package weixin provides Weixin platform bot implementation for ImBot.
package weixin

import (
	"fmt"
	"sync"

	"github.com/tingly-dev/weixin/types"
)

// MemoryStore is an in-memory account store for Weixin.
// This can be replaced with a database-backed store in the future.
// For now, it provides a simple in-memory cache that can be integrated
// with the imbot database layer.
type MemoryStore struct {
	mu       sync.RWMutex
	accounts map[string]*types.WeChatAccount
}

// NewMemoryStore creates a new in-memory account store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		accounts: make(map[string]*types.WeChatAccount),
	}
}

// Save stores an account in memory.
func (s *MemoryStore) Save(account *types.WeChatAccount) error {
	if account == nil {
		return fmt.Errorf("account is nil")
	}
	if account.ID == "" {
		return fmt.Errorf("account ID is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Create a copy to avoid external modifications
	accountCopy := *account
	s.accounts[account.ID] = &accountCopy

	return nil
}

// Get retrieves an account by ID from memory.
func (s *MemoryStore) Get(accountID string) (*types.WeChatAccount, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if accountID == "" {
		return nil, fmt.Errorf("account ID is required")
	}

	account, exists := s.accounts[accountID]
	if !exists {
		return nil, fmt.Errorf("account not found: %s", accountID)
	}

	// Return a copy to avoid external modifications
	accountCopy := *account
	return &accountCopy, nil
}

// ListIDs returns all account IDs from memory.
func (s *MemoryStore) ListIDs() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.accounts))
	for id := range s.accounts {
		ids = append(ids, id)
	}
	return ids, nil
}

// Delete removes an account from memory.
func (s *MemoryStore) Delete(accountID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if accountID == "" {
		return fmt.Errorf("account ID is required")
	}

	delete(s.accounts, accountID)
	return nil
}

// Update updates an existing account in memory.
func (s *MemoryStore) Update(account *types.WeChatAccount) error {
	return s.Save(account)
}

// Has checks if an account exists in memory.
func (s *MemoryStore) Has(accountID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, exists := s.accounts[accountID]
	return exists
}
