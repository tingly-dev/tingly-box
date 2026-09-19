# Managed Agent (MVP)

> Audience: contributors touching the web-based Managed Agent surface, or
> `remote/session`, `agentboot`, or `remote/control.Core`. Records why this
> is a thin front door onto machinery @cc already exercises, not a new
> domain model, and what was deliberately left out of this first landing.

---

## 1. What this is

Managed Agent is a web page that drives Claude Code on the machine the
tingly-box server runs on — the same way `claude` at a local terminal does,
just reachable from a browser: pick a folder, describe a task, watch the
turn happen (thinking, tool calls, approvals), answer approvals from the
page, send follow-up messages, stop or archive a session.

It is explicitly a **base to grow from**, not the full proposal that was
drafted earlier (its own Source/Environment/Workspace model, cloned
checkouts, push/PR support). That fuller shape is preserved, unbuilt, on
branch `claude/managed-agent-heavy-v2-backup` for when this path has proven
itself. A second, smaller narrowing (git-clone/branch/push as a *source*
concept) is parked on `claude/managed-agent-git-source-parked`.

## 2. Why reuse, not a new domain model

`remote/session.Manager` + `agentboot.AgentService` is exactly what IM's
`@cc` already uses to drive Claude Code remotely (see
`remote/control/remoteagent/executor_claude.go`). A web page asking "run
Claude Code somewhere and let me watch/steer it" is the same problem `@cc`
already solves — a new bespoke Folder/Session/EventLog stack next to it
would be two implementations of one idea, one of them untested in
production.

So `internal/managedagent.Service` is a second, independent front door built
from the same sanctioned pieces every remote-host entry point uses:

- `remote/control.NewCore(sessionStore)` — builds its own
  `session.Manager` (in-memory cache) over the **same** underlying
  `SessionStore` (`StoreManager.RemoteSessions()`, SQLite index + one
  JSONL transcript file per session) that `@cc` already writes to. No new
  table, no new file format.
- `agentboot.AgentService.Run` — the same `Execute` + `RunWithPrompter`
  loop `ClaudeCodeExecutor.Execute` drives: dispatches `MessageEvent` to a
  sink, routes `ApprovalRequestEvent`/`AskRequestEvent` to a `Prompter`,
  and (via `ExecutionOptions.Store`) gets session status transitions
  (`SetRunning`/`SetCompleted`/`SetFailed`) for free.
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
written to disk decodes unchanged. `internal/managedagent/convert.go`
(agentboot Claude messages → transcript entries) and `prompter.go`
(approval/ask round trip) are the only writers of the new `Kind` values.

## 3. What `internal/managedagent.Service` actually does

One package, no sub-packages, built directly on the pieces above:

| concern | how |
|---|---|
| pick a folder | `RecentFolders` derives a live list from past web sessions (no separate CRUD store — "recent" just means "used", and nothing here ever scans the filesystem: every path shown is one a session already started in) |
| start a session | `CreateSession` → `session.Manager.CreateWith` + `startTurn`, which runs `agentboot.AgentService.Run` in a goroutine |
| drive a turn | `turn.go`'s `runTurn`: converts agentboot events to transcript messages (`convert.go`), routes approvals through a per-turn `webPrompter` (`prompter.go`) that appends the question to the transcript and blocks for `Respond` |
| steer | `SendMessage` (next turn, `Resume: true`), `Respond` (answer a pending approval/ask), `Interrupt` (cancel the turn's context; the session is left `Completed`+resumable, not `Failed`), `SetPermissionMode` (next turn's `--permission-mode`) |
| end | `Archive` closes the session for good — **nothing on disk is touched**: no clone, no directory the tool created, so there is nothing to delete. Archiving only stops tingly-box from resuming that session id |

There is deliberately no server-side directory browser (no `fs/dirs`-style
endpoint) and no git diff/status endpoint in this first landing — see §6.

Concurrency: several sessions may run in the same folder at once, the same
way several local `claude --session-id <id>` processes can — `Service.runs`
is keyed by session id, not by folder.

### A shared-mutable-state gotcha this surfaced

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

`internal/server/module/managedagent` is a thin adapter: request/response
DTOs with `json` tags (not `session.Session`/`session.Message` directly,
which don't follow this codebase's naming convention), one handler method
per `Service` method, errors mapped from `Service`'s three sentinels
(`ErrNotFound` → 404, `ErrValidation` → 400, `ErrConflict` → 409). Routes
live under `/api/v1/managed-agent/...`; see `routes.go` for the full list.

Wired in two places that must stay in sync (`internal/server/server_control.go`'s
`UseUIEndpoints` for the running server, `internal/server/swagger.go`'s
`registerAllAPIRoutes` for `openapi.json` generation) — both build their own
`control.NewCore(sm.RemoteSessions())`, following the same
one-core-per-entry-point pattern `imbot.NewBotManager` already uses.

## 5. Frontend

`frontend/src/pages/managed-agent/ManagedAgentPage.tsx` (+
`frontend/src/components/managed-agent/*`): one work surface, no
folder-then-session wizard (ux-principles.md §2) — a folder/prompt composer
and the session list share the left column; the right column is the
selected session's transcript and composer. `FolderPicker` is a plain
Autocomplete over typed input + recently-used paths (`RecentFolder[]`) —
no directory-browsing UI, matching the backend having no such endpoint. A
session's one still-open approval/ask request is the only one that renders
action buttons (`findPendingRequest` in `managedAgentUtils.ts`), so the page
always shows exactly what the user can act on next (ux-principles.md §11).

Nav: one row ("Managed Agent") alongside "Remote Control" and "IM Notify"
under the existing "Remote" rail icon in `layout/useActivityItems.tsx` — a
new purpose on the same product pillar, not a new top-level domain.

## 6. Deliberately out of scope for this landing

- No clone/checkout, no branch/push/PR — the agent works in the folder in
  place, same as local `claude`. (Parked: `claude/managed-agent-git-source-parked`.)
- No separate Folder/Workspace entity with its own lifecycle — folders are
  derived from session history only.
- **No server-side directory browser.** An early draft of this landing had
  a `GET /managed-agent/fs/dirs` endpoint (list the subdirectories of any
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
  (`ManagedAgentPage.tsx`'s `SESSIONS_POLL_MS`/`MESSAGES_POLL_MS`), fast
  only while a turn is actually running.
- No tests beyond `internal/managedagent`'s service-level suite
  (`service_test.go`, race-checked) — no HTTP handler tests, no frontend
  tests, no e2e journeys yet.
