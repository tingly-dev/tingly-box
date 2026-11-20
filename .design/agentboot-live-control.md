# AgentBoot: injecting messages and changing a session live (TEP #2114)

Two questions about a persistent Claude Code session, answered by running the
real CLI (`agentboot/claude/ref/live-control.md`):

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
| Into the running turn | the default path itself (observed, `.research/mid-turn-injection.md`) | explicit per-message **Steer** (`turn/steer` with `expectedTurnId`), at the next step boundary |
| Edit before it goes | up-arrow edits queued messages | queued chip, can be promoted or removed |

Neither can cut a running tool call short: "now" always means "at the next
step". Only Stop does that. So there are two real intents, not three:
*change course in this turn* (insert) and *do this afterwards* (next turn).

Decided (YZ, 2026-10-19): the default is Claude Code's behaviour; the user can
choose the other at send time. Revised (2026-11-20, "支持运行中消息即时插入"):
the two modes are named for what they do, **insert** (default) and
**next_turn**; "stop and send" is dropped as a send mode (interrupting is the
Stop button, which also hands held messages back to the box). Insertion was
re-measured with the real CLI against a request-logging mock
(`.research/mid-turn-injection.md`): the message is in the model's next
request after the write, in the same turn; if no request follows, the CLI
runs it as the next turn. Waiting on a pending approval or a foreground
subagent delays it until that finishes.

### As built

- `POST /threads/{id}/messages` takes `mode` (`insert` default, `next_turn`);
  anything else, including the old `send` / `queue` / `stop_and_send`, is
  400. `DELETE /threads/{id}/queue/{qid}` takes a held message back.
- **Capability, declared by the agent.** A transport that reads input
  mid-turn implements `agentboot.MidTurnInputEncoder` (Claude does).
  Without it `PersistentSession.Inject` returns `ErrInjectUnsupported`,
  which `Engine.Inject` reports as `ErrNotInjectable`; `Engine.Injectable(id)`
  says up front whether a running turn would take one. Threads carry it as
  `insertable` (read live, never stored, like `running_tasks`).
- **Fallback is honest.** An insert that cannot join (agent without the
  capability, turn still waiting for a project slot, one-shot turn, failed
  write) is held like next_turn. When the turn runs but the agent reads
  nothing mid-turn, a transcript note says so. The composer reads
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
  (`coordinatorSessions.insert`, wrapped in the same trust envelope the queue
  uses, event id allocated first so the model can cite it) and records it as
  `injected`; no durable input is created, because the turn answers it. It
  falls back to the ordinary input queue when no turn has reached the model,
  for a compaction, a slash command, a paused/archived project, or an agent
  that cannot take it. If the turn ends first, Claude Code answers in a turn
  of its own with no run registered: `answerOutsideRun` keeps that reply as
  an assistant event (and hands it to `OnFinal`) instead of dropping it.
  Trade-off: an inserted message is not a durable coordinator input, so a
  crash mid-turn leaves it with the turn's own recovery card, not its own.
- Not changed (needs a product decision, see the PR): messages the
  *coordinator* sends to a thread (`message_thread`), a thread's relays,
  IM chat and bound-chat messages still go through `Threads.Send` and run
  after the current turn.
- Composer: Enter = insert (button tooltip says so while a turn runs); a
  menu on the send button and Alt+Enter give "send after this turn"; `mode`
  is only passed when chosen (or forced by `insertable=false`).

## Not done

- Directories chosen outside a permission prompt still restart.
- `set_max_thinking_tokens`, MCP control requests: the CLI has them, nothing
  in AgentBoot needs them yet.
