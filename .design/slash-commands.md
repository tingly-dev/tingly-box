# Session control API, starting with `/compact`

> Status: **design, nothing built** (2026-11-06). Evidence: [`.research/slash-commands-claude-code.md`](../.research/slash-commands-claude-code.md)
> (real CLI 2.1.259, scripted model; probes in `.research/slash-command-*.py`) and
> `agentboot/engine/slash_commands_test.go` (what happens today).
> Builds on [agentboot-live-control.md](./agentboot-live-control.md) and [projects-im.md](./projects-im.md).

## Why this is an API design, not a command feature

Projects depends on one external program, the Claude Code CLI, and on its
stream shapes. `/compact` is the first fine-grained behaviour Projects needs
that the CLI exposes only as a quirk (a slash command in a user message).
Instead of teaching Projects that quirk, we put the behaviour behind a small,
backend-neutral session API. Each behaviour wrapped this way adds a line to
the list of what *any* agent backend must provide to be a base for Projects.
That list is what could one day let us swap the backend. We start with
compaction and context usage and add nothing speculative.

Slash commands in web and IM become a thin front-end over that API. No raw CLI
slash text ever leaves Projects.

## What the research found (only what the design depends on)

1. The CLI runs `/compact` sent as a normal stream-json user message (also one-shot `-p "/compact" --resume ID`). (Verified.)
2. There is no `compact`/`clear` control request. `get_context_usage` (control request) returns the exact context size: `totalTokens`, `maxTokens`, `autoCompactThreshold`. (Verified.)
3. A compaction emits `status{compacting}`, `status{compact_result:"success"|"failed", compact_error}`, `compact_boundary{trigger, pre_tokens, post_tokens, duration_ms}`, a synthetic summary user message, then a `num_turns:0` `result` with zero usage. Failure leaves `result.is_error:false`. Auto-compaction emits the same events mid-turn (failure path seen; success path inferred, UNVERIFIED).
4. Today these events are dropped (`accumulator.go`, `convert.go`), and the CLI re-emits preserved messages with the same uuid after a compaction.
5. Text sent to a thread or folder session reaches the CLI raw, so any CLI command typed there already runs, `/clear` and `/model` included. The coordinator wraps user text in an envelope, so nothing reaches the CLI. IM rejects unknown `/foo`.
6. The CLI swallows unknown `/word`; a leading space turns it into plain text. A command sent mid-turn runs as its own turn afterwards.
7. Not verified (no real model): what `post_tokens` counts; the auto-compaction success events; whether the compaction call's tokens appear anywhere (the command turn's `result.usage` is zero; only the process-wide `modelUsage` moves).

## The API

Two layers. Neutral types and interfaces live in `agentboot` (no Claude
names). Projects keeps calling the **Engine**, as it does today.

### Neutral types (`agentboot/session_control.go`)

```go
type CompactTrigger string // "manual" | "auto"

// CompactResult is what a finished compaction reports. A backend that cannot
// report a figure leaves it zero; Reported says which.
type CompactResult struct {
	Trigger      CompactTrigger
	TokensBefore int64
	TokensAfter  int64
	Duration     time.Duration
	Summary      string        // the summary text, if the backend exposes it
	Usage        *TokenUsage   // tokens the compaction call itself spent, if reported (input, output, cache read/write)
	Reported     struct{ Tokens, Summary, Usage bool }
}

type ContextUsage struct {
	Used, Max     int64 // prompt size now / the model's window
	AutoCompactAt int64 // 0 when the backend has no automatic compaction
}

var (
	ErrCompactUnsupported = errors.New("agentboot: backend cannot compact")  // like ErrControlUnsupported
	ErrContextUnsupported = errors.New("agentboot: backend cannot report context usage")
)
// *CompactFailedError{Reason string}: the backend refused ("Not enough messages to compact.").
// engine.ErrBusy: a turn is running.

// CompactEvent is the neutral stream event for a compaction, whether the
// user asked for it or the backend started it: Phase started | done | failed,
// plus a CompactResult once done or the Reason once failed.
type CompactEvent struct{ Phase string; Result CompactResult; Reason string }
```

### Backend seam (optional, per capability, like `ControlRequestEncoder`)

