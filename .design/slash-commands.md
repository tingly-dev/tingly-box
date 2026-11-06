# Slash commands (`/compact` and friends) through AgentBoot

> Status: **design, nothing built** (2026-11-06). Evidence: [`.research/slash-commands-claude-code.md`](../.research/slash-commands-claude-code.md)
> (real CLI 2.1.259 against a scripted model; probes in `.research/slash-command-*.py`) and
> `agentboot/engine/slash_commands_test.go` (characterization of today's behaviour).
> Builds on [agentboot-live-control.md](./agentboot-live-control.md) and [projects-im.md](./projects-im.md).

## Problem

A user typed `/compact` in the coordinator's chat and it was "just text". The
question behind it: Claude Code has in-session commands (`/compact`, `/clear`,
`/context`, `/cost`, `/model`, ...). How do Projects (web and IM) let users run
them against threads, the coordinator and workdir (folder) sessions?

## What we learned (short; details in the research note)

1. **The CLI runs slash commands sent as a stream-json user message** (and as a
   one-shot `-p "/compact" --resume ID`). Same wire message as chat. No new
   protocol is needed to *send* them. (Verified.)
2. **There is no control request for compact or clear.** `/compact` exists only
   as the command. Control requests do exist for the numbers: `get_context_usage`
   (exact `totalTokens`, `maxTokens`, `percentage`, auto-compact threshold),
   `get_usage`, `list_models`, `reload_skills`, plus the ones AgentBoot already
   uses (`set_model`, `set_permission_mode`, `apply_flag_settings`, `interrupt`).
3. **A command turn looks like a turn but is not one**: `init` → (synthetic
   assistant text) → `result` with `num_turns:0` and zero usage. `/compact`
   adds `status{compacting}`, `status{compact_result}`, `compact_boundary`
   (`pre_tokens`, `post_tokens`, `trigger`, `duration_ms`, `preserved_*`),
   a synthetic user message (the summary prompt) and a `<local-command-stdout>`
   user message. A failed compaction is `compact_result:"failed"` with
   `result.is_error:false`.
4. **Auto-compaction emits the same `status` / `compact_boundary` events** in the
   middle of a normal turn (`trigger:"auto"`; the failure path was observed, the
   success path is inferred).
5. **Today the pieces are dropped, so the user sees nothing or something wrong**
   (`TestEngine_SlashCommandsToday`): `/compact` leaves no entry saying it
   compacted, only a re-emitted old `/cost` report (the CLI replays "preserved"
   messages after a compaction, with the same uuid); `/clear` changes the CLI's
   session id behind the engine's back; `/cost` shows list-price numbers.
6. **Where the text goes depends on the surface.** Thread / folder-session
   messages are sent to the CLI raw (`queuedPrompt`), so `/compact` already
   reaches the CLI there and the result is lost. The coordinator wraps every
   user message in a trust envelope with a project-context preamble
   (`renderHistory`), so a slash command never starts the prompt and can never
   run. IM rejects unknown `/foo` ("Unknown command") before anything reaches a
   thread.
7. **Footguns found while probing** (all verified):
   - The CLI swallows any unknown `/word`: "Unknown command", no model call.
     A user message such as `/usage-report is broken` would vanish. `" /compact"`
     (leading space) and path-like text (`/etc/hosts ...`) are ordinary prompts.
   - A command written while a turn runs is **not** injected into it: it runs as
     its own turn after the current one.
   - `/model x` and `/effort x` change CLI state the engine does not know about
     (`launchState` drifts); `/clear` changes the session id the engine pinned
     for `--resume`; `/rename` renames the CLI's session, not the thread.
   - Skills added after the process started are unknown until `reload_skills`.

## Decision: three kinds of command

| Kind | Rule | Why |
| --- | --- | --- |
| **Passthrough** | Send `/name args` to the CLI as the turn's prompt; Projects records a *command event* from the events that come back. | The CLI owns the state: the conversation and its summary. Only the CLI can compact it. |
| **Tingly-side** | The same words, implemented by Projects/AgentBoot through APIs that already exist. Never sent to the CLI. | Tingly owns the state (model routing, session identity, usage accounting, thread title). The CLI's version would drift from it or be wrong. |
| **Unsupported** | Not offered; typed anyway, the user gets a one-line reason. | Interactive-only, or it escapes Projects' model (a second scheduler, MCP/config edits, worktree fan-out). |

Rule of thumb: *if the CLI keeps the state, pass through; if Projects keeps it,
do it ourselves under the same name.* One vocabulary for users, two
implementations behind it.

### Command catalogue

"W" = web chat, "IM" = IM chat bound to that target. T = worker thread,
F = workdir (folder) session, C = coordinator.

| Command | Kind | T | F | C | Mapping |
| --- | --- | --- | --- | --- | --- |
| `/compact [instructions]` | passthrough | W IM | W IM | W IM | Prompt `/compact <instructions>` to the resident session. Event + context numbers, see UX. |
| `/context` | tingly-side (data from the CLI) | W IM | W IM | W IM | Control request `get_context_usage`; Projects formats it (bar, categories, auto-compact threshold). Not the CLI's markdown table. |
| `/usage` (aliases `/cost`, `/stats`) | tingly-side | W IM | W IM | W IM | From recorded thread usage (gateway-priced, not CLI list price). Same numbers as the status card. |
| `/clear` | tingly-side | W IM | W IM | W IM | New CLI session: evict the process, new session id, `Turns=0`; the transcript keeps its history with a "Context cleared" divider. IM `/clear` already means this for folder chats. Coordinator: also a history cut-off, see Coordinator. |
| `/model [name]`, `/effort [level]` | tingly-side | W IM | W IM | W IM | No argument: show current and the choices. With one: the existing `PUT /threads/{id}/model` / `effort` (engine applies live with `set_model`). Names are the profile's models, not CLI aliases. |
| `/rename <title>` | tingly-side | W | W | - | Thread title. |
| `/init`, skills and custom commands (`/security-review`, `/code-review`, `.claude/skills/*`, `.claude/commands/*`) | passthrough | W IM | W IM | no | Plain prompt to the model; the CLI expands it. Call `reload_skills` before the first command of a process so skills added since launch resolve. |
| `/status`, `/help`, `/stop`, `/new`, `/yolo`, `/verbose`, `/session`, `/cd`, `/dir`, `/project` | existing IM commands | IM | IM | IM | Unchanged. `/help` also lists the commands above. |
| `/mcp`, `/config`, `/fast`, `/autocompact`, `/permissions`, `/add-dir`, `/memory`, `/loop`, `/goal`, `/batch`, `/agents`, `/doctor`, `/color`, `/login`, `/logout`, `/resume`, `/rewind`, `/btw` | unsupported | - | - | - | Each has a Projects-native home (settings pages, permission mode, directory UI, threads) or needs a terminal. `/rewind` (`rewind_files`) and `/btw` (`side_question`) are candidates for later. |

How to tell a *skill* from a *built-in* at run time (so that new CLI
built-ins are not accidentally passed through): `get_context_usage` returns
`skills.skillFrontmatter[].name`; `system/init.slash_commands` minus that list
is the built-ins. Only names in Projects' catalogue run as built-ins; names in
the skill list run as skills (threads and folder sessions only); anything else
is not a command.

## Recognising a command, and name clashes

One parser, `projects/internal/project/commands.go` (new), used by the web
handler and the IM bridge, so both resolve a name the same way. Input is only
**text typed by a person**: the `user` event whose source is `web` or `im`.
Thread reports, coordinator-to-thread messages (they open with the coordinator
note), system check-ins and attachments are never parsed.

Resolution order for `/name args` (first match wins):

1. **Projects session commands** (IM today): `new stop session status verbose yolo cd dir project help pair join`.
2. **Tingly-side implementations of CLI names**: `clear model effort usage cost stats rename context`.
3. **Passthrough built-ins** in the catalogue: `compact` (and `init`).
4. **Skills** of this session (threads and folder sessions only).
5. Otherwise it is not a command.

Clashes found, and how the order settles them:

| Name | In IM today | In the CLI | Result |
| --- | --- | --- | --- |
| `/clear` | stop the session, next message starts a new one (folder chats) | new session (aliases `reset`, `new`) | Same meaning. One tingly-side implementation for web and IM. |
| `/new [msg]` | start a session | alias of `/clear` | IM meaning wins (rule 1). Not offered on the web: a new thread has its own button. |
| `/status`, `/help` | tingly's | "isn't available in this environment" | Tingly's. |
| `/cost`, `/stats` | none | alias of `/usage` | Become aliases of tingly's `/usage`. |
| `/model`, `/effort` | none | CLI-local, drifts from the engine | Tingly-side (rule 2). |
| `/compact`, `/context` | none | CLI | No clash. |

So no namespace is needed today: tingly's meaning wins, and where the CLI has
the same name it is the same intent. If a real clash appears later (a CLI
built-in with a different meaning than a Projects command), add an explicit
`/claude <command>` form that always passes through; do not reserve it now. The
existing `/cc` and `/tb` prefixes stay what they are (stripped, then text).

**Not a command → text, safely.**
- IM keeps its rule: unknown `/foo` gets "Unknown command /foo. /help lists what works here." (it never reaches a thread).
- Web: unknown `/word` is sent as an ordinary message. Because the CLI would swallow it, Projects sends it with a leading space (`" /usage-report is broken"`, verified to reach the model). The user sees what they typed.
- Arguments are not validated by Projects; the CLI's usage text for a passthrough command comes back as the command's result.

## UX

### Composer (web, all targets)

Typing `/` as the first character opens a menu above the input, filtered as
the user types, like the existing attachment/mention affordances. Each row:
`/compact  <optional instructions>   Summarise the conversation to free context`.
Rows come from `GET /threads/{id}/commands` (see API): catalogue entries valid
for this target, plus skills, with a disabled row and a reason when something
cannot run right now (for example "Claude is working; runs after this turn").
Enter on a row inserts `/compact ` into the text box; the text is then sent
like any message. No mode picker, no second send path (UX principle: no
modes): the typed text is the command.

While a turn is running, a passthrough command is **held**, never injected: the
existing "after this turn" chip is used automatically, labelled "runs after this
turn" (verified: the CLI itself would run it as its own turn after the current
one). Tingly-side commands that only read (`/usage`, `/context`) run at once.
`/clear` while running asks to stop first (same as "Stop and send").

