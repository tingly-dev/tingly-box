package bot_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/remote/access"
	"github.com/tingly-dev/tingly-box/remote/control/bot"
)

type authorizationOnlineTransport struct{}

func (authorizationOnlineTransport) TransportFacts(string, access.CapabilityName, access.ActionName) (access.TransportStatus, bool) {
	return access.TransportOnline, true
}

func directAuthorizationFixture(t *testing.T) (*db.StoreManager, db.Settings, imbot.Message) {
	t.Helper()
	sm, err := db.NewStoreManager(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = sm.Close() })
	setting, err := sm.ImBotSettings().CreateSettings(db.Settings{Name: "direct", Platform: "weixin", Enabled: true})
	require.NoError(t, err)
	require.NoError(t, sm.BotAccess().PutCapability(context.Background(), access.BotCapability{BotUUID: setting.UUID, Name: access.CapabilityRemoteControl, Enabled: true}))
	sm.BotAccess().SetTransportFactsSource(authorizationOnlineTransport{})
	msg := imbot.Message{
		Sender:    imbot.Sender{ID: "alice", DisplayName: "Alice"},
		Recipient: imbot.Recipient{ID: "direct-alice"},
		ChatType:  imbot.ChatTypeDirect,
		Content:   imbot.NewTextContent("hello"),
	}
	return sm, setting, msg
}

func TestAuthorizationGateAutoPairsDirectPeerWhenPairingDisabled(t *testing.T) {
	sm, setting, msg := directAuthorizationFixture(t)
	gate := bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), false, nil, nil)
	require.False(t, gate(msg, imbot.Platform(setting.Platform), setting.UUID), "authorized direct message must continue to Remote Control")

	chats, err := sm.BotAccess().ListDirectChats(context.Background(), setting.UUID)
	require.NoError(t, err)
	require.Len(t, chats, 1)
	require.NotEmpty(t, chats[0].PeerActorID)
	permissions, err := sm.BotAccess().ListDirectChatPermissions(context.Background(), setting.UUID, chats[0].ID)
	require.NoError(t, err)
	allowed := false
	for _, permission := range permissions {
		if permission.Capability == access.CapabilityRemoteControl && permission.Action == access.ActionRemoteControlStart {
			allowed = permission.Effect == access.EffectAllow
		}
	}
	require.True(t, allowed)
}

func TestAuthorizationGateKeepsUnpairedDirectPeerDeniedWhenPairingRequired(t *testing.T) {
	sm, setting, msg := directAuthorizationFixture(t)
	gate := bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), true, nil, nil)
	require.True(t, gate(msg, imbot.Platform(setting.Platform), setting.UUID), "unpaired direct message must be claimed")

	chats, err := sm.BotAccess().ListDirectChats(context.Background(), setting.UUID)
	require.NoError(t, err)
	require.Len(t, chats, 1)
	require.Empty(t, chats[0].PeerActorID)
}

func TestAuthorizationGateMigratesLegacyPairedDirectPeer(t *testing.T) {
	sm, setting, msg := directAuthorizationFixture(t)
	require.NoError(t, sm.RemoteChats().SetPaired(msg.Recipient.ID, setting.Platform, setting.UUID, msg.Sender.ID))
	gate := bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), true, nil, nil)
	require.False(t, gate(msg, imbot.Platform(setting.Platform), setting.UUID), "legacy paired direct message must remain authorized")

	chats, err := sm.BotAccess().ListDirectChats(context.Background(), setting.UUID)
	require.NoError(t, err)
	require.Len(t, chats, 1)
	require.NotEmpty(t, chats[0].PeerActorID)
}

// TestAuthorizationGateDeniedPromptReplyNotifiesUser reproduces the field
// failure where a paired chat's remote_control.approve row is deny: the reply
// to the bot's own pending prompt must be claimed AND the sender must get
// explicit feedback instead of silence.
func TestAuthorizationGateDeniedPromptReplyNotifiesUser(t *testing.T) {
	sm, setting, msg := directAuthorizationFixture(t)

	// First message auto-pairs the peer (pairing disabled) and allows start.
	pendingNone := func(string) (access.CapabilityName, access.ActionName) { return "", "" }
	gate := bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), false, pendingNone, nil)
	require.False(t, gate(msg, imbot.Platform(setting.Platform), setting.UUID))

	chats, err := sm.BotAccess().ListDirectChats(context.Background(), setting.UUID)
	require.NoError(t, err)
	require.Len(t, chats, 1)

	// Break the approve row the way the field data had it.
	require.NoError(t, sm.BotAccess().SetDirectChatPermission(context.Background(),
		setting.UUID, chats[0].ID, access.CapabilityRemoteControl, access.ActionRemoteControlApprove, access.EffectDeny))

	// A prompt is now pending in the chat; the reply "2" requires approve.
	pendingApprove := func(string) (access.CapabilityName, access.ActionName) {
		return access.CapabilityRemoteControl, access.ActionRemoteControlApprove
	}
	notified := 0
	gate = bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), false, pendingApprove,
		func(imbot.Message, imbot.Platform, string) { notified++ })

	reply := msg
	reply.Content = imbot.NewTextContent("2")
	require.True(t, gate(reply, imbot.Platform(setting.Platform), setting.UUID), "denied approve must be claimed")
	require.Equal(t, 1, notified, "denied prompt reply must notify the sender")

	// Restore approve and confirm the same reply now passes and does not notify.
	require.NoError(t, sm.BotAccess().SetDirectChatPermission(context.Background(),
		setting.UUID, chats[0].ID, access.CapabilityRemoteControl, access.ActionRemoteControlApprove, access.EffectAllow))
	require.False(t, gate(reply, imbot.Platform(setting.Platform), setting.UUID))
	require.Equal(t, 1, notified)
}
