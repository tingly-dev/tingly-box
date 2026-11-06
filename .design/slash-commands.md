# Slash commands through AgentBoot: intercept, parse, handle

> Status: **design, nothing built** (2026-11-06). Evidence: [`.research/slash-commands-claude-code.md`](../.research/slash-commands-claude-code.md)
> (real CLI 2.1.259, scripted model; probes in `.research/slash-command-*.py`) and
> `agentboot/engine/slash_commands_test.go` (what happens today).
> Builds on [agentboot-live-control.md](./agentboot-live-control.md) and [projects-im.md](./projects-im.md).

## Problem

`/compact` typed in the coordinator's web chat was treated as text. Claude Code
has in-session commands; users want some of them from the Projects web chat and
from IM, for threads, workdir (folder) sessions and the coordinator.

## What the research found (the parts the design depends on)

1. **The CLI already runs a command sent as a normal stream-json user message**
   (`"/compact"`; also one-shot `-p "/compact" --resume ID`). Nothing new on the wire.
2. **There is no `compact`/`clear` control request.** The only way to compact is
   the command. `get_context_usage` (a control request) gives the exact context size.
3. **A command turn is not a model turn**: `init` → optional synthetic assistant
   text → `result` with `num_turns:0`, zero usage. `/compact` adds
   `status{compacting}`, `status{compact_result:"success"|"failed"}`,
   `compact_boundary{trigger, pre_tokens, post_tokens, duration_ms}` and a
   synthetic summary user message. A failed compaction is
   `compact_result:"failed"` with `result.is_error:false`. Auto-compaction emits
   the same status/boundary events mid-turn.
4. **Today those events are dropped** (`accumulator.go`, `convert.go`), so
   `/compact` leaves no trace except a re-emitted old message (the CLI replays
   preserved messages after a compaction, same uuid).
5. **Thread and folder-session text is sent to the CLI raw**, so *any* CLI
   command a user types there already runs today, `/clear` and `/model`
   included, with the result lost. The coordinator wraps user text in an
   envelope, so commands never reach the CLI there. IM rejects unknown `/foo`
   before anything reaches a thread.
6. **The CLI swallows unknown `/word`** ("Unknown command", no model call) and a
   command sent mid-turn runs as its own turn afterwards. A leading space
   (`" /foo"`) makes it ordinary text (verified).

## Design

One rule: **Projects intercepts a person's message, parses it, and either runs
a supported command or keeps the text from becoming a command by accident.**

### Supported commands

| Command | Behaviour |
| --- | --- |
| `/compact [instructions]` | Send exactly `/compact [instructions]` to the session (works the same one-shot and resident). Show the result as a command card; refresh the context numbers. |
| `/context` *(optional, phase 3)* | Read-only. `get_context_usage` formatted by Projects. Not stored as an assistant reply. |
| `/usage` *(optional, phase 3)* | Read-only. The thread's recorded usage (gateway-priced), same numbers as the status card. |

That is the whole list for now. Adding one later means adding a row and a handler.

### Not supported, on purpose

- **`/clear` (and its aliases `reset`, `new` in the CLI).** It discards the
  conversation irreversibly, the CLI moves to a new session id behind the
  engine's pinned `--resume` id, and on the coordinator it would be undone by
  the "previous conversation" migration read. Not offered, and blocked (below).
  The existing IM `/clear` is Tingly's own command (stop the session, next
  message starts a new one in a folder chat) and is **unchanged**; it never
  sends anything to the CLI.
- `/model`, `/effort`, `/config`, `/mcp`, `/rename`, `/init`, skills and custom
  commands and the rest: each either has a Projects-native control already
  (model/effort/permission endpoints, settings pages) or changes CLI state the
  engine does not track. Out of scope; revisit one at a time.

### What the parser does with `/word …` (web and IM)

Applies only to text typed by a person (see Safety). First match wins:

