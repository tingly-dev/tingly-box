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

func TestAuthorizationGateAllowsPairingCommandsForUnpairedDirectPeer(t *testing.T) {
	sm, setting, msg := directAuthorizationFixture(t)
	gate := bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), true, nil, nil)
	for _, text := range []string{"/pair CODE", "/bind CODE"} {
		msg.Content = imbot.NewTextContent(text)
		require.False(t, gate(msg, imbot.Platform(setting.Platform), setting.UUID), text)
	}
	msg.Content = imbot.NewTextContent("/pairing CODE")
	require.True(t, gate(msg, imbot.Platform(setting.Platform), setting.UUID))
}

// Telegram groups send /pair@bot, Slack and Discord prefix a mention: an
// unpaired chat must still be able to pair itself.
func TestAuthorizationGateAllowsDecoratedPairingCommands(t *testing.T) {
	sm, setting, msg := directAuthorizationFixture(t)
	gate := bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), true, nil, nil)
	for _, text := range []string{"/pair@tingly_bot CODE", " /pair CODE", "<@U123> /pair CODE", "／pair CODE"} {
		msg.Content = imbot.NewTextContent(text)
		require.False(t, gate(msg, imbot.Platform(setting.Platform), setting.UUID), text)
	}
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
		func(_ imbot.Message, _ imbot.Platform, _ string, action access.ActionName) {
			require.Equal(t, access.ActionRemoteControlApprove, action)
			notified++
		})

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

// A press on a Projects prompt's button ("pj") answers an approval, so it
// needs remote_control.approve like an @cc prompt's ("perm"), even when no
// @cc prompt is pending in the chat.
func TestAuthorizationGateProjectPromptNeedsApprove(t *testing.T) {
	sm, setting, msg := directAuthorizationFixture(t)
	pendingNone := func(string) (access.CapabilityName, access.ActionName) { return "", "" }
	gate := bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), false, pendingNone, nil)
	require.False(t, gate(msg, imbot.Platform(setting.Platform), setting.UUID))
	chats, err := sm.BotAccess().ListDirectChats(context.Background(), setting.UUID)
	require.NoError(t, err)
	require.Len(t, chats, 1)
	require.NoError(t, sm.BotAccess().SetDirectChatPermission(context.Background(),
		setting.UUID, chats[0].ID, access.CapabilityRemoteControl, access.ActionRemoteControlApprove, access.EffectDeny))

	press := msg
	press.Payload = imbot.NewPayload("pj", "thread-1", "req-1", "allow")
	require.True(t, press.IsCallback())
	require.True(t, gate(press, imbot.Platform(setting.Platform), setting.UUID), "a chat that may not approve cannot press it")
	require.False(t, gate(msg, imbot.Platform(setting.Platform), setting.UUID), "it may still talk")

	require.NoError(t, sm.BotAccess().SetDirectChatPermission(context.Background(),
		setting.UUID, chats[0].ID, access.CapabilityRemoteControl, access.ActionRemoteControlApprove, access.EffectAllow))
	require.False(t, gate(press, imbot.Platform(setting.Platform), setting.UUID))
}

func TestPendingApprovalDoesNotGrantDirectoryOrProjectAccess(t *testing.T) {
	sm, setting, msg := directAuthorizationFixture(t)
	gate := bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), false, nil, nil)
	require.False(t, gate(msg, imbot.Platform(setting.Platform), setting.UUID), "first message pairs the chat")
	pending := func(string) (access.CapabilityName, access.ActionName) {
		return access.CapabilityRemoteControl, access.ActionRemoteControlApprove
	}
	gate = bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), false, pending, nil)
	for _, text := range []string{"/dir", "/dir 1", "/project", "/project 1", "/cd /tmp"} {
		m := msg
		m.Content = imbot.NewTextContent(text)
		require.True(t, gate(m, imbot.Platform(setting.Platform), setting.UUID), "%s must require privileged access", text)
	}
	m := msg
	m.Payload = imbot.NewPayload("project", "switch", "/tmp")
	require.True(t, gate(m, imbot.Platform(setting.Platform), setting.UUID), "folder buttons must require privileged access")
}

