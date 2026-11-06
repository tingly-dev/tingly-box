package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/remote/access"
)

// A platform-reported name is stored beside the alias, never in it: an empty or
// id-echo name never wipes it and the operator's alias is untouched.
func TestBotAccessStorePlatformNamesNeverTouchAliases(t *testing.T) {
	sm, err := NewStoreManager(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = sm.Close() })
	bot := createAccessBot(t, sm, "names")
	store, ctx := sm.BotAccess(), context.Background()

	group, err := store.DiscoverGroup(ctx, bot.UUID, "telegram", "telegram:-100", "Release Crew")
	require.NoError(t, err)
	require.Equal(t, "Release Crew", group.Name)
	require.NoError(t, store.SetTargetAlias(ctx, bot.UUID, group.ID, "group", "Boss"))
	for _, name := range []string{"", "telegram:-100", "-100"} {
		_, err = store.DiscoverGroup(ctx, bot.UUID, "telegram", "telegram:-100", name)
		require.NoError(t, err)
	}
	_, err = store.DiscoverGroup(ctx, bot.UUID, "telegram", "telegram:-100", "Release Crew 2")
	require.NoError(t, err)
	groups, err := store.ListGroups(ctx, bot.UUID)
	require.NoError(t, err)
	require.Equal(t, "Release Crew 2", groups[0].Name)
	require.Equal(t, "Boss", groups[0].Alias)

	chat, err := store.DiscoverDirectChat(ctx, bot.UUID, "telegram", "telegram:42")
	require.NoError(t, err)
	require.NoError(t, store.SetDirectChatName(ctx, bot.UUID, chat.ID, "Alice"))
	for _, name := range []string{"", "telegram:42", "42"} {
		require.NoError(t, store.SetDirectChatName(ctx, bot.UUID, chat.ID, name))
	}
	got, ok, err := store.GetDirectChat(ctx, bot.UUID, chat.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "Alice", got.Name)
	require.Empty(t, got.Alias)

	actor, err := store.UpsertActor(ctx, access.Actor{BotUUID: bot.UUID, Platform: "telegram", ExternalActorID: "ou_1", DisplayName: "Alice"})
	require.NoError(t, err)
	for _, name := range []string{"", "ou_1"} {
		actor, err = store.UpsertActor(ctx, access.Actor{BotUUID: bot.UUID, Platform: "telegram", ExternalActorID: "ou_1", DisplayName: name})
		require.NoError(t, err)
	}
	require.Equal(t, "Alice", actor.DisplayName)
}

// A database from before the name column existed gains it on the next start.
func TestMigrateBotAccessTablesAddsDirectChatName(t *testing.T) {
	sm, err := NewStoreManager(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = sm.Close() })
	db := sm.BotAccess().db
	require.NoError(t, db.Exec(`ALTER TABLE remote_direct_chats DROP COLUMN name`).Error)
	require.False(t, db.Migrator().HasColumn(&remoteDirectChatRecord{}, "name"))
	require.NoError(t, migrateBotAccessTables(db))
	require.True(t, db.Migrator().HasColumn(&remoteDirectChatRecord{}, "name"))
}
