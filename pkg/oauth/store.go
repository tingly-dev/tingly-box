package oauth

import (
	"sync"
	"time"
)

// TokenStorage defines the interface for storing and retrieving OAuth tokens
type TokenStorage interface {
	// SaveToken saves a token for the given user and provider
	SaveToken(userID string, provider ProviderType, token *Token) error

	// GetToken retrieves a token for the given user and provider
	GetToken(userID string, provider ProviderType) (*Token, error)

	// DeleteToken removes a token for the given user and provider
	DeleteToken(userID string, provider ProviderType) error

	// CleanupExpired removes all expired tokens from the storage
	CleanupExpired() error
}

// MemoryTokenStorage is an in-memory implementation of TokenStorage
type MemoryTokenStorage struct {
	mu     sync.RWMutex
	tokens map[string]map[ProviderType]*Token // userID -> provider -> token
}

// NewMemoryTokenStorage creates a new in-memory token storage
func NewMemoryTokenStorage() *MemoryTokenStorage {
	return &MemoryTokenStorage{
		tokens: make(map[string]map[ProviderType]*Token),
	}
}

// SaveToken saves a token for the given user and provider
func (s *MemoryTokenStorage) SaveToken(userID string, provider ProviderType, token *Token) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.tokens[userID] == nil {
		s.tokens[userID] = make(map[ProviderType]*Token)
	}

	s.tokens[userID][provider] = token
	return nil
}

// GetToken retrieves a token for the given user and provider
func (s *MemoryTokenStorage) GetToken(userID string, provider ProviderType) (*Token, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.tokens[userID] == nil {
		return nil, ErrTokenNotFound
	}

	token, ok := s.tokens[userID][provider]
	if !ok || token == nil {
		return nil, ErrTokenNotFound
	}

	return token, nil
}

// DeleteToken removes a token for the given user and provider
func (s *MemoryTokenStorage) DeleteToken(userID string, provider ProviderType) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.tokens[userID] == nil {
		return ErrTokenNotFound
	}

	if _, ok := s.tokens[userID][provider]; !ok {
		return ErrTokenNotFound
	}

	delete(s.tokens[userID], provider)
	return nil
}

// CleanupExpired removes all expired tokens from the storage
func (s *MemoryTokenStorage) CleanupExpired() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for userID, providerTokens := range s.tokens {
		for provider, token := range providerTokens {
			if !token.Expiry.IsZero() && now.After(token.Expiry) {
				delete(providerTokens, provider)
			}
		}
		if len(s.tokens[userID]) == 0 {
			delete(s.tokens, userID)
		}
	}
	return nil
}

// TokenWithMetadata represents a token with additional metadata
type TokenWithMetadata struct {
	Token     *Token
	UserID    string
	Provider  ProviderType
	CreatedAt time.Time
	UpdatedAt time.Time
}

// MetadataTokenStorage extends TokenStorage with metadata support
type MetadataTokenStorage interface {
	TokenStorage

	// SaveTokenWithMetadata saves a token with additional metadata
	SaveTokenWithMetadata(userID string, provider ProviderType, token *Token, metadata map[string]string) error

	// GetTokenWithMetadata retrieves a token with metadata
	GetTokenWithMetadata(userID string, provider ProviderType) (*TokenWithMetadata, error)

	// ListAllTokens returns all tokens with their metadata
	ListAllTokens() ([]*TokenWithMetadata, error)
}
