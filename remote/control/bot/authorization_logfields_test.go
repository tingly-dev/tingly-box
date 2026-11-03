package bot_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/remote/access"
	"github.com/tingly-dev/tingly-box/remote/control/bot"
)

// A denied inbound message must be attributable in the log: the bot, the
// external chat id and its alias, and the internal access ids under their own
// key names (never under chat_id / actor_id).
func TestAuthorizationDecisionLogCarriesBotChatAndAlias(t *testing.T) {
	var buf bytes.Buffer
	prev, prevLevel := logrus.StandardLogger().Out, logrus.GetLevel()
	logrus.SetOutput(&buf)
	logrus.SetLevel(logrus.DebugLevel)
	t.Cleanup(func() { logrus.SetOutput(prev); logrus.SetLevel(prevLevel) })

	sm, setting, msg := directAuthorizationFixture(t)
	gate := bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), true, nil, nil)
	require.True(t, gate(msg, imbot.Platform(setting.Platform), setting.UUID), "unpaired direct message is denied")

	out := buf.String()
	for _, want := range []string{"bot_uuid=" + setting.UUID, "chat_id=direct-alice", "chat_alias=Alice", "access_target_id=", "access_actor_id="} {
		require.Contains(t, out, want)
	}
	require.NotContains(t, out, " target_id=")
	require.NotContains(t, out, " actor_id=")
}

// The sender's platform name becomes the direct chat's default alias on the
// first inbound message, with no pairing step.
func TestAuthorizationGateNamesDirectChatAfterItsPeer(t *testing.T) {
	sm, setting, msg := directAuthorizationFixture(t)
	gate := bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), true, nil, nil)
	gate(msg, imbot.Platform(setting.Platform), setting.UUID)

	chats, err := sm.BotAccess().ListDirectChats(context.Background(), setting.UUID)
	require.NoError(t, err)
	require.Len(t, chats, 1)
	require.Equal(t, "Alice", chats[0].Alias)
	require.Equal(t, access.AliasSourcePlatform, chats[0].AliasSource)

	// A later message without a name (or with the id echoed) must not wipe it.
	msg.Sender.DisplayName = ""
	gate(msg, imbot.Platform(setting.Platform), setting.UUID)
	msg.Sender.DisplayName = msg.Sender.ID
	gate(msg, imbot.Platform(setting.Platform), setting.UUID)
	chats, err = sm.BotAccess().ListDirectChats(context.Background(), setting.UUID)
	require.NoError(t, err)
	require.Equal(t, "Alice", chats[0].Alias)
}
