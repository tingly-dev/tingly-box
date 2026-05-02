package testenv_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/testenv"
)

// TestEnv_RoundTrip drives a tiny imbot.Manager-only echo bot through the
// harness: the test sends a text message, the bot's OnMessage handler
// echoes it back via SendText, the test asserts the reply.
func TestEnv_RoundTrip(t *testing.T) {
	env := testenv.NewTestEnv(t)
	uuid := env.BotUUID()

	// The Manager.OnMessage handler runs in a goroutine. Echo any inbound
	// text right back so we can observe it on the outbound side.
	env.Manager().OnMessage(func(msg core.Message, p imbot.Platform, botUUID string) {
		bot := env.Manager().GetBotByUUID(botUUID)
		if bot == nil {
			return
		}
		_, _ = bot.SendText(context.Background(), msg.GetReplyTarget(), "echo: "+msg.GetText())
	})
	require.NoError(t, env.Manager().Start(env.Context()))

	alice := env.NewUser("alice")
	chat := alice.OpenDM(uuid)
	chat.SendText("hello")

	chat.WaitText(2 * time.Second).AssertEquals(t, "echo: hello")
}

func TestEnv_ButtonClick(t *testing.T) {
	env := testenv.NewTestEnv(t)
	uuid := env.BotUUID()

	var (
		mu        sync.Mutex
		callbacks []string
	)

	env.Manager().OnMessage(func(msg core.Message, p imbot.Platform, botUUID string) {
		bot := env.Manager().GetBotByUUID(botUUID)
		if bot == nil {
			return
		}

		if isCb, _ := msg.Metadata["is_callback"].(bool); isCb {
			data, _ := msg.Metadata["callback_data"].(string)
			mu.Lock()
			callbacks = append(callbacks, data)
			mu.Unlock()
			_, _ = bot.SendText(context.Background(), msg.GetReplyTarget(), "got: "+data)
			return
		}

		// First inbound — send a message with a keyboard.
		kb := imbot.NewKeyboardBuilder().
			AddRow(
				imbot.CallbackButton("Approve", "ia:perm:approve"),
				imbot.CallbackButton("Deny", "ia:perm:deny"),
			).Build()
		_, _ = bot.SendMessage(context.Background(), msg.GetReplyTarget(), &core.SendMessageOptions{
			Text:     "decide?",
			Metadata: map[string]any{"replyMarkup": kb},
		})
	})
	require.NoError(t, env.Manager().Start(env.Context()))

	alice := env.NewUser("alice")
	chat := alice.OpenDM(uuid)
	chat.SendText("start")

	prompt := chat.WaitText(2 * time.Second)
	prompt.AssertContains(t, "decide?")
	prompt.AssertHasButton(t, "Approve")

	btn, ok := prompt.ButtonByLabel("Approve")
	require.True(t, ok)
	btn.Click()

	chat.WaitText(2 * time.Second).AssertEquals(t, "got: ia:perm:approve")

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"ia:perm:approve"}, callbacks)
}

func TestEnv_ExpectNoEvent(t *testing.T) {
	env := testenv.NewTestEnv(t)
	uuid := env.BotUUID()

	// No message handler — the bot does nothing.
	require.NoError(t, env.Manager().Start(env.Context()))

	alice := env.NewUser("alice")
	chat := alice.OpenDM(uuid)
	chat.SendText("hello")
	// Bot is silent; ExpectNoEvent should pass.
	chat.ExpectNoEvent(200 * time.Millisecond)
}
