package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/remote/access"
)

type onlineTransport struct{}

func (onlineTransport) TransportFacts(string, access.CapabilityName, access.ActionName) (access.TransportStatus, bool) {
	return access.TransportOnline, true
}

func createAccessBot(t *testing.T, sm *StoreManager, name string) Settings {
	t.Helper()
	bot, err := sm.ImBotSettings().CreateSettings(Settings{Name: name, Platform: "telegram", AuthType: "token", Auth: map[string]string{"token": "test"}, Enabled: true})
	require.NoError(t, err)
	return bot
}

func TestBotAccessStoreExplicitDefaultsAndAuthorization(t *testing.T) {
	sm, err := NewStoreManager(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = sm.Close() })
	bot := createAccessBot(t, sm, "ops")
	store := sm.BotAccess()
	store.SetTransportFactsSource(onlineTransport{})
	ctx := context.Background()
	require.NoError(t, store.PutCapability(ctx, access.BotCapability{BotUUID: bot.UUID, Name: access.CapabilityRemoteControl, Enabled: true}))
	chat, err := store.DiscoverDirectChat(ctx, bot.UUID, "telegram", "external-chat")
	require.NoError(t, err)
	permissions, err := store.ListDirectChatPermissions(ctx, bot.UUID, chat.ID)
	require.NoError(t, err)
	require.Len(t, permissions, 7)
	for _, permission := range permissions {
		require.Equal(t, access.EffectDeny, permission.Effect, "new DirectChat must be explicit deny")
	}

	actor, err := store.UpsertActor(ctx, access.Actor{BotUUID: bot.UUID, Platform: "telegram", ExternalActorID: "alice"})
	require.NoError(t, err)
	authorizer := access.NewEvaluator(store)
	req := access.AuthorizationRequest{BotUUID: bot.UUID, Target: access.TargetRef{Kind: access.TargetDirectChat, ID: chat.ID}, ActorID: actor.ID, Capability: access.CapabilityRemoteControl, Action: access.ActionRemoteControlStart}
	require.Equal(t, access.ReasonTargetCapabilityDenied, authorizer.Evaluate(ctx, req).Reason)
	require.NoError(t, store.PairDirectChat(ctx, chat.ID, "alice", "Alice"))
	actor, err = store.UpsertActor(ctx, access.Actor{BotUUID: bot.UUID, Platform: "telegram", ExternalActorID: "alice"})
	require.NoError(t, err)
	req.ActorID = actor.ID
	require.True(t, authorizer.Evaluate(ctx, req).Allowed)
	req.Action = access.ActionRemoteControlPrivileged
	require.Equal(t, access.ReasonTargetActionDenied, authorizer.Evaluate(ctx, req).Reason,
		"capability access is allowed after pairing; the privileged action row itself is deny")
}

func TestBotAccessStoreGroupDoesNotImplicitlyTrustMembers(t *testing.T) {
	sm, err := NewStoreManager(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = sm.Close() })
	bot := createAccessBot(t, sm, "group-bot")
	store := sm.BotAccess()
	store.SetTransportFactsSource(onlineTransport{})
	ctx := context.Background()
	require.NoError(t, store.PutCapability(ctx, access.BotCapability{BotUUID: bot.UUID, Name: access.CapabilityRemoteControl, Enabled: true}))
	group, err := store.DiscoverGroup(ctx, bot.UUID, "telegram", "engineering", "Engineering")
	require.NoError(t, err)
	require.NoError(t, store.SetGroupCapability(ctx, bot.UUID, group.ID, access.CapabilityRemoteControl, access.EffectAllow))
	actor, err := store.UpsertActor(ctx, access.Actor{BotUUID: bot.UUID, Platform: "telegram", ExternalActorID: "alice"})
	require.NoError(t, err)
	req := access.AuthorizationRequest{BotUUID: bot.UUID, Target: access.TargetRef{Kind: access.TargetGroup, ID: group.ID}, ActorID: actor.ID, Capability: access.CapabilityRemoteControl, Action: access.ActionRemoteControlStart}
	require.Equal(t, access.ReasonActorNotRegistered, access.NewEvaluator(store).Evaluate(ctx, req).Reason)
	actor, err = store.AddGroupActor(ctx, bot.UUID, group.ID, "alice", "Alice", "Controller")
	require.NoError(t, err)
	req.ActorID = actor.ID
	require.True(t, access.NewEvaluator(store).Evaluate(ctx, req).Allowed)
}

