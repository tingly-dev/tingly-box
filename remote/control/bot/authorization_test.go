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

// Pairing happens in a direct chat, where only the plain command counts:
// whitespace and a full-width slash are fine, a mention or the bot's suffix
// make it ordinary text, which an unpaired chat may not send.
func TestAuthorizationGateReadsPairingCommandsAsDirectChatText(t *testing.T) {
	sm, setting, msg := directAuthorizationFixture(t)
	gate := bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), true, nil, nil)
	for _, text := range []string{" /pair CODE", "/pair   CODE  ", "\uff0fpair CODE", "\u00a0/bind CODE"} {
		msg.Content = imbot.NewTextContent(text)
		require.False(t, gate(msg, imbot.Platform(setting.Platform), setting.UUID), text)
	}
	for _, text := range []string{"@tingly_bot /pair CODE", "<@U123> /pair CODE", "/pair@tingly_bot CODE"} {
		msg.Content = imbot.NewTextContent(text)
		require.True(t, gate(msg, imbot.Platform(setting.Platform), setting.UUID), text)
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

// A pending approval answers approve prompts only: it must not let a chat
// walk the filesystem (privileged), and the folder/project pickers still need
// start.
func TestPendingApprovalDoesNotGrantDirectoryOrProjectAccess(t *testing.T) {
	sm, setting, msg := directAuthorizationFixture(t)
	gate := bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), false, nil, nil)
	require.False(t, gate(msg, imbot.Platform(setting.Platform), setting.UUID), "first message pairs the chat")
	pending := func(string) (access.CapabilityName, access.ActionName) {
		return access.CapabilityRemoteControl, access.ActionRemoteControlApprove
	}
	gate = bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), false, pending, nil)
	for _, text := range []string{"/cd /tmp", "/dir /tmp", "/dir ~/work"} {
		m := msg
		m.Content = imbot.NewTextContent(text)
		require.True(t, gate(m, imbot.Platform(setting.Platform), setting.UUID), "%s must require privileged access", text)
	}
	for _, payload := range []imbot.Payload{imbot.NewPayload("bind", "custom"), imbot.NewPayload("bind", "select"), imbot.NewPayload("action", "bind")} {
		m := msg
		m.Payload = payload
		require.True(t, gate(m, imbot.Platform(setting.Platform), setting.UUID), "%v must require privileged access", payload)
	}

	chats, err := sm.BotAccess().ListDirectChats(context.Background(), setting.UUID)
	require.NoError(t, err)
	require.NoError(t, sm.BotAccess().SetDirectChatPermission(context.Background(),
		setting.UUID, chats[0].ID, access.CapabilityRemoteControl, access.ActionRemoteControlStart, access.EffectDeny))
	for _, text := range []string{"/dir", "/dir 1", "/project", "/project 1"} {
		m := msg
		m.Content = imbot.NewTextContent(text)
		require.True(t, gate(m, imbot.Platform(setting.Platform), setting.UUID), "%s needs start, an approve-only chat cannot use it", text)
	}
	m := msg
	m.Payload = imbot.NewPayload("project", "switch", "/tmp")
	require.True(t, gate(m, imbot.Platform(setting.Platform), setting.UUID), "the folder button needs start")
}

// A paired chat is not privileged until the operator grants it. Choosing among
// the folders and projects the operator registered needs only start; a free
// path (/cd, /dir <path>, the directory browser) and file sends need
// privileged. Only the plain command counts in a direct chat: with a mention
// or the bot's suffix it is ordinary text, which the chat may send.
func TestAuthorizationGatePrivilegedCommandsInDirectChat(t *testing.T) {
	sm, setting, msg := directAuthorizationFixture(t)
	gate := bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), false, nil, nil)
	require.False(t, gate(msg, imbot.Platform(setting.Platform), setting.UUID), "first message pairs the chat")
	for _, text := range []string{
		"/cd /tmp", "/cd   /tmp  ", " /cd /tmp", "\uff0fcd /tmp", "/cd", "/dir /tmp", "/dir ~/work", "/dir ../x", "/dir 0", "/dir 1 2", "/dir 1a", "/dir -1", "/dir   path with  spaces",
	} {
		m := msg
		m.Content = imbot.NewTextContent(text)
		require.True(t, gate(m, imbot.Platform(setting.Platform), setting.UUID), "%q must require privileged access", text)
	}
	for _, text := range []string{
		"/dir", "/DIR", " /dir", "\t/dir\n", "\u00a0/dir", "\uff0fdir", "/dir 1", "/dir   2  ", "/project", "/project 1",
		"/status", "/directory", "/bash ls", "/shell ls",
		"@tingly_bot /cd /tmp", "@tingly_bot/dir /tmp", "<@U123> /dir /tmp", "<@U123>/dir", "@_user_1 /dir", "/dir@tingly_bot", "/cd@tingly_bot /tmp",
	} {
		m := msg
		m.Content = imbot.NewTextContent(text)
		require.False(t, gate(m, imbot.Platform(setting.Platform), setting.UUID), "%q needs only start in a direct chat", text)
	}
}

