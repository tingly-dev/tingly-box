package bot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStoreSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tingly-remote-coder.db")
	store, err := NewStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	settings := Settings{
		Token:     "telegram-token",
		Allowlist: []string{"123", "456", "123"},
	}
	require.NoError(t, store.SaveSettings(settings))

	loaded, err := store.GetSettings()
	require.NoError(t, err)
	require.Equal(t, "telegram-token", loaded.Token)
	require.ElementsMatch(t, []string{"123", "456"}, loaded.Allowlist)

	allowed, err := store.IsAllowed("123")
	require.NoError(t, err)
	require.True(t, allowed)

	allowed, err = store.IsAllowed("789")
	require.NoError(t, err)
	require.False(t, allowed)

	_, err = os.Stat(dbPath)
	require.NoError(t, err)
}

func TestStoreSessionMapping(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "tingly-remote-coder.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	require.NoError(t, store.SetSessionForChat("chat-1", "session-1"))
	id, ok, err := store.GetSessionForChat("chat-1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "session-1", id)
}
