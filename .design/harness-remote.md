# Harness Remote — in-process IM chat e2e for `remote/control/remoteagent`

> For contributors adding or debugging `@cc`/`@tb` chat behavior: slash
> commands, pairing, permission prompts, streaming replies, or (as of
> `.design/claude-code.md`'s P2) persistent Claude Code sessions.
>
> Different from [`harness-agent-testing.md`](./harness-agent-testing.md)
> (real agent CLI → real gateway, developer-machine runbook, pre-PR) and
> [`harness-matrix.md`](./harness-matrix.md) (`internal/protocoltest`'s
> gateway protocol-transform harness). This one drives a chat message
> through the real `BotHandler`/`ClaudeCodeExecutor`/`agentboot` pipeline
> with everything below the agent boundary faked — no real IM platform, no
> real `claude` binary, no network. It's a `go test`, not a runbook: fully
> hermetic, CI-safe, ~10–50ms per test.

## 1. Why this exists / what it proves

Unit tests on `agentboot` (`Runner`, `PersistentSession`, `pool.Pool`)
verify the process/protocol machinery in isolation, with test-controlled
contexts that don't model real request lifetimes. They **cannot** catch a
bug where that machinery behaves correctly under a single long-lived test
`ctx` but breaks once wired into a real per-message request handler whose
`ctx` is canceled between messages — see `.design/claude-code.md` §5.2 for
exactly that bug, found by this harness on the first persistent-session test
written against it, three layers above where the bug actually lived.

The signal this harness gives you that a lower-level test cannot: **does the
actual `BotHandler` → `AgentRouter` → `ClaudeCodeExecutor` dispatch, with
real (simulated) IM message delivery timing, produce the right behavior** —
not just "does calling the function correctly with hand-built arguments
work."

## 2. Layers

```
testenv (imbot/platform/tingly/testenv)      — simulates an IM platform
    │   NewTestEnv → NewUser → OpenDM → SendText / WaitText / ExpectInOrderLoose
    ▼
imbot.Manager + tingly.InProcessTransport    — the real bot dispatch loop,
    │                                            same code as production,
    │                                            talking to an in-process
    │                                            transport instead of a
    │                                            real platform API
    ▼
remoteagent.BotHandler (via BootForTest)     — the real production handler:
    │                                            AgentRouter, ClaudeCodeExecutor,
    │                                            SmartGuide, pairing, etc.
    ▼
agentboot.AgentService → claude.Agent        — real Driver/Transport/Runner,
    │                                            but backed by a scripted
    │                                            process.Factory instead of
    │                                            spawning the real `claude`
    ▼
agentboot/claude/fixture.Factory(script)     — the usual choice: one scripted
  OR a raw process.FakeFactory                  wire-format run, ends in Result
                                                — OR your own FakeFactory
                                                  (see §4) for anything a
                                                  single Script can't express
```

Everything above the fixture/fake-factory line is real production code.
Only the very bottom — "what does the claude binary emit" — is scripted.

## 3. Basic pattern (one-shot: `fixture.Script`)

This covers the large majority of `@cc` behavior tests — most of
`tingly_agent_test.go`, `tingly_ask_text_reply_test.go`,
`tingly_handoff_test.go` use exactly this shape:

```go
env := testenv.NewTestEnv(t)
uuid := env.BotUUID()
setting := bot.BotSetting{UUID: uuid, Platform: "tingly", AuthType: "none",
    Auth: map[string]string{}, Enabled: true}

harness := remoteagent.BootForTest(t, env.Manager(), setting,
    remoteagent.TestBootOptions{
        FixtureScript: fixture.Script{
            fixture.AssistantText("hello from fixture"),
            fixture.Result(true),
        },
    })
require.NoError(t, env.Manager().Start(env.Context()))

alice := env.NewUser("alice")
chat := alice.OpenDM(harness.Setting.UUID)
harness.SetCurrentAgent(chat.ChatID, "claude") // bypass the /cc UI ceremony

chat.SendText("hi")
chat.ExpectInOrderLoose(3*time.Second,
    testenv.Matcher{Kind: tingly.EventSend, TextContains: "hello from fixture"},
    testenv.Matcher{Kind: tingly.EventSend, TextContains: "Task done"},
)
```

`fixture.Script` is one scripted `claude` invocation: `System`,
`AssistantText`, `PermissionRequest` (blocks for a stdin response, exactly
like the real CLI waiting on a permission tool), then `Result(true|false)`,
which ends the process. This is enough for anything that fits in one
request/response turn, including the permission round-trip
(`Test_AgentE2E_PermissionApprove`/`_PermissionDeny`).

## 4. Multi-turn / persistent-session pattern (raw `process.Factory`)

`fixture.Script` cannot express "the process stays alive and answers a
*second*, independent turn" — it's designed around one script ending in one
`Result`. For that (persistent-session tests, or anything else that needs
to observe process-spawn *count* rather than just per-turn output), drop
`FixtureScript` and register your own `process.FakeFactory` after
`BootForTest` returns:

```go
factory := process.NewFakeFactory()
factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
    go func() {
        dec := json.NewDecoder(h.StdinR)
        for _, reply := range []string{"first reply", "second reply"} {
            var msg map[string]any
            if err := dec.Decode(&msg); err != nil { h.FinishOutput(); h.SignalExit(err); return }
            // ...write an assistant + result event for `reply`...
        }
        var next map[string]any
        _ = dec.Decode(&next) // blocks until stdin closes (Close())
        h.FinishOutput()
        h.SignalExit(nil)
    }()
}

harness := remoteagent.BootForTest(t, env.Manager(), setting,
    remoteagent.TestBootOptions{SessionPool: pool.New(pool.Config{})})
harness.AgentService.RegisterAgent(agentboot.AgentTypeClaude,
    claude.NewAgentWithFactory(claude.Config{}, factory))
require.NoError(t, harness.AgentService.SetDefaultAgent(agentboot.AgentTypeClaude))
```

`TestBootOptions.SessionPool` (added alongside the persistent-session
feature) threads a real `*pool.Pool` into the handler exactly like
production wiring — omit it (nil, the default) to keep persistent sessions
off, matching every other test.

The assertion that actually proves persistence — not just correct
replies — is on the *fake factory itself*:

```go
require.Len(t, factory.Starts(), 1, "persistent mode must reuse one process across both chat turns")
```

See `remote/control/remoteagent/persistent_session_e2e_test.go` for the
complete worked example (two chat messages, one process, asserted).

## 5. Assertion helpers cheat sheet

| Helper | Use for |
|---|---|
| `chat.WaitText(timeout)` | Next single outbound text message, in order |
| `chat.ExpectInOrderLoose(timeout, matchers...)` | Several expected sends, in order, tolerating unrelated sends between them |
| `chat.WaitApprovalPrompt(timeout)` → `.Approve()`/`.Deny()` | The permission round-trip |
| `waitTextContaining(t, chat, substr, maxScan, perWait)` | Scan forward past unrelated messages for one substring (defined per-file in `remoteagent_test`, not exported — copy the pattern) |

**Gotcha**: inbound messages dispatch through `bot.OnMessage` handlers
**asynchronously**, each in its own goroutine (`imbot/core/base.go`) — never
assume synchronous delivery; always go through a `WaitX`/`ExpectX` helper,
never a bare read of some shared state right after `SendText`.

**Gotcha**: the two-message-preface wording differs (`"⏳ CC: Processing new
session..."` vs. `"⏳ CC: Resuming session..."`) depending on
`IsNewSession` — a test driving more than one message in the same chat
should match on the stable substring (`"CC:"`) rather than hardcoding
`"Processing"`.

## 6. When to reach for something else instead

- Testing `agentboot` primitives themselves (`Runner`, `PersistentSession`,
  `pool.Pool`) in isolation, without any IM/chat layer → their own package
  tests (`agentboot/runner_open_test.go`, `agentboot/pool/pool_test.go`),
  using `process.FakeFactory` directly with no `testenv`/`BotHandler`.
- Verifying a real `claude`/`codex`/`opencode` CLI actually works against a
  real (or mock-upstream) gateway → `harness-agent-testing.md`.
- Verifying the gateway's protocol-transform correctness (not chat/agent
  behavior at all) → `harness-matrix.md`.
