# Claude Code: from one-shot processes to a persistent stream session

> Status: research + proposal (not yet implemented). Written 2026-09-17;
> §3.1's core transport assumption empirically confirmed the same day (§3.1,
> §6 P0).
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

### 5.3 Isolation moves from "process per call" to "session registry"

A small registry (`agentboot.SessionPool` or similar), keyed by the same
`(chatID, agent, project)` tuple `session.Manager.FindBy` already uses:

- One `PersistentSession` per key, at most.
- `Send` on a key with an in-flight turn blocks/queues at the registry
  layer (single-flight per key) rather than relying on the CLI's own
  "session file already in use" error — that error class should become
  unreachable in persistent mode, since only the registry ever touches this
  process's stdin.
- Idle timeout (config, default TBD — start conservative, e.g. 5–10 min of
  no `Send`) auto-`Close()`s and evicts.
- Hard cap on resident persistent processes (config). Over the cap, evict
  the least-recently-active session (`Close()` it) before opening a new one.
  Evicted/idle-timed-out sessions fall back to the existing one-shot
  `--resume` path transparently on the next message — the user sees no
  difference beyond slightly higher latency on that one message.
- `session.Manager`'s existing `ExpiresAt.IsZero()` ("persistent: caller
  drives the lifecycle") seam is the natural place to mark a logical session
  as backed by a live `PersistentSession` versus the default expiring
  one-shot bookkeeping.

### 5.4 Wiring into `@cc`

`remote/control/remoteagent/executor_claude.go`'s `ClaudeCodeExecutor.Execute`
currently calls `AgentService.Run` (spawn, drive to completion, return) for
every message. The persistent path adds an alternative branch: look up (or
open) the registry entry for `(chatID, agent, project)`, `Send` the prompt,
and drive the *existing* `RunWithPrompter`-style event loop against
`PersistentSession.Events()` filtered up to the next `TurnCompleteEvent`
instead of the whole handle closing. `sink`/`prompter` wiring (lines
148–187 today) needs no change in shape — only the source of the event
channel and where "this turn is over" is detected.

This should be **opt-in** (a bot/profile setting, not a global default) for
at least the first shipped iteration — see §6 phasing and
`.design/ux-principles.md` §5/§6 (smart defaults over toggles; but a change
with real resource/crash-blast-radius implications like "how many `claude`
processes stay resident" is exactly the kind of thing that deserves an
explicit, discoverable setting rather than a silent behavior change, at
least until the idle/eviction/crash-recovery paths have real production
mileage).

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
2. **P1 — core primitive.** `PersistentSession`, `Runner.Open`, the two new
   `StreamEvent` types, unit tests against a fake `process.Factory` (the
   existing test seam) proving: two `Send` calls on one process, idle
   timeout firing, `Close()` reusing `shutdownGracefully`, and a crash
   mid-turn producing a `SessionStateEvent{Terminated}` rather than a hang.
3. **P2 — registry + wiring.** `SessionPool`, eviction policy, wire into
   `ClaudeCodeExecutor` behind an opt-in setting.
4. **P3 — observability.** Surface resident-process count / per-session
   idle time somewhere an operator can see it (metrics or a debug endpoint)
   before defaulting anyone into it.

## 7. Open questions / risks

- **The root/`--dangerously-skip-permissions` gap found during P0** applies
  to *today's* one-shot path too, not just the persistent design — worth its
  own small fix independent of this proposal (see P0 note in §6).
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