### Transcript: a command card, not a chat bubble

A new thread event kind `command` (see Events) renders as one compact card
under the user's `/compact` line, updated in place:

```
 you   /compact keep the API notes
 ───────────────────────────────────────────────────────────────
  ◌ Compacting conversation…                        (running)
 ───────────────────────────────────────────────────────────────
  ✓ Compacted  120k → 18k tokens · 12% → 2%  · manual · 4.2s   [Summary ▾]
 ───────────────────────────────────────────────────────────────
  ⚠ Could not compact: Not enough messages to compact.          (failed)
```

- Auto-compaction (no user message) gets the same card with "auto".
- `[Summary ▾]` expands the synthetic summary user message the CLI sends
  (`isSynthetic:true`). It is hidden otherwise; today it is dropped.
- Read-only commands (`/context`, `/usage`) are cards with a small table; they
  are not stored as assistant replies (so they never read as the model
  speaking, and the coordinator never sees them).
- `/clear` leaves a divider "Context cleared"; history above it stays visible
  but is dimmed.
- `/model`, `/effort`, `/rename` confirm in one line and update the info rail
  they already drive.

### Status card (info rail)

- Context numbers come from `get_context_usage` after any command that changes
  the context (`/compact`, `/clear`): `Thread.ContextTokens := totalTokens`,
  `ContextWindow := maxTokens`. Without it the card would stay at the old figure
  until the next real turn (a command turn makes no model call, so no usage
  entry; today an empty `usage` entry for `num_turns:0` also overwrites
  `LastTurnMS` with 0, to be skipped).