A backend transport implements the capabilities it has; a missing one gives the
`Err…Unsupported` error. This is the "what must the agent layer provide" list.

```go
// CompactCodec: how to ask for a compaction and how to read it back.
type CompactCodec interface {
	EncodeCompact(instructions string) any                       // the wire value that starts it
	DecodeCompact(ev protocol.Event) (CompactEvent, bool)         // a compaction event, from any point in the stream
}
// ContextUsageCodec: a query that answers with the context size.
type ContextUsageCodec interface {
	EncodeContextUsage(reqID string) any
	DecodeContextUsage(body map[string]any) (ContextUsage, error)
}
```

`persistentSession` today hands a control answer back as an error only
(`answerControl`); it gains the answer body so `ContextUsageCodec` can read it.
The pump asks `CompactCodec.DecodeCompact` for every event and emits a
`CompactEvent` (also for auto-compaction, with no command in flight).

### Host-facing API (`agentboot/engine`, what Projects calls)

```go
// Compact compacts session id's conversation and returns what it did. It
// claims the session like Start (ErrBusy if a turn runs), runs the backend's
// compaction as one turn (resident or one-shot), records a compact entry in
// the transcript through the Store, and returns when it ends.
func (e *Engine) Compact(ctx context.Context, id, dir, instructions string, launch Launch) (agentboot.CompactResult, error)

// ContextUsage asks the session's live process for its context size. A session
// with no live process returns ErrNoProcess: the caller falls back to the last
// figure it recorded from a turn's usage.
func (e *Engine) ContextUsage(ctx context.Context, id string) (agentboot.ContextUsage, error)
```

Transcript: one neutral entry kind `compact` (payload = `CompactEvent`), next
to `system`, `usage`, `task`. Projects stores it as `db.EvCommand` (see below).
It never sees a Claude stream shape.

### Claude implementation (`agentboot/claude`, the only one)

| Neutral | Claude Code 2.1.259 |
| --- | --- |
| `EncodeCompact(ins)` | user message `/compact` (+ `" " + ins` when given) |
| `DecodeCompact` started | `system/status{status:"compacting"}` |
| `DecodeCompact` done | `system/compact_boundary` → Trigger=`compact_metadata.trigger`, TokensBefore=`pre_tokens`, TokensAfter=`post_tokens`, Duration=`duration_ms`; Summary=the following `isSynthetic` user message |
| `DecodeCompact` failed | `system/status{compact_result:"failed", compact_error}` |
| `EncodeContextUsage` | control request `get_context_usage` |
| `DecodeContextUsage` | `totalTokens` → Used, `maxTokens` → Max, `autoCompactThreshold` → AutoCompactAt |
| Usage | not reported (`result.usage` is zero): `Reported.Usage=false`. Document, do not guess. |

The converter also drops messages whose uuid was seen before (the replay after
a compaction) and skips the empty `usage` entry for a `num_turns:0` turn. Those
are Claude-implementation details inside `agentboot/claude` and the converter.

## The seam: what already fits, what could follow

The pattern is: a neutral method or event, an optional capability on the
backend transport, `ErrXUnsupported` when absent, the Claude mapping in
`agentboot/claude`. Several calls already follow it. This table is the
candidate surface of "what a backend must provide for Projects". **Only the
first two "new" rows are built by this design.**

| Behaviour | Neutral call today | Claude mapping today | Status |
| --- | --- | --- | --- |
| Send / inject a message | `PersistentSession.Send` / `Inject`, `Engine.Start` / `Inject` | stream-json user message | exists |
| Interrupt a turn, stop a task | `Interrupt`, `StopTask` | `interrupt`, `stop_task` control requests | exists |
| Switch model live | `SetModel` | `set_model` | exists |
| Switch permission mode live | `SetPermissionMode` | `set_permission_mode` | exists |
| Layer permission rules live | `ApplySettings` | `apply_flag_settings` | exists (`ApplySettings` is a bag of Claude settings keys; a future backend-neutral form would be `SetPermissionRules`) |
| Per-turn usage and context fill | `engine.TurnUsage` entry | `result.usage`, last call's prompt size | exists (unit: tokens) |
| Approvals and questions | `ApprovalRequestEvent`, `AskRequestEvent`, `Respond` | `can_use_tool` | exists |
| **Compact the conversation** | `Engine.Compact`, `CompactEvent` | `/compact`, `compact_boundary`, `status` | **new, this design** |
| **Context usage now** | `Engine.ContextUsage` | `get_context_usage` | **new, this design** |
| Rename the backend's session | `SetTitle` | `rename_session` | future |
| Rewind file changes | `Rewind(toMessage)` | `rewind_files` | future |
| Side question without touching the turn | `Ask(q)` | `side_question` | future |
| Reload skills / plugins | `Reload()` | `reload_skills`, `reload_plugins` | future |
| Clear / fork the conversation | `Reset` | `/clear` (changes the session id) | **deliberately not offered**: irreversible, and it moves the backend's session id behind the engine's pinned `--resume` id |
| Model list, account, cost | `Models()`, `Cost()` | `list_models`, `get_usage` | future (Projects prices from the gateway, so cost may never be needed) |

