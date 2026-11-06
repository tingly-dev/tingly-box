package bot_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/remote/access"
	"github.com/tingly-dev/tingly-box/remote/control/bot"
)

// The sender's platform name labels the direct chat on the first inbound
// message, without touching the operator's alias.
func TestAuthorizationGateNamesDirectChatAfterItsPeer(t *testing.T) {
	sm, setting, msg := directAuthorizationFixture(t)
	gate := bot.AuthorizationGate(sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), sm.RemoteChats(), true, nil, nil)
	gate(msg, imbot.Platform(setting.Platform), setting.UUID)

	chats, err := sm.BotAccess().ListDirectChats(context.Background(), setting.UUID)
	require.NoError(t, err)
	require.Len(t, chats, 1)
	require.Equal(t, "Alice", chats[0].Name)
	require.Empty(t, chats[0].Alias)

	// A later message without a name (or with the id echoed) must not wipe it.
	msg.Sender.DisplayName = ""
	gate(msg, imbot.Platform(setting.Platform), setting.UUID)
	msg.Sender.DisplayName = msg.Sender.ID
	gate(msg, imbot.Platform(setting.Platform), setting.UUID)
	chats, err = sm.BotAccess().ListDirectChats(context.Background(), setting.UUID)
	require.NoError(t, err)
	require.Equal(t, "Alice", chats[0].Name)
}
