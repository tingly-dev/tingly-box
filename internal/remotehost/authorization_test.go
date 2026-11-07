package bot_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/remote/access"
	"github.com/tingly-dev/tingly-box/remote/bot"
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
		func(imbot.Message, imbot.Platform, string, access.ActionName) { notified++ })

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
	// A free path stays privileged while an approval is pending.
	m := msg
	m.Content = imbot.NewTextContent("/cd /tmp")
	require.True(t, gate(m, imbot.Platform(setting.Platform), setting.UUID), "/cd must require privileged access")

	// The pickers need start, so a chat that may only approve cannot use them.
	chats, err := sm.BotAccess().ListDirectChats(context.Background(), setting.UUID)
	require.NoError(t, err)
	require.NoError(t, sm.BotAccess().SetDirectChatPermission(context.Background(),
		setting.UUID, chats[0].ID, access.CapabilityRemoteControl, access.ActionRemoteControlStart, access.EffectDeny))
	m.Content = imbot.NewTextContent("/dir")
	require.True(t, gate(m, imbot.Platform(setting.Platform), setting.UUID), "/dir needs start, not approve")
}

// Pairing happens in a direct chat, where only the plain command counts.
func TestAuthorizationGateReadsPairingCommandsAsDirectChatText(t *testing.T) {
	sm, setting, msg := directAuthorizationFixture(t)
	gate := bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), true, nil, nil)
	for text, claimed := range map[string]bool{" /pair CODE": false, "\uff0fbind CODE": false, "@tingly_bot /pair CODE": true, "/pair@tingly_bot CODE": true} {
		msg.Content = imbot.NewTextContent(text)
		require.Equal(t, claimed, gate(msg, imbot.Platform(setting.Platform), setting.UUID), text)
	}
}

// Choosing among registered folders and projects needs start; a free path and
// the directory browser need privileged; in a group every one of them does.
// Only a group strips mentions and "/cmd@bot", so a mention before a command in
// a direct chat is ordinary text.
func TestInboundAction(t *testing.T) {
	const start, privileged = access.ActionRemoteControlStart, access.ActionRemoteControlPrivileged
	msg := func(group bool, text string, segments ...string) imbot.Message {
		m := imbot.Message{ChatType: imbot.ChatTypeDirect, Content: imbot.NewTextContent(text)}
		if group {
			m.ChatType = imbot.ChatTypeGroup
		}
		if len(segments) > 0 {
			m.Payload = imbot.NewPayload(segments...)
		}
		return m
	}
	file := imbot.NewMediaContent([]imbot.MediaAttachment{{URL: "x"}}, "")
	direct, group := false, true
	for _, tt := range []struct {
		name string
		msg  imbot.Message
		want access.ActionName
	}{
		{"/dir", msg(direct, "/dir"), start},
		{"/DIR  2 padded", msg(direct, "  /DIR   2  "), start},
		{"full-width /dir", msg(direct, "\uff0fdir"), start},
		{"/project 1", msg(direct, "/project 1"), start},
		{"/dir path", msg(direct, "/dir /tmp"), privileged},
		{"/dir 0", msg(direct, "/dir 0"), privileged},
		{"/dir 1a", msg(direct, "/dir 1a"), privileged},
		{"/cd", msg(direct, "/cd /tmp"), privileged},
		{"/status", msg(direct, "/status"), start},
		{"/directory", msg(direct, "/directory"), start},
		{"mention is text", msg(direct, "@tingly_bot /cd /tmp"), start},
		{"bot suffix is not stripped", msg(direct, "/cd@tingly_bot /tmp"), start},
		{"folder button", msg(direct, "", "project", "switch", "/tmp"), start},
		{"menu project button", msg(direct, "", "action", "project"), start},
		{"menu bind button", msg(direct, "", "action", "bind"), privileged},
		{"browser button", msg(direct, "", "bind", "dir", "x"), privileged},
		{"direct file", func() imbot.Message { m := msg(direct, ""); m.Content = file; return m }(), privileged},

		{"group /dir", msg(group, "/dir"), privileged},
		{"group /dir 1", msg(group, "/dir 1"), privileged},
		{"group /project", msg(group, "/project"), privileged},
		{"group suffix", msg(group, "/dir@tingly_bot 1"), privileged},
		{"group mention", msg(group, "@tingly_bot /dir"), privileged},
		{"group mention no space", msg(group, "@tingly_bot/cd /tmp"), privileged},
		{"group slack mention", msg(group, "<@U123>/project"), privileged},
		{"group feishu mention", msg(group, "@_user_1 /dir"), privileged},
		{"group full-width", msg(group, "\uff0fdir"), privileged},
		{"group /status", msg(group, "@tingly_bot /status"), start},
		{"group folder button", msg(group, "", "project", "switch", "/tmp"), privileged},
		{"group menu project button", msg(group, "", "action", "project"), privileged},
	} {
		_, got := bot.InboundAction(tt.msg, "", "")
		require.Equal(t, tt.want, got, tt.name)
		// A pending approval never changes what a folder command needs.
		_, got = bot.InboundAction(tt.msg, access.CapabilityRemoteControl, access.ActionRemoteControlApprove)
		if tt.want == privileged || tt.msg.IsCallback() {
			require.Equal(t, tt.want, got, tt.name+" with a pending approval")
		}
	}
}