- During compaction the card shows "Compacting…" (from `status{compacting}`),
  also for auto-compaction, which is invisible today.
- Optional: a thin marker for the auto-compact threshold (`autoCompactThreshold`) and "Compacted 12 min ago".
- After an auto-compaction in a normal turn nothing extra is needed: the turn's
  last call reports the new, smaller prompt size.

### IM

The same card as text, through the existing relay, for the chat bound to the
target: `Compacted: 120k → 18k tokens (12% → 2%).`, `Context cleared.`, or the
`/usage` table. Passthrough commands in IM are held until idle like any
message; `/stop` still works. `/help` gains a section listing them.

## Events and engine plumbing

New engine entry kinds (converter, `agentboot/engine/convert.go`), all derived
from events the accumulator drops today:

| CLI event | Entry | Notes |
| --- | --- | --- |
| `system/status` `compacting` / `compact_result` | `command` update (state running / failed + `compact_error`) | `result.is_error` is false even when failed. |
| `system/compact_boundary` | `command` update (state ok; `trigger`, `pre_tokens`, `post_tokens`, `duration_ms`) | Also fired by auto-compaction with no command in flight: create the card. |
| `user{isSynthetic:true}` after a boundary | summary text for the card | Not a user message. |
| synthetic `assistant` in a `num_turns:0` turn | the command's text result | Attached to the command, not stored as an assistant reply. |
| messages with a uuid in `compact_metadata.preserved_messages.all_uuids`, or any uuid already seen | dropped | The CLI re-emits preserved messages after a compaction (verified); text would duplicate. Dedupe by uuid in the converter. |
| `conversation_reset` / `init` with a different `session_id` | engine re-pins the session id and records a note | Defensive: `/clear` is tingly-side, but a CLI-side `/clear` must never silently fork the pinned `--resume` id. |

