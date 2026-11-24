package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/remote/access"
)

// A platform-reported name is kept apart from the alias: an empty or id-echo
// name never wipes it and the operator's alias is untouched.
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
	got, _, err := store.GetDirectChat(ctx, bot.UUID, chat.ID)
	require.NoError(t, err)
	require.Empty(t, got.Name, "an unpaired chat has no peer, so no name")
	require.NoError(t, store.PairDirectChat(ctx, chat.ID, "42", "Alice"))
	for _, name := range []string{"", "42"} {
		require.NoError(t, store.PairDirectChat(ctx, chat.ID, "42", name))
	}
	chats, err := store.ListDirectChats(ctx, bot.UUID)
	require.NoError(t, err)
	require.Equal(t, "Alice", chats[0].Name)
	require.Empty(t, chats[0].Alias)
	got, _, err = store.GetDirectChat(ctx, bot.UUID, chat.ID)
	require.NoError(t, err)
	require.Equal(t, "Alice", got.Name)

	actor, err := store.UpsertActor(ctx, access.Actor{BotUUID: bot.UUID, Platform: "telegram", ExternalActorID: "ou_1", DisplayName: "Alice"})
	require.NoError(t, err)
	for _, name := range []string{"", "ou_1"} {
		actor, err = store.UpsertActor(ctx, access.Actor{BotUUID: bot.UUID, Platform: "telegram", ExternalActorID: "ou_1", DisplayName: name})
		require.NoError(t, err)
	}
	require.Equal(t, "Alice", actor.DisplayName)
}
