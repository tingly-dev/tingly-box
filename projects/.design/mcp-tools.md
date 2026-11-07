# Projects MCP tools: one action layer, three callers

> Status: **proposed** (2026-11-07). Tracking issue: the TEP "Projects: move more actions into MCP tools" (link in the issue body, which links back here).
> Builds on [coordinator-session.md](./coordinator-session.md) (how the coordinator's MCP server is scoped and serialised), [projects-published-files.md](../../.design/projects-published-files.md) (`send_file`) and [message-channel.md](./message-channel.md) (`set_progress`).
> Related, not yet on `main`: `.design/slash-commands.md` (session control API: `Compact`, `ContextUsage`; PRs #2454–#2456), `pr-issue-awareness.md` and `branch-tracking.md` (`link`, `set_branch`; branches `tp/design-branch-pr-and-issue-aware-5b169f`, `tp/design-branch-and-base-tracking-4a5e55`).
> Nothing here is implemented yet. Every `file:line` below is on `main` at `d7b849ccc`.

## Why

The coordinator and every thread are Claude Code sessions, so the only thing they can *do* to Projects is call MCP tools. Today the coordinator has 13 business tools plus 2 shared-file readers, and a thread has exactly one (`send_file`). Meanwhile the web UI reaches dozens more actions through HTTP. When the user says "stop that thread", "put the others on hold, only two at a time", "merge it, it looks fine" or "compact thread X", the coordinator can only answer "I can't do that, please click…". That is a UX gap, not a missing feature: the user is already talking to the coordinator in a chat (often an IM chat with no UI at all).

The recent direction (see `slash-commands.md`) is to put each interaction behaviour behind a small, standard action, so we learn what the layer underneath must provide. This design applies that to Projects itself: **one action layer in `project`, and the web UI, IM commands and MCP tools are thin callers of it.**

## Inventory (what exists)

### MCP tools today

| Tool | Who | Where |
| --- | --- | --- |
| `start_thread` | coordinator | `project/coordinator.go:488`, run at `coordinator.go:587` |
| `propose_threads` | coordinator | `coordinator.go:489`, `:596` |
| `request_decision` | coordinator | `coordinator.go:492`, `:603` |
| `message_thread` (with `cite`) | coordinator | `coordinator.go:504`, `:608` |
| `get_thread` | coordinator | `coordinator.go:508`, `:632` |
| `read_conversation` | coordinator | `coordinator.go:513` |
| `list_threads` | coordinator | `coordinator.go:519` |
| `resolve_thread` | coordinator | `coordinator.go:523` |
| `rename_thread` | coordinator | `coordinator.go:525` |
| `no_update` | coordinator | `coordinator.go:527` |
| `remember`, `forget` | coordinator | `coordinator.go:529`, `:531` |
| `update_instructions` | coordinator | `coordinator.go:532` |
| `read_shared_file`, `list_shared_files` | coordinator | `project/coordinator_mcp.go:109-135` |
| `send_file` | every thread, and the coordinator | `project/session_tools_mcp.go:84` |

Wiring: `coordinatorTools()` (`coordinator.go:480`) is a hand-written list; `coordinatorMCPForTurn` (`coordinator_mcp.go:40`) registers it, validates the input against the JSON Schema and runs the gates; `Coordinator.runTool` (`coordinator.go:566`) is one `switch` over the names. `sessionToolsMCP.config` (`session_tools_mcp.go:39`) starts one loopback HTTP listener, makes a random capability URL per thread, and registers `send_file`; for a project's main thread it first builds the coordinator server and adds `send_file` to it. The thread id is bound to the URL, never taken from the model; the project id is never taken from the model either.

Gates every coordinator call already passes (`coordinator_mcp.go:62-99`): schema validation, an active turn, not `no_update`'d, project not paused/archived, project settings unchanged since the turn began (`configurationChanges`, `service_ops.go:85`), and `no_update` refused on a user-triggered turn. `coordinatorPrompter` auto-approves any `mcp__tingly-projects__*` call (`coordinator_session.go:43`).

Trust labelling: results that carry text written by someone else are wrapped by `envelope(trustRelay, …)` (`envelope.go:24`; used in `get_thread`, `history.go:104,165,210`), which escapes forged `<event>` tags. Only a `principal` event (the user, on the web or in a bound chat) may instruct the coordinator. For an action that needs the user's word, `message_thread` takes `cite` — ids of the user's own messages — and `quoteUser` (`cite.go:25`) copies them into the message and refuses anything that is not a user message (thread reports, assistant text, system events).

### Actions reachable only outside MCP

HTTP routes (all under `projects/internal/server/module/`), with the service call they end in. "MCP" says whether a tool exists.

| Area | Action | Route | Service call | MCP |
| --- | --- | --- | --- | --- |
| thread | stop the running turn | `POST /threads/:id/stop` (`thread/handler.go:224`) | `Threads.Stop` | no |
| thread | retry the last turn | `POST /threads/:id/retry` (`:227`) | `Threads.Retry` | no |
| thread | resolve | `POST /threads/:id/resolve` (`:230`) | `Threads.Resolve` | **`resolve_thread`** |
| thread | reopen | `POST /threads/:id/reopen` (`:239`) | `Threads.Reopen` | no |
| thread | rename | `PUT /threads/:id/title` (`:262`) | `Threads.SetTitle` | **`rename_thread`** |
| thread | permission mode | `PUT /threads/:id/permission-mode` (`:246`) | `Threads.SetPermissionMode` (`sessions.go:166`) | no |
| thread | model / effort / profile | `PUT …/model`, `…/effort`, `…/profile` (`:250-258`) | `SetModel`, `SetEffort`, `SetWorkerProfile` (`launch.go`) | no |
| thread | auto-answer | `PUT …/auto-answer` (`:242`) | `Threads.SetAutoAnswer` (`sessions.go:183`) | no |
| thread | stop / read a background task | `POST …/tasks/:task_id/stop`, `GET …/output` | `StopTask`, `TaskOutput` | no (`get_thread` lists running tasks) |
| thread | answer a worker's approval or question | `POST …/respond` (`:220`) | `Threads.Respond` | **no, on purpose** |
| git | diff | `GET /threads/:id/diff` (`:281`) | `Threads.Diff` | no |
| git | merge into base | `POST /threads/:id/merge` (`:284`) | `Threads.Merge` (`threads.go:1340`) | no |
| git | push, PR, branch drift | — | **nothing in Projects yet** (`grep push` finds no code) | no |
| thread | hand off to a terminal | `POST …/handoff` (`:266`) | `Threads.Handoff` (`launch.go:185`) | no, for people |
| folder session | archive / restore | `POST …/archive`, `…/restore` | `ArchiveSession`, `RestoreSession` | no, no coordinator there |
| project | read settings | `GET /projects/:id` | store | partly (the prompt) |
| project | name, goal, instructions | `PATCH /projects/:id` | **inline in the handler** (`project/handler.go:448-570`) | **`update_instructions`** only |
| project | concurrency limit, thread model / effort / permission mode / worktree, repos, profiles | `PATCH /projects/:id` | **inline in the handler** | no |
| project | pause, archive, star | `PATCH /projects/:id`, `DELETE` | `Service.SetPaused`, `SetArchived`, `Save` (`service_ops.go`) | no |
| memory | write / delete a file | `PUT/DELETE /projects/:id/memory/:name` | `Layout.Remember`, `DeleteMemory` | **`remember`, `forget`** |
| library | list, read | `GET …/library`, `…/library/file` | `Layout` | **`list_shared_files`, `read_shared_file`** |
| library | delete a file | `DELETE …/library/file` (`project/handler.go:273`) | `Layout.RemoveLibraryFile` | no |
| library | zip a folder, upload | `GET …/library/archive`, `POST …/library` | `Layout` | no, for people |
| IM | list / bind / unbind a chat | `/projects/channel-bindings` (`channel/handler.go:55-64`) | `Service.BindChannel`, store | no |
| usage | tokens by model and thread | `GET /projects/:id/usage` | store | no |
| session control | compact, context usage | none yet (PRs #2454–#2456) | `Threads.Compact`, `Engine.ContextUsage` | no |
| IM | chat commands `/new /stop /clear /session /status /verbose /yolo` | host chat bridge | `Service.ChannelCommand` (`project/chat_commands.go:43`), which already sits in the service layer | no (`/stop` and `/yolo` overlap `stop_thread` and `set_thread_settings`) |

The shape to notice: most thread actions are already methods on `Threads` that validate and record a system event (for example `SetPermissionMode` rejects a bad mode and adds `permission mode: …` to the thread). Their HTTP handlers add only the "main thread is not a worker" guard (`thread/handler.go:321`) and error mapping. The exceptions are the project `PATCH` handler (about 120 lines of validation and persistence in the handler) and a few store calls made directly by handlers (`unbind`, `usage`).

## Design

### 1. One action layer

Rule: **an action is a method on `Threads` or `Service` that takes typed input, validates, persists, publishes and records the system event, and returns a typed result or a typed error. HTTP handlers, IM commands and MCP tools only parse their input and map the result and the error.** No validation, no state change and no event in an adapter.

What changes:

- Move the body of `project/handler.go:update` into `Service.UpdateProject(ctx, p, ProjectPatch) (*db.Project, error)`; the handler becomes bind → call → map. `ProjectPatch` is `UpdateProjectRequest` without the swagger tags (pointer fields, nil = unchanged).
- Move the "only on a worker thread" and "only folder sessions can archive" guards (`handler.go:321`, `:577`) into the `Threads` methods as `ErrNotWorker`, `ErrFolderOnly`.
- Move `unbind` and `usage` store calls behind `Service` methods.
- Errors become a small closed set in `project` — `ErrInvalid{Msg}`, `db.ErrNotFound`, `ErrBusy`, `ErrBlocked{Reason, Command}` (generalising `MergeBlockedError`), `ErrNotWorker`, `ErrForbidden` — and each adapter has one mapper: HTTP → status code (existing `apierr`), MCP → `IsError` text that says what to do instead.

### 2. A tool registry instead of a switch

Replace `coordinatorTool` + the `runTool` switch with one table. A tool is data plus a handler that calls the action layer:

```go
type Tool struct {
    Name, Description string
    Input   any               // a struct; the JSON Schema is derived from it
    Scopes  Scope             // Coordinator | Worker | Folder, a bit set
    Risk    Risk              // Read | Write | Confirm
    Handle  func(ctx context.Context, c Caller, in json.RawMessage) (Result, error)
}
type Caller struct { Project *db.Project; Thread *db.Thread; Turn *coordinatorToolTurn } // bound by the capability URL, never by the model
```

`sessionToolsMCP.config` builds the server with `toolsFor(scope)`, where scope is derived once from the thread (main thread → Coordinator; thread in a project → Worker; thread in a folder project → Folder). The existing gates (active turn, paused/archived, configuration snapshot, serialisation) wrap `Coordinator` scope handlers exactly as now. The registry also generates the tool list in the tests and in this document.

### 3. Who may call what

| Scope | Gets | Identity bound by |
| --- | --- | --- |
| **Coordinator** (a project's main thread) | project-wide tools; every thread id it passes is resolved inside its own project (`findThread`, `coordinator.go:550`) | capability URL → main thread → project |
| **Worker** (a thread in a project) | self-scoped tools only; **never** a thread id parameter, never another thread | capability URL → this thread |
| **Folder** (a thread in a working folder; no coordinator) | the worker set | capability URL → this thread |

A worker cannot act on its siblings or on the project: that would let one model's output steer another's. Anything a worker produces for the coordinator keeps going through reports, which stay `relay`-trusted.

### 4. Safety: three risk classes

- **Read** — no state change. Results that quote agent- or user-authored text (diff, transcript, files) are wrapped in `envelope(trustRelay, …)`.
- **Write** — reversible and visible: the action records a system event in the affected thread ("Stopped by the coordinator."), so the user sees what the model did. Examples: stop, reopen, retry, rename, change effort.
- **Confirm** — irreversible, spends money or widens what a session may do. The tool refuses unless the call carries `approved_by: [message ids]` of the user's own messages, checked with the same rule as `cite` (`quoteUser`: a `user` event whose source is the web or a bound chat, in the recent window; never a thread report, assistant text or system event). The system quotes them into the audit event, word for word, so the user can see which of their messages the coordinator relied on. The model's own claim that the user agreed is worthless; only the citation counts. The intended flow is `request_decision` → the user's click is posted as their message → the coordinator cites that message.

Which actions are Confirm: `merge_thread`; `delete_shared_file`; any `set_thread_settings` / `set_project_settings` change that **widens autonomy or changes credentials** (permission mode → `auto` or `bypassPermissions`, auto-answer on, profile change, default permission mode for new threads). Lowering or neutral changes (permission mode down, effort, concurrency limit) are Write.

Never exposed to MCP (stay with the user): create / delete / archive / pause a project, `repos` (it chooses where agents write), the coordinator's own profile / model / effort (a model must not reconfigure itself), channel bind / unbind, answering a worker's permission request (`respond`; the point of that gate is that the user is the one asked), `choose`/`start suggestions` (the user's click), uploads, downloads, zip, directory browsing, handoff.

A coordinator action that changes project settings would trip its own `configurationChanges` guard on the next call in the same turn ("settings changed while this response was running"). The tool therefore refreshes the turn snapshot (`turn.snapshot`) after its own successful change, under the same lock; a change from anywhere else still trips the guard.

### 5. API shape conventions

- Names are `verb_noun`, snake_case, one action each; orthogonal settings of one object may be one tool with optional fields (`set_thread_settings`), applied atomically (all or none).
- Ids: threads by their 8-character prefix, resolved inside the caller's project. A tool never accepts a project id. Worker tools accept no thread id.
- Results are one or two plain lines: what changed, and the state now (`thread 3fa9c1d2 permission mode: default → acceptEdits`). A no-op says so (`already resolved`) and is not an error.
- Errors say what to do next (`the thread is still working; stop it first with stop_thread`). A blocked git operation returns the reason and the command to run by hand, as `MergeBlockedError` does.
- Descriptions are short; the schema, not prose, carries the argument rules. Token budget: the coordinator sees every tool schema on every request (`ENABLE_TOOL_SEARCH=false`, `coordinator-session.md`), so adding tools has a cost per turn (TEP-2079). Phase 1 measures it; the acceptance bound is in the TEP.
- Every tool has a test through the in-memory MCP transport and, where an HTTP route also exists, a parity test (see Testing).

### 6. The tools

Priority is by how often a user would otherwise be told "click it yourself". Calls marked (C) are Confirm.

**Phase 1 — see and steer threads (coordinator).** All of it is a thin call to a method that already exists.

| Tool | Does | Calls |
| --- | --- | --- |
| `stop_thread` | interrupt the running turn; the thread stays and can be messaged | `Threads.Stop` |
| `reopen_thread` | undo `resolve_thread` | `Threads.Reopen` |
| `retry_thread` | rerun the last failed turn | `Threads.Retry` |
| `get_thread_diff` | changed files and a capped diff of a thread's branch against its base | `Threads.Diff` |
| `get_thread` (extended) | add permission mode, model, effort, auto-answer, running tasks, context usage once available | store |
| `get_project` | read the project's settings and limits | store |

**Phase 2 — configure (coordinator).**

| Tool | Does | Calls |
| --- | --- | --- |
| `set_thread_settings` | `permission_mode`, `model`, `effort`, `auto_answer` on one thread (C when widening) | `SetPermissionMode`, `SetModel`, `SetEffort`, `SetAutoAnswer` |
| `set_project_settings` | `max_running_threads`, `thread_model`, `thread_effort`, `thread_permission_mode` (C when widening), `disable_worktree`, `goal`, `name` | new `Service.UpdateProject` |
| `compact_thread` | compact one thread's conversation when it is near its context limit | `Threads.Compact` (after PRs #2454–#2456 land) |
| `stop_task` | stop one background task of a thread | `Threads.StopTask` |

**Phase 3 — git (C) and thread self-description.**

| Tool | Who | Does | Calls |
| --- | --- | --- | --- |
| `merge_thread` (C) | coordinator | merge a thread's branch into its base and resolve it; returns the reason and the manual command when blocked | `Threads.Merge` |
| `set_branch(branch, base?)` | worker, folder | declare the branch or base the thread works on; the host reads git, the declaration is only the intent | `branch-tracking.md` |
| `link(kind, number)` | worker, folder | declare the PR or issue the thread works on | `pr-issue-awareness.md` |
| `set_progress(text)` | worker, folder | one line on the card, not in chat, does not wake the coordinator | `message-channel.md` |
| `start_thread` (extended) | coordinator | optional `base` | `branch-tracking.md` |

Push and PR creation are **not** in this plan: Projects has no code for them, they need credentials the host owns, and the awareness design reads them from `gh`. They become tools only after that design decides where they live (open question in the TEP).

**Phase 4 — project resources.**

| Tool | Does |
| --- | --- |
| `delete_shared_file` (C) | delete a library file (`Layout.RemoveLibraryFile`); memory already has `forget` |
| `list_channel_bindings` | which chats hear this project and its threads (read only; `bind`/`unbind` stay with the user) |
| `get_usage` | token use by model and thread, so the coordinator can answer cost questions |

### 7. Testing

- **Per tool**, in the style of `coordinator_mcp_test.go`: connect an in-memory MCP client, call with valid, invalid and cross-project input, check the result text and the resulting store state.
- **Scope golden**: a test lists the tool names each scope exposes and compares them with a checked-in list, so a new tool can never become visible to workers by accident. A worker server must contain no tool with a `thread_id` parameter.
- **Parity**: one table-driven test per shared action that performs it through the service, through the HTTP handler and through the MCP tool on identical fixtures and asserts the same final database state and the same system event. This is what stops the layers from drifting apart again.
- **Gating**: every Confirm tool refuses with no `approved_by`; with the id of a thread report, an assistant reply, a system event or a message in another project; and accepts the id of a real user message (web and IM source). Idempotency: a second call is a no-op. `configurationChanges` interplay: a settings tool followed by another call in the same turn succeeds.
- **End to end**: `coordtest` (scripted coordinator model calling the real MCP tools on top of `enginetest`) for the flows "stop then message", "ask, user clicks, merge". `showcase/e2e-demo/coordinator_session_probe.py` against the real Claude Code CLI stays the only check that the real CLI discovers and allows the new tools; run it once per phase.
- **Token budget**: a test that renders the coordinator's tool list and fails above the agreed number of schema bytes.

### 8. Implementation order inside a phase

1. Extract the action (`UpdateProject`, guards, error set) with the HTTP tests unchanged — a pure refactor that must keep `server_test.go` green.
2. Introduce the registry and move the 13 existing coordinator tools onto it with no behaviour change (the existing MCP tests are the safety net).
3. Add the new tools of the phase with their tests.

## Decisions taken here and why

- **Service layer over "MCP calls HTTP".** Calling the server's own HTTP API from a tool would add a network hop, an auth path and a second error mapping, and the standalone and embedded hosts mount the routes differently. The `Threads` methods are already the real implementation.
- **One tool with optional fields for settings, not four tools.** Fewer schemas per coordinator request; the fields are applied atomically so a half-applied change cannot happen.
- **Citation, not a new approval store, for Confirm.** The `cite` mechanism already exists, is already verified against forgery, and is visible to the user. A decision-bound approval is stronger and is recorded as an open question in the TEP rather than built first.
- **Workers get no cross-thread reach.** Keeps the trust model of `coordinator-session.md` (threads report, only the principal instructs) intact.
