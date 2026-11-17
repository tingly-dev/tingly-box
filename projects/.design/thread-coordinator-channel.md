# A thread asks for a new thread: the thread-to-coordinator channel

> Status: **proposal, not built.** Nothing here is implemented; the evidence is in [../.research/cross-session-messaging.md](../.research/cross-session-messaging.md) (probed on Claude Code 2.1.259, 2026-11-17). Related: [mcp-tools.md](./mcp-tools.md) (the tool table, `ThreadOnly`), [github-tools.md](./github-tools.md) (the authorization pattern copied here), [message-channel.md](./message-channel.md) (what reaches the coordinator and the chat), [projects-harness.md](./projects-harness.md).

## The question

"Claude Code can message between instances natively. Does Projects support it? Having a thread tell the coordinator to open a new thread did not work."

## What exists today

A thread has **no way to ask for a thread**. Its tools are `send_file` and the six GitHub tools (`projects/internal/project/github_tools.go`); `start_thread` and `propose_threads` are `CoordinatorOnly`. A thread's only channel to the coordinator is its report: each turn end becomes `<event source="thread" kind="report" trust="relay">` in the coordinator's next turn (`Coordinator.onReport`), plus the thread prompt telling it to "mention other work in your report instead of doing it".

So a thread that wants a new thread can do one of two things:

| Route | What happens | Verdict |
|---|---|---|
| Say it in the report | The coordinator reads it as information (`trust="relay"`). Its prompt lists merge, push, delete, spend and settings as decisions that need the user; **starting a thread is not on that list**, so whether it starts one is the model's judgement. No code path refuses it, and none carries it out. | Works by luck, not by contract. A coordinator that is cautious (as the prompt asks) will ask the user or ignore it; a thread cannot tell which. This is the most likely thing the user saw. |
| Native `SendMessage` to the coordinator | Measured: the thread is allowed to call `ListAgents`/`SendMessage` with **no approval card**; the message reaches the coordinator's CLI, which runs a turn Projects never sees; `start_thread` in that turn answers `No coordinator turn is active.` and nothing starts. | **Fails, and invisibly.** Neither the user nor the thread sees why. |

## Native cross-session messaging: what it is and why Projects should not build on it

The CLI binds a socket per session and lists peers from `<config dir>/sessions`. It is on by default in `-p` stream-json sessions (details and measurements in the research note). Properties that matter here:

1. **Reach is wider than a project.** The registry is per config dir, so every Projects session on a machine shares it with every other project's sessions and with the user's own terminal Claude sessions. A thread (which reads web pages, repos and issues, so can be prompt-injected) can `SendMessage` to any of them. The receiver is told to treat it as a teammate's request, within its own permissions: for the user's terminal session that is a real injection path; for another project's coordinator it crosses a project boundary Projects otherwise keeps.
2. **It bypasses the trust model.** The coordinator's inputs are `<event … trust=…>` envelopes with escaped text. A peer message arrives as raw `<cross-session-message>` text with no trust label, and the harness paragraph says to act on it as a teammate. The coordinator prompt has no rule for it.
3. **It is invisible to Projects.** The turn it triggers has no row in the transcript, no state, no relay to the chat, no cancel. It only fails today because the tool gate (`coordinatorToolTurn.run`) refuses calls outside a Projects turn.
4. **It has no approval step of its own.** The receiver's `crossSessionInbound` is the only gate, and an unattended `-p` session cannot show its dialog.

It does fit "agents that already trust each other on one machine". Projects' model is the opposite: the user is the principal, the coordinator is a router, a thread is a worker with a task.

## Recommendation

### 1. A thread tool that proposes a thread (`propose_thread`)

A new `ThreadOnly` tool in the tool table, taking the same fields as one entry of `propose_threads` (`title`, `task`, `repo`, `base`, `branch`) and the same validation (`checkProposals`). It writes a **suggestion card** (`EvSuggestions`) in the project conversation, attributed to the proposing thread, and tells the thread "proposed; the user decides". The user starts it from the card, which already exists (`StartSuggestions`, `Origin: "suggestion"`), on the web. **Gap found while checking: a suggestions card does not reach IM at all** (`Relay.Message` handles assistant, error, thread update and decision events only), so today a chat user is never shown a `propose_threads` card either. Before relying on the card for chat users it needs an IM rendering with a start button, the way `decision` got one; until then the thread's report line ("proposed a thread: …") is what a chat sees.

Why this and not the alternatives:

| | Native `SendMessage` | Report only (today) | **`propose_thread` (MCP)** | `start_thread` for threads |
|---|---|---|---|---|
| User approves | no (the coordinator may act) | the coordinator decides | **yes: the card** | no |
| Visible | no | yes | **yes: card + thread note** | yes |
| Trust envelope | none | `relay` | **host-written event, no free text reaches the coordinator's tools as an instruction** | n/a |
| Structured (title, task, base, branch) | no | no | **yes** | yes |
| Project boundary | crosses it | no | **no: the tool is bound to the thread's capability URL** | no |
| Works in tests (in-memory MCP client) | no | n/a | **yes, like the GitHub tools** | yes |

