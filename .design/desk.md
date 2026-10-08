# Desk (MVP)

> Audience: contributors touching the web-based Desk surface, or
> `remote/session`, `agentboot`, or `remote/control.Core`. Records why this
> is a thin front door onto machinery @cc already exercises, not a new
> domain model, and what was deliberately left out of this first landing.

---

## 1. What this is

Desk is a web page that drives Claude Code on the machine the
tingly-box server runs on — the same way `claude` at a local terminal does,
just reachable from a browser: pick a folder, describe a task, watch the
turn happen (thinking, tool calls, approvals), answer approvals from the
page, send follow-up messages, stop or archive a session.

It is explicitly a **base to grow from**, not the full proposal that was
drafted earlier (its own Source/Environment/Workspace model, cloned
checkouts, push/PR support). That fuller shape is preserved, unbuilt, on
branch `claude/managed-agent-heavy-v2-backup` for when this path has proven
itself. A second, smaller narrowing (git-clone/branch/push as a *source*
concept) is parked on `claude/managed-agent-git-source-parked`. (Both
branches predate the rename below and keep their old names.)

### Naming

It was first called "Managed Agent". Renamed to **Desk** because that is
another company's product name (Anthropic's Claude Managed Agents) and
because it misdescribes the feature: nothing is hosted or managed on the
user's behalf, it is a place to sit and work with a coding agent on this
machine. One word everywhere (ux-principles.md §3): Go packages
`internal/desk` and `internal/server/module/desk`, routes `/api/v1/desk/...`,
feature flag `desk`, frontend route `/desk`, nav label "Desk". Words
deliberately avoided because tingly-box or its neighbors already use them:
"Agent" (the `/agent/*` scenario pages), "Claude Code" (a nav item, and
agentboot is not Claude-only), "Remote Control" (IM `@cc`), "Coder"
(coder.com, and `RemoteCoder`).

### Off by default

The `desk` scenario flag (`constant.ExtensionDesk`, toggled on
`/system/experimental`) gates it on both sides: the frontend hides the nav
row and route, and the backend's `gate` middleware 404s every
`/api/v1/desk` route while the flag is off, failing closed if the check
itself is missing. "Hidden but reachable" is not an acceptable off state for
a surface that runs a local coding agent on the host.

## 2. Why reuse, not a new domain model

`remote/session.Manager` + `agentboot.AgentService` is exactly what IM's
`@cc` already uses to drive Claude Code remotely (see
`remote/control/remoteagent/executor_claude.go`). A web page asking "run
Claude Code somewhere and let me watch/steer it" is the same problem `@cc`
already solves — a new bespoke Folder/Session/EventLog stack next to it
would be two implementations of one idea, one of them untested in
production.

So `internal/desk.Service` is a second, independent front door built
from the same sanctioned pieces every remote-host entry point uses:

- `remote/control.NewCore(sessionStore)` — builds its own
  `session.Manager` (in-memory cache) over the **same** underlying
  `SessionStore` (`StoreManager.RemoteSessions()`, SQLite index + one
  JSONL transcript file per session) that `@cc` already writes to. No new
  table, no new file format.
- `agentboot.AgentService.Run` (one-shot turns) — the same `Execute` + `RunWithPrompter`
  loop `ClaudeCodeExecutor.Execute` drives: dispatches `MessageEvent` to a
  sink, routes `ApprovalRequestEvent`/`AskRequestEvent` to a `Prompter`,
  and (via `ExecutionOptions.Store`) gets session status transitions
  (`SetRunning`/`SetCompleted`/`SetFailed`) for free.
- `agentboot/pool.Pool` + `AgentService.Open` (persistent turns) — the
  same resident-process mechanism `@cc`'s `persistent_session` setting uses
  (`.design/claude-code-session.md` §5.3); see §3.1.
