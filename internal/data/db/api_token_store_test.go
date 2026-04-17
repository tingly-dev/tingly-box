package db

import (
	"testing"

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