// A paired chat is not privileged until the operator grants it. The
// commands that need it must stay denied however the platform decorates them
// (Telegram's /dir@bot, a Slack or Discord mention, a Feishu placeholder, a
// full-width slash, a leading space): a decorated command must not slip past
// as an ordinary message.
func TestAuthorizationGatePrivilegedCommandsStayDeniedWhenDecorated(t *testing.T) {
	sm, setting, msg := directAuthorizationFixture(t)
	gate := bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), false, nil, nil)
	require.False(t, gate(msg, imbot.Platform(setting.Platform), setting.UUID), "first message pairs the chat")
	for _, text := range []string{
		"/dir", "/DIR", " /dir", "/dir@tingly_bot", "/dir@tingly_bot 1", "@tingly_bot /dir",
		"<@U123> /dir", "<@!123> /cd /tmp", "@tingly_bot/dir", "<@U123>/dir", "  /dir   ", "/dir   1", "@_user_1 /project", "／dir", "/cd@tingly_bot /tmp", "/bash ls",
	} {
		m := msg
		m.Content = imbot.NewTextContent(text)
		require.True(t, gate(m, imbot.Platform(setting.Platform), setting.UUID), "%q must require privileged access", text)
	}
	for _, text := range []string{"/status", "/help@tingly_bot", "@tingly_bot /new hello", "/directory"} {
		m := msg
		m.Content = imbot.NewTextContent(text)
		require.False(t, gate(m, imbot.Platform(setting.Platform), setting.UUID), "%q needs no privileged access", text)
	}
}

// A privileged command the chat may not run is answered, not dropped: the
// silent drop made /dir and /cd look broken in every freshly paired chat.
func TestAuthorizationGateDeniedPrivilegedCommandTellsTheChat(t *testing.T) {
	sm, setting, msg := directAuthorizationFixture(t)
	var told []access.ActionName
	notify := func(_ imbot.Message, _ imbot.Platform, _ string, action access.ActionName) {
		told = append(told, action)
	}
	gate := bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), false, nil, notify)
	require.False(t, gate(msg, imbot.Platform(setting.Platform), setting.UUID), "first message pairs the chat")
	require.Empty(t, told)

	for _, text := range []string{"/dir", "/dir@tingly_bot", "/cd /tmp", "/project"} {
		m := msg
		m.Content = imbot.NewTextContent(text)
		require.True(t, gate(m, imbot.Platform(setting.Platform), setting.UUID), text)
	}
	press := msg
	press.Payload = imbot.NewPayload("project", "switch", "/tmp")
	require.True(t, gate(press, imbot.Platform(setting.Platform), setting.UUID))
	require.Equal(t, []access.ActionName{
		access.ActionRemoteControlPrivileged, access.ActionRemoteControlPrivileged, access.ActionRemoteControlPrivileged,
		access.ActionRemoteControlPrivileged, access.ActionRemoteControlPrivileged,
	}, told)
	require.Contains(t, bot.DeniedActionText(access.ActionRemoteControlPrivileged), "privileged")

	// Once granted the command goes through and nothing is said.
	chats, err := sm.BotAccess().ListDirectChats(context.Background(), setting.UUID)
	require.NoError(t, err)
	require.Len(t, chats, 1)
	require.NoError(t, sm.BotAccess().SetDirectChatPermission(context.Background(), setting.UUID, chats[0].ID,
		access.CapabilityRemoteControl, access.ActionRemoteControlPrivileged, access.EffectAllow))
	told = nil
	m := msg
	m.Content = imbot.NewTextContent("/dir@tingly_bot")
	require.False(t, gate(m, imbot.Platform(setting.Platform), setting.UUID))
	require.Empty(t, told)
}

// A chat that may not even start a run learns nothing from the bot.
func TestAuthorizationGateDeniedChatGetsNoPrivilegedNotice(t *testing.T) {
	sm, setting, msg := directAuthorizationFixture(t)
	told := 0
	notify := func(imbot.Message, imbot.Platform, string, access.ActionName) { told++ }
	gate := bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), false, nil, notify)
	require.False(t, gate(msg, imbot.Platform(setting.Platform), setting.UUID), "first message pairs the chat")
	chats, err := sm.BotAccess().ListDirectChats(context.Background(), setting.UUID)
	require.NoError(t, err)
	require.NoError(t, sm.BotAccess().SetDirectChatPermission(context.Background(), setting.UUID, chats[0].ID,
		access.CapabilityRemoteControl, access.ActionRemoteControlStart, access.EffectDeny))
	m := msg
	m.Content = imbot.NewTextContent("/dir")
	require.True(t, gate(m, imbot.Platform(setting.Platform), setting.UUID))
	require.Zero(t, told)
}