Nothing in the "future" rows is implemented, registered or stubbed.

## Slash commands: a thin front-end

The parser (`projects/internal/project/commands.go`, new) turns a person's
message into an API call. It knows names, not the CLI.

| Typed | Call |
| --- | --- |
| `/compact [instructions]` | `Compact(instructions)` |
| `/context` (phase 4, optional) | `ContextUsage()` |
| `/usage` (phase 4, optional) | the thread's recorded usage, no backend call |

Resolution for `/word …`, first match wins:

1. IM session commands, as today (`new stop session status verbose yolo cd dir project help pair join clear`). Unchanged; they never reach the backend. The existing IM `/clear` is Tingly's own (stop the session, next message starts a new one in a folder chat).
2. A command in the table above → the API call.
3. Anything the backend is known to treat as a command (`init.slash_commands` of the session, or a fixed list when no process runs) that is not in the table (`/clear`, `/model`, `/mcp`, skills…) → **intercepted, not sent**: one line, "`/clear` isn't available here." This closes today's hole where such text runs in the CLI.
4. Anything else (`/etc/hosts is broken`, `/usage-report failed`) → ordinary text. The Claude implementation sends it with a leading space so the CLI cannot swallow it as an unknown command; the transcript keeps what the user typed.

IM keeps rejecting unknown `/foo` itself, so rule 4 matters on the web only. No
name needs a namespace: Tingly's meaning wins and the CLI's same-named commands
are rule 3.

Only a person's text is parsed: `user` events whose source is `web` or `im`.
Thread reports, coordinator-to-thread messages (they open with the coordinator
note), system check-ins and coordinator MCP output never are; the server checks
the source, not the text. Rule 3's list is a Claude detail and so it comes from
the backend (`init.slash_commands`), not from Projects.

## Projects side

- **Dependency**: Projects keeps `*engine.Engine` but calls these two methods through a consumer-side interface declared where it is used (`threads.go`, `coordinator_session.go`):
  ```go
  type sessionControl interface {
      Compact(ctx context.Context, id, dir, instructions string, l engine.Launch) (agentboot.CompactResult, error)
      ContextUsage(ctx context.Context, id string) (agentboot.ContextUsage, error)
  }
  ```
  so tests use a fake. No registry, no plugins.
- **Threads and folder sessions** (`Threads.SendWith → enqueue → begin`): a parsed `/compact` calls `sessionControl.Compact` instead of `engine.Start`. The user's line is stored as a `user` event, then one `command` event is created and updated in place (`UpdateThreadEvent` + `publishEvent`, the pattern the coordinator's live reply uses).
- **Coordinator** (phase 3): `coordinatorSessions.turn` builds an envelope and preamble for every message, so a parsed `/compact` takes a different path: `Compact` on `main.SessionID`. After a compaction the project-context preamble may be gone from the summary; mark the main thread so the next turn re-sends it.
- **IM**: `projectCommand` learns `compact`, then calls the same Projects entry as the web handler; the relay prints the result.
- **While a turn runs**: `Compact` returns `ErrBusy`; Projects holds the command as the existing "after this turn" chip (the CLI would run it afterwards anyway) and runs it when idle. A decision pending → refused with the reason.
- **Event**: `db.EvCommand = "command"`, payload `{name, state: running|ok|failed, trigger, tokens_before, tokens_after, window, duration_ms, summary, reason}`; an auto-compaction (a `CompactEvent` with no command in flight) creates the same card with trigger `auto`.
- **Usage**: if `CompactResult.Usage` is reported it is added to the thread's totals; if not (the Claude backend today), the thread's token total omits the compaction call, which only gateway usage sees. Documented, not worked around.
- **Endpoint**: none new for sending. Phase 4 adds `GET /threads/{id}/commands` for the menu; types via `task codegen`, placeholder until then.

