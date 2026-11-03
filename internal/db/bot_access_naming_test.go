package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/remote/access"
)

func newNamingStore(t *testing.T) (*StoreManager, *BotAccessStore, Settings) {
	t.Helper()
	sm, err := NewStoreManager(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = sm.Close() })
	return sm, sm.BotAccess(), createAccessBot(t, sm, "naming")
}

func TestActorNameNeverWipedByEmptyOrIDEcho(t *testing.T) {
	_, store, bot := newNamingStore(t)
	ctx := context.Background()
	first, err := store.UpsertActor(ctx, access.Actor{BotUUID: bot.UUID, Platform: "feishu", ExternalActorID: "ou_1", DisplayName: "Alice"})
	require.NoError(t, err)
	require.Equal(t, "Alice", first.DisplayName)
	require.Equal(t, access.AliasSourcePlatform, first.DisplayNameSource)

	for _, nameless := range []string{"", "  ", "ou_1"} {
		again, err := store.UpsertActor(ctx, access.Actor{BotUUID: bot.UUID, Platform: "feishu", ExternalActorID: "ou_1", DisplayName: nameless})
		require.NoError(t, err)
		require.Equal(t, "Alice", again.DisplayName, "name %q must not wipe", nameless)
	}

	renamed, err := store.UpsertActor(ctx, access.Actor{BotUUID: bot.UUID, Platform: "feishu", ExternalActorID: "ou_1", DisplayName: "Alice B"})
	require.NoError(t, err)
	require.Equal(t, "Alice B", renamed.DisplayName)
}

func TestActorIDEchoIsNotStoredAsName(t *testing.T) {
	_, store, bot := newNamingStore(t)
	actor, err := store.UpsertActor(context.Background(), access.Actor{BotUUID: bot.UUID, Platform: "feishu", ExternalActorID: "ou_1", DisplayName: "ou_1"})
	require.NoError(t, err)
	require.Empty(t, actor.DisplayName)
	require.Empty(t, actor.DisplayNameSource)
}

func TestUserSetActorNameSurvivesPlatformNames(t *testing.T) {
	_, store, bot := newNamingStore(t)
	ctx := context.Background()
	group, err := store.DiscoverGroup(ctx, bot.UUID, "telegram", "g1", "Team")
	require.NoError(t, err)
	manual, err := store.AddGroupActor(ctx, bot.UUID, group.ID, "u1", "Operator Alice", "")
	require.NoError(t, err)
	require.Equal(t, access.AliasSourceUser, manual.DisplayNameSource)

	after, err := store.UpsertActor(ctx, access.Actor{BotUUID: bot.UUID, Platform: "telegram", ExternalActorID: "u1", DisplayName: "alice_tg"})
	require.NoError(t, err)
	require.Equal(t, "Operator Alice", after.DisplayName)
	require.Equal(t, access.AliasSourceUser, after.DisplayNameSource)
}

func TestGroupPlatformNameIsDefaultAliasAndRefreshes(t *testing.T) {
	_, store, bot := newNamingStore(t)
	ctx := context.Background()
	g, err := store.DiscoverGroup(ctx, bot.UUID, "telegram", "g1", "Release Crew")
	require.NoError(t, err)
	require.Equal(t, "Release Crew", g.Alias)
	require.Equal(t, access.AliasSourcePlatform, g.AliasSource)

	// empty inbound name never wipes
	g, err = store.DiscoverGroup(ctx, bot.UUID, "telegram", "g1", "")
	require.NoError(t, err)
	require.Equal(t, "Release Crew", g.Alias)

	// platform rename refreshes a platform-sourced alias
	g, err = store.DiscoverGroup(ctx, bot.UUID, "telegram", "g1", "Release Crew 2")
	require.NoError(t, err)
	require.Equal(t, "Release Crew 2", g.Alias)
	require.Equal(t, "Release Crew 2", g.Name)

	// user alias wins and is never overwritten, but the cached name still moves
	require.NoError(t, store.SetTargetAlias(ctx, bot.UUID, g.ID, "group", "Ops"))
	g, err = store.DiscoverGroup(ctx, bot.UUID, "telegram", "g1", "Release Crew 3")
	require.NoError(t, err)
	require.Equal(t, "Ops", g.Alias)
	require.Equal(t, access.AliasSourceUser, g.AliasSource)
	require.Equal(t, "Release Crew 3", g.Name)

	// clearing the user alias falls back to the latest platform name
	require.NoError(t, store.SetTargetAlias(ctx, bot.UUID, g.ID, "group", ""))
	g, _, err = store.GetGroup(ctx, bot.UUID, g.ID)
	require.NoError(t, err)
	require.Equal(t, "Release Crew 3", g.Alias)
	require.Equal(t, access.AliasSourcePlatform, g.AliasSource)
}

func TestGroupWithoutPlatformNameHasNoAlias(t *testing.T) {
	_, store, bot := newNamingStore(t)
	g, err := store.DiscoverGroup(context.Background(), bot.UUID, "feishu", "oc_1", "oc_1")
	require.NoError(t, err)
	require.Empty(t, g.Alias)
	require.Empty(t, g.Name)
}

