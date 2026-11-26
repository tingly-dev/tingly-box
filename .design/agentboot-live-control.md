# AgentBoot: injecting messages and changing a session live (TEP #2114)

Two questions about a persistent Claude Code session, answered by running the
real CLI (`agentboot/claude/ref/live-control.md`, `.research/mid-turn-injection.md`):

1. Can a message join a turn that is already running? **Yes.**
2. Can model / mode / directories change without restarting? **Model and
   permission mode yes; directories only together with an approval.**

## 1. Message injection

`PersistentSession.Inject(ctx, prompt)` writes a user message while a turn is
Running. The CLI queues it and takes it up after the current tool call, in the
same turn: one `TurnCompleteEvent`, session state untouched.

- Idle → `ErrNoTurnInFlight` (it is a new turn: use `Send`). Closed →
  `ErrSessionClosed`. `Send` during a turn still returns `ErrTurnInFlight`;
  injecting is a different, explicit act, not a relaxation of turn-taking.
- A turn ending between the state check and the write leaves the message to
  start a turn of the agent's own; the pump already tracks those
  (`TurnStartDetector`), so nothing is lost or stuck.
- Returning means "written", not "read". `--replay-user-messages` would give a
  "taken up" marker but also echoes every message into the transcript, so it
  is left off; a UI should show an injected message as sent and let the
  model's next step make it visible.
- `Engine.Inject(id, prompt)` injects into the live turn and records the
  user message; `ErrNotInjectable` tells the host to `Start` a turn instead.
  Hosts that today refuse a message while busy can call it first.
- **When the model reads it** (measured against a request-logging mock,
  CLI 2.1.259, `.research/mid-turn-injection.md`): in the first model
  request that starts after the write, in the same turn. That is after the
  running tool call, after a pending approval is answered, after a
  foreground subagent returns; a request already streaming cannot change.
  Two messages written close together arrive as one.
- **Capability, declared by the agent.** A transport that reads input
  mid-turn implements `MidTurnInputEncoder` (Claude does). Without it
  `Inject` returns `ErrInjectUnsupported`, which `Engine.Inject` reports as
  `ErrAgentReadsBetweenTurns` (an `ErrNotInjectable`), so a host can say why
  the message waits. `Engine.Injectable(id)` says up front whether a
  message sent now would join the running turn.

## 2. Live control

`PersistentSession.SetModel` / `SetPermissionMode` send `set_model` /
`set_permission_mode` and **wait for the `control_response`**
(`ControlTimeout`, 10s). Mechanism: the transport implements
`ControlResponseDecoder`; the pump hands each response to the caller waiting
under its `request_id` and drops the ones nobody waits for (`Interrupt`,
`StopTask` stay fire-and-forget). A refusal is `*ControlRejectedError`; an
agent without the capability gives `ErrControlUnsupported`.

`ApprovalResponse.AddDirectories` answers a permission prompt with an
`addDirectories` permission update, scoped to the session. This is the only
live way to widen accessible directories; elsewhere it is `--add-dir` and a
restart.

### Engine: what restarts and what does not

`launchSignature` now covers only what a live process cannot change: settings
file content and environment (gateway, key, profile). Model and permission
mode moved out of it into `launchState`; on each turn `applyLive` sends the
differences to the resident process. Any failure (refused, timeout, no
capability, or "back to the launch default mode", which has no name to send)
falls back to the old path: evict, then reopen with `--resume`, which picks
the settings up at launch. A failed live change therefore costs a restart,
never a stale setting.

### A permission mode change while a turn runs

`applyLive` only runs when a turn starts, so a mode changed while a thread
works waits for the next message — up to hours on a long turn.
`Engine.SetPermissionMode(id, mode)` (`agentboot/engine/engine.go`) pushes it
to the resident process now, no restart, and remembers it only if the process
took it (a refusal leaves the old mode in `launchState`, so the next turn's
`applyLive` tries again; `auto` on a model without it lands in `acceptEdits`
with the same note as the turn path). Projects calls it from
`Threads.SetPermissionMode`, and notes a refusal in the transcript. The
bypassPermissions auto-approval wrapper reads the mode at each request
instead of at turn start, so tightening stops the engine approving on the
user's behalf at once. What a live change cannot do: stop a command or
background task already running (modes gate tool calls, not processes); take
back an allow rule the process started with (see "Settings" below, which
applies only from the next start); and widen to `bypassPermissions`, which the
CLI refuses unless the process was started with
`--dangerously-skip-permissions` — that one falls back to the next turn's
restart, which launches with the mode. The analysis, the measurements and the
remaining gaps are in `.research/agentboot-stop-and-permissions.md` §2.

### A mode Claude Code cannot run (auto)

Claude Code runs `--permission-mode auto` as **default** mode, silently, when
the model has no auto mode (checked with CLI 2.1.259: `claude-sonnet-4-5`,
`claude-haiku-4-5`, `claude-opus-4-5` and older; the aliases `sonnet` / `opus` /
`haiku`, `claude-sonnet-4-6`, `claude-opus-4-6` and gateway names such as
`tingly/cc` have it). The only sign is `permissionMode` in the turn's
`system/init`. A live `set_permission_mode auto` is refused with
"auto mode unavailable for this model". Default mode asks for every edit, so a
thread set to auto looked as if its mode had no effect, and the refused switch
cost a restart that came back in default mode again.

