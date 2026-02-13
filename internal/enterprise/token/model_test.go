package token

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/enterprise/db"
)

// mockTokenRepository is a mock implementation for testing
type mockTokenRepository struct {
	tokens map[int64]*db.APIToken
	users  map[int64]*db.User
	nextID int64
}

func newMockTokenRepository() *mockTokenRepository {
	return &mockTokenRepository{
		tokens: make(map[int64]*db.APIToken),
		users:  make(map[int64]*db.User),
		nextID: 1,
	}
}

func (m *mockTokenRepository) Create(token *db.APIToken) error {
	token.ID = m.nextID
	m.tokens[m.nextID] = token
	m.nextID++
	return nil
}

func (m *mockTokenRepository) GetByID(id int64) (*db.APIToken, error) {
	token, exists := m.tokens[id]
	if !exists {
		return nil, ErrTokenNotFound
	}
	// Attach user if exists
	if token.UserID != 0 {
		token.User = m.users[token.UserID]
	}
	return token, nil
}

func (m *mockTokenRepository) GetByUUID(uuid string) (*db.APIToken, error) {
	for _, token := range m.tokens {
		if token.UUID == uuid {
			if token.UserID != 0 {
				token.User = m.users[token.UserID]
			}
			return token, nil
		}
	}
	return nil, ErrTokenNotFound
}

func (m *mockTokenRepository) GetByTokenHash(tokenHash string) (*db.APIToken, error) {
	for _, token := range m.tokens {
		if token.TokenHash == tokenHash {
			if token.UserID != 0 {
				token.User = m.users[token.UserID]
			}
			return token, nil
		}
	}
	return nil, ErrTokenNotFound
}

func (m *mockTokenRepository) ListByUserID(userID int64) ([]*db.APIToken, error) {
	tokens := make([]*db.APIToken, 0)
	for _, token := range m.tokens {
		if token.UserID == userID {
			tokens = append(tokens, token)
		}
	}
	return tokens, nil
}

func (m *mockTokenRepository) List(offset, limit int) ([]*db.APIToken, int64, error) {
	tokens := make([]*db.APIToken, 0, len(m.tokens))
	for _, token := range m.tokens {
		tokens = append(tokens, token)
	}
	total := int64(len(tokens))

	if offset >= len(tokens) {
		return []*db.APIToken{}, total, nil
	}

	end := offset + limit
	if end > len(tokens) {
		end = len(tokens)
	}

	return tokens[offset:end], total, nil
}

func (m *mockTokenRepository) Update(token *db.APIToken) error {
	m.tokens[token.ID] = token
	return nil
}

func (m *mockTokenRepository) Delete(id int64) error {
	delete(m.tokens, id)
	return nil
}

func (m *mockTokenRepository) DeleteByUUID(uuid string) error {
	for id, token := range m.tokens {
		if token.UUID == uuid {
			delete(m.tokens, id)
			return nil
		}
	}
	return ErrTokenNotFound
}

func (m *mockTokenRepository) UpdateLastUsed(id int64) error {
	token, exists := m.tokens[id]
	if !exists {
		return ErrTokenNotFound
	}
	now := time.Now()
	token.LastUsedAt = &now
	m.tokens[id] = token
	return nil
}

func (m *mockTokenRepository) Deactivate(id int64) error {
	token, exists := m.tokens[id]
	if !exists {
		return ErrTokenNotFound
	}
	token.IsActive = false
	m.tokens[id] = token
	return nil
}

func (m *mockTokenRepository) DeactivateByUUID(uuid string) error {
	for _, token := range m.tokens {
		if token.UUID == uuid {
			token.IsActive = false
			return nil
		}
	}
	return ErrTokenNotFound
}

func (m *mockTokenRepository) CleanupExpired() (int64, error) {
	now := time.Now()
	count := int64(0)
	for id, token := range m.tokens {
		if token.ExpiresAt != nil && token.ExpiresAt.Before(now) {
			delete(m.tokens, id)
			count++
		}
	}
	return count, nil
}

func (m *mockTokenRepository) addUser(user *db.User) {
	m.users[user.ID] = user
}

func TestModel_Create(t *testing.T) {
	repo := newMockTokenRepository()
	model := NewModel(repo)

	user := &db.User{
		ID:       1,
		Username: "testuser",
		IsActive: true,
	}
	repo.addUser(user)

	data := CreateTokenData{
		UserID:    1,
		Name:      "Test Token",
		Scopes:    []db.Scope{db.ScopeReadProviders, db.ScopeReadRules},
		ExpiresAt: nil,
	}

	token, rawToken, err := model.Create(data)

	require.NoError(t, err)
	assert.NotNil(t, token)
	assert.NotEmpty(t, rawToken)
	assert.Equal(t, "Test Token", token.Name)
	assert.True(t, token.IsActive)
	assert.NotEmpty(t, token.TokenHash)
	assert.NotEmpty(t, token.TokenPrefix)
	assert.Equal(t, 8, len(token.TokenPrefix)) // Prefix should be 8 chars
}

