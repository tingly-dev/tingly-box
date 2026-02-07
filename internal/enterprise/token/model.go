package token

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/internal/enterprise/db"
)

// Model provides token domain model operations
type Model struct {
	repo Repository
}

// NewModel creates a new token model
func NewModel(repo Repository) *Model {
	return &Model{repo: repo}
}

// CreateTokenData contains data for creating a new API token
type CreateTokenData struct {
	UserID    int64
	Name      string
	Scopes    []db.Scope
	ExpiresAt *time.Time
}

// TokenWithUserData represents a token with user information
type TokenWithUserData struct {
	*db.APIToken
	Username string `json:"username"`
	Email    string `json:"email"`
}

// Create creates a new API token
// Returns the created token and the raw token string (only shown once)
func (m *Model) Create(data CreateTokenData) (*db.APIToken, string, error) {
	// Generate raw token
	rawToken := m.generateRawToken()

	// Hash the token for storage
	tokenHash := m.hashToken(rawToken)

	// Extract prefix (first 8 characters) for identification
	tokenPrefix := rawToken[:8]

	// Marshal scopes to JSON
	scopesJSON, err := json.Marshal(data.Scopes)
	if err != nil {
		return nil, "", err
	}

	token := &db.APIToken{
		UUID:        uuid.New().String(),
		UserID:      data.UserID,
		TokenHash:   tokenHash,
		TokenPrefix: tokenPrefix,
		Name:        data.Name,
		Scopes:      string(scopesJSON),
		ExpiresAt:   data.ExpiresAt,
		IsActive:    true,
	}

	if err := m.repo.Create(token); err != nil {
		return nil, "", err
	}

	return token, rawToken, nil
}

// GetByID retrieves a token by ID
func (m *Model) GetByID(id int64) (*db.APIToken, error) {
	return m.repo.GetByID(id)
}

// GetByUUID retrieves a token by UUID
func (m *Model) GetByUUID(uuid string) (*db.APIToken, error) {
	return m.repo.GetByUUID(uuid)
}

// ValidateToken validates a raw token string and returns the token record
func (m *Model) ValidateToken(rawToken string) (*db.APIToken, error) {
	tokenHash := m.hashToken(rawToken)
	token, err := m.repo.GetByTokenHash(tokenHash)
	if err != nil {
		return nil, err
	}

	// Check if token is active
	if !token.IsActive {
		return nil, ErrTokenInactive
	}

	// Check if token has expired
	if token.ExpiresAt != nil && token.ExpiresAt.Before(time.Now()) {
		return nil, ErrTokenExpired
	}

	return token, nil
}

// ListByUserID retrieves all tokens for a user
func (m *Model) ListByUserID(userID int64) ([]*db.APIToken, error) {
	return m.repo.ListByUserID(userID)
}

// List retrieves tokens with pagination
func (m *Model) List(page, pageSize int) ([]*db.APIToken, int64, error) {
	offset := (page - 1) * pageSize
	return m.repo.List(offset, pageSize)
}

// Update updates a token (name, scopes, expiration)
func (m *Model) Update(id int64, name string, scopes []db.Scope, expiresAt *time.Time) (*db.APIToken, error) {
	token, err := m.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	token.Name = name

	if scopes != nil {
		scopesJSON, err := json.Marshal(scopes)
		if err != nil {
			return nil, err
		}
		token.Scopes = string(scopesJSON)
	}

	if expiresAt != nil {
		token.ExpiresAt = expiresAt
	}

	if err := m.repo.Update(token); err != nil {
		return nil, err
	}

	return token, nil
}

// Delete deletes a token by ID
func (m *Model) Delete(id int64) error {
	return m.repo.Delete(id)
}

// DeleteByUUID deletes a token by UUID
func (m *Model) DeleteByUUID(uuid string) error {
	return m.repo.DeleteByUUID(uuid)
}

// RecordUsage records that a token was used
func (m *Model) RecordUsage(id int64) error {
	return m.repo.UpdateLastUsed(id)
}

// RecordUsageByUUID records that a token was used by UUID
func (m *Model) RecordUsageByUUID(uuid string) error {
	token, err := m.repo.GetByUUID(uuid)
	if err != nil {
		return err
	}
	return m.repo.UpdateLastUsed(token.ID)
}

// Deactivate deactivates a token by ID
func (m *Model) Deactivate(id int64) error {
	return m.repo.Deactivate(id)
}

// DeactivateByUUID deactivates a token by UUID
func (m *Model) DeactivateByUUID(uuid string) error {
	return m.repo.DeactivateByUUID(uuid)
}

// Activate activates a token by ID
func (m *Model) Activate(id int64) error {
	token, err := m.repo.GetByID(id)
	if err != nil {
		return err
	}
	token.IsActive = true
	return m.repo.Update(token)
}

// CleanupExpired removes expired tokens from the database
func (m *Model) CleanupExpired() (int64, error) {
	return m.repo.CleanupExpired()
}

// GetScopes parses and returns the scopes from a token
func GetScopes(token *db.APIToken) ([]db.Scope, error) {
	var scopes []db.Scope
	if err := json.Unmarshal([]byte(token.Scopes), &scopes); err != nil {
		return nil, err
	}
	return scopes, nil
}

// HasScope checks if a token has a specific scope
func HasScope(token *db.APIToken, scope db.Scope) (bool, error) {
	scopes, err := GetScopes(token)
	if err != nil {
		return false, err
	}

	for _, s := range scopes {
		if s == scope || s == db.ScopeAdminAll {
			return true, nil
		}
	}

	return false, nil
}

// HasAnyScope checks if a token has any of the specified scopes
func HasAnyScope(token *db.APIToken, scopes []db.Scope) (bool, error) {
	tokenScopes, err := GetScopes(token)
	if err != nil {
		return false, err
	}

	// Check if token has admin scope
	for _, s := range tokenScopes {
		if s == db.ScopeAdminAll {
			return true, nil
		}
	}

	// Check for matching scopes
	tokenScopeMap := make(map[db.Scope]bool)
	for _, s := range tokenScopes {
		tokenScopeMap[s] = true
	}

	for _, scope := range scopes {
		if tokenScopeMap[scope] {
			return true, nil
		}
	}

	return false, nil
}

// generateRawToken generates a new raw API token string
func (m *Model) generateRawToken() string {
	return "ent-" + uuid.New().String()
}

// hashToken hashes a token string using SHA-256
func (m *Model) hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
