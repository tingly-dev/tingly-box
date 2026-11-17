# Threads talk to the coordinator through Projects tools: the wrapped channel

> Status: **proposal, not built.** Nothing here is implemented. Evidence: [../.research/cross-session-messaging.md](../.research/cross-session-messaging.md) (Claude Code 2.1.259, probed 2026-11-17; the probe script is next to it). Related: [mcp-tools.md](./mcp-tools.md) (the tool table, `ThreadOnly`), [github-tools.md](./github-tools.md) (the authorization pattern reused here), [message-channel.md](./message-channel.md) (what reaches the coordinator and the chat), [projects-harness.md](./projects-harness.md).
>
> Direction set by the user (2026-11-17): *"Native is fine, but not used directly; it should be wrapped and managed, by our other Projects MCP tools."* The model never calls native `ListAgents`/`SendMessage` across sessions; Projects tools do the sending, and Projects delivers, labels and records.
>
> **Still waiting on the user:** whether a proposed thread starts from a card (written below as the default) or by other means. Auto-start is an off-by-default option.

## The question

"Claude Code can message between instances natively. Does Projects support it? A thread telling the coordinator to open a new thread did not work."

## What exists today

A thread has **no Projects tool to reach the coordinator.** Its tools are `send_file` and the six GitHub tools; `start_thread` and `propose_threads` are `CoordinatorOnly` (`coordinator_tools.go`). Its only channel is the report: each turn end becomes `<event source="thread" kind="report" trust="relay">` in the coordinator's next turn (`Coordinator.onReport` → `AddCoordinatorInput` → `wake`).

| Route a thread has today | What happens |
|---|---|
| Say it in the report | The coordinator reads it as information. Its prompt names merge, push, delete, spend and settings as things to ask the user about; **starting a thread is not on that list**, so whether it starts one is the model's call. Nothing refuses it, nothing carries it out. This is the most likely thing the user saw. |
| Native `SendMessage` | **Allowed with no approval card** (measured). The message reaches the coordinator's CLI, which runs a turn Projects never sees; `start_thread` in it answers `No coordinator turn is active.` and nothing starts. Fails invisibly, and it can equally be sent to another project's coordinator or to the user's terminal sessions. |

## Why native is not exposed to the model

Properties measured on 2.1.259 (details in the research note):

1. **Reach is the config dir, not the project.** Peers are listed from `<CLAUDE_CONFIG_DIR>/sessions`, so every Projects session shares a registry with every other project's sessions and with the user's own terminal sessions. A thread reads web pages, repositories and issues, so it can be prompt-injected; with native it can message any of them, and the receiver is told to treat the message as a teammate's request.
2. **It bypasses the trust model.** The coordinator's inputs are `<event … trust=…>` envelopes with escaped text. A peer message arrives raw, with no trust label.
3. **It is invisible.** The turn it starts has no transcript row, no state, no chat relay, no cancel.
4. **No approval step.** Only the receiver's `crossSessionInbound` gates it, and an unattended `-p` session cannot show its dialog.

## Decision: wrap it, and carry it on Projects' own relay

The user accepts the native mechanism if it is wrapped. The wrapper is a Projects MCP tool. What is open is the **transport underneath the tool**. Two choices:

| | A. Tool writes into the peer's native socket | **B. Tool uses Projects' relay (recommended)** |
|---|---|---|
| How | The host reads the target's `.key` file from the registry and speaks the CLI's socket protocol (`auth` line, then a `user` frame). | The tool records an event and queues an input, as reports do today (`AddCoordinatorInput` + `wake`; for a thread, `Threads.Send` with `fromThread`). |
| Protocol | Internal and undocumented (`peerProtocol: 1`, frames seen only in the binary's strings). Can change in any CLI release; no compatibility promise. | Ours. |
| Credentials | The host would hold auth keys for **every** session in the registry, the user's terminal sessions included. | None. |
| Needs the receiver's inbound open | **Yes**: the coordinator would have to accept `<cross-session-message>`, which is exactly the hole to close. | No. Inbound stays `refuse`. |
| Turn is visible to Projects | No: the CLI starts it on its own, with no `user` frame (measured). The engine cannot attribute it, show it, or cancel it. | Yes: a queued input starts an ordinary turn. |
| Trust label, record, authorization | Would have to be re-added around a path that has none. | Already there: `envelope(trustRelay, …)`, `ThreadEvent`, the tool table's gates, the project's settings. |
| Gain | Reaches sessions outside Projects. | None needed inside Projects: the coordinator and every thread are Projects sessions. |

So **B**: the wrapped tool is Projects-native in transport. Native stays only where the CLI uses it itself and Projects can leave it alone: a thread continuing its own subagent, which is in-process, not the socket (measured). If the user wants their own terminal sessions to talk to a thread, that is a separate, opt-in bridge (below), the only place where A would be the transport.

## The tools

All are `ThreadOnly` entries in the tool table: identity comes from the thread's capability URL (no `project_id`, no source thread id in the input), validation and guards live in a `Threads`/`Coordinator` method, and a tool call is recorded in the thread's own transcript. Reads and writes stay separate, one verb each (project memory `mcp-tool-granularity`), no `action` argument.

### `propose_thread` — ask for a new thread (card)

Input: the same fields as one entry of `propose_threads` (`title`, `task`, `repo`, `base`, `branch`), checked by `checkProposals` before anything exists. Effect: a **suggestion card** (`EvSuggestions`) in the project conversation, attributed to the proposing thread (new `from_thread` on the suggestion), and a system note to the coordinator. The user starts it from the card (`StartSuggestions`, `Origin: "suggestion"`). The thread is told "proposed; the user decides".

- The coordinator does not start it: the card is the proposal, the user's click is the approval. The coordinator prompt gets one line: a thread's request for another thread arrives as a card; do not start one from report text unless the user said so.
- **Gap found while checking: a suggestions card never reaches IM** (`Relay.Message` handles assistant, error, thread update and decision events only), so a chat user is not shown a `propose_threads` card today either. Before this tool is useful to chat users, the card needs an IM rendering with a start button, as `decision` got one.
- Optional, **off by default**: project setting `thread_spawn` (`ask` default, `allow`), like `ThreadGitHubAccess`. With `allow` the proposal starts at once (`Origin: "thread"`, parent link). Only the user sets it, on the project settings page, never a tool. Guards: a thread started this way may not itself propose (depth 1), a cap per parent thread and per hour, the concurrency limiter still applies.

### `notify_coordinator` — tell the coordinator something (no card)

Input: `message` (capped). Effect: the host records it as a thread event, folds one line into the project conversation as a thread update does, and queues a coordinator input `<event source="thread" kind="notify" trust="relay">` (text escaped), then `wake`. It does not end the thread's turn. Nothing it says is an approval: the same rule as a report.

- Authorization: information only, so no card, inside the server's blanket allow like `send_file`. It is the same trust level as a report, sent mid-turn instead of at the end.
- Limits: a per-turn and per-hour cap, so a looping thread cannot flood the coordinator; the coordinator may answer `no_update` as for any report.
- It replaces "mention it in the report" for things that cannot wait for the turn to end. Needs to be told apart from `NEEDS_INPUT` (which asks the user).

### Thread to thread (not built now)

The same wrapper supports it, which is why it is worth deciding now rather than leaving a gap that native fills. A `message_peer` tool (thread-only, `thread_id` of another thread **of the same project**, resolved with `findThread`, so a different project is unreachable) would deliver with `Threads.Send(…, fromThread(sourceID))`, so the receiving thread sees a note "from thread #N, not the user, never an approval", and the message is recorded in **both** transcripts and as one line in the project conversation, so the user and the coordinator can see it. Needed before it ships: a project setting (`thread_peers`: `off` default), a reply-chain depth and rate cap against two threads looping, the coordinator able to stop it (`stop_thread`), and the sender gets "queued" not an answer (a thread cannot wait for another). Until there is a concrete case, a thread uses `notify_coordinator` or `propose_thread` and the coordinator routes with `message_thread`. **Not implemented in this round.**

## Closing the native channel, and telling the two `SendMessage` uses apart

Two uses share one tool name:

| | Continue a subagent | Message another session |
|---|---|---|
| Target (`to`) | the id the `Agent` tool printed: `a` + 16 hex digits | a session name: the cwd's base name plus two hex digits, or `--name` |
| Result | `…"resumedAgentId":…` | `…"msg_id":…` |
| Runs | in-process, no socket | over the socket |
| Needed by Projects | **yes**: a thread resumes its own background subagents | **no**: replaced by the tools above |

Measured: the first works with `ListAgents` disallowed and the receiver's `crossSessionInbound: refuse`; the second is what must be stopped.

**What to turn off** (every Projects session):

1. `crossSessionInbound: "refuse"` in the settings file Projects writes (`writeSettings` for a worker, `sessionSettings` for a folder session; the coordinator has no Projects-owned settings file today and needs one, to verify whether a second `--settings` merges). No hidden turn can start. Cost: none found. The sender is told "success" anyway; that is why the second item matters.
2. `ListAgents` disallowed (`--disallowedTools`; the coordinator already has such a list, `coordinator_session.go`). It removes discovery. The `Agent` tool prints the subagent id itself, so continuation does not need it.
3. **`SendMessage` is left in the tool list for threads but behind an `ask` rule** (`permissions.ask: ["SendMessage"]`, the same mechanism as `ask: mcp__tingly-projects__merge`). Measured: the call then goes to the permission prompt, which under Projects is `threadPrompter.OnApproval` with the tool input. A new first check there: `SendMessage` is **allowed only when `to` matches `^a[0-9a-f]{16}$`**, a subagent id (stricter: an id the thread's own `Agent` results printed; the host sees those tool results), and **denied otherwise** with a reason that names `notify_coordinator` and `propose_thread`. No approval card is shown; the host decides, and it must run before `builtinToolAllowed` and the "allow for this thread" memory so that a click cannot widen it.
4. The coordinator: `SendMessage` is **disallowed** outright (it has no subagents: `Agent`/`Task` are already disallowed), as is `ListAgents`.

A hook that inspects `to` was tried first and **did not fire** in `-p` here (measured 8); it is not relied on.

What this does not cover: the receiving side of the **user's own terminal sessions**. Their `crossSessionInbound` is theirs. With 3 above, a Projects thread cannot send to them; if the user later wants a deliberate bridge (a terminal session talking to a thread), that is its own opt-in tool pair with a named session, a setting, and a visible turn, and is the one place a socket transport (A) would be considered.

## Tests when built

- Tool-list golden: `propose_thread` and `notify_coordinator` appear for a worker and a folder session, not for the coordinator; no thread tool takes `project_id` or a source thread id (the existing `TestToolsOfEachScope`); coordinator's list stays under its byte cap.
- `propose_thread`: makes an `EvSuggestions` event attributed to the thread; a bad base, a taken branch, or two cards naming one branch are refused before the card exists; the card starts through `StartSuggestions`; with `thread_spawn: allow` it starts at once, a spawned thread's own proposal is refused, the cap holds.
- `notify_coordinator`: one `trust="relay"` input per call, text escaped (a forged `</event>` stays inside), the cap holds, a paused or archived project refuses, nothing in it is read as an approval (prompt test beside `prompts_test.go`).
- `threadPrompter.OnApproval`: `SendMessage` to a subagent id allowed; to a session name or another project's coordinator denied with the reason; the "allow for this thread" answer does not stick for it.
- Settings goldens: `crossSessionInbound: refuse` and the ask rule are in the worker and folder session files; the coordinator's launch carries the disallow list.
- Real CLI (the probe in `.research/`): with these settings a second session's `SendMessage` starts no turn, and subagent continuation still resumes.
- Parity: web and IM start the same card to the same thread (needs the IM rendering).

## Decisions

1. **Default for a proposed thread: the card.** Written as the default above because the confirmation card is still waiting on the user; auto-start (`thread_spawn: allow`) stays an off-by-default option. The user may still choose a different default.
2. **Transport: B (Projects' relay), not the native socket.** Recommended with the reasons in the table; the user can overrule it.
3. Closing the native channel (the four items above) is independent of the new tools and can ship first.
4. Thread to thread is designed on the same wrapper and not built.