1. IM session commands, as today (`new stop session status verbose yolo cd dir project help pair join clear`). Unchanged; they never reach the CLI.
2. Supported commands above → run the command.
3. `word` is a known Claude Code built-in or skill (from the session's `init.slash_commands`, or a fixed list when no process is running) but not supported → **do not send it**. Reply with one line: "`/clear` isn't available here." This closes today's hole where `/clear` or `/model` typed in a thread silently runs in the CLI.
4. Anything else (`/etc/hosts is broken`, `/usage-report failed`) → ordinary text. It is sent with a leading space so the CLI cannot treat it as an unknown command and swallow it; the transcript keeps what the user typed.

IM keeps its own rule for unknown `/foo` (rejected with "Unknown command"),
so rule 4 matters on the web only.

Name clashes: `/clear`, `/new`, `/status`, `/help` are IM commands and keep
their IM meaning; the CLI's same-named commands are rule 3 and never run. No
namespace is needed.

### Where each target is handled

- **Threads and folder sessions**: the message path is `Threads.SendWith → enqueue → begin → engine.Start`. The parser runs first; a command becomes `Engine.Command(id, "compact", args)`, i.e. a turn whose prompt is exactly the command text, flagged so the converter treats its output as a command result.
- **Coordinator**: `coordinatorSessions.turn` always builds the envelope and context preamble, so a command must bypass the prompt builder and go to `Engine.Command` with the bare text. Only `/compact` (and the optional read-only ones). After a compaction the project-context preamble may be gone from the summary: mark the main thread so the next turn re-sends it.
- **IM**: `handler_message.go` already routes commands first; `projectCommand` learns `compact`. The bridge then calls the same Projects entry as the web handler, so both behave identically. The result is printed back by the relay.
- **While a turn is running**: a command is held ("runs after this turn", the existing `queue` chip) because the CLI would run it afterwards anyway; never injected.

## UX

- **Composer**: typing `/` as the first character shows a small menu with the supported commands for this target (`/compact  optional instructions  Summarise the conversation to free context`). Enter inserts the text; sending is the normal send. No mode picker.
- **Transcript**: the user's `/compact` line, then one command card, updated in place:
  ```
  ◌ Compacting conversation…
  ✓ Compacted  120k → 18k tokens · 12% → 2% · manual · 4.2s   [Summary ▾]
  ⚠ Could not compact: Not enough messages to compact.
  ```
  Auto-compaction (no user message) gets the same card labelled "auto". The summary is collapsed by default (the synthetic user message, hidden today).
- **Status card**: after a compaction, set the context numbers from `get_context_usage` (`Thread.ContextTokens = totalTokens`, `ContextWindow = maxTokens`); a command turn has no usage entry, so without this the card keeps the old figure until the next real turn. Show "Compacting…" while `status{compacting}` is on (also for auto-compaction).
- **IM**: the same card as one line: `Compacted: 120k → 18k tokens (12% → 2%).`

## Engine and data

- Converter (`agentboot/engine/convert.go`): turn `status{compacting|compact_result}`, `compact_boundary` and the synthetic summary into a new entry kind `command`; attach a `num_turns:0` turn's synthetic assistant text to the command instead of storing it as a reply; **drop messages whose uuid was already seen** (replayed after compaction); skip the empty `usage` entry for `num_turns:0`; if an `init` reports a different `session_id` than the pinned one, record a note and keep the pin (defence against any CLI-side `/clear`).
- `Engine.Command(id, name, args)` and `Engine.ContextUsage(id)` (the control response body must be returned; today `answerControl` only returns an error).
- Projects: `db.EvCommand = "command"`, payload `{name, args, state: running|ok|failed, text, trigger, tokens_before, tokens_after, window, duration_ms, summary}`, created when the command is accepted and updated with `UpdateThreadEvent` + `publishEvent` (the pattern the coordinator's live reply uses). `GET /threads/{id}/commands` lists what the menu may show (`name, description, args_hint, available, reason`). Types come from `task codegen`; until then a placeholder.

## Safety

- Only a person's text is parsed: `user` events with source `web` (the authenticated Projects user) or `im` (a chat that passed `AuthorizationGate`). Thread reports, coordinator-to-thread messages (they open with the coordinator note), system check-ins and coordinator MCP output are never commands. The server enforces this on the source, not the text.
- IM: `/compact` needs `remote_control.start`, like `/stop` today. Web: same as posting a message (no roles).
- Allow-list: only the table above runs. A new CLI built-in is inert (rule 3 or 4).
- `/compact` makes a real model call, through the thread's own profile/gateway. Refused with a reason while a decision is pending.

## Implementation plan

**Phase 0, agentboot only.** Converter changes above, `Engine.ContextUsage`.
Tests: `TestEngine_SlashCommandsToday` (already added with the fixture
`claude-2.1.259-slash-commands.jsonl`) flips to expect a compaction entry, no stale
text, no replayed duplicates; fixtures for `compact_result:"failed"` and for an
auto-compaction captured from a real model; `get_context_usage` over the fake process.

**Phase 1, `/compact` in threads and folder sessions (web and IM).** Parser,
`Threads.RunCommand`/`Engine.Command`, `EvCommand`, command card, context refresh, held-while-running.
Tests: parser table (every supported/blocked/ordinary example above, `/etc/hosts`, leading space, full-width `／`, group `@bot /compact`, relayed and system text never a command);
`Threads` tests with a fake engine (running turn holds it; `/clear` and `/model` are not sent);
IM handler tests in the `harness` style (unknown `/foo` still rejected; `/compact` reaches the thread; IM `/clear` unchanged);
frontend component tests for the card states plus a showcase-mock screenshot; one `e2e`-tagged live test that sends `/compact` to the real CLI.

**Phase 2, coordinator.** Bypass in `coordinatorSessions.turn`, preamble re-send after compaction.
Tests: a command event never reaches `renderHistory`; normal messages are still enveloped; a relayed or system event containing `/compact` is inert.

**Phase 3, polish.** `/` menu and `GET /threads/{id}/commands`, optional `/context` and `/usage`, `/help` lists the commands, "Compacting…" and an auto-compact threshold marker on the status card.

## Open questions

1. `/compact` on the coordinator: allow (proposed) or threads only?
2. Count the compaction call's tokens in the thread's totals? The command turn's `result` reports zeros; only the process-wide `modelUsage` moves.
3. Before phase 0: capture one real-model compaction (manual and auto) to confirm what `post_tokens` means and the auto success events. Needs real model credentials; the scripted model cannot show it.

Not covered: skills and custom commands, `/rewind`, `/btw`, PreCompact/PostCompact hooks, a user-facing auto-compact setting.
