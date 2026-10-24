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

## 3. Composer: steer and queue (proposal, not built)

What the two tools do when a message is sent mid-turn:

| | Claude Code | Codex |
| --- | --- | --- |
| Default | queue, drained into the model request at the next step boundary (after the running tool call); leftovers run as the next turn | queue as a follow-up for the next turn |
| Into the running turn | the default path itself (observed, `ref/live-control.md`) | explicit per-message **Steer** (`turn/steer` with `expectedTurnId`), at the next step boundary |
| Edit before it goes | up-arrow edits queued messages | queued chip, can be promoted or removed |
| Known friction | none reported | Enter/Tab swapped between versions; users had to Ctrl+C to send "now" (openai/codex#13595); request for a "steer by default" toggle (#32709) |

Neither can cut a running tool call short: "now" always means "at the next
step". Only Stop does that. So there are two real intents, not three:
*change course in this turn* (steer) and *do this afterwards* (queue).

Decision (YZ, 2026-10-19): the default is Claude Code's behaviour, and the
user can choose another at send time.

Checked on CLI 2.1.302 (`ref/live-control.md`): a message written mid-turn is
read at the next step boundary; if the turn ends first (a reply with no tool
call), the CLI runs it as the next turn by itself. So the default needs no
client-side branching: one send, and the CLI decides step or next turn.

One composer, no mode setting:

- **Send (Enter), the default.** Idle: a normal turn. Running: `Engine.Inject`.
  The message appears in the transcript at once, marked "Claude will read it
  at its next step" until the next assistant step shows it was taken up. If
  the turn ended first, the CLI's own next turn starts and the transcript
  shows it as a turn start. Irrevocable once sent, which the mark says.
- **Choose another way at send time**, only while a turn runs, from a menu on
  the send button (and a key, e.g. Alt+Enter), per message, never sticky:
  - **After this turn**: the host holds the message as an editable, removable
    chip above the composer and sends it as a normal turn when the session is
    idle. Chips survive Stop.
  - **Stop and send**: interrupt, then send, so nobody has to Ctrl+C and
    retype.
- **Fallbacks are silent and honest.** If the session cannot take an injection
  (`ErrNotInjectable`, a non-steerable turn, an agent without steering), Send
  becomes a held chip with a one-line reason.
- Host API: `POST message {mode: send|queue|stop_and_send}` (default `send`),
  `GET` / `PATCH` / `DELETE` queue items. A Codex backend maps `send` while
  running to `turn/steer` (`expectedTurnId`; on a race or on review / compact
  turns fall back to a held chip).
- Gap to close in the engine: between a turn's `result` and the CLI's own next
  turn's `init` the session reads Idle, so a host `Send` in that instant could
  start a second turn. `Inject`-then-`Send` fallback must tolerate
  `ErrTurnInFlight` (it already surfaces as `NotSent`).

## Not done

- No host (Desk, Projects) calls `Engine.Inject` yet; their HTTP routes and UI
  decide how a busy session's composer behaves.
- Directories chosen outside a permission prompt still restart.
- `set_max_thinking_tokens`, MCP control requests: the CLI has them, nothing
  in AgentBoot needs them yet.