- `tbclient.TBClient` — the same gateway/profile routing (`GetClaudeCodeEnv`
  for the main scenario, `GetClaudeCodeSettingsPathForProfile` for a
  profile's `--settings`) `@cc` uses; see `.design/remote-cc-profile.md`.

Every web session gets a fixed `ChatID = "web"` (`webChatID` in
`service.go`) so `ListByChat`/`SnapshotsByChat` show only the web's own
sessions, the same way one IM chat ID scopes `@cc`'s.

### The one schema change: `session.Message` grew three optional fields

The web UI wants a turn's actual structure (a tool call and its result, an
approval question and its answer, "thinking" text) — not just the two-line
chat summary a text-only IM consumer needs. Rather than a parallel
event-log format, `remote/session.Message` gained three additive, optional
fields:

```go
type Message struct {
    Role, Content, Summary string
    Timestamp time.Time

    Kind      string          `json:"kind,omitempty"`       // "", "thinking", "tool_use", "tool_result", "approval_request", "approval_response", "ask_request", "ask_response", "system", "error"
    RequestID string          `json:"request_id,omitempty"` // correlates a request with its response
    Payload   json.RawMessage `json:"payload,omitempty"`    // kind-specific structured data
}
```

`Kind == ""` is still a plain chat message — every line `@cc` has already
written to disk decodes unchanged. `internal/desk/convert.go`
(agentboot Claude messages → transcript entries) and `prompter.go`
(approval/ask round trip) are the only writers of the new `Kind` values.

## 3. What `internal/desk.Service` actually does

One package, no sub-packages, built directly on the pieces above:

| concern | how |
|---|---|
| pick a folder | `RecentFolders` derives a live list from past web sessions (no separate CRUD store — "recent" just means "used", and nothing here ever scans the filesystem: every path shown is one a session already started in) |
| start a session | `CreateSession` → `session.Manager.CreateWith` + `startTurn`, which runs the turn in a goroutine (persistent if possible, see §3.1) |
| drive a turn | `turn.go`'s `runTurn`: converts agentboot events to transcript messages (`convert.go`), routes approvals through a per-turn `webPrompter` (`prompter.go`) that appends the question to the transcript and blocks for `Respond` |
| steer | `SendMessage` (next turn, `Resume: true`), `Respond` (answer a pending approval/ask), `Interrupt` (cancel the turn's context; the session is left `Completed`+resumable, not `Failed`), `SetPermissionMode` (next turn's `--permission-mode`) |
| end | `Archive` closes the session for good and closes its resident process, if any — **nothing on disk is touched**: no clone, no directory the tool created, so there is nothing to delete. Archiving only stops tingly-box from resuming that session id |

There is deliberately no server-side directory browser (no `fs/dirs`-style
endpoint) and no git diff/status endpoint in this first landing — see §6.

### Browser project shortcuts

Desk's **Add project** action saves an existing absolute directory path in
this browser's local storage, independently of task creation. The sidebar
shows saved projects even before their first task. Adding a project opens
the task form with its directory selected; it does not create a directory
or launch an agent. Removing an empty project shortcut only removes that
browser entry. Other browsers do not share these shortcuts.

The task form exposes the directory above the prompt and combines saved
projects with directories returned by the existing recent-folders API.
The existing task-start API remains responsible for filesystem validation;
there is no new backend project entity or endpoint. The configuration menu's
empty profile means **Default configuration profile**, using the existing
main Claude Code configuration.

Desk uses the shared `fontMono` and `fontSizes` tokens for directory paths
and small labels. Status indicators use the fixed `getStatusColor` set;
status text and icons use `getReadableAccent` for the current light/dark
surface. Custom expandable rows follow the native cursor convention from
the shared theme, while directory inputs remain selectable.

Concurrency: several sessions may run in the same folder at once, the same
way several local `claude --session-id <id>` processes can — `Service.runs`
is keyed by session id, not by folder.

### 3.1 Persistent turns

