package remoteagent_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/agentboot/pool"
	"github.com/tingly-dev/tingly-box/agentboot/process"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/testenv"
	"github.com/tingly-dev/tingly-box/remote/control/bot"
	"github.com/tingly-dev/tingly-box/remote/control/remoteagent"
)

// twoTurnPersistentFactory scripts a single fake claude process that answers
// len(replies) sequential turns on the same stdin without exiting between
// them — the multi-turn behavior .design/claude-code.md §3.1 confirmed
// against the real CLI. Unlike fixture.Script (one process, one Result,
// exits), this stays alive so a persistent-session test can assert only one
// process was ever spawned across multiple chat messages.
func twoTurnPersistentFactory(replies ...string) *process.FakeFactory {
	factory := process.NewFakeFactory()
	factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		go func() {
			dec := json.NewDecoder(h.StdinR)
			for _, reply := range replies {
				var msg map[string]any
				if err := dec.Decode(&msg); err != nil {
					h.FinishOutput()
					h.SignalExit(err)
					return
				}
				assistant, _ := json.Marshal(map[string]any{
					"type": claude.SDKAssistantMessage,
					"message": map[string]any{
						"role": "assistant",
						"content": []any{
							map[string]any{"type": "text", "text": reply},
						},
					},
				})
				_, _ = h.WriteOutput(append(assistant, '\n'))
				result, _ := json.Marshal(map[string]any{
					"type":     claude.SDKResultMessage,
					"subtype":  claude.ResultSubtypeSuccess,
					"is_error": false,
				})
				_, _ = h.WriteOutput(append(result, '\n'))
			}
			// Wait for stdin to close (the executor's Close call) before
			// exiting cleanly, exactly like the real CLI.
			var next map[string]any
			_ = dec.Decode(&next)
			h.FinishOutput()
			h.SignalExit(nil)
		}()
	}
	return factory
}

// TestPersistentSession_E2E_ReusesOneProcessAcrossTurns drives two @cc chat
// messages, in the same DM, through the real IM dispatch → BotHandler →
// ClaudeCodeExecutor → agentboot pipeline with PersistentSession enabled on
// the bot, and asserts:
//  1. both replies come back correctly through the actual chat surface, and
//  2. only one underlying process was ever spawned — proving the second
//     message reused the first's live session (Acquire+Send) instead of
//     going through the one-shot Execute path.
//
// This is the one test in the suite that exercises runPersistentTurn's
// glue (pool key derivation, the bot-setting gate, Acquire/Open/Send
// dispatch) behaviorally; agentboot's own PersistentSession/pool.Pool have
// their own unit tests but never touch ClaudeCodeExecutor.
func TestPersistentSession_E2E_ReusesOneProcessAcrossTurns(t *testing.T) {
	env := testenv.NewTestEnv(t)
	uuid := env.BotUUID()

	persistentOn := true
	setting := bot.BotSetting{
		UUID:              uuid,
		Name:              "tingly-persistent-test",
		Platform:          "tingly",
		AuthType:          "none",
		Auth:              map[string]string{},
		Enabled:           true,
		PersistentSession: &persistentOn,
	}

	factory := twoTurnPersistentFactory("first reply", "second reply")
	sessionPool := pool.New(pool.Config{})
	t.Cleanup(func() { sessionPool.Shutdown(context.Background()) })

	harness := remoteagent.BootForTest(t, env.Manager(), setting, remoteagent.TestBootOptions{
		SessionPool: sessionPool,
	})
	harness.AgentService.RegisterAgent(agentboot.AgentTypeClaude, claude.NewAgentWithFactory(claude.Config{}, factory))
	require.NoError(t, harness.AgentService.SetDefaultAgent(agentboot.AgentTypeClaude))

	require.NoError(t, env.Manager().Start(env.Context()))

	alice := env.NewUser("alice")
	chat := alice.OpenDM(harness.Setting.UUID)
	harness.SetCurrentAgent(chat.ChatID, "claude")

	chat.SendText("hello one")
	waitTextContaining(t, chat, "CC:", 5, 3*time.Second)
	waitTextContaining(t, chat, "first reply", 5, 3*time.Second)
	waitTextContaining(t, chat, "Task done", 5, 3*time.Second)

	chat.SendText("hello two")
	waitTextContaining(t, chat, "CC:", 5, 3*time.Second)
	waitTextContaining(t, chat, "second reply", 5, 3*time.Second)
	waitTextContaining(t, chat, "Task done", 5, 3*time.Second)

	require.Len(t, factory.Starts(), 1, "persistent mode must reuse one process across both chat turns")
}
