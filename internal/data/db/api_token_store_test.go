package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestAPITokenStore(t *testing.T) (*APITokenStore, string) {
	t.Helper()

	tmpDir := t.TempDir()

	store, err := NewAPITokenStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create API token store: %v", err)
	}

	return store, tmpDir
}

func TestNewAPITokenStore(t *testing.T) {
	store, _ := setupTestAPITokenStore(t)
	defer store.Close()

	// Verify store was created
	assert.NotNil(t, store)

	// Create a token to verify DB is working
	_, err := store.CreateToken("test-user", "Test Token", "test", nil)
	assert.NoError(t, err, "Should be able to create a token")
}

func TestAPITokenStore_CreateToken(t *testing.T) {
	store, _ := setupTestAPITokenStore(t)
	defer store.Close()

	userUUID := "user-123"
	displayName := "My API Token"
	createdBy := "admin"
	expiresAt := time.Now().Add(30 * 24 * time.Hour)

	record, err := store.CreateToken(userUUID, displayName, createdBy, &expiresAt)
	require.NoError(t, err)

	assert.NotEmpty(t, record.TokenID)
	assert.Equal(t, userUUID, record.UserUUID)
	assert.Equal(t, displayName, record.DisplayName)
	assert.Equal(t, createdBy, record.CreatedBy)
	assert.True(t, record.Enabled)
	assert.NotNil(t, record.ExpiresAt)
	assert.WithinDuration(t, expiresAt, *record.ExpiresAt, time.Second)
	assert.Nil(t, record.LastUsedAt) // LastUsedAt is nil initially
	assert.False(t, record.CreatedAt.IsZero())
	assert.Nil(t, record.RevokedAt) // RevokedAt is nil initially
}

func TestAPITokenStore_CreateToken_DefaultExpiry(t *testing.T) {
	store, _ := setupTestAPITokenStore(t)
	defer store.Close()

	userUUID := "user-456"
	displayName := "Token with default expiry"

	record, err := store.CreateToken(userUUID, displayName, "admin", nil)
	require.NoError(t, err)

	assert.NotEmpty(t, record.TokenID)
	assert.Equal(t, userUUID, record.UserUUID)
	assert.Nil(t, record.ExpiresAt) // No expiry set
}

func TestAPITokenStore_GetToken(t *testing.T) {
	store, _ := setupTestAPITokenStore(t)
	defer store.Close()

	// Create a token
	created, err := store.CreateToken("user-789", "Test Token", "admin", nil)
	require.NoError(t, err)

	// Get the token
	found, err := store.GetToken(created.TokenID)
	require.NoError(t, err)

	assert.Equal(t, created.TokenID, found.TokenID)
	assert.Equal(t, created.UserUUID, found.UserUUID)
	assert.Equal(t, created.DisplayName, found.DisplayName)
}

func TestAPITokenStore_GetToken_NotFound(t *testing.T) {
	store, _ := setupTestAPITokenStore(t)
	defer store.Close()

	_, err := store.GetToken("non-existent-token")
	assert.Error(t, err)
}

func TestAPITokenStore_ValidateToken(t *testing.T) {
	store, _ := setupTestAPITokenStore(t)
	defer store.Close()

	// Create an enabled token
	enabled, err := store.CreateToken("user-001", "Enabled Token", "admin", nil)
	require.NoError(t, err)

	// Validate enabled token
	record, err := store.ValidateToken(enabled.TokenID)
	require.NoError(t, err)
	assert.True(t, record.Enabled)
	assert.Equal(t, enabled.UserUUID, record.UserUUID)

	// Create a disabled token
	disabled, err := store.CreateToken("user-002", "Disabled Token", "admin", nil)
	require.NoError(t, err)

	// Disable the token manually through DB
	store.GetDB().Model(&APITokenRecord{}).Where("token_id = ?", disabled.TokenID).Update("enabled", false)

	// Validate disabled token
	_, err = store.ValidateToken(disabled.TokenID)
	assert.Error(t, err)
}