// The buttons follow the commands: the folder list buttons and the menu's
// project button are the registered pickers, everything that browses or names
// a free path is privileged.
func TestInboundActionButtonsInDirectChat(t *testing.T) {
	press := func(group bool, segments ...string) imbot.Message {
		m := imbot.Message{ChatType: imbot.ChatTypeDirect, Payload: imbot.NewPayload(segments...)}
		if group {
			m.ChatType = imbot.ChatTypeGroup
		}
		return m
	}
	start := []imbot.Message{press(false, "project", "switch", "/tmp"), press(false, "action", "project")}
	privileged := []imbot.Message{
		press(false, "action", "bind"), press(false, "bind", "dir", "x"), press(false, "bind", "up"), press(false, "bind", "select"),
		press(false, "bind", "custom"), press(false, "bind", "create", "/tmp/x"), press(false, "bind", "cancel"),
	}
	for _, m := range start {
		_, action := bot.InboundAction(m, "", "")
		require.Equal(t, access.ActionRemoteControlStart, action, "%v", m.Payload)
		_, action = bot.InboundAction(m, access.CapabilityRemoteControl, access.ActionRemoteControlApprove)
		require.Equal(t, access.ActionRemoteControlStart, action, "%v: a pending approval does not change what the picker needs", m.Payload)
	}
	for _, m := range privileged {
		_, action := bot.InboundAction(m, "", "")
		require.Equal(t, access.ActionRemoteControlPrivileged, action, "%v", m.Payload)
	}
	// In a group nothing of this lowers: every button stays privileged.
	for _, m := range append(append([]imbot.Message{}, start...), privileged...) {
		m.ChatType = imbot.ChatTypeGroup
		_, action := bot.InboundAction(m, "", "")
		require.Equal(t, access.ActionRemoteControlPrivileged, action, "group %v", m.Payload)
	}
	// Files stay privileged everywhere.
	for _, chatType := range []imbot.ChatType{imbot.ChatTypeDirect, imbot.ChatTypeGroup} {
		m := imbot.Message{ChatType: chatType, Content: imbot.NewMediaContent([]imbot.MediaAttachment{{URL: "x"}}, "")}
		_, action := bot.InboundAction(m, "", "")
		require.Equal(t, access.ActionRemoteControlPrivileged, action, "%v files", chatType)
	}
}