Engine API additions:
- `Engine.Command(id, name, args)` – start a turn whose prompt is exactly `/name args`, flagged so the converter treats its output as a command result (same `Start` path otherwise; works one-shot and resident; `--resume` was verified for the one-shot form).
- `Engine.ContextUsage(id)` – `get_context_usage` over the control path. `PersistentSession` today waits for a `control_response` but only returns the error; it needs to return the response body (one small change in `answerControl` / `ControlResponseDecoder`).
- `Engine.Skills(id)` – the skill/built-in split above and `reload_skills`.
- Nothing is added to `launchSignature` or `launchState`.

Projects: `db.EvCommand = "command"` with payload
`{name, args, state: running|ok|failed, text, trigger, tokens_before, tokens_after, window, duration_ms, summary}`;
created when the user's command is accepted, updated in place with
`UpdateThreadEvent` and `publishEvent` (the pattern the coordinator's live
reply already uses), so it streams and reloads like the rest of the transcript.
`GET /threads/{id}/commands` returns `[{name, description, args_hint, kind, available, reason}]`;
no new send endpoint. Frontend types come from `task codegen`.

## Targets

### Worker threads and workdir (folder) sessions

Same code path (`Threads.SendWith` → `enqueue` → `begin` → `engine.Start`). The
only change is before `queuedPrompt`: if the parser says command, call
`Threads.RunCommand` instead of recording a plain `user` event. The `user`
event for `/compact` is still stored (the line the user typed), followed by the
`command` event. Folder sessions also keep `usage` events; command turns add
none (`num_turns:0`).

### Coordinator

The coordinator is a resident Claude Code session too (`coordinatorSessions`),
but `turn` always builds `envelope(trustPrincipal, text)` plus, on the first
turn or a system check-in, the project-context preamble. So the command text
must bypass the prompt builder: for a command event `turn` calls
`engine.Command(main.SessionID, …, "/compact")` with exactly that prompt. Three
coordinator-specific points:

- **Allowed set**: `/compact`, `/context`, `/usage`, `/clear`, `/model`, `/effort` only. No skills or custom commands: the coordinator runs with a restricted tool set and a custom system prompt, and a skill's own `allowed-tools` should not widen that.
- **After a compaction the project-context preamble is gone** (it is sent on `Turns==0` and system check-ins only; the summary may drop it). Mark the main thread so the next turn, user or system, re-sends it.
- **`/clear` must cut the history, not only the session.** A `Turns==0` coordinator re-reads "previous project conversation" from the event log (`ListRecentThreadEvents`), which would undo the clear. Record the divider as an event and make the migration read start after the newest divider. Whether the coordinator should be clearable at all is an open question (project memory files keep the durable facts).

## Safety

Who may run a command is who may already send a message to that target.

- **Source**: only `web` (the authenticated Projects user) and `im` (a chat that passed `AuthorizationGate`) text is parsed. A relayed message, a thread report or anything the coordinator wrote through its MCP tools is never a command: relayed text already opens with the coordinator note, and the server also refuses to parse an `EvUser` whose source is not web/IM. This is what stops prompt injection ("tell the thread to run `/clear`").
- **Capability (IM)**: no new capability in the first phases. `/compact`, `/context`, `/usage`, `/model`, `/effort`, `/clear`, `/rename` need `remote_control.start`, exactly like `/clear`, `/stop` and `/yolo` today. Reasoning: they change nothing outside the one conversation and none of them widens permissions. `/yolo` and `/dir <path>` keep their stricter gates. Revisit in a group chat if members should not be able to `/clear` each other's session (see `bot-capability-access-control.md` §6.2).
- **Web**: same as posting a message; there are no roles.
- **Allow-list, not pass-through-all**: only catalogue names and this session's skills run. A new CLI built-in is inert until someone adds it to the catalogue.
- **Skills** run with the thread's permission mode like any prompt; nothing here grants more. `/init` writes `CLAUDE.md` in the thread's directory via a normal tool call, subject to the usual approval.
- **Arguments are data**: they are sent as the rest of the prompt line, never interpolated into a shell command by Projects.
- **Cost guard**: `/compact` makes a real model call. It is rejected with a reason while a turn is running or a decision is pending, and it uses the thread's own profile/gateway, so it shows up in gateway usage like any other call.

