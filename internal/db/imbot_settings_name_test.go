package db

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBotNameIsPlatformDefaultNeverOverwritesUserName(t *testing.T) {
	sm, err := NewStoreManager(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = sm.Close() })
	settings := sm.ImBotSettings()
	unnamed, err := settings.CreateSettings(Settings{Platform: "telegram", AuthType: "token", Auth: map[string]string{"token": "t"}, Enabled: true})
	require.NoError(t, err)
	require.Empty(t, unnamed.Name, "an unnamed bot stays unnamed instead of '<platform> Bot'")

	wrote, err := settings.SetNameIfEmpty(unnamed.UUID, "@ops_bot")
	require.NoError(t, err)
	require.True(t, wrote)
	got, err := settings.GetSettingsByUUID(unnamed.UUID)
	require.NoError(t, err)
	require.Equal(t, "@ops_bot", got.Name)

	wrote, err = settings.SetNameIfEmpty(unnamed.UUID, "@other")
	require.NoError(t, err)
	require.False(t, wrote, "an existing name is never replaced")

	wrote, err = settings.SetNameIfEmpty(unnamed.UUID, "  ")
	require.NoError(t, err)
	require.False(t, wrote)
}

func TestBotNameCanBeClearedExplicitly(t *testing.T) {
	sm, err := NewStoreManager(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = sm.Close() })
	settings := sm.ImBotSettings()
	bot := createAccessBot(t, sm, "naming")
	require.NoError(t, settings.UpdateSettings(bot.UUID, Settings{Platform: "telegram", AuthType: "token", Enabled: true}))
	got, err := settings.GetSettingsByUUID(bot.UUID)
	require.NoError(t, err)
	require.Equal(t, "naming", got.Name, "an empty Name means unchanged")

	require.NoError(t, settings.UpdateSettings(bot.UUID, Settings{Platform: "telegram", AuthType: "token", Enabled: true, ClearName: true}))
	got, err = settings.GetSettingsByUUID(bot.UUID)
	require.NoError(t, err)
	require.Empty(t, got.Name)
}
