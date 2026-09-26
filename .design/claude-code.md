# Claude Code: from one-shot processes to a persistent stream session

> 本文件分两部分：**Part A**（下文 §1–§7）是 `@cc` 持久流式会话设计；
> **Part B**（§B0–§B8，见文末）是 Claude OAuth 链路的客户端兼容层（`claude_code_version`）。

> Status: P0 (feasibility), P1 (core primitive), and P2 (pool + wiring into
> `@cc`, behind the `persistent_session` bot setting, default off) done,
> 2026-09-17. P3 (observability) not started — see §6.
> Scope: `@cc` (Claude Code) execution via `agentboot`, as driven by
> `remote/control/remoteagent`. Does not touch `@tb` (SmartGuide/AFK), which
> is already a long-lived in-process ReAct loop — see `.design/afk.md`.

## 1. Problem

Every `@cc` message today pays full Claude Code CLI startup cost: process
spawn, CLAUDE.md/settings discovery, MCP server bring-up, project indexing —
per **message**, not per **conversation**. `.design/agentboot-refactor.md`
§P3 made this one-shot model an explicit decision, modeled directly on the
Python `claude-agent-sdk`'s one-shot `query()`. That section should be read
as background for this document rather than duplicated; this document is the
follow-up it anticipated but did not scope: *"persistent bidirectional
clients... [are] not required by the current remote-control product path"* —
a persistent stream is now worth designing because the product path now
includes latency-sensitive, high-frequency chat turns where re-paying startup
cost every message is the dominant inefficiency.

This document is research + a proposed design. It does not implement
anything yet.

## 2. How execution works today (confirmed from code)

One call = one `claude` process = one turn:

- `agentboot/claude/driver.go` (`Driver.Prepare`) builds a `process.LaunchSpec`
  per call: resolves the `claude` binary, builds CLI args, and — for
  stream-json — builds an `InitialInput` channel that feeds **exactly one**
  user message (via `StreamPromptBuilder`, `agentboot/claude/prompt_builder.go`)
  into the child's stdin, then closes the channel.
- `agentboot/process/osexec.go` is the only real `exec.Command` spawn site.
- Args always include `--print` (one-shot query mode), `--output-format
  stream-json --verbose`, `--input-format stream-json`, and
  `--permission-prompt-tool stdio`.
- `agentboot/runner_execute.go` (`Runner.Execute`): spawn → decode/encode
  pump → on the first terminal `result` event, `shutdownGracefully()` closes
  stdin, waits up to `shutdownGracePeriod` (default 5s) for the process to
  exit on its own (so Claude Code can flush its session file), then
  `Kill()s`. `proc.Wait()` is joined before `Result` is finalized. Nothing
  survives a turn except Claude Code's own on-disk transcript
  (`~/.claude/projects/<cwd-hash>/<session-id>.jsonl`).
- Multi-turn continuity is entirely `--resume <session-id>` / `--session-id`
  / `--continue` (`agentboot/claude/cli_builder.go`, `BuildCommonArgs`), not a
  live connection. `remote/control/remoteagent/executor_claude.go`
  (`ClaudeCodeExecutor.Execute`) computes `shouldResume := !req.IsNewSession`
  and calls `AgentService.Run` fresh for **every** inbound chat message.
  `isSessionInUseText` exists specifically to detect the CLI's *"session file
  already in use by another process"* error — a direct symptom of two turns
  racing two processes against one on-disk session file.
- `Runner` deliberately calls the transport factory once per `Execute` "so
  Claude's mutable message accumulator and routing context cannot leak across
  concurrent bot chats" — i.e. today's isolation guarantee is *the process
  boundary itself*. Any persistent design must replace that guarantee with
  something else (§5.3).

Two "session" concepts already exist and must not be conflated:

- `remote/session/manager.go` (`session.Manager`) — a **logical** session
  keyed by `(chatID, agent, project)`: status (pending/running/completed/
  failed/expired/closed), `LastActivity`, an append-only transcript. It has
  no idea a process ever ran; `SetRunning`/`SetCompleted`/`SetFailed` are
  bookkeeping calls made by the runner. `CreateWithID` already exists to bind
  a remote session's ID to a Claude on-disk `session_id` after `/resume`,
  and `ExpiresAt.IsZero()` already means *"persistent: caller drives the
  lifecycle"* — the manager has a pre-existing seam for non-expiring
  sessions.
- `agentboot.ExecutionHandle` (`agentboot/handle.go`) — a **per-process**
  handle: `Events()`/`Respond()`/`Wait()`/`Cancel()`. Its doc comment is
  explicit that the channel closes "after the underlying process has
  exited." This is what a persistent design has to extend or wrap, not
  reinvent — `StreamEvent`'s sealed sum type (`MessageEvent`,
  `ApprovalRequestEvent`, `AskRequestEvent`, `ErrorEvent`) is agent-neutral
  and reusable as-is.

No `.design/*.md` file currently proposes a persistent session — this is new
ground. `.design/afk.md` describes an architecturally unrelated precedent
(`@tb` never spawns `claude` at all; it's a Go-native ReAct loop against the
Messages API), useful only as proof that "long-lived agent loop" already
exists as a pattern in this codebase, just not for `@cc`.

## 3. Prior art (what "official" persistent-session designs look like)

Researched directly against the installed CLI (`/opt/claude-code/bin/claude`,
native 2.1.274) and `libs/anthropic-sdk-go` / `libs/go-genai` sources, plus a
`claude-code-guide` lookup against `code.claude.com` docs. Four independent
precedents, each answering a different piece of "how do you keep a model
conversation open across turns":

### 3.1 Claude Code CLI: `--input-format stream-json` is already a streaming
input, not a one-shot json blob

The CLI's own `--help` describes `--input-format stream-json` as **"realtime
streaming input"**, and separately documents `--replay-user-messages`:
*"Re-emit user messages from stdin back on stdout for acknowledgment (only
works with --input-format=stream-json and --output-format=stream-json)"*.
An acknowledgment-matching flag only makes sense if the caller can push
**more than one** user message down the same stdin over the lifetime of one
process — a single-shot input wouldn't need one. This is the same mechanism
that reportedly backs the official Python (`ClaudeSDKClient`) and TypeScript
Agent SDKs' persistent/streaming-input client mode: keep one subprocess
alive, keep stdin open, push additional `{"type":"user",...}` messages as
they arrive, and consume however many `result` events come back over time
(one per turn) instead of exiting after the first.

**Confirmed empirically (2026-09-17, native CLI 2.1.274).** A direct
Python-driven test against `/opt/claude-code/bin/claude` proves the
assumption: one process, invoked once as

```
claude -p --input-format stream-json --output-format stream-json --verbose \
       --permission-prompts none
```

(`--permission-prompts none` in place of `--dangerously-skip-permissions`,
which the CLI refuses outright when running as root/sudo — see §6 P0 for why
that matters for how tingly-box already handles this), fed two
`{"type":"user","message":{...}}` lines on the same stdin with a real pause
between them (no intervening close/reopen), produced:

- Turn 1 (`"Reply with exactly the single word: ALPHA"`) → a full
  `system(init) → assistant → system(post_turn_summary) → result(success)`
  event sequence, assistant text `"ALPHA"`, and the process still alive
  (`proc.poll() is None`) afterward.
- Turn 2, sent on the *same* stdin with no new process
  (`"What was the single word I asked you to reply with, just now?"`) →
  another complete turn, assistant text `"ALPHA"` — i.e. real conversational
  context carried across turns **within one live process**, not just two
  independent stateless calls.
- Both turns reported the identical `session_id` in their `result` event.
- The process only exited (code 0) once stdin was explicitly closed.

This directly validates §4's premise and unblocks the rest of this design.
One incidental finding worth carrying into the implementation: the CLI
re-emits a `system(init)` event at the start of every turn, not just the
first — a persistent-mode event consumer needs to treat that as a per-turn
marker, not a one-time handshake.

Today's code already stops at the first `result` regardless
(`Runner.Execute`'s `shutdownGracefully` fires on `EventKindTerminalSuccess`
and closes stdin) — that's the one thing that has to change, not the CLI
invocation itself.

### 3.2 Claude Code CLI: `--bg` / `agents` / `attach` / `stop` / `rm` — a
different, coarser persistence primitive

The same CLI also ships a *separate*, higher-level background-session
facility: `claude --bg` detaches a session and prints an id; `claude agents
--json` lists active/background sessions machine-readably; `claude attach
<id>` reattaches a terminal; `claude logs <id>` prints recent output; `claude
stop|kill <id>` stops but *keeps the conversation* (`claude attach`/`--resume`
reopen it); `claude rm <id>` deletes it. This is real prior art for the
**lifecycle vocabulary** (list / attach / stop-but-keep / delete) but not for
the **transport**: there is no documented way to inject a new structured turn
into an already-running background session other than an interactive
terminal attach. It's designed for long autonomous background tasks you
check in on, not for a chat bot feeding it one short message at a time. Worth
borrowing the *naming* (idle vs. running vs. stopped vs. removed), not the
mechanism.

### 3.3 Anthropic Managed Agents: Session/Thread/Event, over SSE
(`libs/anthropic-sdk-go`, beta `managed-agents-2026-04-01`)

This is the most directly analogous **server-hosted** precedent, and the
cleanest lifecycle vocabulary to copy:

- `BetaManagedAgentsSession.Status`: `rescheduling | running | idle |
  terminated` (`betasession.go`). A session is a standing resource with
  `Budget` (a hard spend ceiling — *"the session stops issuing new model
  requests once the tracked list cost reaches `max_list_cost`"*), cumulative
  `Usage`, `Stats`, and `ArchivedAt`.
- A caller opens one long-lived SSE stream
  (`client.Beta.Sessions.Events.StreamEvents`) and, independently, calls
  `Events.Send` for each new turn (`examples/managed-agents-streaming-deltas/
  main.go`). The stream delivers `event_delta` (incremental text),
  `agent.message` (a completed turn), `session.status_idle` (the session
  went idle — check `StopReason.Type == "end_turn"`), and `session.error`.
  **This is exactly the "turn boundary vs. session boundary" distinction
  today's `Runner.Execute` collapses into one** (§2): one long stream, many
  discrete turns, an explicit idle signal between them.
- Lifecycle verbs: `New`, `Get`, `Update`, `List`, `Delete`, `Archive` — a
  session can be archived (kept, read-only) distinctly from deleted.

### 3.4 Gemini Live API: WebSocket `Session` (`libs/go-genai/live.go`)

A lower-level bidirectional-transport precedent: `Live.Connect` opens one
WebSocket, blocks for a `LiveServerSetupComplete` handshake, then exposes
`SendClientContent` / `SendRealtimeInput` / `SendToolResponse` (send) and a
blocking `Receive()` (read one server message at a time) on the same
long-lived `*Session`, with an explicit `Close()`. Confirms the general shape
(one connection object, asymmetric send/receive methods, explicit close) but
nothing here needs a websocket — it's evidence for the *shape* of the Go API
(`Session.Send(...)`, `Session.Receive()`/event channel, `Session.Close()`),
not for transport choice.

### 3.5 What to take from each

| Precedent | Borrow |
|---|---|
| CLI streaming input | The transport itself: one process, many turns over one stdin — confirmed §3.1/§6 P0 |
| CLI `--bg`/`agents`/`attach` | Status vocabulary: idle / running / stopped-but-resumable / removed |
| Managed Agents Session | The *state machine* (`running/idle/terminated`) and the turn-boundary event (`session.status_idle`) distinct from the stream-lifetime boundary |
| Live API | The Go object shape: `Send`/events channel/`Close()` on one long-lived handle |

None of the four is a drop-in library for Go subprocess management — this
still has to be built in `agentboot`, informed by all four.

## 4. What "persistent" has to solve that "one-shot" avoided for free

The one-shot model got several properties for free, purely from process
boundaries. A persistent design has to solve each explicitly:

1. **Turn isolation.** Today, the transport factory is called once per
   `Execute` specifically so mutable state "cannot leak across concurrent bot
   chats" (`runner_execute.go` doc comment). A persistent process must
   guarantee at most one in-flight turn per session (serialize `Send` calls;
   reject or queue a second `Send` while one is running) — the process no
   longer enforces this by construction.
2. **Idle resource cost.** A process that never exits holds memory, open MCP
   server connections, and a live API-key/credential context indefinitely.
   Needs an idle timeout and a hard max lifetime, independent of any one
   turn's timeout.
3. **Bounding concurrency.** Today, N concurrent chats simply means N
   independent processes that come and go. Persistent sessions accumulate —
   need a cap on how many stay resident at once, with an eviction policy
   (LRU by `LastActivity`) for the rest, falling back to the existing
   one-shot+`--resume` path when evicted.
4. **Crash recovery.** A killed/crashed persistent process must not strand
   the conversation. Because Claude Code's own on-disk transcript is the
   durable source of truth (`--resume` already works across arbitrary
   process boundaries today), recovery is "transparently fall back to a
   fresh one-shot `--resume <session-id>` call, exactly like the current
   path" — the persistent process is an *optimization*, not a new durability
   mechanism.
5. **Turn boundary vs. session boundary as distinct signals.** Today
   `EventKindTerminalSuccess` means both at once. A persistent handle needs
   its own idle/turn-complete event distinct from stream-closed/terminated
   (§3.3's `session.status_idle` vs. the stream ending).
6. **Where isolation moves to.** If the process no longer isolates one turn
   from the next, exactly one logical session (`chatID`, `agent`,
   `project`) may ever bind to one persistent process at a time — the same
   key `session.Manager.FindBy` already uses.

## 5. Proposed design

### 5.1 New primitive: `agentboot.PersistentSession`

A new type alongside (not replacing) `Runner.Execute`/`ExecutionHandle`:

```go
type PersistentSession interface {
    // Send submits the next user turn on the session's single running
    // process. Returns ErrTurnInFlight if a previous turn hasn't reached
    // its terminal event yet — callers serialize at the chat/IM layer,
    // this is the last-resort guard.
    Send(ctx context.Context, prompt string) error

    // Events is the ordered, agent-neutral stream for the session's whole
    // lifetime — the existing StreamEvent sum type, plus two additions
    // (below): TurnCompleteEvent and SessionStateEvent.
    Events() <-chan StreamEvent

    Respond(reqID string, resp ControlResponse) error

    Status() SessionState // Idle | Running | Closing | Terminated

    // Close asks the current turn (if any) to finish, then shuts the
    // process down the same way Runner.Execute does today (stdin close,
    // grace period, Kill). Idempotent.
    Close(ctx context.Context) error
}
```

Two additions to `agentboot/events.go`'s sealed `StreamEvent` set:

- `TurnCompleteEvent{Result *Result}` — replaces "the handle closes" as the
  per-turn boundary signal (mirrors §3.3's `session.status_idle`). The
  *stream* stays open; only the *turn* ended.
- `SessionStateEvent{State SessionState, Reason string}` — carries
  idle-timeout firing, unexpected process exit (crash), and graceful close,
  so a consumer doesn't have to poll `Status()`.

`ExecutionHandle` is untouched. `Runner.Execute` (one-shot) is untouched and
remains the default for `cli/harness`, tests, and any caller that doesn't
opt in.

### 5.2 Where it lives: extend `Runner`, don't fork it

`Runner.Open(ctx, opts) (PersistentSession, error)` reuses everything from
`Runner.Execute` up through spawning the process and starting the
decode/encode pump (`runner_execute.go` lines ~41–174 today), with one
behavioral fork: on `EventKindTerminalSuccess`, instead of calling
`shutdownGracefully()`, emit `TurnCompleteEvent` and return to an idle wait
state, keeping the encoder/stdin open for the next `Send`. `Send` pushes a
new `StreamPromptBuilder`-shaped message onto the same encoder used for
control responses today — no new wire format, just no `Close()` on the
input channel after the first message.

`shutdownGracefully` (stdin close → grace period → `Kill`) is reused verbatim
for `Close()`.

**Context lifetime is not the same as `Execute`'s, and getting this wrong is
a silent, not a loud, failure.** `Execute`'s one-shot process correctly ties
its whole life to the `ctx` its single caller passed in — that process
*is* the request. `Open`'s process must not: its typical caller is a
per-message request handler whose `ctx` is canceled the moment that one
message finishes, but the session is meant to outlive that message. An
early version of this code derived `runCtx` from the caller's `ctx`
(`context.WithCancel(ctx)`), which — via both `Open`'s own
"kill on `runCtx.Done()`" goroutine and, for the real OS factory,
`exec.CommandContext`'s automatic kill — silently killed the process right
after the first turn, before the *next* `Send` ever arrived. Every session
degraded into a one-shot one on its second message, with no error: §5.3's
transparent fallback (open a fresh session when the pool finds a dead one)
papered over it perfectly. `runner_open_test.go`'s fake-process tests never
caught this because their one `ctx` stayed alive for the whole test
function — only an end-to-end test with independently-scoped,
per-message contexts (§6 P2) surfaced it. Fixed by deriving `runCtx` from
`context.Background()`: a persistent session's lifetime answers to `Close`
alone, never to the `ctx` of whichever call happened to touch it.

### 5.3 Isolation moves from "process per call" to "session registry"

**Implemented (2026-09-17) as `agentboot/pool` (`pool.Pool`), a separate
package from `agentboot` root** — pooling is a distinct concern from the
process/protocol lifecycle `Runner`/`PersistentSession` own; `pool.Pool`
only ever calls the public `PersistentSession` interface (`Status`/`Close`),
never anything about how the underlying process runs. It is agent-neutral
and key-format-agnostic: the caller decides what a key means. For `@cc`
that's the same `(chatID, agent, project)` tuple `session.Manager.FindBy`
already uses (not yet wired — see P2 remaining scope at the end of this
section).

- One `PersistentSession` per key, at most (`Open` returns
  `ErrKeyAlreadyOpen` on a live duplicate; callers `Acquire` first).
- `Open`/`Acquire`/`Touch`/`CloseAndRemove`/`Remove`/`Len`/`Shutdown`.
  `Send` serialization is *not* the pool's job — `PersistentSession.Send`
  already returns `ErrTurnInFlight` on its own (§5.1); the pool only ever
  decides which session a key maps to.
- Idle timeout (`Config.IdleTimeout`, a background sweep) auto-`Close()`s
  and evicts — but **only entries observed `Idle`**, never `Running`: idle
  time is measured from the caller's `Touch()` call after a
  `TurnCompleteEvent`, not from time-since-last-`Send`, so a
  longer-than-`IdleTimeout` turn is never mistaken for an idle session.
- Hard cap (`Config.MaxSessions`). Over the cap, `Open` evicts the
  least-recently-touched **Idle** entry to make room; if every resident
  entry is `Running`, `Open` returns `ErrFull` instead of force-closing an
  active turn — the caller falls back to a one-shot `Execute` for that turn.
  Evicted/idle-timed-out sessions fall back to the existing one-shot
  `--resume` path transparently on the next message — the user sees no
  difference beyond slightly higher latency on that one message.
- `session.Manager`'s existing `ExpiresAt.IsZero()` ("persistent: caller
  drives the lifecycle") seam is the natural place to mark a logical session
  as backed by a live `PersistentSession` versus the default expiring
  one-shot bookkeeping — still unwired, see below.

**Wired (2026-09-17).** `pool.Pool` now has a real consumer:
`ClaudeCodeExecutor.Execute` (`executor_claude.go`) branches into
`runPersistentTurn` when the bot opted in (§5.4). `Touch` is called after a
successful `RunTurnWithPrompter` return; a session observed
`SessionStateTerminated` after a failed turn is `Remove`d (not `Close`d
again — it already tore itself down) so the next message opens a fresh one.
`session.Manager`'s `SetRunning`/`SetCompleted` are called directly from
`runPersistentTurn` (the persistent path never sets `opts.Store`, since
`Runner.Open`/`PersistentSession` don't consume it — see §5.1) rather than
through the runner-internal wiring the one-shot path relies on.

### 5.4 Wiring into `@cc` — done

`ClaudeCodeExecutor.Execute` (`executor_claude.go`) now branches on
`e.deps.SessionPool != nil && bot.IsPersistentSession()`:

- **No pool entry for this `(botUUID, chatID, projectPath)` key**
  (`persistentPoolKey`) → `AgentService.Open` starts a new
  `PersistentSession` with this message as its first turn, then
  `pool.Pool.Open` registers it.
- **A live entry exists** → `PersistentSession.Send` submits this message as
  the next turn on the already-running process.
- Either way, the turn is then driven by `RunTurnWithPrompter` (§5.1) — the
  exact same `sink`/`prompter` closures the one-shot path already built,
  unchanged in shape.
- **Any failure to use the persistent path** (no capacity, a stale entry
  that raced closed, the agent not supporting `Open`, a `Send` failing
  outright) is treated as *not handled*: the caller falls straight through
  to the pre-existing one-shot `AgentService.Run` call for that one message,
  transparently, per §5.3's fallback contract. A session that terminates
  **mid-turn** (a crash) is the one case that does *not* silently retry as
  one-shot — tool calls may have already run once, so re-running the prompt
  risks doing them twice; that turn's error is reported like any other
  execution failure, and the dead session is dropped from the pool so the
  *next* message starts clean.

**Bot setting**: `persistent_session` (`*bool`, default unset = off) landed
end-to-end — `db.ImBotSettingsRecord`/`db.Settings` →
`imbot.CreateRequest`/`UpdateRequest` → `bot.BotSetting.IsPersistentSession()`
— following the exact same shape as the existing `require_pairing` tri-state
field. Per ux-principles.md §6 ("smart defaults over toggles"), this is a
deliberate, documented deviation: the blast radius (how many `claude`
processes stay resident, crash/eviction behavior) has no production mileage
yet, so it ships as an explicit opt-in rather than a default. UI: a `Switch`
in `CCProfileDialog.tsx`, presented as a separate control below (not inside)
the profile radio list — profile selection and persistent-session are
orthogonal axes (ux-principles.md §4) and must not share one control.

**Pool sizing**: one process-wide `pool.Pool` (`MaxSessions: 10`,
`IdleTimeout: 10m`, `internal/server/module/imbot/manager.go`'s
`sessionPoolConfig`), shared across every bot the server runs — not a
per-bot pool, since the cap is meant to bound total resident `claude`
processes for the whole instance. `MaxSessions` counts top-level entry
agents only — each may spawn subagents of its own, so 10 is a conservative
retention budget on top-level sessions, not a hard ceiling on total
processes. Not yet a tunable setting (§6 P3).

This should be **opt-in** (a bot/profile setting, not a global default) for
at least the first shipped iteration — see §6 phasing and
`.design/ux-principles.md` §5/§6 (smart defaults over toggles; but a change
with real resource/crash-blast-radius implications like "how many `claude`
processes stay resident" is exactly the kind of thing that deserves an
explicit, discoverable setting rather than a silent behavior change, at
least until the idle/eviction/crash-recovery paths have real production
mileage).

**Hardening found by code review (2026-09-17).** Three gaps surfaced once
this wiring existed to review, each fixed at the primitive it actually
belongs to rather than patched at the call site that noticed it:

- **`/stop` (or any caller `ctx` cancellation) could not interrupt a
  persistent turn.** `Execute`'s process is tied to its caller's `ctx`, so
  cancelling that `ctx` kills the process; a `PersistentSession`'s process is
  deliberately detached from any one caller's `ctx` (§5.2), so nothing
  bounded a persistent turn at all — the underlying `claude` process just
  kept running after the request that started the turn gave up on it. Fixed
  in the shared primitive: `RunTurnWithPrompter` (`agentboot/run.go`) now
  selects on `ctx.Done()` and closes the *whole session* on cancellation —
  there is no way to interrupt just the in-flight turn without ending the
  process, so this is the same effect `ctx` cancellation already has on a
  one-shot `Execute`, just applied consistently to the persistent path.
  *Superseded (2026-09-25):* the CLI does accept an `interrupt`
  control_request that ends only the turn (verified against 2.1.282);
  `PersistentSession.Interrupt` sends it, and `agentboot.Conductor` uses it
  on cancellation, closing the session only if the agent can't or doesn't
  comply. `RunTurnWithPrompter` keeps the close-on-cancel behavior for its
  existing callers. See `.design/desk.md` §3.7.
- **No execution timeout on a persistent turn**, unlike one-shot execution's
  30-minute default (`Runner.Execute` applies `defaultTimeout`/`opts.Timeout`
  to the whole process). `runPersistentTurn` now applies the same
  `ExecutionOptions.Timeout` zero/negative/positive semantics to a single
  turn, sourced from `AgentService.Config().DefaultExecutionTimeout`, via a
  shared `agentboot.ResolveTimeout` helper `Runner.Execute` also uses now
  (one resolution rule, not two hand-rolled copies).
- **Turning `persistent_session` off, or stopping the bot, left the resident
  pool session running.** The abandoned process kept the on-disk Claude
  session file open while a later message resumed the same session ID in a
  fresh one-shot process — a session-file-conflict race, the same failure
  mode §5.3's "any failure falls back to one-shot" already treats as
  recoverable, except here nothing ever told the pool to let go. Fixed by
  giving `pool.Pool` a `CloseAllWhere(ctx, match func(key string) bool) int`
  (bulk, predicate-based eviction — `Pool` still has no notion of what a key
  *means*, so the predicate is the caller's) and a new exported
  `remoteagent.EvictPersistentSessionsForBot(pool, botUUID)` /
  `BotManager.EvictPersistentSessions(uuid)` pair that call it from
  `BotManager.StopBot` and from `Handler.UpdateSettings` when
  `persistent_session` is explicitly turned off while the bot keeps running.
  This is also why `persistentPoolKey` dropped the `platform` segment:
  `BotUUID` alone already uniquely identifies one bot on one platform (it's
  the `imbot_settings` primary key), so leading the key with it — instead of
  `platform` — is what makes a `"<botUUID>|"` prefix match possible.

All three were caught by a dedicated `code-review` pass over this branch's
diff, not by the existing unit/e2e tests — `TestRunTurnWithPrompter_CtxCancelClosesSession`
(`agentboot/run_turn_test.go`) and `TestPool_CloseAllWhere`
(`agentboot/pool/pool_test.go`) now cover the first and third directly. A
follow-up `simplify` pass (four review agents: reuse, simplification,
efficiency, altitude) on the same diff found and fixed: the same
zero/negative/positive timeout logic duplicated between `Runner.Execute` and
`runPersistentTurn` (→ `agentboot.ResolveTimeout`); three independent copies
of the same 15-second "give a session time to close" literal (→
`agentboot.SessionCloseTimeout`); `ClaudeCodeExecutor.Execute` fetching the
bot setting twice per message via a call that hits the DB, not an actual
cache; and `BotManager.StopBot` holding its instance-wide mutex across the
new `EvictPersistentSessions` call, which can block for
`SessionCloseTimeout` waiting on a session's process — serializing an
unrelated bot's start/stop behind this one's teardown. One suggestion from
that pass was deliberately not taken: replacing `persistentPoolKey`'s opaque
`"|"`-joined string with a structured `pool.Key`/owner index so eviction
doesn't need to reverse-engineer a prefix. That's a real design
improvement, but it changes `pool.Pool`'s already-published public API
surface for a benefit (avoiding one documented prefix convention) that
doesn't yet justify the churn — worth revisiting alongside §6 P3.

### 5.5 Explicitly out of scope for the first version

Mirroring §P3's own "deliberately not copied" list, kept deliberately small:
hooks, in-process MCP servers, rewind/task controls, dynamic model/permission
mutation, interrupting a turn mid-flight (Claude Code CLI does support
interrupt in interactive mode, but wiring that through IM chat's UX is a
separate feature), and cross-process session migration (moving a live
`PersistentSession` between tingly-box instances). None of these block the
core efficiency win (skip process-spawn/startup cost per message).

## 6. Phasing

1. **P0 — feasibility spike — DONE (2026-09-17).** §3.1 confirms the core
   assumption directly with a real two-turn run against the native CLI: one
   process, one stdin left open, two independent turns, one `session_id`,
   context preserved across turns. One root-specific gotcha surfaced along
   the way: `--dangerously-skip-permissions` is refused outright when the
   process runs as root/sudo ("cannot be used with root/sudo privileges for
   security reasons"); `agentboot/claude/driver.go`'s `isRoot()` check
   already knows to omit that flag as root, but does not yet substitute
   `--permission-prompts none` (or an equivalent) in that case for
   stream-json/persistent execution — worth checking whether tingly-box's
   own deployment containers run as root, since that changes which flag P1's
   permission plumbing needs by default.
2. **P1 — core primitive — DONE (2026-09-17).** `PersistentSession`,
   `Runner.Open` (`agentboot/persistent_session.go`,
   `agentboot/runner_open.go`), the two new `StreamEvent` types, and
   `AgentTransport.EncodeUserMessage` (+ `claude.Transport`'s
   implementation and `claude.Agent.Open`), with unit tests against
   `process.FakeFactory` (`agentboot/runner_open_test.go`) proving: two
   `Send` calls complete on one process without a respawn, `ErrTurnInFlight`
   while a turn is running, `Close()` reusing the same close-stdin/grace/
   Kill sequence as `Execute`'s `shutdownGracefully`, and an unprompted
   process exit producing `SessionStateEvent{Terminated}` (not a hang) with
   `Send` afterward returning `ErrSessionClosed`. `go build`/`go vet`/
   `go test -race` green in `agentboot` and in the root module's
   `remote/...` tree (no other `AgentTransport` implementer existed to
   update). One correction from the plan below: idle timeout is *not* part
   of this primitive — `PersistentSession` only exposes `Close`/`Status`;
   idle-timeout-driven `Close` calls are a P2/registry concern per §5.3, not
   something the primitive enforces on itself.
   Not yet wired to anything — `Runner.Execute`/`ExecutionHandle` are
   untouched and remain the only path `@cc` actually uses.
3. **P2 — registry + wiring — DONE (2026-09-17).** `pool.Pool`
   (capacity/LRU eviction, idle-timeout sweep, never evicts a `Running`
   session) in `agentboot/pool`, wired into `ClaudeCodeExecutor` behind the
   `persistent_session` bot setting (§5.3/§5.4). One process-wide pool
   shared across every bot. Frontend toggle in `CCProfileDialog.tsx`; full
   `task codegen` (backend `openapi.json` + `pnpm gen:api`) run and
   committed as part of this phase. `go build`/`go vet`/`go test -race`
   green across the whole repo; `pnpm typecheck`/`pnpm lint` clean on the
   frontend.
4. **P3 — observability — not started.** Surface resident-process count /
   per-session idle time somewhere an operator can see it (metrics or a
   debug endpoint), and make pool sizing (`MaxSessions`/`IdleTimeout`,
   currently hardcoded in `sessionPoolConfig`) a real setting once there's
   production mileage to tune against.

## 7. Open questions / risks

- **The root/`--dangerously-skip-permissions` gap found during P0** applies
  to *today's* one-shot path too, not just the persistent design. Low actual
  risk today — `SetSkipPermissions` has zero callers in product code or
  tests (`@cc`'s real bypass path is `--permission-mode bypassPermissions` +
  the app-level `autoApprovePrompter`, neither of which touches `isRoot()`)
  — so this is flagged with a code comment at the call site
  (`agentboot/claude/driver.go`) rather than fixed speculatively; revisit if
  a future caller actually wires `skipPerms` up under a root deployment.
- **Crash blast radius.** A persistent process holds credentials/env for its
  full idle lifetime instead of a few seconds — worth an explicit look at
  whether `execEnv`/`settingsPath` (per-message today, `executor_claude.go`
  lines 126–146) can change *mid-session* (e.g. a profile edit in the web UI
  while a persistent session is idle) and whether that should force a
  session close+reopen rather than silently keep stale routing.
- **Context growth.** A long-idle-but-not-closed session still accumulates
  conversation history the same way `--resume` does today; Claude Code's own
  auto-compaction applies regardless of whether the process stayed alive, so
  this isn't a new problem, just worth confirming it isn't made worse.
- **Interaction with `/stop` and the "session in use" UX.** Today's
  `isSessionInUseText` error and its user-facing message
  (`executor_claude.go`) become dead code for sessions handled by the
  registry (§5.3) — should be confirmed unreachable, not just superseded, or
  removed once persistent mode covers all `@cc` traffic for a bot.
- **This document should be cross-linked from `.design/agentboot-refactor.md`
  §P3** (which currently reads as a closed decision) — done as part of this
  change; a future reader of §P3 should land here.

---

# Part B — Claude Code 客户端兼容层（`claude_code_version`）：逆向分析与实现映射

> 受众：维护 Claude OAuth 链路（`internal/client/claude_client.go` 及其周边）的后端贡献者，以及下一次
> Anthropic 抬高 Claude Code 最低版本时负责升版的人。
>
> 本部分记录 tingly-box 如何把发往 Claude OAuth provider 的请求重签为官方 Claude Code CLI 的样子：模拟了哪些
> wire 元素、官方客户端怎么生成它们、我们对齐到什么程度、哪些地方有意不对齐。结论全部来自官方 npm 包的逆向和
> 真实二进制的抓包，可复现（§B2）。
>
> flag 有三个取值：空值 **Default**（解析为最新版本，当前即 2.1.280）、原生 profile **2.1.280**、**2.1.86**（Legacy 模拟，需显式选择）。先看 §B0；升版看 §B6。

---

## B0. 概览

Anthropic 按 `User-Agent` 里的 claude-cli 版本做门控，低于要求的版本直接拒绝：

```
400 {"type":"error","error":{"type":"invalid_request_error",
 "message":"Claude Code 2.1.258 does not support this model; version 2.1.280 or newer is required. ...",
 "details":{"error_code":"claude_code_version_too_old"}}}
```

只改版本号不够：同期的 SDK 版本、beta 列表、billing header、metadata 都变了，"新 UA + 旧的其他一切"本身就是
指纹异常。所以原生 profile 逐项对齐真实客户端。

### B0.1 启用方式：`claude_code_version` rule flag，默认最新版本

| 值 | 行为 |
|---|---|
| `""`（Default） | 跟随 `typ.ClaudeCodeVersionLatest`，当前解析为 `2.1.280`。存量配置里没写这个字段的规则自动走这一档，无需迁移 |
| `2.1.280` | 原生客户端 profile（§B3，2.1.280 的差异见 §B8） |
| `2.1.86`（Legacy） | 与 flag 出现之前**逐字节相同**的 2.1.86 模拟：`claude_round_tripper.go` 的常量、静态 beta 串、随机 `cch`、按字节的 fingerprint、`\u003c` 转义都原样保留 |

- 定义在 `typ.RuleFlags.ClaudeCodeVersion`（registry：`claude_code_version`，enum，`request_anthropic` 分类）；
  也可在 scenario 级设置（`ScenarioFlags.ClaudeCodeVersion`，rule 值优先）。
- **只作用于 Claude OAuth provider**：同一规则经负载均衡或 failover 落到其它 provider 时，flag 解析会清掉该值
  （其它客户端没有 cch 中间件）。未知值按 Default（最新版本）处理，不会出现半套 profile。
- 解析点唯一：`ResolveRuleFlagsWithScenario` 在 scenario 继承与 probe overlay 之后，对 Claude OAuth provider 调
  `typ.ResolveClaudeCodeVersion` 把配置值落成具体版本（`2.1.86` 或原生版本），下游只看到具体值，`""` 只表示"不适用"。
  没经过规则解析的旁路（模型列表、light probe、vision proxy、advisor 等直接用 client pool 的调用）上下文里没有 flag，
  仍按 Legacy 头发出。
- 覆盖 `/v1/messages` 与 `/v1/messages/count_tokens`；Legacy 的 count_tokens 仍用不带 flag 的 context，行为不变。
- provider 级 probe 的合成规则与普通规则一样按 Default 解析为最新 profile，并补 Claude Code preamble（见 `.design/probe.md`）。
- 数据流：`ResolveRuleFlagsWithScenario` 合并 → `RulePreVendorTransforms` 挂 `ClaudeCodeVersionTransform`，把版本写进
  chain `Extra` → `ops.ApplyAnthropic*MetadataTransform` 按 `ClaudeCodeVersionFromExtra` 分派到
  `applyNativeClaudeCodeIdentity*`（Legacy 路径一行未动）→ `NewClaudeClient` 按 `claudeCodeNativeVersion(ctx)` 叠加
  `claude_version.go` 的 header / beta / cch 覆盖层。
- **只保留 Legacy + 最新版本**：被拒的旧版本没有保留价值，升版时原地升级原生 profile，不做版本间门控（§B6）。
- 与 flag 无关的唯一改动：`transform/vendor.go::isClaudeCodeBackend`。Claude OAuth issuer 挂在非
  `api.anthropic.com` host 上时也做 identity 注入，此前这种配置会因缺 metadata 在 `Guard` 里 panic。

### B0.2 状态总表

✅ = 已实现并有测试；↩ = 不合成，只在入站已带时校验后保留；— = 依赖代理侧看不到的信息，无法对齐。

| 类别 | 项 | Legacy（2.1.86） | 2.1.280（默认） | 备注 |
|---|---|---|---|---|
| client header | `User-Agent` | `claude-cli/2.1.86 (external, cli)` | ✅ `claude-cli/2.1.280 (external, cli)` | |
| client header | `X-Stainless-Package/Runtime-Version` | `0.74.0` / `v24.3.0` | ✅ `0.112.1` / `v26.3.0` | 原生 Bun 二进制伪装的 Node 版本 |
| client header | `X-Stainless-OS/Arch` | Go 的 `linux`/`amd64` | ✅ SDK 映射名 `Linux`/`x64`、`MacOS`/`arm64` | |
| client header | `x-stainless-helper-method` | 发 `stream` | ✅ 不发 | CLI 不用 `.stream()` helper |
| client header | `x-app` | 一律 `cli` | ✅ `cli`；入站为 `cli-bg`（后台会话）时回放 | |
| client header | 子 agent 头 `x-claude-code-agent-id` / `-parent-agent-id` | 丢弃 | ✅ 透传 | |
| client header | `x-claude-code-request-class` / `-agent-type` | 不发 | ✅ class 默认 `main`，两者入站回放 | 直连专属 |
| beta | `anthropic-beta` | 固定串（含已废弃的 `token-efficient-tools`），可能两行 | ✅ 单值，逐请求合成：model 基线 + body 派生 + 入站白名单回放，按官方 push 顺序 | §B3.2 |
| beta | `structured-outputs` 灰度分支（`tengu_tool_pear`） | — | — | 改为按 body `format` 派生 |
| beta | count_tokens | Legacy 串 | ✅ 四项子集 | 与 CLI 相同，不含 context-1m |
| billing header | `cc_version` 与 fingerprint | 2.1.86，按字节取字符 | ✅ 2.1.280；跳过 `<system-reminder>`；按 UTF-16 码元取字符 | §B3.3.2 |
| billing header | `cc_entrypoint` | `cli` | ✅ `cli` | |
| billing header | `cch` | 每请求随机 5 hex | ✅ 请求体 xxHash64（Zig 变体），只替换 billing header 里的占位符 | 6 组抓包逐字节复现；§B3.3.4 |
| billing header | 入站 `cc_workload` / `cc_is_subagent` | 整块覆盖 | ✅ 校验后保留 | |
| billing header | `cc_prev_req` / `cc_prompt_id` / `cc_turn_origin` | 丢弃 | ↩ | 合成需跨请求状态（§B7） |
| body | JSON 转义还原为 JS 形态（`\u003c` → `<`） | Go 转义 | ✅ | 也是 cch 预像必需 |
| metadata | `device_id` / `account_uuid` 改写、`session_id` 保留 | ✅ | ✅ | |
| metadata | `parent_session_id` | 丢弃 | ✅ 透传 | |
| metadata | `ti` / `tk` | — | — | remote 专属，本地 CLI 不发 |
| preamble | 三句身份句 | ✅ | ✅ | 未变 |
| clean header | 非 Claude OAuth provider 上剥离 billing header / 隐写标记 | ✅ | ✅ | 未改 |
| 其他 | 隐写 normalizer | 保留 | 保留 | bundle 中无对应代码（§B3.6） |

---

## B1. 我们在模拟什么（wire 元素清单）

Claude OAuth 链路（`ClientPool.GetAnthropicClient` → `NewClaudeClient`）的每个
请求由下面几层拼出来。术语对应用户口中的四类内容：

| 用户术语 | 具体内容 | 产生位置 |
|---|---|---|
| **client header** | `User-Agent`、`x-app`、`X-Claude-Code-Session-Id`、`X-Stainless-*`、`anthropic-beta`、`anthropic-version`、`anthropic-dangerous-direct-browser-access`、`accept`、子 agent 头 | `internal/client/claude_client.go`（`applyClaudeCodeHeaders`，Legacy）+ `claude_version.go`（原生覆盖层）、`claude_betas.go` |
| **system header** | `system[0]` 的 `x-anthropic-billing-header: ...` 文本块；`system[1]` 的身份 preamble | `internal/protocol/ops/request_anthropic_model.go`（`ApplyAnthropic*MetadataTransform`）、`claude_code_billing_header.go` |
| **meta data** | `metadata.user_id` 的 JSON 串 | Legacy：`internal/protocol/metaid`；原生：`claude_code_billing_header.go::buildNativeMetadataUserID` |
| **clean header** | 转发到**非** Claude OAuth provider 时剥掉 billing header / 隐写标记 / preamble | `internal/protocolserver/transform/transform_clean_header.go`；flag 解析见 `rule_flags.go`（Claude OAuth provider 上自动关闭） |

---

## B2. 分析方法（可复现）

### B2.1 获取官方包

```bash
curl -sS https://registry.npmjs.org/@anthropic-ai/claude-code | jq '.["dist-tags"]'
V=2.1.280   # 取 latest
curl -sSL https://registry.npmjs.org/@anthropic-ai/claude-code/-/claude-code-$V.tgz | tar xz
```

**打包形态在 2.1.251 前后变了**：

- `≤ 2.1.2xx`（含 2.1.86）：`package/cli.js` 是 12 MB 的 bundle，可直接用 node 跑，也可直接 grep。
- `≥ 2.1.251`：主包只剩 `install.cjs` / `cli-wrapper.cjs` / `bin/claude.exe`，真正的 CLI 是
  平台原生 Bun 二进制，来自 `optionalDependencies`：`@anthropic-ai/claude-code-{darwin-arm64,darwin-x64,linux-x64,linux-arm64,linux-x64-musl,linux-arm64-musl,win32-x64,win32-arm64}`。
  ```bash
  curl -sSL https://registry.npmjs.org/@anthropic-ai/claude-code-linux-x64/-/claude-code-linux-x64-$V.tgz | tar xz
  ./package/claude --version
  ```
  这也解释了 `X-Stainless-Runtime-Version` 的变化：不再是用户机器的 node，而是 Bun 伪装的 Node 版本（Bun 1.4.1 → `v26.3.0`）。

### B2.2 从原生二进制里取 JS 源

Bun 单文件可执行把 bundle **明文**嵌在 ELF 里（`/$bunfs/root/*.js`），没有 bytecode 编译。
二进制里 `claude-cli/` 出现两次：第一次是 JSC 字符串表的碎片，第二次落在完整源码区。
向两侧扫"连续可打印字节区"即可切出 30 多 MB 的 bundle，之后的分析方法与 `cli.js` 完全一样：

```python
data = open('claude','rb').read()
i = data.find(b'claude-cli/', data.find(b'claude-cli/') + 1)   # 第二次出现
def txt(b): return b in (9,10,13) or 32 <= b < 127 or b >= 128
lo = hi = i; n = 0
while lo > 0 and n <= 2: n = n + 1 if not txt(data[lo-1]) else 0; lo -= 1
n = 0
while hi < len(data) and n <= 2: n = n + 1 if not txt(data[hi]) else 0; hi += 1
open('cli.js','wb').write(data[lo:hi])
```

### B2.3 静态分析：怎么定位关键函数

bundle 是 minified 的，符号名每版都变，靠字面量锚定：

| 要找的东西 | 锚定字符串 | 形态（2.1.258 时的符号名，每版会变） |
|---|---|---|
| SDK client 创建 / 默认头 | `"x-app":` | `async function jF({apiKey,maxRetries,model,...}){... G={"x-app":St()?"cli-bg":"cli","User-Agent":rI(),[TFe]:Q(), ...}` |
| User-Agent | `claude-cli/` | `` `claude-cli/${VERSION} (external, ${CLAUDE_CODE_ENTRYPOINT??"cli"}${agent-sdk}${client-app}${workload})` `` |
| billing header 渲染 | `x-anthropic-billing-header: cc_version` | `function r4t(fp, agentContext, prevReqId, promptId, opts)` |
| fingerprint | `59cf53e54c78` | `Nct(text, version)`：`[4,7,20].map(i=>text[i]||"0")`，`sha256(salt+chars+version).slice(0,3)` |
| beta 注册表 | `"interleaved-thinking-2025-05-14"` | `Ee(name, header)` 冻结对象；`XZ` 为全量注册表 |
| beta 组成 | `DISABLE_INTERLEAVED_THINKING` | `function Jne(model)`（allModelBetas）；`rEt` 追加 per-query flag |
| metadata.user_id | `account_uuid:` 附近的 `session_id:` | `function XH({agentContext})` |
| SDK 版本 | `X-Stainless-Package-Version` 引用的变量 | `var se="0.112.1"` |
| OS/Arch 映射 | `"MacOS"` | SDK 的 `xs(platform)` / `Ss(arch)` |
| system preamble | `"You are Claude Code` | 三个变体：CLI / Agent SDK 内的 CLI / 纯 Agent SDK |

### B2.4 动态抓包（最可信的证据）

把 `ANTHROPIC_BASE_URL` 指到本地假 API，让**真实二进制**自己发请求，落盘头和 body：

```python
# fake_api.py  <port> <outdir>  —— 对 POST /v1/messages 回一段最小 SSE，其余 404
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json, sys, os
class H(BaseHTTPRequestHandler):
    protocol_version = 'HTTP/1.1'
    def do_POST(self):
        n = int(self.headers.get('Content-Length') or 0); body = self.rfile.read(n)
        json.dump({'path': self.path, 'headers': dict(self.headers), 'body': json.loads(body)},
                  open(os.path.join(sys.argv[2], 'req.json'), 'w'), indent=1)
        evs = [('message_start', {'type':'message_start','message':{'id':'msg_01','type':'message','role':'assistant',
                 'model':'x','content':[],'stop_reason':None,'usage':{'input_tokens':1,'output_tokens':1}}}),
               ('content_block_start', {'type':'content_block_start','index':0,'content_block':{'type':'text','text':''}}),
               ('content_block_delta', {'type':'content_block_delta','index':0,'delta':{'type':'text_delta','text':'hi'}}),
               ('content_block_stop', {'type':'content_block_stop','index':0}),
               ('message_delta', {'type':'message_delta','delta':{'stop_reason':'end_turn'},'usage':{'output_tokens':1}}),
               ('message_stop', {'type':'message_stop'})]
        data = ''.join(f'event: {e}\ndata: {json.dumps(d)}\n\n' for e, d in evs).encode()
        self.send_response(200); self.send_header('Content-Type','text/event-stream')
        self.send_header('Content-Length', str(len(data))); self.end_headers(); self.wfile.write(data)
ThreadingHTTPServer(('127.0.0.1', int(sys.argv[1])), H).serve_forever()
```

```bash
mkdir -p h && echo '{"hasCompletedOnboarding":true}' > h/.claude.json
env -i PATH=$PATH TERM=xterm HOME=$PWD/h \
    CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1 DISABLE_TELEMETRY=1 DISABLE_ERROR_REPORTING=1 \
    ANTHROPIC_BASE_URL=http://127.0.0.1:18091 \
    CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-fake \          # 或 ANTHROPIC_API_KEY=sk-ant-api03-fake
    ./claude -p "say hi" --model claude-sonnet-4-6 < /dev/null
```

抓包时的坑（都踩过）：

- **必须 `env -i`**。宿主环境里若有 `CLAUDE_CODE_ENTRYPOINT=remote`、`CLAUDE_CODE_OAUTH_TOKEN`、
  `CLAUDE_CODE_CONTAINER_ID` 等，二进制会原样带上（UA 变成 `(external, remote)`，还会用宿主的真 token）。
- `-p` 模式的 entrypoint 是 `sdk-cli`，system prompt 也是 Agent SDK 变体；交互式终端才是 `cli`。
  交互式与 `-p` 在 beta 上只差一个 `redact-thinking-2026-02-12`（`-p` 不发）。
- `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1` 会跳过 GrowthBook 拉取，所有 `tengu_*` 灰度 gate 走默认值
  （例如 `structured-outputs` 的 `tengu_tool_pear` 默认 false）。线上用户拿到的 gate 值不可知。
- 假 API 若不给 `request-id` 响应头也没关系，但真实服务会给，客户端会把它作为下一请求的 `cc_prev_req`（仅直连时）。
- 加 `_CLAUDE_CODE_ASSUME_FIRST_PARTY_BASE_URL=1` 才能看到直连专属字段（`cch`、`cc_prompt_id`、`cc_prev_req`）。
  **原生层会改写 body**：要研究这类字段必须跑原生二进制而不是 node + cli.js，并保存原始字节（`.raw`）。

---

## B3. 逆向结论：官方客户端怎么生成这些字段

> 抓包和符号名来自 2.1.258（与 2.1.86 对比），结论由 2.1.280 沿用；2.1.280 的差异见 §B8。

### B3.1 Client headers（抓包，`-p` 模式，OAuth token）

```
# 2.1.86 (node cli.js)                          # 2.1.258 (native binary)
User-Agent: claude-cli/2.1.86 (external, sdk-cli)   claude-cli/2.1.258 (external, sdk-cli)
x-app: cli                                          cli
X-Claude-Code-Session-Id: <uuid>                    <uuid>
X-Stainless-Lang: js                                js
X-Stainless-Package-Version: 0.74.0                 0.112.1
X-Stainless-OS: Linux                               Linux
X-Stainless-Arch: x64                               x64
X-Stainless-Runtime: node                           node
X-Stainless-Runtime-Version: v20.20.2 (宿主 node)    v26.3.0 (Bun 1.4.1)
X-Stainless-Retry-Count: 0                          0
X-Stainless-Timeout: 600                            600
anthropic-version: 2023-06-01                       2023-06-01
anthropic-dangerous-direct-browser-access: true     true
Accept: application/json                            application/json
Authorization: Bearer sk-ant-oat01-…                （同）
anthropic-beta: …见 3.2                             …见 3.2
POST /v1/messages?beta=true                         同
```

要点：

- **没有 `x-stainless-helper-method`**。CLI 直接 `beta.messages.create({stream:true})`，不走 `.stream()` helper；
  该头只在 SDK 的 tool-runner 路径出现。旧实现固定发 `stream` 是错的，已移除。
- `X-Stainless-OS/Arch` 是 SDK 对 `process.platform/arch` 的映射（`darwin→MacOS`、`win32→Windows`、`linux→Linux`；
  `x64/arm64/x32/arm`）。旧实现直接发 Go 的 `runtime.GOOS/GOARCH`（`darwin`/`amd64`），真实客户端从不会这样。
- `x-app` 在后台会话（`CLAUDE_CODE_SESSION_KIND=bg`）时是 `cli-bg`；原生 profile 在入站为 `cli-bg` 时回放，否则发 `cli`。
- 2.1.258 新增可选头：`x-claude-code-agent-id`、`x-claude-code-parent-agent-id`（子 agent 上下文）、
  `x-claude-remote-container-id`、`x-claude-remote-session-id`（CCR）、`x-client-app`（Agent SDK 宿主）、
  `x-anthropic-additional-protection`。我们只透传前两个（真实子 agent 请求经过 tingly 时会带）。
- Header 值经过 `C4t` 校验（拒绝非法 header value）；agent id 用 `SAn` 做百分号编码（`%` 及非可打印 ASCII）。
  `client.sanitizeClaudeHeaderValue` 复刻了它。

### B3.2 `anthropic-beta`

#### B3.2.1 官方逻辑（2.1.258，去混淆后的伪代码）

```js
// allModelBetas(model) —— 每个请求都会带的"基线"
betas = []
if (!model.includes("haiku"))                       betas.push("claude-code-20250219")
if (isClaudeAiOAuth())                              betas.push("oauth-2025-04-20")
if (/\[1m\]/i.test(model) && !DISABLE_1M_CONTEXT)   betas.push("context-1m-2025-08-07")
if (!DISABLE_INTERLEAVED_THINKING && supportsInterleaved(model))
                                                    betas.push("interleaved-thinking-2025-05-14")
if (firstParty && supportsInterleaved(model) && isInteractive && !showThinkingSummaries)
                                                    betas.push("redact-thinking-2026-02-12")
if (supportsInterleaved(model) && !DISABLE_EXPERIMENTAL_BETAS && provider=="firstParty")
                                                    betas.push("thinking-token-count-2026-05-13")
if (!model.includes("claude-3-") && !DISABLE_EXPERIMENTAL_BETAS)
                                                    betas.push("context-management-2025-06-27")
if (growthbook("tengu_tool_pear") && supportsStructured(model))
                                                    betas.push("structured-outputs-2025-12-15")   // 灰度
if (provider=="vertex"||"foundry")                  betas.push("web-search-2025-03-05")
if (firstParty)                                     betas.push("prompt-caching-scope-2026-01-05")
if (supportsMidConversationSystem(model))           betas.push("mid-conversation-system-2026-04-07")
betas.push(...ANTHROPIC_BETAS.split(","))

// per-query (rEt)
if (perTurnEffort(model))                           betas.push("per-turn-control-2026-07-01")
if (growthbook("tengu_mossy_lantern") && midConvToolChange(model))
                                                    betas.push("mid-conversation-tool-changes-2026-07-01")

// 主查询循环里按调用顺序追加
effort 存在              → "effort-2025-11-24"
task_budget 存在         → "task-budgets-2026-03-13"
output_config.format     → "structured-outputs-2025-12-15"
thinking.display=updates → "thinking-display-updates-2026-08-18"
fast mode                → "fast-mode-2026-02-01"        (body: speed:"fast")
auto mode                → "afk-mode-2026-01-31"
cache ttl 1h             → "extended-cache-ttl-2025-04-11"
context hint             → "context-hint-2026-04-09"
evict-on-complete        → "prompt-caching-evict-2026-05-12"
cache diagnosis          → "cache-diagnosis-2026-04-07"
tool search 工具在列     → "advanced-tool-use-2025-11-20"（Bedrock/Vertex 为 "tool-search-tool-2025-10-19"）
```

model 能力判断（`Ve()` 归一化后：小写、去 `[1m]`、去 `-YYYYMMDD` 快照日期）：

| 判断 | 为假的模型 |
|---|---|
| `supportsInterleaved` | `claude-haiku-4-5`、`claude-3-*` |
| `supportsContextManagement` | `claude-3-*` |
| `supportsMidConversationSystem` | `claude-3-*`、opus 4.0/4.1/4.5/4.6/4.7、sonnet 4.0/4.5/4.6、haiku 4.5；其余（sonnet-5 / opus-5 / mythos / fable …）为真 |

count_tokens 只保留 `{claude-code, interleaved-thinking, context-management, oauth}`。

#### B3.2.2 抓包实例

```
2.1.86  OAuth sonnet-4-6 -p : claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,context-management-2025-06-27,prompt-caching-scope-2026-01-05,effort-2025-11-24
2.1.258 OAuth sonnet-4-6 -p : claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,thinking-token-count-2026-05-13,context-management-2025-06-27,prompt-caching-scope-2026-01-05,effort-2025-11-24,extended-cache-ttl-2025-04-11
2.1.258 API-key sonnet-4-6 -p: claude-code-20250219,interleaved-thinking-2025-05-14,thinking-token-count-2026-05-13,context-management-2025-06-27,prompt-caching-scope-2026-01-05,effort-2025-11-24
```

（2.1.258 OAuth 多出 `extended-cache-ttl`：订阅用户的 system 块 `cache_control` 带 `ttl:"1h"`。）

#### B3.2.3 旧实现的问题

旧的固定串 `claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,context-management-2025-06-27,prompt-caching-scope-2026-01-05,structured-outputs-2025-12-15,fast-mode-2026-02-01,redact-thinking-2026-02-12,token-efficient-tools-2026-03-28`：

- `token-efficient-tools-2026-03-28` 在 2.1.86 里已经是空串常量（`g54=""`），2.1.258 注册表里根本没有这个 flag；
- `fast-mode` / `structured-outputs` 无条件发送，而官方只在开了 fast mode / 带 `format` 时发；
- 缺 `thinking-token-count-2026-05-13`（2.1.258 基线）；
- 对 haiku 也发 `claude-code-20250219`；
- 顺序与官方 push 顺序不一致（`redact-thinking` 应紧跟 `interleaved-thinking`）。

#### B3.2.4 现在的做法（`internal/client/claude_betas.go`）

三层合成，按官方 push 顺序（`claudeCodeBetaEmissionOrder`）输出成**单个** header 值：

1. **基线**：按上表用 outbound model 判断，persona 固定为交互式终端（含 `redact-thinking`）；
   `oauth` 仅在 provider 凭证是 `sk-ant-oat…` 时加；`context-1m` 来自 `context_1m` rule flag
   （`applyContextOneM` 已把入站 header 里的 1M 转成 flag）。
2. **请求体派生**：`output_config.effort` → effort；`output_config.format` / `output_format` → structured-outputs；
   `output_config.task_budget` → task-budgets；`thinking.display=="updates"` → thinking-display-updates；
   `speed=="fast"` → fast-mode；任一 `cache_control.ttl=="1h"`（system / tools / messages）→ extended-cache-ttl；
   tools 含 `tool_search_tool_*`、`defer_loading` 或名为 `ToolSearch` 的工具 → advanced-tool-use（v1 与 beta 路径都派生）；
   thinking 为 adaptive / enabled → thinking-binding-controls；body 带 `diagnostics` → cache-diagnosis。
3. **入站回放（白名单）**：真实 Claude Code 客户端自己协商出的、无法从 body 反推的 flag
   （per-turn-control、mid-conversation-tool-changes、afk-mode、context-hint、prompt-caching-evict、cache-diagnosis …）
   从入站 `anthropic-beta` 头回放；白名单之外的一律丢弃（`message-batches`、`pdfs`、`managed-agents` 等
   SDK flag 真实 CLI 从不发）。入站 header 由 `protocolserver.applyClaudeCodeClientHints` 挂进 context（`typ.ClaudeCodeClientHints`）。

`ClaudeClient` 的 `MessagesNew*/BetaMessagesNew*` 直接调 SDK 而不再经过 `AnthropicClient` 包装：
包装层的 `withContext1MBeta` 会用 `WithHeaderAdd` 再追加一次 `context-1m`，导致 Go 发出两行同名 header。
`req.Betas` 在 Guard 里清空，理由相同。

### B3.3 Billing header（system header）

#### B3.3.1 格式

```
2.1.86 : x-anthropic-billing-header: cc_version=2.1.86.d9e; cc_entrypoint=sdk-cli; cch=00000;
2.1.258: x-anthropic-billing-header: cc_version=2.1.258.8ee; cc_entrypoint=sdk-cli;
```

2.1.258 渲染函数 `r4t(fp, agentContext, prevReqId, promptId, opts)`，字段顺序固定：

| 字段 | 条件（2.1.258） | 说明 |
|---|---|---|
| `cc_version=<VERSION>.<fp>` | 总是 | fp 见 3.3.2 |
| `cc_entrypoint=<ep>` | 总是 | `CLAUDE_CODE_ENTRYPOINT ?? "unknown"`；交互式 = `cli`，`-p`/SDK = `sdk-cli`，CCR = `remote` |
| `cch=<5 hex>` | `provider=="firstParty" && baseURL 是 api.anthropic.com`（或 `_CLAUDE_CODE_ASSUME_FIRST_PARTY_BASE_URL`），或 vertex | JS 层写的是占位符 `00000`，**原生层在发送前改写为请求体哈希**（§B3.3.4）；2.1.86 无条件发 |
| `cc_workload=<w>` | 有 AsyncLocalStorage workload（如 `cron`） | 不受 base URL 影响 |
| `cc_is_subagent=true` | agentContext 非主会话 | 不受 base URL 影响 |
| `cc_prev_req=<req_…>` | 直连 && 上一响应的 `request-id` 匹配 `^req_[A-Za-z0-9_-]{1,36}$` | 2.1.86 没有 |
| `cc_prompt_id=<uuid>` | 直连 && 当前 user turn 的 UUID | 2.1.86 没有 |

`CLAUDE_CODE_ATTRIBUTION_HEADER=0` 可整体关闭（非直连时）。

系统块布局（抓包）：`system[0]` = billing header（无 `cache_control`），`system[1]` = 身份 preamble，
`system[2]` = 主 prompt；后两者带 `cache_control:{type:"ephemeral"[,ttl:"1h"]}`（OAuth 订阅为 1h）。

#### B3.3.2 fingerprint

算法两版相同：`sha256("59cf53e54c78" + text[4] + text[7] + text[20] + VERSION).hex[:3]`（越界位用 `"0"`）。
**输入文本变了**：取"第一条非 meta 的 user 消息"的第一个 text block。

- 2.1.86 抓包：`cc_version=2.1.86.d9e`，只有用 `<system-reminder>\nThe following skills…`（首条 user 消息的第一个块）才能复现 → reminder 当时是同一条消息的一部分。
- 2.1.258 抓包：`cc_version=2.1.258.8ee`，只有用 `say hi` 才能复现；同一 wire 消息里排在前面的 4 个 `<system-reminder>` 块都对不上 → reminder 在 2.1.258 内部是独立的 meta 消息，发送时才折叠进同一条 user 消息。

因此原生路径用 `ops.extractFirstUserPromptText`：跳过以 `<system-reminder>` 开头的 text block，全是 reminder 的 user 消息整体跳过，
首条 user 消息没有任何 text（纯图片）时返回 `""`（与官方一致）。Legacy 的 `extractFirstUserMessageText` 不变。

取字符的方式也要对齐：官方是 JS 的 `text[i]`，按 **UTF-16 码元**下标，哈希输入按 UTF-8 编码（孤立的代理项按 Node 的做法编成
U+FFFD）。原生路径用 `computeFingerprintJS` 复刻；Legacy 的 `computeFingerprint` 按字节取，只在 ASCII 上两者一致。
测试向量直接在 Node 里跑 CLI 原函数得到（`TestComputeFingerprintJS_MatchesCLI`）。

#### B3.3.3 我们的实现（`ops.BuildClaudeCodeBillingHeader`）

- 始终输出 `cc_version=<pinned>.<fp>; cc_entrypoint=cli; cch=00000;`——这里的 `00000` 是**占位符**，与官方 JS 层一致；真正的值由 client 层中间件在最终字节上计算并原地替换（§B3.3.4）。旧实现的随机 5 hex 与任何官方版本都对不上。
- 入站若已有 billing header，**原地重建**（保持 `system[0]` 位置），并按官方顺序保留通过校验的
  `cc_workload / cc_is_subagent / cc_prev_req / cc_prompt_id / cc_turn_origin`；校验正则与官方一致，其余键一律丢弃。
- 不合成 `cc_prev_req` / `cc_prompt_id`（见 §B5）。

#### B3.3.4 `cch`：原生层的请求体哈希

JS bundle 里 `cch` 只有一处字面量 `" cch=00000;"`，这只是 JS 层的占位符：原生（Bun/Zig）层在发送前把它改写成请求体哈希。
只看 bundle、或者用 node 跑 `cli.js`、或者走代理（该字段被门控省略）都看不到真值。**判断任何"占位符"都必须让原生二进制
真的把字段发出来再看**：加 `_CLAUDE_CODE_ASSUME_FIRST_PARTY_BASE_URL=1` 强制直连后，发出的是 `cch=4f05e` / `01571` / `d4767`，随 body 变化。

算法（三组抓包逐字节复现，含中文/emoji 与 API-key 路径；外部报告与此一致）：

```text
preimage = 最终 wire JSON（cch 仍为占位 00000），仅做两处修改：
           顶层 "model" 值置为 ""；删除顶层 "max_tokens" 成员
cch      = xxHash64(preimage, seed = 0x4D659218E32A3268) & 0xFFFFF   → 5 位小写 hex
```

- seed 是二进制里唯一一处该 64 位常量（LE，偏移 `0x30341cc`）；据外部报告 2.1.138 起未变，2.1.37–2.1.137 为 `0x6E52736AC806831E`，2.1.172 起 preimage 才加入 model/max_tokens 修改。
- **不是标准 xxHash64**：Bun/Zig 的实现 `PRIME64_4 = 0x85EBCA77C2B2AE63`（标准为 `0x85EBCA6B3B7B36EF`）。二进制里 Zig 常量出现 13 次、标准常量 0 次；用标准库算出的值全部不匹配。
- 与 fingerprint 无关的另一处证据：JS 里 `Bun.hash()` 只用于诊断，不参与 cch。

tingly 的实现（`internal/client/claude_cch.go`）复刻官方分层：ops 照常写占位符，`ClaudeClient` 在 SDK 中间件（发送前最后一站）里：

1. 把 Go 编码器特有的转义还原成 JS `JSON.stringify` 形态（`\u003c \u003e \u0026 \u2028 \u2029` → 原字符）。
   这一步本身也是修复：此前所有 `<system-reminder>` 都以 `\u003csystem-reminder\u003e` 上行，JS 客户端绝不会这样。
2. 扫描顶层成员做 model/max_tokens 修改得到 preimage（不建 parse tree，仅字节扫描）。
3. 计算 Zig 变体 xxHash64，把占位符原地替换（等长，Content-Length 不变）。只在顶层 `system` 的 billing header 块里
   定位占位符：SDK 把 `messages` 序列化在 `system` 前面，对话内容里恰好出现的 `cch=00000;` 不能被改写。

因为哈希的是我们自己即将发送的字节，SDK 的 key 顺序与 JS 不同不影响正确性；服务端若按收到的字节重算，结果一致。
验证：`TestRewriteClaudeCodeCCH_LiveCaptures`（设 `TINGLY_CC_CAPTURE_DIR` 指向抓包目录时对真实 body 逐字节回放）、
`TestXXHash64Zig_Vectors`（Python 参考实现向量，其标准模式复现 `xxh64("")=ef46db3751d8e999`）。

### B3.4 `metadata.user_id`

2.1.258 `XH({agentContext})`：

```js
{ ...CLAUDE_CODE_EXTRA_METADATA,             // 可选，超长时被裁掉
  device_id: <64 hex 设备指纹>,
  account_uuid: <claude.ai 账号 uuid 或 "">,
  session_id: <uuid>,
  ...parent ? {parent_session_id: parent} : {},   // 子 agent
  ...tk ? {tk} : {} }                              // 仅 CLAUDE_CODE_REMOTE
→ metadata.user_id = JSON.stringify(obj)
```

抓包：`{"device_id":"676852d4…","account_uuid":"","session_id":"bf97f200-…"}`（无账号时 `account_uuid` 为空串，键仍在）。
与旧实现的 `MetadataUserID{device_id,account_uuid,session_id}` 一致；新增 `parent_session_id`（`omitempty`）透传。`tk` 不建模。

### B3.5 System prompt preamble

三个身份句不变（`nke` 集合）：

```
You are Claude Code, Anthropic's official CLI for Claude.
You are Claude Code, Anthropic's official CLI for Claude, running within the Claude Agent SDK.
You are a Claude agent, built on Anthropic's Claude Agent SDK.
```

`client.ClaudeCodeSystemHeader` / `smartrouting.claudeCodeMainPreamble` / `clean_header` 的 preamble 列表都还匹配。
`-p` 模式发的是第三句（这也是抓包里看到 Agent SDK 句子的原因），交互式发第一句。

### B3.6 地理位置隐写

`transform_clean_header.go` 描述的 `Today’s`（U+2019/U+02BC/U+2018）与 `2026/09/02` 标记：
对 2.1.86 `cli.js` 和 2.1.258 全部 32 MB bundle（以及整个 215 MB ELF）搜 `Asia/Shanghai` / `Urumqi` 均为 0 命中；
`TZ=Asia/Shanghai` 抓包中 `Today's date is 2026-09-02.` 为普通 ASCII 撇号与连字符。
结论：该代码在这两个版本里不存在（可能更早被移除或本就来自其他构建）。normalizer 零成本，保留。

### B3.7 其他观察（未改代码，供后续参考）

- `thinking` 现在带 `display`（`-p` 为 `omitted`，交互式可 `summarized`，`updates` 对应 `thinking-display-updates` beta）；Go SDK 已有该字段，透传无损。
- `context_management.edits=[{type:"clear_thinking_20251015",keep:"all"}]` 两版都发；`ClaudeClient.GuardBeta` 在强制关 thinking 时剥掉它的逻辑仍然必要。
- `output_config.effort` 默认值：2.1.86 `medium`，2.1.258 `high`。
- 内置工具集变化很大（2.1.258 多了 `TaskCreate/TaskList/…`、`Workflow`、`ScheduleWakeup`、`SendMessage`、`ListAgents`、`ReportFindings`、`RemoteTrigger` 等）。
  `oauthToolRenameMap` 只负责把第三方客户端的小写名映射回 TitleCase，未受影响。
- CLAUDE_CODE_EXTRA_BODY / `anthropic_beta` body 字段仅 Bedrock 路径使用。

---

## B4. 代码映射与验证

### B4.1 代码映射

| wire 元素 | 生成 / 处理位置 | 关键符号 |
|---|---|---|
| 版本与开关 | `internal/typ/type.go`、`flag_registry.go`；`protocolserver/rule_flags.go`（scenario 继承、只作用于 Claude OAuth、默认值解析、`ClaudeCodeVersionTransform` 挂载） | `typ.ClaudeCodeVersion2_1_280`、`ClaudeCodeVersionLatest`、`ResolveClaudeCodeVersion`、`ClaudeCodeVersionEnabled` |
| UA / stainless / 固定头 | Legacy：`internal/client/claude_round_tripper.go` + `claude_client.go::applyClaudeCodeHeaders`（未改）；原生覆盖层：`claude_version.go::applyNativeClaudeCodeHeaders` | `nativeClaudeCLIUserAgent`、`nativeStainless*`、`stainlessOSName/ArchName` |
| `anthropic-beta` | `internal/client/claude_betas.go` | `composeClaudeCodeBetas`、`claudeCodeBetaEmissionOrder`、`claudeCodeClientReplayableBetas`、`v1/betaClaudeBetaSignals` |
| 逐请求头与 cch 中间件 | `claude_version.go::nativeRequestOptions`（Guard / GuardBeta 在 `c.native` 时追加） | `sanitizeClaudeHeaderValue`、`claudeHintHeaderValueRe`、`claudeCodeCCHMiddleware` |
| `cch` | `internal/client/claude_cch.go` | `xxhash64Zig`、`canonicalizeJSONEscapes`、`claudeCodeCCHPreimage`、`claudeCodeCCHIndex` |
| count_tokens | `protocolserver/anthropic_count_tokens.go`（解析 flag，只在原生时传带 flag 的 context）；`claude_version.go::nativeCountTokensClient` | `filterClaudeCodeCountTokensBetas` |
| 入站 hint 采集 | `protocolserver/rule_flags.go::applyClaudeCodeClientHints` | `typ.ClaudeCodeClientHints` |
| billing header 与 metadata | `internal/protocol/ops/claude_code_billing_header.go`；`request_anthropic_model.go::ApplyAnthropic{V1,Beta}MetadataTransform` 开头分派（Legacy 原样）；调用方 `transform/vendor.go::isClaudeCodeBackend` | `applyNativeClaudeCodeIdentity{V1,Beta}`、`BuildClaudeCodeBillingHeader`、`billingHeaderPreservedFields`、`computeFingerprintJS`、`extractFirstUserPromptText`、`buildNativeMetadataUserID` |
| probe | `internal/probe/e2e_probe.go`（`targetIsClaudeCode` → preamble）；版本默认值的解析在 `rule_flags.go` | 见 `.design/probe.md` |
| clean header | `protocolserver/transform/transform_clean_header.go`（未改） | — |

`typ.DefaultUserAgents` 里的 2.1.86 字面量只是 `custom_user_agent` 的快选建议，与本 flag 无关，未改。

### B4.2 验证

- **单元与 wire 测试**：`internal/client/claude_betas_test.go`（beta 抓包原文、httptest 断言发出的头）、`claude_cch_test.go`
  （Python 参考实现向量、占位符定位）、`internal/protocol/ops/claude_code_billing_header_test.go`（fingerprint `31f`、
  Node 算出的非 ASCII 向量、Legacy 回归）、`protocolserver/rule_flags_test.go`（provider 范围、默认解析为最新）、
  `claude_code_hints_test.go`、`transform/vendor_claude_relay_test.go`。
  Legacy 护栏：`TestClaudeClient_LegacyProfileUnchanged`、`TestApplyAnthropicBetaMetadataTransform_LegacyUnchanged`。
- **网关端到端**：`internal/protocoltest/flags.go` 的 `claude_code_version` case（`go test ./internal/protocoltest/ -run
  TestRuleFlags/claude_code_version`，或 `harness matrix --mode=flags`）让同一请求分别以 Legacy、2.1.280 与未设置（Default）走完真实网关，
  逐项断言上游收到的 UA、beta、子 agent 头、`x-app`、billing header、metadata，以及 count_tokens。升版后先跑它。
- **抓包回放**：设 `TINGLY_CC_CAPTURE_DIR=<抓包目录>` 后，`TestRewriteClaudeCodeCCH_LiveCaptures` 对真实 body 逐字节重算 `cch`。
- **真实上游**：唯一能证明服务端接受 `cch` 和指纹的方式。harness 的 real-provider 模式支持 Claude Code OAuth entry
  （`providers.yaml` 用 `oauth_token` 代替 `apikey`），签成 `ClaudeCodeVersionLatest`，并像登录一样先用 token 取真实账号 uuid：

  ```bash
  export CLAUDE_CODE_OAUTH_TOKEN=$(claude setup-token)   # 或复制 tingly-box 里 Claude Code OAuth provider 的 access_token
  go run ./cli/harness init-config --output providers.yaml
  go run ./cli/harness replay claude --upstream real --config providers.yaml   # 三个 fixture，200 即通过
  go run ./cli/harness agent  claude --config providers.yaml                   # 由真实 claude CLI 发请求
  ```

  `TestSetupRealOAuthAgent_ClaudeCode` 是它的封闭版：虚拟上游充当 Anthropic，断言 Bearer 认证、UA、request-class、
  billing header 与 `cch`、preamble、account uuid。live 跑绿时被接受的就是这一形态。

---

## B5. 决策与取舍

1. **persona 固定为"交互式终端、直连 api.anthropic.com"**。UA `(external, cli)`、`cc_entrypoint=cli`、基线含
   `redact-thinking`、默认 request-class `main` 都按这个 persona 取值，不跟随入站客户端的 entrypoint（入站可能是 `-p`、
   Agent SDK、CCR，甚至不是 Claude Code）。一个自洽的 persona 优于把入站的碎片拼起来。
2. **`cch` 按官方算法计算**（§B3.3.4）。随机值不匹配任何官方版本；哈希的是我们自己即将发送的字节，所以 Go 与 JS 的 key
   顺序不同不影响正确性。
3. **不合成 `cc_prev_req` / `cc_prompt_id` / `cc_turn_origin`**，只透传入站已有的。合成需要跨请求状态，且服务端语义
   （缓存亲和、限流）未知，猜错的代价大于缺省（§B7）。
4. **beta 合成而非透传**：入站 header 不可信，但真实 Claude Code 协商的请求级 flag 只有它知道，所以是"基线合成 + body 派生 +
   白名单回放"。`structured-outputs` 官方受 GrowthBook 灰度，我们按 body 是否带 `format` 决定。
5. **入站 UA 不透传**（延续 `.design/user-agent.md` 的"特种链"结论）：pinned UA 是决定性的。
6. **stainless OS/Arch 按 SDK 映射、移除 `x-stainless-helper-method`**：两者都是真实客户端从不这样发的旧偏差。
7. **fingerprint 按我们声称的版本重算**：客户端自己算的值从不上行，所以老客户端经过网关也会得到与 2.1.280 一致的 fp。
8. **只保留 Legacy + 最新版本**：旧版本一旦被拒就没有用处，保留只会带来版本间门控的复杂度。

---

## B6. 升版 checklist

1. 看 registry 的 `dist-tags.latest`，下载主包和 `claude-code-linux-x64`（§B2.1），`./claude --version` 确认。
2. 用 §B2.2 的脚本切出 bundle，按 §B2.3 的锚点逐项核对：
   - SDK 版本常量 → `nativeStainlessPackageVersion`；抓包看 `X-Stainless-Runtime-Version` → `nativeStainlessRuntimeVersion`；
   - beta 注册表：新增或删除的 flag → `claude_betas.go` 常量、`claudeCodeBetaEmissionOrder`、回放白名单；
   - allModelBetas 与主循环的 `.push(` 顺序 → `composeClaudeCodeBetas`；
   - billing header 渲染函数的字段与门控 → `BuildClaudeCodeBillingHeader`、`billingHeaderPreservedFields`；
   - salt `59cf53e54c78` 与 `[4,7,20]` → `computeFingerprintJS`；
   - 二进制里的 seed `0x4D659218E32A3268`（LE）；强制直连抓 ≥3 个 body，跑
     `TINGLY_CC_CAPTURE_DIR=<dir> go test ./internal/client/ -run LiveCaptures`；
   - `account_uuid:` / `session_id:` 对象字面量 → `nativeMetadataUserID`；
   - `"You are Claude Code` 三句 → preamble 常量；`"x-app":` 处的默认头 → 是否有新头要回放；
   - 强制直连时 body 的顶层 key 与 tools 名单 → body 派生的 beta 信号。
3. 按 §B2.4 抓包（API key / OAuth × 代理 / 强制直连，`env -i`），把 `anthropic-beta` 原文和 `cc_version` 写进测试
   （`TestComposeClaudeCodeBetas_*Capture`、`TestComputeCCVersionFor_MatchesLiveCapture`；`-p` 比交互式少一个 `redact-thinking`）。
4. 原地升级：把 `ClaudeCodeVersion2_1_280` 常量与 registry 选项改成新版本（`ClaudeCodeVersionLatest` 随之指向它），
   不做版本间门控。已存储旧版本号的规则会被判为未知、按 Default 解析为新的最新版本，无需迁移。
5. 跑 `go test ./internal/client/ ./internal/protocol/... ./internal/protocolserver/ ./internal/protocoltest/`，`task codegen`
   刷新 openapi 和前端类型；用 §B4.2 的真实上游命令验证一次。
6. 更新本部分的 §B0 与 §B8（新版本相对上一版的差异表）。

---

## B7. 未做与风险

| 项 | 官方行为 | 我们的行为 | 原因 / 补法 |
|---|---|---|---|
| `cc_prev_req` / `cc_prompt_id` 合成 | 直连时带 `cc_prompt_id`（当前人类 prompt 的 UUID），第二个请求起带 `cc_prev_req`（上一响应的 `request-id`） | 入站没有就省略，即官方"经代理"的形态 | `cc_prev_req` 需按 session 记上一响应的 `request-id`；`cc_prompt_id` 可由最后一条人类消息确定性派生。语义未验证，等观察到收益再做 |
| `cc_turn_origin` 合成 | 直连专属：交互式为 `human`，`-p` 为 `sdk` | 只透传 | 与 `cc_prompt_id` 同层，将来一并合成 |
| `structured-outputs` 灰度分支 | `tengu_tool_pear` 开启且模型支持时进基线 | 仅 body 带 `format` 时加，另接受入站回放 | 灰度值按用户下发，代理侧看不到 |
| 灰度 / env 门控的 beta（`timing`、`inline-tools`、`mid-conversation-tool-changes`、`thinking-resumption`、`message-threads`、`dangerous-tool-use`、`mid-conversation-system-clear-at`） | 由 env、GrowthBook 或服务端分类器决定 | 只回放入站已带的 | 代理侧看不到门控条件 |
| `x-claude-code-agent-type` 默认值 | 仅子 agent 请求发 agent 名 | 只回放 | agent 名只有客户端知道 |

风险：

- **`cch` 的服务端校验方式未验证**。我们哈希自己发出的字节并已做 JS 形态归一化；若服务端重算前还做 key 重排等归一化，
  Go 与 JS 的 key 顺序差异会导致不匹配。上线前用 §B4.2 的真实上游命令跑一次；OAuth 流量出现异常时优先怀疑这里。
- **thinking 默认值**：`Guard` 在 thinking 未指定时强制 `disabled`（adaptive-only 模型除外）；交互式客户端会显式给
  `adaptive` + `display`，通常不会触发。
- 抓包用的假服务与脚本没有入库（§B2.4 已内联，足以复现）。

---

## B8. 2.1.258 → 2.1.280 的差异

触发：`Claude Code 2.1.258 does not support this model; version 2.1.280 or newer is required`（2026-09-22）。按 §B2 重跑：
下载 `claude-code-linux-x64-2.1.280`，切出约 38 MB 的 bundle，`-p` 抓包 4 组（OAuth / API key × 代理 / 强制直连）。

| 项目 | 2.1.258 | 2.1.280 | 处理 |
|---|---|---|---|
| `User-Agent` | `claude-cli/2.1.258 (…)` | `claude-cli/2.1.280 (…)` | `nativeClaudeCLIUserAgent(version)` |
| SDK / runtime / OS-Arch / helper-method / `x-app` | — | **全部不变**（SDK `0.112.1`、Bun `v26.3.0`） | 无 |
| beta 注册表 | 34 项 | 42 项：新增 `thinking-resumption-2026-07-17`、`timing-2026-09-09`、`inline-tools-2026-09-15`、`dangerous-tool-use-2026-09-03`、`thinking-binding-controls-2026-08-01`、`message-threads-2026-08-12`、`mid-conversation-system-clear-at-2026-08-21`、`advisor-tool`… | 新 flag 进入回放白名单 |
| beta 基线（`Cw` 表） | 同 | **同**：haiku 无 `claude-code`、claude-3 无 thinking 类、`mid-conversation-system` 规则不变（新增 opus-4-8 例外） | 无 |
| 代理模式抓包的 beta 串 | `claude-code,oauth,interleaved,thinking-token-count,context-management,prompt-caching-scope,effort,extended-cache-ttl` | **逐字符相同** | — |
| 直连模式抓包的 beta 串 | （258 未抓） | `…prompt-caching-scope,advanced-tool-use,effort,thinking-binding-controls,extended-cache-ttl,cache-diagnosis` | 三项都是**身体派生**：tools 含 `ToolSearch`/`defer_loading` → `advanced-tool-use`；thinking adaptive/enabled → `thinking-binding-controls`；body 含 `diagnostics` → `cache-diagnosis`。顺序：tool-search 在 effort 之前 |
| billing header | `…cc_prev_req; cc_prompt_id` | 末尾新增 **`cc_turn_origin=<[a-z][a-z_]{0,31}>`**（直连专属；值来自 `t3r`：`human` / `sdk` / `scheduled` / `task_notification` / `peer` / `auto_continuation` / `host_synthetic` / `system` / `unknown`，或 host 指定） | 透传入站、正则校验；不合成（与 `cc_prompt_id` 同理） |
| fingerprint | 同 | 同（`say hi` → `2.1.280.31f`） | 版本参数化 |
| `cch` | xxHash64(seed `0x4D659218E32A3268`) | **同**：seed 唯一一处不变、Zig `PRIME64_4` 17 处、三组直连抓包逐字节复现 | 无 |
| `metadata.user_id` | 同 | 同（`ti`/`tk` 仍是 remote 专属） | 无 |
| 默认头 | — | 新增 **`x-claude-code-request-class`**（`Js(querySource)`：`main` / `subagent` / `auxiliary`，或 `compaction` / `workflow`）与 **`x-claude-code-agent-type`**（仅 `agent:*` 来源：内置 agent 名 / `custom` / `teammate`）。门控 `S5t()`：env `CLAUDE_CODE_GATEWAY_HINT_HEADERS`，否则**直连即发**（`Ba()`），代理走灰度 `tengu_splendid_sutton`（默认 false） | 直连 persona：默认 `x-claude-code-request-class: main`；入站带这两个头时校验后回放（`^[a-z][a-z0-9_-]{0,63}$`） |
| `x-app` | `cli` / `cli-bg` | 同 | 入站 `cli-bg` 回放 |
| preamble 三句 | 同 | 同 | 无 |
| `-p` 请求体 | — | 新增顶层 `diagnostics:{previous_message_id:null}`（直连时） | 透传，作为 `cache-diagnosis` 的信号 |
