package bot_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/interaction"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// newChannelTestManager builds a bot.Manager wired like production: notify
// consumer plus remote_agent (inbound catch-all, last), and a channel
// registry for /tingly/:scenario routing. The channel itself — registry
// entry, shared prompter, reply routing — is bot-host infrastructure.
func newChannelTestManager(t *testing.T, uuid, scenarios string) (*bot.Manager, *fakeSettingsStore, *channel.Registry, *tingly.InProcessTransport) {
	t.Helper()

	tr := tingly.NewInProcessTransport()
	tingly.Register(uuid, tr)
	t.Cleanup(func() { tingly.Unregister(uuid) })

	store := &fakeSettingsStore{
		settings: map[string]db.Settings{
			uuid: {
				UUID:      uuid,
				Name:      "channel-test",
				Platform:  "tingly",
				AuthType:  "none",
				Auth:      map[string]string{},
				Enabled:   true,
				Scenarios: scenarios,
			},
		},
	}

	sessionMgr := session.NewManager(session.Config{
		Timeout:          10 * time.Minute,
		MessageRetention: time.Hour,
	}, nil)
	svc, err := agentboot.NewAgentService(agentboot.Config{ClaudeProjectsDir: t.TempDir()})
	require.NoError(t, err)

	registry := channel.NewRegistry()
	m := bot.NewManager(store,
		bot.NewNotifyConsumer(),
		bot.NewRemoteAgentConsumer(sessionMgr, svc, nil, store))
	m.SetDataPath(t.TempDir() + "/chats.json")
	m.SetChannelRegistry(registry)

	return m, store, registry, tr
}

// promptAndAnswer drives an interactive prompt through the bot's registered
// channel and answers it over the wire with a plain-text "y", asserting the
// reply resolves as approved (routed by the host's prompt-reply router).
func promptAndAnswer(t *testing.T, registry *channel.Registry, tr *tingly.InProcessTransport, uuid, chatID string) {
	t.Helper()

	ch, ok := registry.Get(uuid)
	require.True(t, ok, "channel must be registered for uuid %s", uuid)

	events := tr.Channel(chatID)

	type promptResult struct {
		reply interaction.Reply
		err   error
	}
	resCh := make(chan promptResult, 1)
	go func() {
		reply, err := ch.Prompt(context.Background(), channel.Target{ChatID: chatID}, interaction.Interaction{
			ID:      fmt.Sprintf("ix-%d", time.Now().UnixNano()),
			Kind:    interaction.KindConfirm,
			Title:   "Deploy?",
			Body:    "deploy to prod",
			Timeout: 5 * time.Second,
		})
		resCh <- promptResult{reply, err}
	}()

	// Wait for the outbound prompt message; once it is on the wire the
	// prompter has registered the pending request.
	select {
	case <-events:
	case <-time.After(3 * time.Second):
		t.Fatal("prompt was never sent to the chat")
	}

	// Answer as the operator with a plain-text approval.
	tr.Inject(tingly.NewIncomingTextMessage(
		fmt.Sprintf("in-%d", time.Now().UnixNano()),
		chatID, core.Sender{ID: "ops-user"}, "y", core.ChatTypeDirect))

	select {
	case res := <-resCh:
		require.NoError(t, res.err)
		require.Equal(t, interaction.StatusAnswered, res.reply.Status)
		require.Equal(t, "allow", res.reply.Selected)
	case <-time.After(5 * time.Second):
		t.Fatal("prompt did not resolve — reply was not routed back to the prompter")
	}
}