// A denied privileged command is answered, not dropped, so it does not look
// broken; a chat that may not even start learns nothing, and a file posted in
// a group is chatter that is refused without a reply.
func TestAuthorizationGateTellsTheChatWhatItLacks(t *testing.T) {
	sm, setting, msg := directAuthorizationFixture(t)
	var told []access.ActionName
	notify := func(_ imbot.Message, _ imbot.Platform, _ string, action access.ActionName) {
		told = append(told, action)
	}
	gate := bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), false, nil, notify)
	platform := imbot.Platform(setting.Platform)
	require.False(t, gate(msg, platform, setting.UUID), "first message pairs the chat")
	send := func(m imbot.Message, text string) bool {
		m.Content = imbot.NewTextContent(text)
		return gate(m, platform, setting.UUID)
	}

	for _, text := range []string{"/cd /tmp", "/dir /tmp", "\uff0fcd /tmp"} {
		require.True(t, send(msg, text), text)
	}
	require.Equal(t, []access.ActionName{access.ActionRemoteControlPrivileged, access.ActionRemoteControlPrivileged, access.ActionRemoteControlPrivileged}, told)
	require.Contains(t, bot.DeniedActionText(access.ActionRemoteControlPrivileged), "Any folder and files")

	told = nil
	for _, text := range []string{"/dir", "/dir 1", "/project", "/project 1"} {
		require.False(t, send(msg, text), "%s needs only start", text)
	}
	require.Empty(t, told)

	chats, err := sm.BotAccess().ListDirectChats(context.Background(), setting.UUID)
	require.NoError(t, err)
	file := msg
	file.Content = imbot.NewMediaContent([]imbot.MediaAttachment{{Type: "document", URL: "x"}}, "")
	require.True(t, gate(file, platform, setting.UUID))
	require.Len(t, told, 1, "a direct chat's file is a request, so it is answered")

	group, err := sm.BotAccess().DiscoverGroup(context.Background(), setting.UUID, setting.Platform, "group-1", "eng")
	require.NoError(t, err)
	require.NoError(t, sm.BotAccess().SetGroupCapability(context.Background(), setting.UUID, group.ID, access.CapabilityRemoteControl, access.EffectAllow))
	_, err = sm.BotAccess().AddGroupActor(context.Background(), setting.UUID, group.ID, msg.Sender.ID, "alice", "Controller")
	require.NoError(t, err)
	told = nil
	inGroup := msg
	inGroup.ChatType, inGroup.Recipient = imbot.ChatTypeGroup, imbot.Recipient{ID: "group-1", DisplayName: "eng"}
	groupFile := inGroup
	groupFile.Content = file.Content
	require.True(t, gate(groupFile, platform, setting.UUID))
	require.Empty(t, told, "a file posted in a group is not answered")
	require.True(t, send(inGroup, "@tingly_bot /cd /tmp"))
	require.Len(t, told, 1, "a group command is a request, so it is answered")

	require.NoError(t, sm.BotAccess().SetDirectChatPermission(context.Background(), setting.UUID, chats[0].ID,
		access.CapabilityRemoteControl, access.ActionRemoteControlStart, access.EffectDeny))
	told = nil
	require.True(t, send(msg, "/cd /tmp"))
	require.Empty(t, told, "a chat that may not start is told nothing")
	require.NoError(t, sm.BotAccess().SetDirectChatPermission(context.Background(), setting.UUID, chats[0].ID,
		access.CapabilityRemoteControl, access.ActionRemoteControlStart, access.EffectAllow))
	require.NoError(t, sm.BotAccess().SetDirectChatPermission(context.Background(), setting.UUID, chats[0].ID,
		access.CapabilityRemoteControl, access.ActionRemoteControlPrivileged, access.EffectAllow))
	require.False(t, send(msg, "/cd /tmp"), "granted, it goes through")
}

// The privilege check behind /help asks the same authorizer as the gate.
func TestPrivilegeCheckerFollowsThePrivilegedPermission(t *testing.T) {
	sm, setting, msg := directAuthorizationFixture(t)
	gate := bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), false, nil, nil)
	require.False(t, gate(msg, imbot.Platform(setting.Platform), setting.UUID), "first message pairs the chat")
	check := bot.NewPrivilegeChecker(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()))
	platform := imbot.Platform(setting.Platform)
	require.False(t, check(platform, setting.UUID, msg.Recipient.ID, msg.Sender.ID, false), "paired, not privileged")

	chats, err := sm.BotAccess().ListDirectChats(context.Background(), setting.UUID)
	require.NoError(t, err)
	require.NoError(t, sm.BotAccess().SetDirectChatPermission(context.Background(), setting.UUID, chats[0].ID,
		access.CapabilityRemoteControl, access.ActionRemoteControlPrivileged, access.EffectAllow))
	require.True(t, check(platform, setting.UUID, msg.Recipient.ID, msg.Sender.ID, false))
	require.False(t, check(platform, setting.UUID, msg.Recipient.ID, "someone-else", false), "another actor in the chat is not the peer")
}

// A paired direct chat is labelled by its peer's platform name, read from the
// actor; the chat's own alias stays empty.
func TestAuthorizationGateLabelsPairedDirectChatByPeerName(t *testing.T) {
	sm, setting, msg := directAuthorizationFixture(t)
	gate := bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), false, nil, nil)
	gate(msg, imbot.Platform(setting.Platform), setting.UUID)
	msg.Sender.DisplayName = ""
	gate(msg, imbot.Platform(setting.Platform), setting.UUID)

	chats, err := sm.BotAccess().ListDirectChats(context.Background(), setting.UUID)
	require.NoError(t, err)
	require.Len(t, chats, 1)
	require.Equal(t, "Alice", chats[0].Name)
	require.Empty(t, chats[0].Alias)
}
