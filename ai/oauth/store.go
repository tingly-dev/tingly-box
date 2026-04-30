package oauth

import (
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/ai"
)

// TokenStorage defines the interface for storing and retrieving OAuth tokens
type TokenStorage interface {
	// SaveToken saves a token for the given user and provider
	SaveToken(userID string, provider ai.Issuer, token *Token) error

	// GetToken retrieves a token for the given user and provider
	GetToken(userID string, provider ai.Issuer) (*Token, error)

	// DeleteToken removes a token for the given user and provider
	DeleteToken(userID string, provider ai.Issuer) error

	// ListProviders returns all providers that have tokens for the user
	ListProviders(userID string) ([]ai.Issuer, error)

	// CleanupExpired removes all expired tokens from the storage
	CleanupExpired() error
}

// MemoryTokenStorage is an in-memory implementation of TokenStorage
type MemoryTokenStorage struct {
	mu     sync.RWMutex
	tokens map[string]map[ai.Issuer]*Token // userID -> provider -> token
}

// NewMemoryTokenStorage creates a new in-memory token storage
func NewMemoryTokenStorage() *MemoryTokenStorage {
	return &MemoryTokenStorage{
		tokens: make(map[string]map[ai.Issuer]*Token),
	}
}

// SaveToken saves a token for the given user and provider
func (s *MemoryTokenStorage) SaveToken(userID string, provider ai.Issuer, token *Token) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.tokens[userID] == nil {
		s.tokens[userID] = make(map[ai.Issuer]*Token)
	}

	s.tokens[userID][provider] = token
	return nil
}

// GetToken retrieves a token for the given user and provider
func (s *MemoryTokenStorage) GetToken(userID string, provider ai.Issuer) (*Token, error) {
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
func (s *MemoryTokenStorage) DeleteToken(userID string, provider ai.Issuer) error {
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

// ListProviders returns all providers that have tokens for the user
func (s *MemoryTokenStorage) ListProviders(userID string) ([]ai.Issuer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.tokens[userID] == nil {
		return []ai.Issuer{}, nil
	}

	providers := make([]ai.Issuer, 0, len(s.tokens[userID]))
	for provider := range s.tokens[userID] {
		providers = append(providers, provider)
	}

	return providers, nil
}

// TokenWithMetadata represents a token with additional metadata
type TokenWithMetadata struct {
	Token     *Token
	UserID    string
	Provider  ai.Issuer
	CreatedAt time.Time
	UpdatedAt time.Time
}

// MetadataTokenStorage extends TokenStorage with metadata support
type MetadataTokenStorage interface {
	TokenStorage

	// SaveTokenWithMetadata saves a token with additional metadata
	SaveTokenWithMetadata(userID string, provider ai.Issuer, token *Token, metadata map[string]string) error

	// GetTokenWithMetadata retrieves a token with metadata
	GetTokenWithMetadata(userID string, provider ai.Issuer) (*TokenWithMetadata, error)

	// ListAllTokens returns all tokens with their metadata
	ListAllTokens() ([]*TokenWithMetadata, error)
}
