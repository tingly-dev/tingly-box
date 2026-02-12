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
		Token:         "telegram-token",
		Platform:      "telegram",
		ProxyURL:      "http://proxy.test:8080",
		ChatIDLock:    "chat-123",
		BashAllowlist: []string{"cd", "ls", "pwd"},
	}
	require.NoError(t, store.SaveSettings(settings))

	loaded, err := store.GetSettings()
	require.NoError(t, err)
	require.Equal(t, "telegram-token", loaded.Token)
	require.Equal(t, "telegram", loaded.Platform)
	require.Equal(t, "http://proxy.test:8080", loaded.ProxyURL)
	require.Equal(t, "chat-123", loaded.ChatIDLock)
	require.Equal(t, []string{"cd", "ls", "pwd"}, loaded.BashAllowlist)

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
