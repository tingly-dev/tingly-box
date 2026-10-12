package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

func TestAPITokenStore_GetToken_NotFound(t *testing.T) {
	store, _ := setupTestAPITokenStore(t)
	defer store.Close()

	_, err := store.GetToken("non-existent-token")
	assert.Error(t, err)
}

func TestAPITokenStore_RevokeToken_NotFound(t *testing.T) {
	store, _ := setupTestAPITokenStore(t)
	defer store.Close()

	err := store.RevokeToken("non-existent", "reason")
	assert.Error(t, err)
}

func TestAPITokenStore_UpdateLastUsed_FirstCallPersists(t *testing.T) {
	store, _ := setupTestAPITokenStore(t)
	defer store.Close()

	_, err := store.CreateTokenWithTokenID("user-1", "token-1", "display", "admin", nil)
	assert.NoError(t, err)

	assert.NoError(t, store.UpdateLastUsed("token-1"))

	cached, err := store.GetToken("token-1")
	assert.NoError(t, err)
	assert.NotNil(t, cached.LastUsedAt)

	// Confirm it was actually persisted to SQLite, not just cached.
	var dbRecord APITokenRecord
	assert.NoError(t, store.db.Where("token_id = ?", "token-1").First(&dbRecord).Error)
	assert.NotNil(t, dbRecord.LastUsedAt)
}

func TestAPITokenStore_UpdateLastUsed_DebouncesWithinWindow(t *testing.T) {
	store, _ := setupTestAPITokenStore(t)
	defer store.Close()

	_, err := store.CreateTokenWithTokenID("user-1", "token-1", "display", "admin", nil)
	assert.NoError(t, err)

	assert.NoError(t, store.UpdateLastUsed("token-1"))
	first, err := store.GetToken("token-1")
	assert.NoError(t, err)

	// A second call inside the debounce window must not move last_used_at.
	assert.NoError(t, store.UpdateLastUsed("token-1"))
	second, err := store.GetToken("token-1")
	assert.NoError(t, err)

	assert.True(t, first.LastUsedAt.Equal(*second.LastUsedAt))
}

func TestAPITokenStore_UpdateLastUsed_WritesAgainAfterWindow(t *testing.T) {
	store, _ := setupTestAPITokenStore(t)
	defer store.Close()

	_, err := store.CreateTokenWithTokenID("user-1", "token-1", "display", "admin", nil)
	assert.NoError(t, err)
	assert.NoError(t, store.UpdateLastUsed("token-1"))

	// Simulate the debounce window having elapsed without sleeping in the test.
	store.mu.Lock()
	stale := time.Now().Add(-2 * defaultLastUsedDebounce)
	store.cache["token-1"].LastUsedAt = &stale
	store.mu.Unlock()

	assert.NoError(t, store.UpdateLastUsed("token-1"))
	refreshed, err := store.GetToken("token-1")
	assert.NoError(t, err)
	assert.True(t, refreshed.LastUsedAt.After(stale))
}

func TestAPITokenStore_RPMLimit_DefaultAndSet(t *testing.T) {
	store, tmpDir := setupTestAPITokenStore(t)

	// Legacy constructor path → default budget.
	rec, err := store.CreateTokenWithTokenID("user-1", "token-default", "display", "admin", nil)
	assert.NoError(t, err)
	assert.Equal(t, DefaultSharingKeyRPMLimit, rec.RPMLimit)

	// Explicit budget, including 0 = unlimited.
	rec, err = store.CreateTokenForTeamWithRPMLimit("user-2", "token-unlimited", DefaultTeamID, "display", "admin", nil, 0)
	assert.NoError(t, err)
	assert.Equal(t, 0, rec.RPMLimit)

	_, err = store.CreateTokenForTeamWithRPMLimit("user-3", "token-neg", DefaultTeamID, "display", "admin", nil, -1)
	assert.Error(t, err)
	_, err = store.CreateTokenForTeamWithRPMLimit("user-4", "token-huge", DefaultTeamID, "display", "admin", nil, MaxSharingKeyRPMLimit+1)
	assert.Error(t, err)
	rec, err = store.CreateTokenForTeamWithRPMLimit("user-5", "token-max", DefaultTeamID, "display", "admin", nil, MaxSharingKeyRPMLimit)
	assert.NoError(t, err)
	assert.Equal(t, MaxSharingKeyRPMLimit, rec.RPMLimit)

	// SetTokenRPMLimit updates both SQLite and the cache ValidateToken reads.
	assert.NoError(t, store.SetTokenRPMLimit("token-default", 7))
	got, err := store.ValidateToken("token-default")
	assert.NoError(t, err)
	assert.Equal(t, 7, got.RPMLimit)
	assert.Error(t, store.SetTokenRPMLimit("token-default", -5))
	assert.Error(t, store.SetTokenRPMLimit("token-default", MaxSharingKeyRPMLimit+1))
	assert.Error(t, store.SetTokenRPMLimit("missing", 3))

	// Persisted: a fresh store over the same directory sees the value.
	assert.NoError(t, store.Close())
	reopened, err := NewAPITokenStore(tmpDir)
	assert.NoError(t, err)
	defer reopened.Close()
	persisted, err := reopened.GetToken("token-default")
	assert.NoError(t, err)
	assert.Equal(t, 7, persisted.RPMLimit)
	unlimited, err := reopened.GetToken("token-unlimited")
	assert.NoError(t, err)
	assert.Equal(t, 0, unlimited.RPMLimit)
}