`turn.go`'s `runPersistentTurn` mirrors `ClaudeCodeExecutor.runPersistentTurn`
(`.design/claude-code-session.md` §5.3), with its own `pool.Pool` instance (10
sessions, 10 min idle — the same config as imbot's) keyed by session id,
which is already globally unique:

- **Acquire or Open, then Send.** A session's later messages reuse its live
  process instead of spawning one per message.
- **Fallback contract.** `handled=false` (nothing was sent yet: `Open` or the
  pool's bookkeeping failed) falls back to a one-shot `Run`. `handled=true`
  means the turn was driven, so an error is final and is never retried
  one-shot. A crash mid-turn also drops the dead process from the pool.
- **Launch settings are fixed per process.** A live process cannot change
  its `--permission-mode` or env, so `Service.launch` remembers each resident
  process's launch signature (permission mode + sorted gateway env; the env
  is built from a map, hence the sort). A mismatch on the next message
  closes it and reopens with `--resume`. Without this, changing the mode on
  the page would be silently ignored until the pool evicted the process.
- **Status is driven directly.** `ExecutionOptions.Store` only applies to
  the one-shot `Run`, so the persistent path calls `SetRunning`/
  `SetCompleted`/`SetFailed` itself.

### 3.2 Process lifecycle

- **Server stop.** `Server.Stop` calls `Service.Shutdown`: it cancels
  in-flight turns and shuts the pool down, so no `claude` process outlives
  the server holding its session file open (`.design/claude-code-session.md` §5.4).
- **Restart recovery.** `NewService` marks web sessions still `running` or
  `pending` in the store as completed-and-resumable, with a system note in
  the transcript. No turn can be in flight in a process that just started,
  so that status is necessarily stale, and `session.Manager`'s cleanup skips
  running sessions, so without this they would read as running forever.
  Only `ChatID == "web"` sessions are touched, never `@cc`'s.
- **Construct once, and only in the running server.** Because of the
  recovery step, building a `Service` rewrites stored sessions. See §4 for
  why schema generation must not build one.

### 3.3 Profiles

A session can run with a Claude Code profile (`Session.Profile`, a
`remote_sessions.profile` column). Its turns launch with the profile's
materialized `--settings` file (`Routing.GetClaudeCodeSettingsPathForProfile`)
instead of the main scenario's env, the same either/or @cc's
`ClaudeCodeExecutor` uses, and fall back to the main routing with a system
note if the profile can't be resolved. Create and `SetProfile` reject an
unknown profile up front. The settings path is part of the launch signature
(§3.1) together with a hash of the file's content, so switching or editing
a profile restarts the resident process with `--resume` on the next turn.

**Model.** Beside the profile, the composer always names the model the
session runs on (ux-principles.md §5), and profile and model are shown the
same way for every profile. The tiers come from the standard scenario API,
`GET /scenario/claude_code/models?profile=` (scenario module, not Desk: what
a profile offers is profile knowledge, reusable by the profile pages and the
CLI). It reads them from the env Claude Code is actually given
(`ANTHROPIC_MODEL` and `ANTHROPIC_DEFAULT_{OPUS,SONNET,HAIKU}_MODEL`, from the
main env or the profile's resolved settings — resolved, not materialized, so
the GET writes nothing) through `agent.ClaudeCodeTiersFromEnv`, each with its
predicted route (`statusline.PreviewRoute`). Desk only owns the session's
choice (`PUT /desk/sessions/:id/model`) and validates it with the same
helper:

- A **separate** profile (tiers differ) lets the session pick a tier
  (`Session.Model`, a `remote_sessions.model` column), passed to Claude Code
  as `--model opus|sonnet|haiku`; "" is `ANTHROPIC_MODEL`. The tier is part
  of the launch signature.
- A **unified** profile (every tier the same model) shows its one model with
  no menu, and the backend rejects a tier. Changing that model means editing
  the profile's rules, which would change every client using the profile,
  so it is not offered from a session (ux-principles.md §12). Switching a
  session to a unified profile clears its tier.

Picking an arbitrary provider model per session is deliberately not offered:
it would bypass the rules' load balancing and quota fallback, and become a
second control over the same thing the profile decides.

### 3.4 Status line

The composer carries the web version of the status line tingly-box
installs for Claude Code in a terminal (`internal/server/module/statusline`):
requested model → routed model @ provider, context use, session tokens, and
the routed provider's quota and balance.

- **Tokens** come from a `usage` transcript entry the converter writes per
  turn (`turnUsage` in `convert.go`): calls deduplicated by message id and
  summed, as the gateway returned them; context use from the last
  main-thread call; model and window from the init message and the result's
  `modelUsage`. Claude Code's own cost figure is left out: it prices every
  call at Anthropic list prices, which is wrong once a profile routes
  elsewhere. Being transcript entries, they persist without a schema change.
- **Routing and quota** come from `GET /desk/sessions/:id/status`, which
  resolves the chosen tier's model (else the latest turn's requested model) in the session's scenario
  (`claude_code` or `claude_code:<profile>`) through the statusline
  handler's own `ResolveRoute`, and renders quota windows with the same
  `QuotaSegments` the terminal line uses, so the two never disagree. It is
  a prediction of the load balancer's pick, as the terminal line is, made
  with `PreviewService` so a status read never claims a recovering
  provider's half-open probe slot.