// In a group the same command arrives with the bot mentioned, and every
// form of it needs privileged access: the gate must not read it as ordinary
// text.
func TestInboundActionPrivilegedCommandsInGroup(t *testing.T) {
	group := func(text string) imbot.Message {
		return imbot.Message{ChatType: imbot.ChatTypeGroup, Content: imbot.NewTextContent(text)}
	}
	for _, text := range []string{
		"/dir", "/DIR", " /dir", "/dir   1", "/dir@tingly_bot", "/dir@tingly_bot 1", "@tingly_bot /dir", "@tingly_bot/dir",
		"@tingly_bot   /cd /tmp", "<@U123> /dir", "<@!123> /cd /tmp", "<@U123>/dir", "@_user_1 /project", "@_user_1/project",
		"\uff0fdir", "/cd@tingly_bot /tmp", "/project", "/project 1", "/dir /tmp",
	} {
		_, action := bot.InboundAction(group(text), "", "")
		require.Equal(t, access.ActionRemoteControlPrivileged, action, "%q must require privileged access", text)
	}
	for _, text := range []string{"/status", "/help@tingly_bot", "@tingly_bot /new hello", "/directory", "@tingly_bot hello", "/bash ls"} {
		_, action := bot.InboundAction(group(text), "", "")
		require.Equal(t, access.ActionRemoteControlStart, action, "%q needs no privileged access", text)
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

	for _, text := range []string{"/cd /tmp", "\uff0fcd /tmp", "/dir /tmp", "/dir ~/work"} {
		m := msg
		m.Content = imbot.NewTextContent(text)
		require.True(t, gate(m, imbot.Platform(setting.Platform), setting.UUID), text)
	}
	press := msg
	press.Payload = imbot.NewPayload("action", "bind")
	require.True(t, gate(press, imbot.Platform(setting.Platform), setting.UUID))
	require.Equal(t, []access.ActionName{
		access.ActionRemoteControlPrivileged, access.ActionRemoteControlPrivileged, access.ActionRemoteControlPrivileged,
		access.ActionRemoteControlPrivileged, access.ActionRemoteControlPrivileged,
	}, told)

	// The registered pickers need only start, so they run and nothing is said.
	told = nil
	for _, text := range []string{"/dir", "/dir 1", "/project", "/project 1"} {
		m := msg
		m.Content = imbot.NewTextContent(text)
		require.False(t, gate(m, imbot.Platform(setting.Platform), setting.UUID), text)
	}
	require.Empty(t, told)
	require.Contains(t, bot.DeniedActionText(access.ActionRemoteControlPrivileged), "Any folder and files")

	// Once granted the command goes through and nothing is said.
	chats, err := sm.BotAccess().ListDirectChats(context.Background(), setting.UUID)
	require.NoError(t, err)
	require.Len(t, chats, 1)
	require.NoError(t, sm.BotAccess().SetDirectChatPermission(context.Background(), setting.UUID, chats[0].ID,
		access.CapabilityRemoteControl, access.ActionRemoteControlPrivileged, access.EffectAllow))
	told = nil
	m := msg
	m.Content = imbot.NewTextContent("\uff0fcd /tmp")
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
	for _, text := range []string{"/dir", "/cd /tmp"} {
		m := msg
		m.Content = imbot.NewTextContent(text)
		require.True(t, gate(m, imbot.Platform(setting.Platform), setting.UUID), text)
	}
	require.Zero(t, told)
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
	require.Len(t, chats, 1)
	require.NoError(t, sm.BotAccess().SetDirectChatPermission(context.Background(), setting.UUID, chats[0].ID,
		access.CapabilityRemoteControl, access.ActionRemoteControlPrivileged, access.EffectAllow))
	require.True(t, check(platform, setting.UUID, msg.Recipient.ID, msg.Sender.ID, false))
	require.False(t, check(platform, setting.UUID, msg.Recipient.ID, "someone-else", false), "another actor in the chat is not the peer")
}

func TestPrivilegeCheckerTravelsInTheContext(t *testing.T) {
	require.Nil(t, bot.PrivilegeCheckerFrom(context.Background()))
	called := false
	ctx := bot.WithPrivilegeChecker(context.Background(), func(imbot.Platform, string, string, string, bool) bool { called = true; return true })
	require.True(t, bot.PrivilegeCheckerFrom(ctx)("p", "b", "c", "s", false))
	require.True(t, called)
}

func TestDeniedPrivilegedTextNamesTheSwitch(t *testing.T) {
	require.Contains(t, bot.DeniedActionText(access.ActionRemoteControlPrivileged), "Any folder and files")
}

// A command with another bot's name on it is not for this bot: nothing here
// runs it or answers it. A command for this bot, or with no name on it, is.
func TestAddressedToAnotherBot(t *testing.T) {
	group := func(text string) imbot.Message {
		return imbot.Message{ChatType: imbot.ChatTypeGroup, Content: imbot.NewTextContent(text)}
	}
	direct := imbot.Message{ChatType: imbot.ChatTypeDirect, Content: imbot.NewTextContent("/dir@otherbot")}
	for _, tt := range []struct {
		msg  imbot.Message
		own  string
		want bool
	}{
		{group("/new@otherbot"), "tingly_bot", true},
		{group("/dir@OtherBot 1"), "Tingly_Bot", true},
		{group("@tingly_bot /dir@otherbot"), "tingly_bot", true},
		{group("/new@tingly_bot"), "tingly_bot", false},
		{group("/new@Tingly_Bot"), "tingly_bot", false},
		{group("/new"), "tingly_bot", false},
		{group("hello@otherbot"), "tingly_bot", false},
		{group("/new@otherbot"), "", false},
		{direct, "tingly_bot", false},
	} {
		require.Equal(t, tt.want, bot.AddressedToAnotherBot(tt.msg, tt.own), "%q as %q", tt.msg.GetText(), tt.own)
	}
}

// A file posted in a group is ordinary chatter, not a request to the bot: it
// is not let through, but the bot does not answer it either. In a direct chat
// it is a request, so the sender is told.
func TestAuthorizationGateDoesNotAnswerFilesInGroups(t *testing.T) {
	sm, setting, msg := directAuthorizationFixture(t)
	told := 0
	notify := func(imbot.Message, imbot.Platform, string, access.ActionName) { told++ }
	gate := bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), false, nil, notify)
	require.False(t, gate(msg, imbot.Platform(setting.Platform), setting.UUID), "first message pairs the chat")

	file := msg
	file.Content = imbot.NewMediaContent([]imbot.MediaAttachment{{Type: "document", URL: "x"}}, "")
	require.True(t, gate(file, imbot.Platform(setting.Platform), setting.UUID))
	require.Equal(t, 1, told, "a direct chat's file earns the notice")

	// The same sender in a group that may start runs: a posted file is chatter.
	ctx := context.Background()
	group, err := sm.BotAccess().DiscoverGroup(ctx, setting.UUID, setting.Platform, "group-1", "eng")
	require.NoError(t, err)
	require.NoError(t, sm.BotAccess().SetGroupCapability(ctx, setting.UUID, group.ID, access.CapabilityRemoteControl, access.EffectAllow))
	_, err = sm.BotAccess().AddGroupActor(ctx, setting.UUID, group.ID, msg.Sender.ID, "alice", "Controller")
	require.NoError(t, err)
	told = 0
	groupFile := file
	groupFile.ChatType = imbot.ChatTypeGroup
	groupFile.Recipient = imbot.Recipient{ID: "group-1", DisplayName: "eng"}
	require.True(t, gate(groupFile, imbot.Platform(setting.Platform), setting.UUID), "still not let through")
	require.Zero(t, told, "and not answered")

	groupCmd := groupFile
	groupCmd.Content = imbot.NewTextContent("/cd /tmp")
	require.True(t, gate(groupCmd, imbot.Platform(setting.Platform), setting.UUID))
	require.Equal(t, 1, told, "a group command is a request, so it is answered")
}
