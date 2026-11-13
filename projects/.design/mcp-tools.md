# Projects MCP tools: grow the surface one action at a time

> Status: **in progress**: step 1 (creation with base and branch) is built; the rest is proposed. Narrowed on 2026-11-07. Tracking issue: [TEP-2462](https://github.com/tingly-dev/tingly-box/issues/2462).
> Builds on [coordinator-session.md](./coordinator-session.md) (how the coordinator's MCP server is scoped and serialised) and [message-channel.md](./message-channel.md).
> Related, not on `main` yet: `.design/slash-commands.md` (session control API; PRs #2454–#2456), `pr-issue-awareness.md`, `branch-tracking.md`.
> `file:line` references are on `main` at `d7b849ccc`.

## Approach

The coordinator and every thread are Claude Code sessions, so MCP tools are the only way they can act on Projects. Today the coordinator has 13 business tools plus 2 shared-file readers, and a thread has `send_file`. The web UI and IM commands reach many more actions.

We do **not** design the whole surface up front. The order, each step small and shipped before the next:

1. **Creation, done well** (built): `start_thread` can choose the base and the branch name.
2. **A thread state-management API** in the service layer (stop, reopen, resolve alike), guarded and with an actor.
3. **Tool boundaries**: who may call what, and what needs the user's word, decided when the first tool needs it.
4. **The tools** on top (`stop_thread`, `reopen_thread`, then the rest).

 Each step moves one small group of actions behind a proper service method and exposes it as a tool. The architecture changes only as far as that group needs, and every step leaves HTTP, IM and MCP calling the same code. A step is done when it has shipped, not when the catalogue is complete.

The rule that carries over every step: **an action is a method on `Threads` or `Service` that validates, persists, publishes and records the system event; HTTP handlers, IM commands and MCP tools only parse input and map the result and the error.** Guards that make an action safe belong in the method, because a model will call it in states a button would never offer (see Reopen below for a real example).

## What exists

Tools today (registered per turn by `coordinatorMCPForTurn`, `coordinator_mcp.go:40`; defined in `coordinatorTools()`, `coordinator.go:480`; run by one `switch`, `Coordinator.runTool`, `coordinator.go:566`):
`start_thread` (`:488`), `propose_threads`, `request_decision`, `message_thread`, `get_thread`, `read_conversation`, `list_threads`, `resolve_thread`, `rename_thread`, `no_update`, `remember`, `forget`, `update_instructions`, plus `read_shared_file` / `list_shared_files` (`coordinator_mcp.go:109`) and `send_file` for every thread (`session_tools_mcp.go:84`). A worker's server comes from `sessionToolsMCP.config` (`session_tools_mcp.go:39`): one loopback listener, a random capability URL per thread; the thread id is bound to the URL and the project id is never taken from the model.

Gates every coordinator call passes (`coordinator_mcp.go:62-99`): schema validation, an active turn, project not paused or archived, project settings unchanged since the turn began, `no_update` refused on a user-triggered turn. Trust: text from others is wrapped in `envelope(trustRelay, …)` (`envelope.go:24`); `message_thread` takes `cite` (ids of the user's own messages) checked by `quoteUser` (`cite.go:25`).

Actions only the web UI or HTTP can do (a selection, with the method they end in): stop (`Threads.Stop`), retry (`Retry`), reopen (`Reopen`), permission mode / model / effort / auto-answer (`sessions.go:166`, `launch.go`), diff (`Diff`), merge (`Merge`, `threads.go:1340`), project settings such as the concurrency limit (inline in `project/handler.go:448`, no service method), library delete (`Layout.RemoveLibraryFile`), channel bindings, usage. Not in Projects at all: push and PR. Most thread actions are already clean `Threads` methods; the project `PATCH` handler is the exception.

## The thread lifecycle, and why stop + reopen come before the other thread tools

A thread has two independent things going on:

| Axis | Values | Changed by |
| --- | --- | --- |
| **Activity** (transient) | a turn is running, waiting on a question, or nothing | a message starts a turn; **stop** ends it |
| **Lifecycle** (durable) | open, **resolved** (`ResolvedAt`); a folder session also has an independent archive flag | **resolve** closes it; **reopen** opens it again |

The displayed `status` (`working / waiting / failed / ready / idle / resolved`) is derived from both plus the branch (`restingStatus`, `background.go:151`: commits ahead → `ready`, else `idle`). So the three actions are not the same kind of thing:

- **Create** (`Threads.Start`) makes a thread and runs its first turn. It is already a tool (`start_thread`) and is also what a suggestion card calls (`coordinator.go:764`). There is nothing to add for a user, so it is **not a new tool**. It is only worth touching as the first tool moved onto the new plumbing (below). Worker creation has no HTTP route; folder sessions have a separate `StartFolderThread` (`sessions.go:93`) because they are created in place, not on a branch. The two methods differ for a good reason, so we do not merge them now.
- **Stop** is not a state. It interrupts the turn (`end.Stopped` → status `idle`, queued messages dropped, the resident process and its background tasks survive; `Threads.stop`, `threads.go:1193`). A durable `stopped` state would be invented: the next message simply starts a new turn. It is an activity command whose result is a derived status.
- **Resolve / reopen** are the real state change. `resolve_thread` exists, but its undo does not: `Send` refuses a resolved thread with `ErrResolved` ("the thread is resolved; reopen it first", `threads.go:28`, `:668`), and the coordinator has no way to follow that advice. That dead end is the strongest reason for the first step.

Why not one `set_thread_state(stopped | resolved | open)`: it is a mode picker over three operations with different preconditions and side effects (stop drops queued messages and may drop a pending question; resolve also evicts the process, hands IM chats back to the parent and cleans the tmp dir; reopen restores). It would also force renaming the existing `resolve_thread`. Three verbs stay clearer to the model and to the audit trail. What we keep from the "state change" view: the three service methods are shaped alike (thread id, who is acting, the thread back) and each says in its result what the thread's status is now.

## Step 1: creation with base and branch (built)

Problem: a new thread always branched from the repository's base branch (the branch that was checked out when the repository was added, usually `main`), with a generated name `tp/<title>-<id>`. Work that has to build on a release or feature branch could not be started from the coordinator.

`start_thread` and `propose_threads` take two optional fields; `StartSpec` carries them (`threads.go`), and so does a stored suggestion, so a proposal started from its card keeps them:

| Field | Meaning | Rules |
| --- | --- | --- |
| `base` | the local branch the thread's branch starts from, and that its work is merged back into (`Thread.BaseBranch`; diff and merge already use it) | must be an existing local branch; default is the repository's base branch |
| `branch` | name of the new branch (`Thread.Branch`) | must be a valid ref name and must not exist yet; default is generated |

Both need a repository and a worktree: with no repository, or a project whose threads share the checkout (`DisableWorktree`), they are refused with `ErrNoOwnBranch` instead of being ignored.

Decisions, small on purpose:

- **Validate before anything is created.** A bad `base` or `branch` fails before the thread row, the worktree or the branch exist, so a refused start leaves nothing to clean up. The coordinator gets the reason as the tool error.
- **`AddWorktree` no longer deletes a branch it did not create.** Its failure path ran `git branch -D <branch>` to remove a half-made branch; with a caller-chosen name that would have deleted the user's own branch of that name. It now refuses an existing branch first.
- **Local branches only.** `origin/x` or a tag is not accepted as `base` yet; the error names the repository so the coordinator can say so.
- **No taking over an existing branch** (continue work on a branch or PR that exists). That is a different intent (the branch is not new, and its base must be found), and belongs with the `set_branch` / PR-awareness designs.
- **Not merged into one creation method.** `Threads.Start` (branch-based worker) and `StartFolderThread` (in-place session) stay apart.
- **No UI change.** The suggestion card does not show base or branch yet; the thread's badges already show both once it exists.

Tests (`thread_branch_test.go`): the worktree starts at the chosen base's commit and not at `main`; the default is unchanged; a missing base, a taken, invalid or option-like branch name are refused and leave no thread; `DisableWorktree` and `repo: "none"` refuse; `AddWorktree` keeps an existing branch (mutation-checked: removing the guard fails the test); through the real MCP tool and through a proposal card.

## Step 2: the thread state-management API (next)

The service-layer part of "Step 3" below, done first and without any new tool: a guarded `Reopen`, an actor on `Stop` and `Reopen`, and handlers that only bind, call and map. It stands on its own: it fixes a latent bug (`Reopen` on a thread that is not resolved corrupts its status) and makes the later tools thin.

## Step 3: tool boundaries (when the first new tool needs it)

Decide only what stop and reopen need: coordinator-only, reversible, recorded in the thread, no approval. The approval rule for irreversible actions (merge) and per-scope tool lists for worker tools are designed when those tools come, not before.

## Step 4: stop + reopen tools (the original first slice)

### Service changes (this is Step 2; one small PR, no new tool yet)

- `Reopen` gets a guard: it acts only on a resolved thread. Today it sets `ResolvedAt = nil` and `Status = idle/ready` unconditionally (`threads.go:1300`), so calling it on a working thread would flip its status while a turn runs. The button never offers that; a model can. On an open thread it is a no-op that says so.
- `Stop` and `Reopen` take who is acting (the existing `Source`; a coordinator call is `fromThread(mainThreadID)`, as `message_thread` already uses) and write one system event that names it: "Stopped by the coordinator." A web stop keeps its current wording. `Stop` also reports how many queued messages it dropped, so the coordinator and the user can see if a message was lost.
- Both return the thread as it is now. The HTTP handlers (`thread/handler.go:530`, `:620`) shrink to bind, call, map, with no behaviour change.

### Tool plumbing (small, only what these tools need)

Give `coordinatorTool` an optional `Handle func(ctx, p *db.Project, in json.RawMessage) (string, error)`. A tool with a handler skips the `switch`. The two new tools use it; `start_thread` moves onto it as the worked example of migrating an old tool (zero behaviour change, existing tests as the safety net). The other tools stay in the `switch` until someone touches them. There are no scopes, risk classes or generated schemas yet: those are added when the first tool needs them (see Later).

### The two tools

| Tool | Input | Does | Result |
| --- | --- | --- | --- |
| `stop_thread` | `thread_id` | interrupt the thread's running turn, drop its queued messages | one line: status now, number of dropped messages, or "not running" (not an error) |
| `reopen_thread` | `thread_id` | undo `resolve_thread`: the thread accepts messages again | one line: status now, or "already open" (not an error) |

Both are coordinator-only, reversible and recorded in the thread, so they need no approval step. `message_thread` to a resolved thread keeps returning `ErrResolved`, and its text now names `reopen_thread`.

To verify while implementing, not assumed here: what happens to a pending approval card of a `waiting` thread when it is stopped.

### Tests

- In-memory MCP client (as `coordinator_mcp_test.go`): each tool on a running, idle, resolved and cross-project thread; the not-an-error no-op paths; the `ErrResolved` hint.
- One parity test per action: the same fixture through the service, the HTTP route and the MCP tool ends in the same thread state and the same system event, apart from the actor wording.
- `Reopen` on a working thread leaves it working.
- `coordtest` flow: stop a running thread, message it, resolve it, reopen it, message it.

## Later (only when asked for, each its own change)

Listed so the direction is visible, not as a commitment. Each needs its own small decision first.

- **More thread actions:** `retry_thread`, `get_thread_diff`, `set_thread_settings` (permission mode, model, effort, auto-answer). Widening a session's permissions should not be silent; that is the first case where an approval rule is needed.
- **Merge** (`merge_thread`): irreversible. This is when to add an approval rule: the call must carry the ids of the user's own messages, checked like `cite`, and quoted into an audit event. A stronger form binds approval to an answered decision card. Decide then, not now.
- **Project settings** (`set_project_settings` for the concurrency limit and similar): needs `Service.UpdateProject` extracted from the handler, and the tool must refresh the turn's settings snapshot after its own change or it trips the "settings changed" guard.
- **Worker and folder-session tools** (`set_branch`, `link`, `set_progress`, from the awareness and message-channel designs): the first tools a thread calls on itself. That is when scopes (coordinator vs worker) appear in the plumbing, with a test that no worker tool takes a thread id.
- **Resources:** library delete, channel bindings (read-only first), usage.
- **Session control:** `compact_thread` after the slash-command PRs land.

Never for a model: create, delete, pause or archive a project; `repos`; the coordinator's own profile, model or effort; answering a worker's permission request; choosing a decision card; uploads and downloads.

Push and PR creation are out of scope: Projects has no such code, and credentials belong to the host.

## Decisions taken here and why

- **Service methods, not "MCP calls the local HTTP API".** An extra hop, an auth path and a second error mapping, and the standalone and embedded hosts mount routes differently. The `Threads` methods are already the implementation.
- **Three verbs, not a state picker** (above).
- **Strangler plumbing:** add the handler field and migrate tools as they are touched, instead of rewriting registration and moving 13 tools at once.
- **Leave the two creation methods apart:** in-place folder sessions and branch-based worker threads are different enough that one entry point would hide the difference.
