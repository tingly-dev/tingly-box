# Projects MCP tools: grow the surface one action at a time

> Status: **in progress**: steps 1–3 (creation with base and branch, the state API, the tool table and boundaries) are built; step 4 (new tools) is proposed. Narrowed on 2026-11-07. Tracking issue: [TEP-2462](https://github.com/tingly-dev/tingly-box/issues/2462).
> Builds on [coordinator-session.md](./coordinator-session.md) (how the coordinator's MCP server is scoped and serialised) and [message-channel.md](./message-channel.md).
> Related, not on `main` yet: `.design/slash-commands.md` (session control API; PRs #2454–#2456), `pr-issue-awareness.md`, `branch-tracking.md`.
> `file:line` references were taken on `main` at `d7b849ccc`, before steps 1–3 moved some code; search by name when a line number is off.

## Approach

The coordinator and every thread are Claude Code sessions, so MCP tools are the only way they can act on Projects. Today the coordinator has 13 business tools plus 2 shared-file readers, and a thread has `send_file`. The web UI and IM commands reach many more actions.

We do **not** design the whole surface up front. The order, each step small and shipped before the next:

1. **Creation, done well** (built): `start_thread` can choose the base and the branch name.
2. **A thread state-management API** in the service layer (stop, reopen), guarded and with an actor (built).
3. **Tool table and boundaries**: one table of tools with scopes and risk, and the rule for what needs the user's word (built; no new tool).
4. **The tools** on top (`stop_thread`, `reopen_thread`, then the rest).

 Each step moves one small group of actions behind a proper service method and exposes it as a tool. The architecture changes only as far as that group needs, and every step leaves HTTP, IM and MCP calling the same code. A step is done when it has shipped, not when the catalogue is complete.

The rule that carries over every step: **an action is a method on `Threads` or `Service` that validates, persists, publishes and records the system event; HTTP handlers, IM commands and MCP tools only parse input and map the result and the error.** Guards that make an action safe belong in the method, because a model will call it in states a button would never offer (see Reopen below for a real example).

## What exists

Tools today (one table, `Coordinator.coordinatorTools()` in `coordinator_tools.go`, plus the shared-file readers and `send_file` in `tool_server.go`; built per session by `sessionToolsMCP.buildServer`):
`start_thread`, `propose_threads`, `request_decision`, `message_thread`, `get_thread`, `read_conversation`, `list_threads`, `resolve_thread`, `rename_thread`, `no_update`, `remember`, `forget`, `update_instructions`, plus `read_shared_file` / `list_shared_files` (`coordinator_mcp.go:109`) and `send_file` for every thread (`session_tools_mcp.go:84`). A worker's server comes from `sessionToolsMCP.config` (`session_tools_mcp.go:39`): one loopback listener, a random capability URL per thread; the thread id is bound to the URL and the project id is never taken from the model.

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

## Step 2: the thread state-management API (built)

The service-layer part of stop and reopen, without any new tool.

- `Reopen(threadID, by Source)` acts only on a resolved thread. Before, it set `ResolvedAt = nil` and `Status = idle/ready` whatever the status, so calling it on a running thread flipped it to idle while a turn ran (reproduced; the new test fails when the guard is removed). On a thread that is not resolved it returns the thread as it is and writes nothing. A main thread is refused with `ErrNotWorker`.
- `Stop(threadID, by Source) StopResult` returns whether it interrupted a turn and how many queued messages it dropped. Stopping a thread with nothing running does nothing.
- `by` is the existing `Source`. The thread's note names it: "Stopped." (web), "Stopped from a chat." (IM), "Stopped by the coordinator." (a thread source, which today is only the coordinator); the same for "Reopened". Pause and archive stop every thread with a system source.
- The HTTP handlers and the IM `/stop` / `/clear` pass their own source and are otherwise unchanged. `findThread` already excludes the main thread (checked by a test), so the coordinator's thread tools cannot reach it.

## Step 3: the tool table and boundaries (built)

One table replaces three separate registrations (the coordinator's list, the two shared-file readers, `send_file`) and the gate code that was copied into each of them.

`tools.go`: `Tool{Name, Description, Properties, Required, Scopes, Risk, Handle}`.

| Scope | Session | Tools today |
| --- | --- | --- |
| `ScopeCoordinator` | a project's main thread | the 13 business tools, `read_shared_file`, `list_shared_files`, `send_file` |
| `ScopeWorker` | a thread in a project | `send_file` |
| `ScopeFolder` | a thread in a folder project | `send_file` |

- **Building a server** (`tool_server.go`): `sessionToolsMCP.buildServer(threadID, scope, turn, tools)` registers the tools whose scope includes the session's. The scope comes from the thread and its project (`scopeOf`), never from the model; a worker server cannot contain a coordinator tool.
- **One gate** (`coordinatorToolTurn.run`): active turn, not ended with `no_update`, project not paused or archived, settings unchanged, `no_update` refused on a user turn. Every coordinator-scope tool passes it, `send_file` included.
- **All tools are in the table.** The 13 coordinator tools used to run in one `switch` (`Coordinator.runTool`); each is now a method with a typed input struct (`coordinator_tools.go`: `startThread`, `messageThread`, ...) bound to its definition with `textTool`, which decodes the input, calls the method and turns an error into a failed result. `runTool` is gone. A new tool is one definition and one method.
- **Migration was checked, not trusted.** Before moving anything, two golden files pinned the current behaviour (`testdata/tool_list.golden`: every tool's name, description and schema in each scope; `testdata/tool_calls.golden`: the exact answer of all 16 tools on a fixed script of about 60 calls, with ids, paths and times normalised). They are byte-identical after the move. Mutation check: changing one answer or one description fails the matching golden test. Regenerate on purpose with `UPDATE_GOLDEN=1` and read the diff.
- **Risk**: `RiskRead`, `RiskWrite` (reversible, leaves a note) and `RiskConfirm`.
  - A `RiskConfirm` tool automatically gets a required `approved_by` input: ids of the user's own messages. They are checked with the same rule as `cite` (`quoteUser`): a user message from the web or a chat, never a thread report, the coordinator's own reply or a system event. After the tool succeeds, a system event in the conversation records "`<tool>` ran on the user's word" with the quoted messages.
  - Only the coordinator scope may have such a tool; registering one for a worker is an error.
  - No product tool uses it yet. It is tested with an injected tool. Whether approval should instead be bound to an answered decision card stays open (TEP-2462, question 1).
- **Tests** (`tools_test.go`): the exact tool list of each scope (a tool added to a scope has to be added to the test, so it cannot reach a worker by accident); no tool takes `project_id`, and no worker tool takes `thread_id`; the coordinator's tool list stays under 12000 bytes (10.7 KB now, 8.2 KB for the 13 business tools before `base`/`branch` and the shared tools; each tool costs tokens on every turn); the confirm gate refuses a missing, empty or unknown approval, a thread report and the coordinator's own reply, runs with a real user message and writes the audit note (mutation-checked: skipping the check fails three cases).

Findings while pinning the behaviour (recorded, deliberately not changed here):

- `update_instructions` changes the project's own settings, so the turn's "settings changed" guard refuses every later tool call of that turn. A settings tool must refresh the turn snapshot after its own change (already noted for `set_project_settings`).
- `list_shared_files` lists a directory in file-system order, not sorted. The golden test sorts the lines; sorting in the tool would give the model a stable order.
- `remember` with an empty topic saves `note-<unix time>.md`.

Not changed: the tools' names, schemas (apart from `base` / `branch`) and behaviour; the real-CLI probe is not rerun.

## Step 4: stop + reopen tools (next)

### Service changes (done in Step 2)

See Step 2: guarded `Reopen`, an actor on `Stop` and `Reopen`, and the dropped-message count.

### Tool plumbing (done in Step 3)

The two tools are written as handlers in the table with `Scopes: ScopeCoordinator`, `Risk: RiskWrite`.

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
- **Move all tools at once, with golden files as the safety net** (Step 3), instead of a piecemeal migration: the model sees the same schemas and answers, which two golden files prove.
- **Leave the two creation methods apart:** in-place folder sessions and branch-based worker threads are different enough that one entry point would hide the difference.
