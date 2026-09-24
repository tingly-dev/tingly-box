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
  (`.design/claude-code.md` §5.3); see §3.1.
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

Concurrency: several sessions may run in the same folder at once, the same
way several local `claude --session-id <id>` processes can — `Service.runs`
is keyed by session id, not by folder.

### 3.1 Persistent turns

`turn.go`'s `runPersistentTurn` mirrors `ClaudeCodeExecutor.runPersistentTurn`
(`.design/claude-code.md` §5.3), with its own `pool.Pool` instance (10
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
  the server holding its session file open (`.design/claude-code.md` §5.4).
- **Restart recovery.** `NewService` marks web sessions still `running` or
  `pending` in the store as completed-and-resumable, with a system note in
  the transcript. No turn can be in flight in a process that just started,
  so that status is necessarily stale, and `session.Manager`'s cleanup skips
  running sessions, so without this they would read as running forever.
  Only `ChatID == "web"` sessions are touched, never `@cc`'s.
- **Construct once, and only in the running server.** Because of the
  recovery step, building a `Service` rewrites stored sessions. See §4 for
  why schema generation must not build one.

### 3.3 A shared-mutable-state gotcha this surfaced

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
  mark only when it needs attention (spinner while running, red dot when
  failed); archived sessions are dimmed.
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
  IME composition's Enter never sends. While a turn runs the send button
  becomes Stop.
- **Narrow screens**: the list and the session are two views, with a back
  button in the session's title bar.

Mock mode (`src/mocks/deskHandlers.ts`) serves every state above (a turn
waiting on approval, finished, failed, archived) and simulates turns, so the
page can be previewed and screenshotted without a backend.

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
  only while a turn is actually running.
- Tests: `internal/desk`'s race-checked service suite (fake agent and
  sessions), `persistent_e2e_test.go` (the real `claude.Agent` driver and
  pool with a fake process in place of the binary), and a route test for
  the flag gate. Nothing yet runs the real `claude` binary, and there are no
  frontend tests.