func TestModel_CreateWithExpiry(t *testing.T) {
	repo := newMockTokenRepository()
	model := NewModel(repo)

	user := &db.User{
		ID:       1,
		Username: "testuser",
		IsActive: true,
	}
	repo.addUser(user)

	expiresAt := time.Now().Add(24 * time.Hour)
	data := CreateTokenData{
		UserID:    1,
		Name:      "Test Token",
		Scopes:    []db.Scope{db.ScopeReadProviders},
		ExpiresAt: &expiresAt,
	}

	token, rawToken, err := model.Create(data)

	require.NoError(t, err)
	assert.NotNil(t, token)
	assert.NotNil(t, token.ExpiresAt)
	assert.Equal(t, expiresAt.Unix(), token.ExpiresAt.Unix())
	assert.NotEmpty(t, rawToken)
}

func TestModel_ValidateToken(t *testing.T) {
	repo := newMockTokenRepository()
	model := NewModel(repo)

	user := &db.User{
		ID:       1,
		Username: "testuser",
		IsActive: true,
	}
	repo.addUser(user)

	data := CreateTokenData{
		UserID: 1,
		Name:   "Test Token",
		Scopes: []db.Scope{db.ScopeReadProviders},
	}

	_, rawToken, err := model.Create(data)
	require.NoError(t, err)

	// Validate valid token
	validatedToken, err := model.ValidateToken(rawToken)
	require.NoError(t, err)
	assert.NotNil(t, validatedToken)
	assert.Equal(t, "Test Token", validatedToken.Name)

	// Validate invalid token
	_, err = model.ValidateToken("invalid-token")
	assert.Error(t, err)
}

func TestModel_ValidateToken_Inactive(t *testing.T) {
	repo := newMockTokenRepository()
	model := NewModel(repo)

	user := &db.User{
		ID:       1,
		Username: "testuser",
		IsActive: true,
	}
	repo.addUser(user)

	data := CreateTokenData{
		UserID: 1,
		Name:   "Test Token",
		Scopes: []db.Scope{db.ScopeReadProviders},
	}

	token, rawToken, err := model.Create(data)
	require.NoError(t, err)

	// Deactivate the token
	err = model.Deactivate(token.ID)
	require.NoError(t, err)

	// Try to validate the inactive token
	_, err = model.ValidateToken(rawToken)
	assert.Error(t, err)
	assert.Equal(t, ErrTokenInactive, err)
}

func TestModel_ValidateToken_Expired(t *testing.T) {
	repo := newMockTokenRepository()
	model := NewModel(repo)

	user := &db.User{
		ID:       1,
		Username: "testuser",
		IsActive: true,
	}
	repo.addUser(user)

	// Create token with past expiration
	past := time.Now().Add(-1 * time.Hour)
	data := CreateTokenData{
		UserID:    1,
		Name:      "Test Token",
		Scopes:    []db.Scope{db.ScopeReadProviders},
		ExpiresAt: &past,
	}

	token, rawToken, err := model.Create(data)
	require.NoError(t, err)

	// Try to validate the expired token
	_, err = model.ValidateToken(rawToken)
	assert.Error(t, err)
	assert.Equal(t, ErrTokenExpired, err)
}

func TestModel_ValidateToken_InactiveUser(t *testing.T) {
	repo := newMockTokenRepository()
	model := NewModel(repo)

	user := &db.User{
		ID:       1,
		Username: "testuser",
		IsActive: false, // Inactive user
	}
	repo.addUser(user)

	data := CreateTokenData{
		UserID: 1,
		Name:   "Test Token",
		Scopes: []db.Scope{db.ScopeReadProviders},
	}

	_, rawToken, err := model.Create(data)
	require.NoError(t, err)

	// Try to validate token for inactive user
	_, err = model.ValidateToken(rawToken)
	assert.Error(t, err)
}

func TestModel_ListByUserID(t *testing.T) {
	repo := newMockTokenRepository()
	model := NewModel(repo)

	user := &db.User{
		ID:       1,
		Username: "testuser",
		IsActive: true,
	}
	repo.addUser(user)

	// Create multiple tokens for the user
	for i := 0; i < 3; i++ {
		data := CreateTokenData{
			UserID: 1,
			Name:   fmt.Sprintf("Token %d", i),
			Scopes: []db.Scope{db.ScopeReadProviders},
		}
		_, _, err := model.Create(data)
		require.NoError(t, err)
	}

	// List tokens
	tokens, err := model.ListByUserID(1)
	require.NoError(t, err)
	assert.Len(t, tokens, 3)
}

func TestModel_Delete(t *testing.T) {
	repo := newMockTokenRepository()
	model := NewModel(repo)

	user := &db.User{
		ID:       1,
		Username: "testuser",
		IsActive: true,
	}
	repo.addUser(user)

	data := CreateTokenData{
		UserID: 1,
		Name:   "Test Token",
		Scopes: []db.Scope{db.ScopeReadProviders},
	}

	token, _, err := model.Create(data)
	require.NoError(t, err)

	// Delete token
	err = model.Delete(token.ID)
	require.NoError(t, err)

	// Verify token is deleted
	_, err = model.GetByID(token.ID)
	assert.Error(t, err)
}