func TestBotAccessStoreRouteCannotCrossBots(t *testing.T) {
	sm, err := NewStoreManager(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = sm.Close() })
	a := createAccessBot(t, sm, "a")
	b := createAccessBot(t, sm, "b")
	ctx := context.Background()
	chat, err := sm.BotAccess().DiscoverDirectChat(ctx, a.UUID, "telegram", "one")
	require.NoError(t, err)
	_, err = sm.BotAccess().CreateRoute(ctx, access.Route{BotUUID: b.UUID, Name: "bad", Source: "claude_code", Target: access.TargetRef{Kind: access.TargetDirectChat, ID: chat.ID}}, false)
	require.ErrorIs(t, err, ErrCrossBotTarget)
}

func TestBotAccessStoreUpdateRoutePreservesIdentityAndValidatesTarget(t *testing.T) {
	sm, err := NewStoreManager(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = sm.Close() })
	bot := createAccessBot(t, sm, "routes")
	other := createAccessBot(t, sm, "other")
	ctx := context.Background()
	chat, err := sm.BotAccess().DiscoverDirectChat(ctx, bot.UUID, "telegram", "ops")
	require.NoError(t, err)
	foreign, err := sm.BotAccess().DiscoverDirectChat(ctx, other.UUID, "telegram", "foreign")
	require.NoError(t, err)
	created, err := sm.BotAccess().CreateRoute(ctx, access.Route{BotUUID: bot.UUID, Name: "before", Source: "claude_code", Target: access.TargetRef{Kind: access.TargetDirectChat, ID: chat.ID}, Enabled: true}, false)
	require.NoError(t, err)

	updated, err := sm.BotAccess().UpdateRoute(ctx, access.Route{ID: created.ID, BotUUID: bot.UUID, Name: "after", Source: "webhook", Target: access.TargetRef{Kind: access.TargetDirectChat, ID: chat.ID}}, false)
	require.NoError(t, err)
	require.Equal(t, created.ID, updated.ID)
	require.Equal(t, created.CreatedAt, updated.CreatedAt)
	require.Equal(t, "after", updated.Name)

	_, err = sm.BotAccess().UpdateRoute(ctx, access.Route{ID: created.ID, BotUUID: bot.UUID, Name: "invalid", Source: "webhook", Target: access.TargetRef{Kind: access.TargetDirectChat, ID: foreign.ID}}, false)
	require.ErrorIs(t, err, ErrCrossBotTarget)
	stored, ok, err := sm.BotAccess().GetRoute(ctx, bot.UUID, created.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "after", stored.Name)
}

func TestBotAccessMigrationBackfillsLegacyDefaultsWithoutOverwritingChoice(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewStoreManager(dir)
	require.NoError(t, err)
	bot := createAccessBot(t, sm, "legacy")
	ctx := context.Background()

	// Simulate a pre-Capability Bot and a user who explicitly disabled Remote
	// Control after the first upgrade. A subsequent migration must distinguish
	// those two states.
	require.NoError(t, sm.BotAccess().PutCapability(ctx, access.BotCapability{BotUUID: bot.UUID, Name: access.CapabilityRemoteControl, Enabled: false}))
	require.NoError(t, sm.BotAccess().db.Where("bot_uuid = ? AND capability = ?", bot.UUID, access.CapabilityNotify).Delete(&botCapabilityRecord{}).Error)
	require.NoError(t, sm.Close())

	sm, err = NewStoreManager(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sm.Close() })
	remoteControl, ok, err := sm.BotAccess().GetCapability(ctx, bot.UUID, access.CapabilityRemoteControl)
	require.NoError(t, err)
	require.True(t, ok)
	require.False(t, remoteControl.Enabled, "explicit disabled choice must survive migration")
	notify, ok, err := sm.BotAccess().GetCapability(ctx, bot.UUID, access.CapabilityNotify)
	require.NoError(t, err)
	require.True(t, ok)
	require.False(t, notify.Enabled)
}