The engine therefore treats this as a fallback, not a failure
(`modeFallback`, `checkInitMode`, `fallBack`): an init that reports `default`
for a launch in auto, or a refusal that says "unavailable", moves the process
to `acceptEdits` live, adds a system note to the transcript, and remembers
mode and model in `launchState.refused`. Later turns do not ask again, restart,
or repeat the note; a different model asks for auto again. Only the two named
conditions are handled: any other refusal still restarts, and a mode the
process reports differently on its own (plan mode left through ExitPlanMode,
the classifier's prompt fallback after repeated blocks) is left alone.
`permissions.disableAutoMode: "disable"` in any settings layer (the user's
`~/.claude/settings.json` included) has the same effect as an unsupported
model, so the transcript note names both causes.

Why Claude Code asks is on the prompt itself: `can_use_tool` carries
`decision_reason` (the classifier's words, e.g. "3 consecutive actions were
blocked") or `decision_reason_type: "rule"` (an ask rule matched); neither
means the mode itself requires the prompt. `ApprovalRequestEvent.Reason`
carries it, and Projects stores it in the approval event's `reason` (the web
card does not show it yet).

## 3. Composer: insert and next turn

What the two tools do when a message is sent mid-turn:

| | Claude Code | Codex |
| --- | --- | --- |
| Default | queue, drained into the model request at the next step boundary (after the running tool call); leftovers run as the next turn | queue as a follow-up for the next turn |
| Into the running turn | the default path itself (§1) | explicit per-message **Steer** (`turn/steer` with `expectedTurnId`), at the next step boundary |
| Edit before it goes | up-arrow edits queued messages | queued chip, can be promoted or removed |

Neither can cut a running tool call short: "now" always means "at the next
step". Only Stop does that. So there are two real intents, not three:
*change course in this turn* (insert) and *do this afterwards* (next turn).

Decided (YZ, 2026-10-19): the default is Claude Code's behaviour; the user can
choose the other at send time. Revised (2026-11-20): the two modes are named
for what they do, **insert** (default) and **next_turn**; "stop and send" is
dropped as a send mode (interrupting is the Stop button, which also hands
held messages back to the box). Decided by the user the same day: every
sender inserts by default, not only the web composer.

### As built

- `POST /threads/{id}/messages` takes `mode` (`insert` default,
  `next_turn`). The old names `send` / `queue` are accepted as aliases (a
  page loaded before the rename); `stop_and_send` is refused (400).
  `DELETE /threads/{id}/queue/{qid}` takes a held message back.
- **Every sender inserts by default**: the web composer, IM chats
  (`Service.Send`, including bound chats) and the coordinator's
  `message_thread` all go through `SendWith` with insert; a relay keeps its
  coordinator note. `message_thread` tells the coordinator how it went
  ("sent into the thread's running turn" / "queued").
- Threads carry `insertable` (read live from `Engine.Injectable`, never
  stored, like `running_tasks`).
- **Fallback is honest.** An insert that cannot join (agent without the
  capability, turn still waiting for a project slot, one-shot turn, failed
  write) is held like next_turn. When the turn runs but the agent reads
  input only between turns, a transcript note says so. The composer reads
  `insertable === false`: Enter then sends `next_turn`, the menu shows
  Insert disabled with the reason.
- A held message is a `queued` thread event (`held`, then `sent` or
  `removed`); it becomes a `user` event only when its turn starts. Chips left
  over from a previous run are marked removed at startup (`Recover`).
- A message that joined a running turn is a `user` event with
  `injected: true`; the page shows "Claude will read this at its next step"
  until a later tool result or reply of the main conversation appears.
- **Coordinator (main thread).** Same two modes. `Coordinator.PostWith`
  with insert writes the message into the coordinator turn in flight
  (`coordinatorSessions.insert`, in the same trust envelope the queue uses,
  event id allocated first so the model can cite it) and records it as
  `injected`; no durable input is created, because the turn answers it. It
  falls back to the ordinary input queue when no turn has reached the model,
  for a compaction, a slash command, a paused/archived project, a turn that
  already ended itself with `no_update`, or an agent that cannot take it.
  - An insert makes the run a user turn (`adoptUser`): `no_update` is
    refused from then on and the reply streams, so a check-in started by a
    thread's report cannot swallow the user's message.
  - If the turn ends before reading it, Claude Code answers in a turn of its
    own with no run registered: `answerOutsideRun` keeps that reply as an
    assistant event (and hands it to `OnFinal`). The flag is cleared when
    that turn ends, when the insert fails, and when our next turn starts (a
    coordinator has no background tools, so it starts no other turn).
  - The page knows whether an insert would land: `coordinator_insertable` in
    the project detail and `insertable` in the `coordinator` event, published
    when a turn reaches the engine and when it ends.
  - Decision answers (`Choose`) still go through the input queue.
- Composer: Enter = insert (button tooltip says so while a turn runs); a
  menu on the send button and Alt+Enter give "send after this turn"; `mode`
  is only passed when chosen (or forced by `insertable=false`).

## Not done

- **Reply to an insert the turn never read** (narrow race: the turn ends
  between the write and its next request). Claude Code answers in a turn of
  its own with no run and no tool turn: its text is kept, but coordinator
  tools refuse ("No coordinator turn is active") and usage is not tallied.
  Fix: register that unsolicited turn as a run with a tool turn.
- **Inserted coordinator messages are not durable inputs.** If the turn
  fails or the app restarts, the turn's input is recovered but the inserted
  message is not replayed. Fix: record it as a coordinator input marked
  handled by the turn it joined, recovered with it.
- **Repeated "cannot take a message" notes.** For an agent that reads input
  only between turns, every insert (the default for chats and relays too)
  adds the note again. Fix: once per turn. No production agent has this today.
- Directories chosen outside a permission prompt still restart.
- `set_max_thinking_tokens`, MCP control requests: the CLI has them, nothing
  in AgentBoot needs them yet.
