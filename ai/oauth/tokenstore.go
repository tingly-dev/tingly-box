package oauth

import (
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
)

// TokenStorage defines the interface for storing and retrieving OAuth tokens
type TokenStorage interface {
	// SaveToken saves a token for the given user and issuer
	SaveToken(userID string, issuer ai.Issuer, token *Token) error

	// GetToken retrieves a token for the given user and issuer
	GetToken(userID string, issuer ai.Issuer) (*Token, error)

	// DeleteToken removes a token for the given user and issuer
	DeleteToken(userID string, issuer ai.Issuer) error

	// ListIssuers returns all providers that have tokens for the user
	ListIssuers(userID string) ([]ai.Issuer, error)

	// CleanupExpired removes all expired tokens from the storage
	CleanupExpired() error
}

// MemoryTokenStorage is an in-memory implementation of TokenStorage
type MemoryTokenStorage struct {
	mu     sync.RWMutex
	tokens map[string]map[ai.Issuer]*Token // userID -> issuer -> token
}

// NewMemoryTokenStorage creates a new in-memory token storage
func NewMemoryTokenStorage() *MemoryTokenStorage {
	return &MemoryTokenStorage{
		tokens: make(map[string]map[ai.Issuer]*Token),
	}
}

// SaveToken saves a token for the given user and issuer
func (s *MemoryTokenStorage) SaveToken(userID string, issuer ai.Issuer, token *Token) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.tokens[userID] == nil {
		s.tokens[userID] = make(map[ai.Issuer]*Token)
	}

	s.tokens[userID][issuer] = token
	return nil
}

// GetToken retrieves a token for the given user and issuer
func (s *MemoryTokenStorage) GetToken(userID string, issuer ai.Issuer) (*Token, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.tokens[userID] == nil {
		return nil, ErrTokenNotFound
	}

	token, ok := s.tokens[userID][issuer]
	if !ok || token == nil {
		return nil, ErrTokenNotFound
	}

	return token, nil
}

// DeleteToken removes a token for the given user and issuer
func (s *MemoryTokenStorage) DeleteToken(userID string, issuer ai.Issuer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.tokens[userID] == nil {
		return ErrTokenNotFound
	}

	if _, ok := s.tokens[userID][issuer]; !ok {
		return ErrTokenNotFound
	}

	delete(s.tokens[userID], issuer)
	return nil
}

// CleanupExpired removes all expired tokens from the storage
func (s *MemoryTokenStorage) CleanupExpired() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for userID, providerTokens := range s.tokens {
		for issuer, token := range providerTokens {
			if !token.Expiry.IsZero() && now.After(token.Expiry) {
				delete(providerTokens, issuer)
			}
		}
		if len(s.tokens[userID]) == 0 {
			delete(s.tokens, userID)
		}
	}
	return nil
}

// ListIssuers returns all providers that have tokens for the user
func (s *MemoryTokenStorage) ListIssuers(userID string) ([]ai.Issuer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.tokens[userID] == nil {
		return []ai.Issuer{}, nil
	}

	providers := make([]ai.Issuer, 0, len(s.tokens[userID]))
	for issuer := range s.tokens[userID] {
		providers = append(providers, issuer)
	}

	return providers, nil
}