func TestAPITokenStore_ValidateToken_Expired(t *testing.T) {
	store, _ := setupTestAPITokenStore(t)
	defer store.Close()

	// Create an expired token
	past := time.Now().Add(-1 * time.Hour)
	expired, err := store.CreateToken("user-003", "Expired Token", "admin", &past)
	require.NoError(t, err)

	// Validate expired token
	_, err = store.ValidateToken(expired.TokenID)
	assert.Error(t, err)
}

func TestAPITokenStore_ValidateToken_Revoked(t *testing.T) {
	store, _ := setupTestAPITokenStore(t)
	defer store.Close()

	// Create and revoke a token
	revoked, err := store.CreateToken("user-004", "Revoked Token", "admin", nil)
	require.NoError(t, err)

	err = store.RevokeToken(revoked.TokenID, "Test revocation")
	require.NoError(t, err)

	// Validate revoked token
	_, err = store.ValidateToken(revoked.TokenID)
	assert.Error(t, err)
}

func TestAPITokenStore_RevokeToken(t *testing.T) {
	store, _ := setupTestAPITokenStore(t)
	defer store.Close()

	// Create a token
	token, err := store.CreateToken("user-005", "Revoke Test", "admin", nil)
	require.NoError(t, err)

	reason := "Security concern"
	err = store.RevokeToken(token.TokenID, reason)
	require.NoError(t, err)

	// Verify revocation
	updated, err := store.GetToken(token.TokenID)
	require.NoError(t, err)

	assert.False(t, updated.Enabled)
	assert.NotNil(t, updated.RevokedAt)
	assert.False(t, updated.RevokedAt.IsZero())
	assert.Equal(t, reason, updated.RevokeReason)
}

func TestAPITokenStore_RevokeToken_NotFound(t *testing.T) {
	store, _ := setupTestAPITokenStore(t)
	defer store.Close()

	err := store.RevokeToken("non-existent", "reason")
	assert.Error(t, err)
}

func TestAPITokenStore_UpdateLastUsed(t *testing.T) {
	store, _ := setupTestAPITokenStore(t)
	defer store.Close()

	// Create a token
	token, err := store.CreateToken("user-006", "Last Used Test", "admin", nil)
	require.NoError(t, err)

	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Update last used
	err = store.UpdateLastUsed(token.TokenID)
	require.NoError(t, err)

	// Verify update
	updated, err := store.GetToken(token.TokenID)
	require.NoError(t, err)

	assert.True(t, updated.LastUsedAt.After(token.CreatedAt))
}

func TestAPITokenStore_ListTokens(t *testing.T) {
	store, _ := setupTestAPITokenStore(t)
	defer store.Close()

	// Create multiple tokens
	user1 := "user-list-1"
	user2 := "user-list-2"

	_, _ = store.CreateToken(user1, "Token 1", "admin", nil)
	_, _ = store.CreateToken(user1, "Token 2", "admin", nil)
	_, _ = store.CreateToken(user2, "Token 3", "admin", nil)

	// List all tokens
	records, total, err := store.ListTokens("", nil, 100, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, records, 3)

	// Filter by user
	records, total, err = store.ListTokens(user1, nil, 100, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, records, 2)

	// Filter enabled only
	enabledOnly := true
	records, total, err = store.ListTokens("", &enabledOnly, 100, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
}

func TestAPITokenStore_ListTokens_Pagination(t *testing.T) {
	store, _ := setupTestAPITokenStore(t)
	defer store.Close()

	// Create multiple tokens
	for i := 0; i < 5; i++ {
		_, _ = store.CreateToken("user-page", "Token", "admin", nil)
	}

	// Test pagination
	records, total, err := store.ListTokens("", nil, 2, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, records, 2)

	records, total, err = store.ListTokens("", nil, 2, 2)
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, records, 2)

	records, total, err = store.ListTokens("", nil, 2, 4)
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, records, 1)
}