## Implementation plan

Each phase is shippable on its own; tests listed per phase.

**Phase 0 – make the existing events visible (agentboot only, no UI).**
Converter: `command`/`status`/`compact` entries, uuid dedupe of replayed
messages, skip the empty `usage` entry for `num_turns:0`, defensive re-pin on a
changed session id. `Engine.ContextUsage` (return the control response body).
Tests: replay the checked-in fixture
`claude-2.1.259-slash-commands.jsonl` (already added with
`TestEngine_SlashCommandsToday`, which Phase 0 must flip: `/compact` gets a
compaction entry and no stale `/cost` text, no duplicate preserved messages);
unit tests for the compact-failure fixture (`compact_result:"failed"`) and an
auto-compaction fixture captured from a real model; `get_context_usage` over a
fake process.

**Phase 1 – `/compact` and `/context` in threads and folder sessions (web + IM).**
`commands.go` parser and precedence; `Threads.RunCommand`; `EvCommand`;
`Engine.Command`; context numbers refreshed from `get_context_usage`;
command card in the transcript; the IM relay prints the result; held-until-idle
behaviour. Tests: parser table test (every name in the clash table, the leading-space escape,
`/etc/hosts`, full-width `／`, group `@bot /compact`), `Threads` tests with the fake engine
(running turn → held chip; relayed/system text never a command), IM handler tests
in `harness` style (unknown `/foo` still rejected; `/compact` reaches the thread),
frontend component tests for the card states, a showcase-mock screenshot of
running/ok/failed, and a live test behind the `e2e` tag
(`agentboot/live_control_e2e_test.go` style) that sends `/compact` to the real
CLI and asserts the boundary and refreshed numbers.

**Phase 2 – composer menu, `/help`, tingly-side commands.**
`GET /threads/{id}/commands`, the `/` menu, `/usage`, `/model`, `/effort`,
`/rename`, `/clear` (new session in place, divider, `Turns=0`, session id
re-pinned) for threads and folder sessions, `/help` section in IM. Tests: handler tests for the
commands list per target, menu keyboard tests, `/clear` then `--resume` uses the new id
(engine test with two fake processes), `/usage` equals the status card numbers.

**Phase 3 – coordinator.**
Command bypass in `coordinatorSessions.turn`, allowed set, context preamble
re-send after compaction, `/clear` history cut-off. Tests: a command event does not reach
`renderHistory`; envelope text still wraps normal messages; `/clear` followed by a
restart does not resurrect history; relayed/system events carrying `/compact` are
inert.

**Phase 4 – skills and custom commands in threads/folder sessions, auto-compaction polish.**
`reload_skills` before first use, skill list in the menu, "Compacting…" and the threshold marker on the
status card for auto-compaction. Tests: a project skill added after launch runs after `reload_skills`;
an unknown name is not offered.

Codegen (`task codegen`) is needed once for the new endpoint and event kind;
until then the frontend uses a placeholder for `GET /threads/{id}/commands`.

## Open questions for the user

1. **`/clear` in a worker thread / folder session**: reset the context inside the same thread (proposed: divider, same history visible), or start a *new* thread (what IM folder chats do today)?
2. **Coordinator `/clear`**: allow it (with the history cut-off), or only `/compact`?
3. **Skills/custom commands** (`/security-review`, project skills): in scope for threads now, or later? Proposed: Phase 4, threads and folder sessions only.
4. **Group chats**: may any member with `start` run `/clear` or `/compact`, as they can run `/stop` and `/yolo` today? Proposed: yes for now.
5. **Compaction cost**: the summary call's tokens are not in the per-turn usage (the result of a command turn reports zeros; only the process-wide `modelUsage` moves). Count them (via `modelUsage` deltas) in the thread's token totals, or leave them to gateway usage?
6. **Numbers to confirm on a real model**: `post_tokens` semantics (summary only, or summary plus preserved messages?) and the auto-compaction success events were not observable with the scripted model. Phase 0 starts with one real-model capture.

## Not covered

`/rewind` (`rewind_files`), `/btw` (`side_question`), `/mcp reconnect`, PreCompact/PostCompact hooks for project-specific compaction rules, and a user-facing auto-compact setting (`DISABLE_AUTO_COMPACT`, `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE`).