func TestModel_CleanupExpired(t *testing.T) {
	repo := newMockTokenRepository()
	model := NewModel(repo)

	user := &db.User{
		ID:       1,
		Username: "testuser",
		IsActive: true,
	}
	repo.addUser(user)

	// Create expired token
	past := time.Now().Add(-1 * time.Hour)
	expiredData := CreateTokenData{
		UserID:    1,
		Name:      "Expired Token",
		Scopes:    []db.Scope{db.ScopeReadProviders},
		ExpiresAt: &past,
	}
	_, _, err := model.Create(expiredData)
	require.NoError(t, err)

	// Create valid token
	validData := CreateTokenData{
		UserID: 1,
		Name:   "Valid Token",
		Scopes: []db.Scope{db.ScopeReadProviders},
	}
	validToken, _, err := model.Create(validData)
	require.NoError(t, err)

	// Cleanup expired
	count, err := model.CleanupExpired()
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Verify expired token was deleted but valid token remains
	_, err = model.GetByID(validToken.ID)
	assert.NoError(t, err)
}

func TestModel_RecordUsage(t *testing.T) {
	repo := newMockTokenRepository()
	model := NewModel(repo)

	user := &db.User{
		ID:       1,
		Username: "testuser",
		IsActive: true,
	}
	repo.addUser(user)

	data := CreateTokenData{
		UserID: 1,
		Name:   "Test Token",
		Scopes: []db.Scope{db.ScopeReadProviders},
	}

	token, _, err := model.Create(data)
	require.NoError(t, err)
	assert.Nil(t, token.LastUsedAt)

	// Record usage
	err = model.RecordUsage(token.ID)
	require.NoError(t, err)

	// Verify LastUsedAt was updated
	updatedToken, err := model.GetByID(token.ID)
	require.NoError(t, err)
	assert.NotNil(t, updatedToken.LastUsedAt)
}

func TestHasScope(t *testing.T) {
	repo := newMockTokenRepository()
	model := NewModel(repo)

	user := &db.User{
		ID:       1,
		Username: "testuser",
		IsActive: true,
	}
	repo.addUser(user)

	scopes := []db.Scope{db.ScopeReadProviders, db.ScopeWriteProviders, db.ScopeReadRules}
	scopesJSON, _ := json.Marshal(scopes)

	token := &db.APIToken{
		ID:        1,
		UserID:    1,
		TokenHash: "hash",
		Name:      "Test Token",
		Scopes:    string(scopesJSON),
		IsActive:  true,
	}

	// Test has scope
	hasScope, err := HasScope(token, db.ScopeReadProviders)
	require.NoError(t, err)
	assert.True(t, hasScope)

	hasScope, err = HasScope(token, db.ScopeReadUsers)
	require.NoError(t, err)
	assert.False(t, hasScope)
}

func TestHasAnyScope(t *testing.T) {
	repo := newMockTokenRepository()
	model := NewModel(repo)

	user := &db.User{
		ID:       1,
		Username: "testuser",
		IsActive: true,
	}
	repo.addUser(user)

	scopes := []db.Scope{db.ScopeReadProviders, db.ScopeWriteProviders}
	scopesJSON, _ := json.Marshal(scopes)

	token := &db.APIToken{
		ID:        1,
		UserID:    1,
		TokenHash: "hash",
		Name:      "Test Token",
		Scopes:    string(scopesJSON),
		IsActive:  true,
	}

	// Test has any of the requested scopes
	hasAny, err := HasAnyScope(token, []db.Scope{db.ScopeReadUsers, db.ScopeReadProviders})
	require.NoError(t, err)
	assert.True(t, hasAny) // Has read:providers

	hasAny, err = HasAnyScope(token, []db.Scope{db.ScopeReadUsers, db.ScopeWriteUsers})
	require.NoError(t, err)
	assert.False(t, hasAny) // Has neither
}

func TestModel_ActivateDeactivate(t *testing.T) {
	repo := newMockTokenRepository()
	model := NewModel(repo)

	user := &db.User{
		ID:       1,
		Username: "testuser",
		IsActive: true,
	}
	repo.addUser(user)

	data := CreateTokenData{
		UserID: 1,
		Name:   "Test Token",
		Scopes: []db.Scope{db.ScopeReadProviders},
	}

	token, _, err := model.Create(data)
	require.NoError(t, err)
	assert.True(t, token.IsActive)

	// Deactivate
	err = model.Deactivate(token.ID)
	require.NoError(t, err)

	deactivatedToken, err := model.GetByID(token.ID)
	require.NoError(t, err)
	assert.False(t, deactivatedToken.IsActive)

	// Activate
	err = model.Activate(token.ID)
	require.NoError(t, err)

	activatedToken, err := model.GetByID(token.ID)
	require.NoError(t, err)
	assert.True(t, activatedToken.IsActive)
}