## UX

- **Composer**: typing `/` first shows a small menu of the supported commands for this target (`/compact  optional instructions  Summarise the conversation to free context`). Enter inserts the text; sending is the normal send. No mode picker.
- **Transcript** (one card, updated in place):
  ```
  ◌ Compacting conversation…
  ✓ Compacted  120k → 18k tokens · 12% → 2% · manual · 4.2s   [Summary ▾]
  ⚠ Could not compact: Not enough messages to compact.
  ```
  Figures not reported by the backend are omitted, not shown as 0.
- **Status card**: after a compaction, `Thread.ContextTokens := TokensAfter` (or `ContextUsage().Used` when available) and `ContextWindow := Max`; a command turn records no usage entry, so without this the card keeps the old figure until the next real turn. "Compacting…" shows while a compaction is running, also an automatic one.
- **IM**: `Compacted: 120k → 18k tokens (12% → 2%).`

## Safety

- Parsing and the allow-list are above. A new backend command is inert (rule 3 or 4) until someone adds a row.
- IM: `/compact` needs `remote_control.start`, like `/stop`. Web: same as posting a message (no roles).
- `/compact` is a real model call through the thread's own profile and gateway.
- `/clear` is intercepted everywhere except as the existing IM command.

## Phases

**Phase 0, agentboot: the neutral layer.** `session_control.go` types and capability interfaces; `claude` implements `CompactCodec` and `ContextUsageCodec`; the pump emits `CompactEvent`; the control answer body is kept; the converter writes the `compact` entry, dedupes replayed uuids, skips the empty `usage` for `num_turns:0`; `Engine.Compact` / `Engine.ContextUsage` (resident and one-shot).
Tests: `TestEngine_SlashCommandsToday` (fixture `claude-2.1.259-slash-commands.jsonl`, already committed) flips to expect a compact entry and no stale text or duplicates; a second fixture for `compact_result:"failed"`; a fake backend (a transport without the capability) gives `ErrCompactUnsupported`; `Engine.Compact` while a turn runs gives `ErrBusy`; `ContextUsage` over the fake process; an `e2e`-tagged live test that sends `/compact` to the real CLI.

**Phase 1, Projects: `/compact` in threads and folder sessions, web and IM.** Parser and resolution rules, `sessionControl` calls, `EvCommand` card, status-card refresh, held-while-running.
Tests with a fake `sessionControl`: parser table (every example above, leading space, full-width `／`, group `@bot /compact`; relayed and system text never a command); `/clear` and `/model` are intercepted and never reach the engine; running turn holds the command; IM handler tests in the `harness` style (unknown `/foo` still rejected, IM `/clear` unchanged); frontend component tests for the card states and a showcase-mock screenshot.

**Phase 2, auto-compaction.** The card and "Compacting…" for compactions nobody asked for. Test: a `CompactEvent` with no command in flight; needs a real-model capture of the success path (UNVERIFIED until then).

**Phase 3, coordinator.** The bypass in `coordinatorSessions.turn`, preamble re-send. Tests: a command event never reaches `renderHistory`; normal messages are still enveloped; a relayed or system event containing `/compact` is inert.

**Phase 4, menu and read-only commands.** `/` menu, `GET /threads/{id}/commands`, `/context`, `/usage`, `/help` lists them.

## Defaults taken (from the project owner)

- The coordinator supports `/compact` through the same API (phase 3).
- Compaction tokens count toward the thread's usage if the backend reports them; the Claude backend does not today, which is documented above.
- No real-model capture now. Unverified until then: what `post_tokens` counts, the auto-compaction success events.

Not covered: skills and custom commands, `/rewind`, `/btw`, PreCompact/PostCompact hooks, a user-facing auto-compact setting, any second backend.