It follows the granularity rule (project memory `mcp-tool-granularity`: few tools per verb, no `action` argument): one new tool, write-only, named for the verb. It is not folded into `propose_threads`, because that one is coordinator-only and takes a list.

The coordinator is told with a system note (`trust="system"`) that the card exists, like a thread update that needs no turn. It does not have to act, and nothing it reads is the thread's free text as an instruction.

### 2. Authorization: card by default, opt-in to auto-start

- **Default: the card.** The user confirms. This is the only mode that needs no setting.
- **Opt-in `thread_spawn` project setting** (like `ThreadGitHubAccess`: `ask` default, `allow`): when `allow`, a proposal starts at once with `Origin: "thread"` and a parent link. Only the user sets it (the project settings page; never a tool). Guards when on: a thread started this way may not itself propose (depth 1), a cap on proposals per parent thread and per hour, and the existing concurrency limiter still applies.
- **Never**: the coordinator starting a thread because a report or a peer asked is not treated as approval. The coordinator prompt gets one line: a thread's request for another thread arrives as a card; do not start one from report text unless the user said so.
- The thread prompt changes from "mention other work in your report instead of doing it" to "propose it with `propose_thread` and say so in your report".

### 3. Close the native channel for Projects sessions (hardening, separate from 1)

Independent of the tool, because the channel is reachable now. Measured options:

| Control | Effect | Cost |
|---|---|---|
| `crossSessionInbound: "refuse"` in the settings Projects already writes for the coordinator and every thread | Nothing can start a hidden turn in a Projects session. Sender sees `success: true` (silent drop). | None found. Measured 4 in the research note. |
| `--disallowedTools ListAgents` | Removes peer discovery from the model's tools (a model can still guess a name). | Low. |
| `--disallowedTools SendMessage` | Removes outbound messaging, **but** the `Agent` tool's own result tells the model to continue a background subagent with `SendMessage`. | Breaks subagent continuation in threads; **do not** unless that is checked. |
| A different `CLAUDE_CONFIG_DIR` per project | Partitions the registry. | Moves the user's settings, skills and login with it. Not viable. |
| `PreToolUse` hook on `SendMessage` that denies a non-subagent target | Closes outbound too. | A hook script to ship and keep. A later step if needed. |

Proposed now: inbound `refuse` for all Projects sessions and `ListAgents` disallowed. Residual: a thread can still `SendMessage` a peer whose name it guesses; the user's terminal sessions are protected only by their own `crossSessionInbound`.

If the user *wants* terminal sessions to talk to threads (a feature, not a leak), that is a separate, explicit design: a named session per thread (`--name`), an opt-in setting, and a visible turn. Not proposed here.

### 4. Thread to thread

**No direct channel now.** Threads cannot see each other's context, share a repository only through branches, and have no agreed identity or approval for it; the coordinator already routes (`message_thread` with `cite`), and the proposal card covers "this needs another thread". A later need (two threads negotiating an interface) should start as a coordinator-mediated message, with the user's approval rule, not a peer socket. Revisit with a concrete case.

## Tests when built

- Tool list golden: `propose_thread` appears for a worker and a folder session and not for the coordinator; no thread tool takes `project_id` or a target thread id (`TestToolsOfEachScope`, the existing check).
- In-memory MCP client: a proposal makes an `EvSuggestions` event attributed to the thread; a bad base or a taken branch is refused before the card exists; two cards naming one branch are refused; the card starts through `StartSuggestions`.
- With `thread_spawn: allow`: the thread starts, `Origin: "thread"`, a spawned thread's `propose_thread` is refused, the cap holds.
- A coordinator turn given a report that asks for a thread does not call `start_thread` without the card (prompt test, as `prompts_test.go`).
- Real CLI: settings carry `crossSessionInbound: refuse`; a second session's `SendMessage` starts no turn (the probe in `.research/`).
- Parity: web and IM start the same card to the same thread (needs the IM rendering above).

## Decisions for the user

1. **Default for a proposed thread**: the card only (recommended), or let the coordinator decide as now, or auto-start behind an opt-in setting (offered above, off by default)?
2. **Close the native channel** (inbound `refuse` + `ListAgents` disallowed) now, as its own small change? Or keep it for sessions you drive yourself?
3. **Which route did you try?** A report saying "start a thread X", or a thread calling `SendMessage`? It decides whether to also tighten the coordinator prompt now.
4. **Thread to thread**: leave out until there is a concrete case (recommended)?