- A window at 80% is marked, one at 90% or exhausted is red, and an
  exhausted one points at the profile picker beside it.

### 3.5 Handoff to a terminal

`POST /desk/sessions/:id/handoff` returns `cd '<folder>' && '<tingly-box>'
cc --resume '<id>'` (`… profile '<profile>' …` for a profile), all
shell-quoted. `<tingly-box>` is the server's own executable by absolute path
(`os.Executable`, symlinks resolved), so the command works when tingly-box
isn't on PATH, e.g. under npx. Going through tingly-box rather than bare `claude` keeps the
terminal on the same gateway routing the web turns used, with no token in
the command. It refuses during a turn, and claims the session while it
releases the resident process, so a turn can't start in between and two
processes never write one Claude session file. A later message from the web
starts a new resident process with `--resume`, so the page tells the user to
use one place at a time rather than trying to lock either side.

`SessionInfo.awaiting_input` is true while an approval or question is open
(`webPrompter.hasPending`), so the list can say "waiting" without loading
every transcript.

### 3.6 A shared-mutable-state gotcha this surfaced

`session.Manager.Get`/`GetOrLoad`/`ListByChat` return the **live**,
mutably-shared `*Session` held in the manager's map — safe only if read
before any other goroutine can call `Update` on the same id. A web request
returning that pointer to its HTTP caller while a turn's background
goroutine concurrently calls `SetRunning`/`SetCompleted` is a real data
race (caught by `go test -race` once this package's tests exercised that
concurrency `@cc`'s own call sites hadn't). Fixed by adding
`Manager.Snapshot`/`SnapshotOrLoad`/`SnapshotsByChat` — same lookups, but
return a copy taken under the manager's lock — and using those instead of
`Get`/`GetOrLoad`/`ListByChat` everywhere a `Session` value crosses out of
`Service` to a caller.

### 3.7 Resident processes are read for their whole life

A resident process does not only speak during Desk's turns. Verified against
Claude Code 2.1.282 in stream-json mode (a scripted fake Messages API in
front of the real CLI):

