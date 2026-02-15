package bot

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/remote_coder/session"
)

func TestManagerStartStop(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tingly.db")

	store, err := NewStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	// Create a session manager
	msgStore, err := session.NewMessageStore(dbPath)
	require.NoError(t, err)
	sessionMgr := session.NewManager(session.Config{
		Timeout:          30 * time.Minute,
		MessageRetention: 24 * time.Hour,
	}, msgStore)

	// Create manager
	manager := NewManager(store, sessionMgr)

	// Test: IsRunning returns false for non-existent bot
	require.False(t, manager.IsRunning("non-existent-uuid"))

	// Test: Stop on non-running bot is safe (no panic)
	manager.Stop("non-existent-uuid")
}

func TestManagerStartEnabledBots(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tingly.db")

	store, err := NewStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	// Create a session manager
	msgStore, err := session.NewMessageStore(dbPath)
	require.NoError(t, err)
	sessionMgr := session.NewManager(session.Config{
		Timeout:          30 * time.Minute,
		MessageRetention: 24 * time.Hour,
	}, msgStore)

	// Create enabled bot (without token - won't actually run)
	_, err = store.CreateSettings(Settings{
		Name:     "Test Bot",
		Platform: "telegram",
		AuthType: "token",
		Auth:     map[string]string{}, // No token
		Enabled:  true,
	})
	require.NoError(t, err)

	// Create disabled bot
	_, err = store.CreateSettings(Settings{
		Name:     "Disabled Bot",
		Platform: "telegram",
		AuthType: "token",
		Auth:     map[string]string{"token": "test-token"},
		Enabled:  false,
	})
	require.NoError(t, err)

	// Create manager
	manager := NewManager(store, sessionMgr)

	// Start enabled bots - should not fail
	ctx := context.Background()
	err = manager.StartEnabled(ctx)
	require.NoError(t, err)

	// Give time for goroutines to process
	time.Sleep(100 * time.Millisecond)

	// Bot without token should not be running
	settings, err := store.ListEnabledSettings()
	require.NoError(t, err)
	if len(settings) > 0 {
		// The bot without token won't start
		require.False(t, manager.IsRunning(settings[0].UUID))
	}
}

func TestManagerStopAll(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tingly.db")

	store, err := NewStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	msgStore, err := session.NewMessageStore(dbPath)
	require.NoError(t, err)
	sessionMgr := session.NewManager(session.Config{
		Timeout:          30 * time.Minute,
		MessageRetention: 24 * time.Hour,
	}, msgStore)

	manager := NewManager(store, sessionMgr)

	// StopAll should be safe even with no running bots
	manager.StopAll()
}
