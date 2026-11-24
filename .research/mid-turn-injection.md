# Does a message written mid-turn reach the model, and when?

Question (Workspace "insert now" mode): with Claude Code running under
`--input-format stream-json`, what happens to a user message written to stdin
while a turn is running? Is it in the *next model request* of the same turn
(after the running tool call), or does it wait for the turn to end?

## Method

`inject-probe/probe.py <scenario>` runs the real `claude` CLI
(**2.1.259**, `-p --input-format stream-json --output-format stream-json
--verbose --permission-prompt-tool stdio`, no `--replay-user-messages`)
against a scripted Anthropic Messages API on 127.0.0.1. The mock logs the
**messages array of every model request** (time, role, block types, first
characters), so "the model could read it" is read off the request body, not
inferred from the CLI's echo. HOME is a scratch directory, the API key is
fake; nothing real is touched. The driver writes the user message at a known
time and prints a timeline of what the CLI emitted next to the requests.

Why not `tb vmodel --capture`: the installed `tb` has no `vmodel` command,
and its capture default answers a fixed "ok", while these scenarios need
scripted tool calls (a slow Bash, a subagent). The scripted mock replaces it.
Reproduce: `python3 .research/inject-probe/probe.py tool` (and `stream`,
`final`, `perm`, `multi`, `task`).

## Results

Times are seconds from the first write; "request #n" is the model request
number of the main loop.

| scenario | message written | what the model saw |
| --- | --- | --- |
| `tool`: Bash `sleep 6` running | t=3.0 | Not at write time. Bash ends at 6.57; **request #2 at 6.62 contains it**, right after the tool result. One `result` for the whole turn. |
| `stream`: model still streaming text that ends in a tool call (stream ends 7.0) | t=3.0 | The in-flight request (#1) cannot contain it. **Request #2 (7.16) does**, after the tool result. No second turn. |
| `final`: model streaming its last text, no tool call follows | t=3.0 | Turn ends (`result` 7.02), then the CLI starts a **new turn by itself** (`system/init`, request #2 with the message as a user message after the assistant text), second `result`. So: it is the next turn, run by the CLI. |
| `perm`: Bash needs approval, prompt unanswered for 5 s (default mode) | t=2.5 | Nothing moves while the prompt is open. After approval (5.5) **request #2 contains it**. |
| `multi`: two messages 1.5 s apart during one tool call | t=3.0, 4.5 | Both arrive **coalesced in one message** in request #2. |
| `task`: foreground subagent (Agent tool) running a 6 s Bash | t=3.0 | The subagent's own requests (`sub#2` at 6.65) **do not** contain it; the main loop's request after the subagent returned (6.71) does. Insert waits for the whole subagent. |

How it appears in the request: a message in the array, placed after the last
tool result, whose text is
`The user sent a new message while you were working: <your text> …` (the
CLI wraps it; a message that becomes its own turn is a plain user message).

## What this means

- Mid-turn input is **really read by the model within the same turn**, at the
  first model request that starts after the write. That is the earliest
  possible point: a request already streaming cannot be changed, and a tool
  call cannot be cut short (only Stop does that).
- "As soon as reasonable" therefore means: after the running tool call, after
  the stream in flight, after a pending approval is answered, after a running
  subagent returns. The UI must not promise more ("Claude reads it at its next
  step").
- If the turn ends with no further model request, the CLI runs the message as
  the next turn by itself. Nothing is lost and no host-side branching is
  needed. (The host must still handle that turn: AgentBoot tracks it as an
  unsolicited turn.)
- Same behaviour as noted for 2.1.302 in `agentboot/claude/ref/live-control.md`.

Not tested: other agents (only Claude Code is a production AgentBoot agent;
the capability seam is described in `.design/agentboot-live-control.md`).