// TestManager_ChannelOnlyBot_Tingly asserts the notify-only (channel-only)
// bot: remote_agent mount OFF but an active outbound scenario binding keeps
// the bot running — the host registers its remote.channel.Channel, a scenario
// prompt is delivered, and the user's reply routes back, all with zero
// remote-agent machinery attached. Turning the last mount off stops the bot
// and unregisters the channel.
func TestManager_ChannelOnlyBot_Tingly(t *testing.T) {
	uuid := fmt.Sprintf("channel-bot-%d", time.Now().UnixNano())
	const chatID = "dm:ops"

	m, store, registry, tr := newChannelTestManager(t, uuid,
		`[{"name":"remote_agent","enabled":false},{"name":"claude_code","chat_id":"dm:ops"}]`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// remote_agent off + outbound binding on → the bot runs (channel-only).
	require.NoError(t, m.Start(ctx, uuid))
	require.True(t, m.IsRunning(uuid), "bot with an outbound binding must run even with remote_agent off")

	// The channel registration happens post-start in the bot goroutine.
	require.Eventually(t, func() bool {
		_, ok := registry.Get(uuid)
		return ok
	}, 3*time.Second, 20*time.Millisecond, "channel was never registered")

	promptAndAnswer(t, registry, tr, uuid, chatID)

	// Turning the last mount off stops the bot; the channel goes with it.
	store.setScenarios(uuid, `[{"name":"remote_agent","enabled":false},{"name":"claude_code","chat_id":"dm:ops","enabled":false}]`)
	require.NoError(t, m.Sync(ctx))
	require.True(t, m.WaitForStop(uuid, 5*time.Second))
	require.False(t, m.IsRunning(uuid))
	_, ok := registry.Get(uuid)
	require.False(t, ok, "channel must be unregistered when the bot stops")
}

// TestManager_ChannelAndRemoteAgentCoexist_Tingly asserts the dispatch order
// when both purposes are mounted on one bot: ordinary messages fall through
// the host's prompt-reply router to the remote-agent handler, while replies
// to a pending scenario prompt are claimed by the router.
func TestManager_ChannelAndRemoteAgentCoexist_Tingly(t *testing.T) {
	uuid := fmt.Sprintf("coexist-bot-%d", time.Now().UnixNano())
	const chatID = "dm:ops"

	m, _, registry, tr := newChannelTestManager(t, uuid,
		`[{"name":"remote_agent","enabled":true},{"name":"claude_code","chat_id":"dm:ops"}]`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, m.Start(ctx, uuid))
	require.True(t, m.IsRunning(uuid))
	require.Eventually(t, func() bool {
		_, ok := registry.Get(uuid)
		return ok
	}, 3*time.Second, 20*time.Millisecond, "channel was never registered")

	// An ordinary command is NOT claimed by the prompt-reply router (nothing
	// is pending) and reaches the remote-agent handler.
	events := tr.Channel(chatID)
	tr.Inject(tingly.NewIncomingTextMessage("in-help", chatID, core.Sender{ID: "ops-user"}, "/help", core.ChatTypeDirect))
	deadline := time.After(3 * time.Second)
waitHelp:
	for {
		select {
		case evt := <-events:
			// Skip reactions/edits; the help reply is the first text send.
			if evt.Kind == tingly.EventSend && evt.Text != "" {
				require.Contains(t, evt.Text, "@cc", "expected the remote-agent help reply")
				break waitHelp
			}
		case <-deadline:
			t.Fatal("remote-agent handler never replied to /help")
		}
	}

	// A scenario prompt is answered through the host router even though the
	// remote-agent catch-all sits behind it.
	promptAndAnswer(t, registry, tr, uuid, chatID)

	m.Stop(uuid)
	require.True(t, m.WaitForStop(uuid, 5*time.Second))
}

// TestManager_AgentOnlyBotRegistersChannel_Tingly asserts that the channel is
// host infrastructure, not a purpose: a bot running with ONLY remote_agent
// mounted (no outbound bindings) still registers its remote.channel.Channel,
// and unregisters it on stop.
func TestManager_AgentOnlyBotRegistersChannel_Tingly(t *testing.T) {
	uuid := fmt.Sprintf("agent-only-bot-%d", time.Now().UnixNano())

	m, _, registry, _ := newChannelTestManager(t, uuid,
		`[{"name":"remote_agent","enabled":true}]`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, m.Start(ctx, uuid))
	require.True(t, m.IsRunning(uuid))
	require.Eventually(t, func() bool {
		_, ok := registry.Get(uuid)
		return ok
	}, 3*time.Second, 20*time.Millisecond, "running bot must register its channel")

	m.Stop(uuid)
	require.True(t, m.WaitForStop(uuid, 5*time.Second))
	_, ok := registry.Get(uuid)
	require.False(t, ok, "channel must be unregistered when the bot stops")
}