func TestDirectChatTakesPeerNameAsDefaultAlias(t *testing.T) {
	_, store, bot := newNamingStore(t)
	ctx := context.Background()
	chat, err := store.DiscoverDirectChat(ctx, bot.UUID, "telegram", "42")
	require.NoError(t, err)
	require.Empty(t, chat.Alias)

	require.NoError(t, store.SyncDirectChatName(ctx, bot.UUID, chat.ID, ""))
	chat, _, _ = store.GetDirectChat(ctx, bot.UUID, chat.ID)
	require.Empty(t, chat.Alias, "no name, no alias: display falls back to the id")

	require.NoError(t, store.SyncDirectChatName(ctx, bot.UUID, chat.ID, "Alice"))
	chat, _, _ = store.GetDirectChat(ctx, bot.UUID, chat.ID)
	require.Equal(t, "Alice", chat.Alias)
	require.Equal(t, access.AliasSourcePlatform, chat.AliasSource)

	require.NoError(t, store.SyncDirectChatName(ctx, bot.UUID, chat.ID, ""))
	chat, _, _ = store.GetDirectChat(ctx, bot.UUID, chat.ID)
	require.Equal(t, "Alice", chat.Alias, "empty name never wipes")

	require.NoError(t, store.SetTargetAlias(ctx, bot.UUID, chat.ID, "chat", "Boss"))
	require.NoError(t, store.SyncDirectChatName(ctx, bot.UUID, chat.ID, "Alice Renamed"))
	chat, _, _ = store.GetDirectChat(ctx, bot.UUID, chat.ID)
	require.Equal(t, "Boss", chat.Alias, "user alias is never overwritten")
	require.Equal(t, access.AliasSourceUser, chat.AliasSource)
}

func TestPairingFillsDirectChatAliasFromPeer(t *testing.T) {
	_, store, bot := newNamingStore(t)
	ctx := context.Background()
	chat, err := store.DiscoverDirectChat(ctx, bot.UUID, "telegram", "42")
	require.NoError(t, err)
	require.NoError(t, store.PairDirectChat(ctx, chat.ID, "42", "Alice"))
	chat, _, _ = store.GetDirectChat(ctx, bot.UUID, chat.ID)
	require.Equal(t, "Alice", chat.Alias)
}

func TestClearingChatAliasFallsBackToPeerName(t *testing.T) {
	_, store, bot := newNamingStore(t)
	ctx := context.Background()
	chat, err := store.DiscoverDirectChat(ctx, bot.UUID, "telegram", "42")
	require.NoError(t, err)
	require.NoError(t, store.PairDirectChat(ctx, chat.ID, "42", "Alice"))
	require.NoError(t, store.SetTargetAlias(ctx, bot.UUID, chat.ID, "chat", "Boss"))
	require.NoError(t, store.SetTargetAlias(ctx, bot.UUID, chat.ID, "chat", ""))
	chat, _, _ = store.GetDirectChat(ctx, bot.UUID, chat.ID)
	require.Equal(t, "Alice", chat.Alias)
	require.Equal(t, access.AliasSourcePlatform, chat.AliasSource)
}

func TestNamingMigrationBackfillsLegacyRowsOnce(t *testing.T) {
	sm, store, bot := newNamingStore(t)
	ctx := context.Background()
	db := store.db
	// Simulate a pre-provenance database: drop the new columns and seed legacy rows.
	for _, stmt := range []string{
		`ALTER TABLE remote_actors DROP COLUMN display_name_source`,
		`ALTER TABLE remote_groups DROP COLUMN alias_source`,
		`ALTER TABLE remote_direct_chats DROP COLUMN alias_source`,
	} {
		require.NoError(t, db.Exec(stmt).Error)
	}
	require.NoError(t, db.Exec(`INSERT INTO remote_actors (id,bot_uuid,platform,external_actor_id,display_name,created_at,updated_at) VALUES ('a1',?,'telegram','1','Alice',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),('a2',?,'feishu','ou_2','ou_2',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, bot.UUID, bot.UUID).Error)
	require.NoError(t, db.Exec(`INSERT INTO remote_groups (id,bot_uuid,platform,external_group_id,alias,name,created_at,updated_at) VALUES ('g1',?,'telegram','-1','Mine','Plat',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),('g2',?,'telegram','-2','','Plat2',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),('g3',?,'feishu','oc_3','','oc_3',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, bot.UUID, bot.UUID, bot.UUID).Error)
	require.NoError(t, db.Exec(`INSERT INTO remote_direct_chats (id,bot_uuid,platform,external_chat_id,alias,peer_actor_id,created_at,updated_at) VALUES ('c1',?,'telegram','1','Boss','a1',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),('c2',?,'telegram','9','','a1',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, bot.UUID, bot.UUID).Error)

	require.NoError(t, migrateNamingModel(db))

	g1, _, _ := store.GetGroup(ctx, bot.UUID, "g1")
	require.Equal(t, "Mine", g1.Alias)
	require.Equal(t, access.AliasSourceUser, g1.AliasSource)
	g2, _, _ := store.GetGroup(ctx, bot.UUID, "g2")
	require.Equal(t, "Plat2", g2.Alias)
	require.Equal(t, access.AliasSourcePlatform, g2.AliasSource)
	g3, _, _ := store.GetGroup(ctx, bot.UUID, "g3")
	require.Empty(t, g3.Alias, "id-echo group name is not an alias")
	c1, _, _ := store.GetDirectChat(ctx, bot.UUID, "c1")
	require.Equal(t, "Boss", c1.Alias)
	require.Equal(t, access.AliasSourceUser, c1.AliasSource)
	c2, _, _ := store.GetDirectChat(ctx, bot.UUID, "c2")
	require.Equal(t, "Alice", c2.Alias)
	require.Equal(t, access.AliasSourcePlatform, c2.AliasSource)
	var echo remoteActorRecord
	require.NoError(t, db.Where("id = 'a2'").First(&echo).Error)
	require.Empty(t, echo.DisplayName)

	// A second run must not re-label anything.
	require.NoError(t, store.SetTargetAlias(ctx, bot.UUID, "c2", "chat", ""))
	require.NoError(t, migrateNamingModel(db))
	_ = sm
}