func TestBotAccessMigrationEnablesRemoteControlForLegacyBot(t *testing.T) {
	dir := t.TempDir()
	sm, err := NewStoreManager(dir)
	require.NoError(t, err)
	bot := createAccessBot(t, sm, "legacy-missing")
	ctx := context.Background()
	require.NoError(t, sm.BotAccess().db.Where("bot_uuid = ?", bot.UUID).Delete(&botCapabilityRecord{}).Error)
	require.NoError(t, sm.Close())

	sm, err = NewStoreManager(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sm.Close() })
	remoteControl, ok, err := sm.BotAccess().GetCapability(ctx, bot.UUID, access.CapabilityRemoteControl)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, remoteControl.Enabled, "legacy Bot must default to Remote Control enabled")
}

// TestBotAccessStoreSetDirectChatPermissionsAtomic verifies the batch write
// is all-or-nothing: a valid batch lands completely, and a batch containing
// one invalid row changes nothing.
func TestBotAccessStoreSetDirectChatPermissionsAtomic(t *testing.T) {
	sm, err := NewStoreManager(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = sm.Close() })
	store := sm.BotAccess()
	ctx := context.Background()

	bot, err := sm.ImBotSettings().CreateSettings(Settings{Name: "batch", Platform: "telegram", Enabled: true})
	require.NoError(t, err)
	chat, err := store.DiscoverDirectChat(ctx, bot.UUID, "telegram", "chat-1")
	require.NoError(t, err)

	effectOf := func(action access.ActionName) access.AccessEffect {
		perms, err := store.ListDirectChatPermissions(ctx, bot.UUID, chat.ID)
		require.NoError(t, err)
		for _, p := range perms {
			if p.Capability == access.CapabilityRemoteControl && p.Action == action {
				return p.Effect
			}
		}
		return ""
	}

	// Valid batch: the preset trio lands together.
	batch := []access.Permission{
		{Capability: access.CapabilityRemoteControl, Action: access.ActionAccess, Effect: access.EffectAllow},
		{Capability: access.CapabilityRemoteControl, Action: access.ActionRemoteControlStart, Effect: access.EffectAllow},
		{Capability: access.CapabilityRemoteControl, Action: access.ActionRemoteControlApprove, Effect: access.EffectAllow},
	}
	require.NoError(t, store.SetDirectChatPermissions(ctx, bot.UUID, chat.ID, batch))
	require.Equal(t, access.EffectAllow, effectOf(access.ActionRemoteControlStart))
	require.Equal(t, access.EffectAllow, effectOf(access.ActionRemoteControlApprove))

	// Invalid row anywhere in the batch: nothing changes.
	bad := []access.Permission{
		{Capability: access.CapabilityRemoteControl, Action: access.ActionRemoteControlStart, Effect: access.EffectDeny},
		{Capability: access.CapabilityRemoteControl, Action: access.ActionRemoteControlApprove, Effect: access.AccessEffect("bogus")},
	}
	require.Error(t, store.SetDirectChatPermissions(ctx, bot.UUID, chat.ID, bad))
	require.Equal(t, access.EffectAllow, effectOf(access.ActionRemoteControlStart), "failed batch must not half-apply")
	require.Equal(t, access.EffectAllow, effectOf(access.ActionRemoteControlApprove))

	// Unknown chat and empty batch are rejected.
	require.ErrorIs(t, store.SetDirectChatPermissions(ctx, bot.UUID, "ghost", batch), ErrAccessTargetNotFound)
	require.ErrorIs(t, store.SetDirectChatPermissions(ctx, bot.UUID, chat.ID, nil), ErrInvalidPermission)
}