- **Background work reports between turns.** Subagents run in the
  background by default (`Agent`'s `run_in_background` defaults to true),
  and `Bash` can too. Their progress arrives as `system` messages
  (`background_tasks_changed`, `task_started`, `task_progress`,
  `task_updated`, `task_notification`) whenever it happens — including after
  the turn that started them has ended.
- **Claude Code starts turns by itself.** When a background task finishes
  after its turn, the CLI hands the notification to the model and runs a
  whole turn (`system/init` … `result`) with no user message.
- **The host can stop a turn or a task without the process.** An
  `interrupt` control_request ends only the in-flight turn (its result is
  `error_during_execution`) and the process takes the next message
  normally; `stop_task {task_id}` stops one background task
  (`task_notification` status `stopped`). Neither is in the public docs.

`RunTurnWithPrompter` read events only while the caller's turn was in
flight, so everything above sat in the session's buffer and the next message
read it as its own turn — including the unsolicited turn's `result`, which
ended that message's turn before it started. Hence `agentboot.Conductor`:
one reader per process lifetime (`resident.go`), created right after `Open`.

- **Every message is recorded as it arrives**, in or between turns, and
  keeps the pool from evicting a process that is still reporting.
- **An unsolicited turn is a turn.** The session reads as running with a
  note saying why, gets a `run` (so its approvals, Stop and the "busy"
  check behave as usual), and settles when its result arrives. A message
  sent meanwhile is refused with 409, which the page queues.
- **Stop interrupts, it no longer kills.** Stop sends `interrupt`, so the
  process and its background tasks survive and the next message reuses the
  process. Only an agent that can't, or doesn't within the grace period, is
  closed as before.
- **The one-shot fallback can't keep background work**: its process exits
  with the turn. Background tasks need the pooled path.

### 3.8 Subagents and task events in the transcript

Two things the converter now keeps instead of flattening or dropping:

- **Attribution.** Claude Code tags a subagent's messages with
  `parent_tool_use_id` (the `Agent` call that runs it). Transcript entries
  carry it as `Message.Parent` (JSON `parent`, omitted for the main
  conversation; the transcript is an append-only JSONL file, so old
  transcripts read unchanged). Tool results don't carry the id, so a result
  inherits its call's parent.
- **Task lifecycle.** `task_started`, `task_progress`, `task_updated`,
  `task_notification` and `background_tasks_changed` become `task` entries
  whose `RequestID` is the tool call that started the task (the Agent call,
  or a backgrounded `Bash`); `task_updated`, which names only the task, is
  mapped back through the task id. The payload (`taskEvent`) keeps only what
  the page shows: subagent type, background flag, current action, status,
  summary, output file and usage (tokens, tool uses, time).

`convert_test.go` replays a captured 2.1.282 run
(`testdata/claude-2.1.282-agents-and-background.jsonl`: a background
subagent, a background command, and a foreground subagent that runs a
command of its own) through the real accumulator, so a CLI change that
moves these fields fails there first.

On the page (`buildTranscript`), each `Agent` call becomes an `agent` block:
a card with the subagent's description and type, a background tag, its
state (running with its current action, done, stopped, failed) and usage,
and — expanded — the prompt, everything it did (its own activity rows) and
its report (its last reply). Its entries never reach the main column, so the
conversation reads as the main agent's. A foreground run still "running"
after its turn ended was cut off with the turn and reads as stopped. A
backgrounded command's step carries a `background · <state>` tag.

### 3.9 Background tasks

What a session runs in the background (a `Bash` or subagent started with
`run_in_background`) outlives the turn that started it, so it gets its own
place rather than living only in the transcript:

- **Live set** (`tasks.go`). `background_tasks_changed` carries the full set
  of running tasks; the Service keeps the latest per session and reports it
  as `SessionInfo.background_tasks`. The set dies with its process: the
  resident's `OnTerminated` clears it, and so does the end of a one-shot
  turn. The page trusts it over the transcript, so a task whose process went
  away without a final event (a restart, an archive) reads as ended, not
  running forever.
- **Kept alive.** A process with background tasks is idle between turns, and
  a long command can go minutes without an event, so the pool's idle sweep
  would reclaim it and end its tasks. While a session has live tasks it is
  touched every minute.
- **Stop one task**: `POST /desk/sessions/:id/tasks/:task_id/stop` sends
  `stop_task`; the task's "stopped" notification settles it.
- **Read output**: `GET /desk/sessions/:id/tasks/:task_id/output` returns the
  tail of its output file (a command's stdout and exit code). The path is
  the one Claude Code reported for that task in the call's result ("Output
  is being written to: …"), recorded as an `output_file` task event; only a
  file shaped `…/tasks/<task_id>.output` is read.

- **Finished tasks stay.** A command's output file is temporary (Claude
  Code's temp dir), so when a command finishes the Service copies the last
  8 KiB into the transcript (an `output_snapshot` task event), and the page
  reads a finished command from that copy. Subagents need no copy: their
  work is already in the transcript entry by entry.

On the page: a header entry, always present so it can be found before it
is needed (an icon, whose empty state says what will appear there), turns
into a labeled "N running" pill while work runs, and opens the list in two groups — Running, and Finished (newest first, kept
for looking back) — each row with its state and elapsed time. A row opens into what the task is: a
command shows its command line and its output (loaded on open, refreshed
every 2 s while it runs, exit code included once done); a subagent shows its
type, usage, what it was asked, what it is doing now, its latest steps and
its latest reply or report. Every row can Stop (while running) and "Show in
conversation", which scrolls to the call that started it and flashes it. The
sidebar marks a session with running background work (quieter than a
running turn: nothing waits on the user). Archive and Continue in terminal
end the process, so with tasks running they ask first; a profile, model or
permission change says that the next message's restart will stop them. A
session with live tasks is polled like one mid-turn, since that is when
their progress, and the turn Claude starts when one finishes, arrive.

## 4. HTTP surface

`internal/server/module/desk` is a thin adapter: request/response
DTOs with `json` tags (not `session.Session`/`session.Message` directly,
which don't follow this codebase's naming convention), one handler method
per `Service` method, errors mapped from `Service`'s three sentinels
(`ErrNotFound` → 404, `ErrValidation` → 400, `ErrConflict` → 409). Routes
live under `/api/v1/desk/...`; see `routes.go` for the full list.

Both route registrations (`server_control.go`'s `UseUIEndpoints` for the
running server, `swagger.go`'s `registerAllAPIRoutes` for `openapi.json`
generation) go through one `registerDeskRoutes`, so the route set and its
gate cannot drift between them. Only the running server builds the service
(`newDeskService`, which builds its own `control.NewCore(sm.RemoteSessions())`
following the one-core-per-entry-point pattern `imbot.NewBotManager` uses);
schema generation registers the routes with a nil service. Schema generation
can see the real session store, so building a `Service` there would run the
restart recovery (§3.2) against a live server's sessions.

## 5. Frontend

`frontend/src/pages/desk/DeskPage.tsx` (+ `frontend/src/components/desk/*`)
follows the layout of Claude Code on the web, so it reads as the same kind
of tool:

- **Sidebar** (`DeskSidebar`): "New session", a search box, and sessions
  grouped by folder (most recent first), each group with a "+" that starts a
  new session in that folder. A row is the session's first prompt plus a
  mark only when it needs attention: an amber "waiting" pill when an
  approval or question is open, a spinner while running, a red dot when
  failed, and a dot (plus a bold title) for a turn that finished while the
  user was elsewhere; archived sessions are dimmed.
- **Attention** (`useDeskAttention`): while the tab is in the background, a
  browser notification when a session starts waiting on the user or a turn
  ends (permission is asked on the first send, a user gesture); the tab title
  carries a `(n)` count of waiting and unseen sessions. Both are scoped to
  the page and undone when it unmounts (ux-principles.md §12).
- **New session** (`NewSessionView`): opens straight onto the prompt, no
  wizard (ux-principles.md §2). Folder and permission mode are context on
  the composer, prefilled with the folder used last. `FolderPicker` is
  typed input + recently-used paths only, matching the backend having no
  directory-browsing endpoint (§6).
- **Session** (`SessionView` + `Transcript`): a title bar (first prompt,
  folder chip with the full path on hover, archive), one centered
  conversation column, and the composer pinned below it. `buildTranscript`
  (`deskUtils.ts`) collapses each run of thinking and tool calls into one
  "Used N tools" row, pairing every call with its result by `request_id`,
  and attaches an approval's or question's answer to it, so the replies stay
  the visual anchor (ux-principles.md §9). Only the one request still waiting
  on a live turn is actionable (§11); an answered one collapses to a line.
  Replies render as Markdown through `@ant-design/x-markdown` (already used
  by the Skills page) with `escapeRawHtml` on, so HTML in a model's output is
  shown as text, never rendered; fenced code goes through `CodeBlock`.
- **Composer** (`Composer`): Enter sends, Shift+Enter adds a line, and an
  IME composition's Enter never sends. While a turn runs, Stop shows and
  the input stays open: a message sent then is queued above the composer
  and all queued ones go out as one message when the turn completes. ✕ on a
  queued one puts it back into the input; Stop puts them all back (changing
  course, as in the terminal); after a failed turn they're held with "Send
  now". Drafts and queues are per session and survive switching sessions.
  Accepted sends clear only the unchanged submitted text: writing the next
  message during a slow send keeps that new draft. A synchronous submission
  guard prevents repeated Enter events from submitting twice.
- **Title bar actions**: expand all tool calls (remembered in
  `localStorage`, each row still toggles on its own) and "Continue in
  terminal" (§3.5), which copies the command and keeps it on screen with its
  own copy button, since a copy right after a request can be refused.
- **Narrow screens**: the list and the session are two views, with a back
  button in the session's title bar.

Mock mode (`src/mocks/deskHandlers.ts`) serves every state above (a turn
waiting on approval, finished, failed, archived) and simulates turns, so the
page can be previewed and screenshotted without a backend.

### 5.1 Local daily use: refresh and draft recovery

- Refreshes are scheduled after the preceding request settles, rather than
  overlapping on an interval. The selected transcript and session refresh
  every 1.5 s while active and every 5 s while idle; the list refreshes every
  5 s. Idle refresh matters after an initial failure, a server restart or
  terminal handoff. Returning to the tab or going online refreshes immediately.
- Read requests time out after 15 s and are aborted on poll disposal.
  Background read failures keep the last loaded data and show one inline
  Retry action. Successful reads clear their own failure; no toast is emitted
  on every failed poll. Mutation failures still notify the user and keep input.
- A transcript is tagged with its session id. Switching sessions immediately
  hides the previous transcript, and request generations discard old responses,
  including switching away and back to the same id. Session row generations
  prevent an older list response from rolling back a fresher row or setting.
  The session view remounts on selection so tool panels and confirmations
  cannot carry over into another session. Unknown session links show an
  explicit unavailable state instead of silently opening a new-task form.
- Unsent session drafts and new-task forms live in tab-scoped `sessionStorage`
  (`useDeskDrafts`). New-task forms remember prompt, folder, profile, model and
  permissions together, separately for each folder entry point. Acceptance
  clears only the matching saved prompt, even if navigation unmounted the form.
  Shared tab stores keep late acceptance callbacks connected to newer mounted
  drafts and in-flight queues, without overwriting subsequent edits.
  Corrupt, blocked or full storage falls back to memory. Queues also persist
  in tab-scoped storage (`useDeskQueues`), including text in flight. Only an
  accepted prefix is removed. Restored queues are held for explicit review and
  sending: a reload cannot prove whether the previous POST was accepted.
  Failed turns/sends, interrupt and terminal handoff hold automatic delivery.
- Mutation acceptance releases the input independently of background reads.
  Accepted sends immediately mark the session pending, so the next draft joins
  its queue until refreshed state arrives. Manual reads also have the 15 s
  deadline. Profile/model/permission edits serialize across the three controls
  and temporarily disable sending while their launch settings are being saved.
- Approvals show the full tool input, preserve question drafts and serialize
  responses. Accepted responses stay disabled while waiting for the transcript;
  failed responses retain the answer and permit retry. Polling never autofocuses
  a new question. An explicit Review action scrolls only the conversation.
- Conversation scrolling follows live output and delayed Markdown layout only
  while already at the bottom. Back to latest and new activity affordances keep
  older messages readable. The Desk surface fits the dynamic viewport, with
  independent transcript scrolling, bounded queues and a compact mobile menu.
  Mobile Enter adds a line; Ctrl/Cmd+Enter or the Send button sends.
- The sidebar separates active and archived sessions while retaining search.
  Archive confirms that the current turn/tasks stop, guards duplicate requests,
  and keeps history and unsent text accessible for copying or taking back.
- Desk's conversation, task and setup strings are defined in English, Chinese
  and Russian, including plural tool counts and the feature-enabling notice.

- **Trajectory** (`Trajectory.tsx`, `trajectoryRows.ts`): a second view of the
  same transcript, for reviewing what the agent did rather than reading
  what it said (modeled on DeepSeek Harness's Trajectory tab). It adds no
  data and no API: `trajectoryRows` projects the `buildTranscript` blocks to
  one row per action — user message, tool call (name + `toolSummary`, failed
  or not), subagent (its own steps indented one level), approval/question
  with its answer, error, and a reply's first line; thinking gets no row.
  A summary above (files written by Edit/Write, commands, failures,
  approvals) doubles as filters. There is no inspector: a row switches to
  Chat and flashes its message (`data-call-ids` / `data-request-id` /
  `data-block` anchors), since Chat already renders the detail well. The
  Chat / Trajectory switch sits beside the title (in the ⋮ menu on narrow
  screens) and is remembered. Left out on purpose: served model, routing,
  tokens per step, timing bars.

Regression coverage: Composer, new-task forms, draft storage, polling lifecycle
and the Desk page's selection/reconnect/error flows have frontend tests. These
exercise actual React state and deferred read/send promises, alongside the
existing transcript and attention unit tests. Real Claude Code/gateway execution
still needs a configured local installation to validate end to end.

Nav: one row ("Desk") alongside "Remote Control" and "IM Notify"
under the existing "Remote" rail icon in `layout/useActivityItems.tsx` — a
new purpose on the same product pillar, not a new top-level domain.

## 6. Deliberately out of scope for this landing

- No clone/checkout, no branch/push/PR — the agent works in the folder in
  place, same as local `claude`. (Parked: `claude/managed-agent-git-source-parked`.)
- No separate Folder/Workspace entity with its own lifecycle — folders are
  derived from session history only.
- **No server-side directory browser.** An early draft of this landing had
  a `GET /desk/fs/dirs` endpoint (list the subdirectories of any
  absolute path, no allowlist — the same trust model @cc's own IM directory
  picker already uses) plus a `FolderPicker` browse popover on top of it.
  Cut on review: it is a new "list arbitrary filesystem paths on request"
  surface, and this first landing does not need it — typing a path, or
  reusing one already used (`RecentFolder`), is enough to get started. If
  directory browsing comes back, it should have an explicit, reviewed
  security stance (e.g. scoped to configured roots) rather than reusing
  @cc's allowlist-free precedent by default.
- **No git diff/status endpoint.** Same review: a `GET .../diff` (read-only
  `git diff`/`status` in the folder) plus a "Changes" panel existed in the
  same draft. Seeing what changed isn't on the create→chat→approve
  critical path, so it's cut for now; the person can just look at their
  own working tree. Both this and the directory browser's code briefly
  existed on this branch (`gitdiff.go`, `fsbrowse.go`) and are easy to
  reintroduce from history if wanted later.
- No live streaming transport (SSE/WS) — the frontend polls
  (`DeskPage.tsx`'s `SESSIONS_POLL_MS`/`MESSAGES_POLL_MS`), fast
  while the selected session has a turn or background tasks running.
- Tests: `internal/desk`'s race-checked service suite (fake agent and
  sessions), `persistent_e2e_test.go` (the real `claude.Agent` driver and
  pool with a fake process in place of the binary), and a route test for
  the flag gate. Nothing yet runs the real `claude` binary, and there are no
  real-CLI frontend integration tests. Frontend recovery and interaction
  coverage is described in §5.1.
