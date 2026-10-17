# Projects absorbs Desk

Diagrams: [projects-desk-absorption.pencil.md](./projects-desk-absorption.pencil.md).

> Status: 已实施 (phases 1–4). Desk is gone; working in a plain folder is a
> Projects working-folder session. Phase 5 is still open. This note supersedes
> the "out of scope: merging the two products" line of TEP #2063.

## Decision

There is no standalone Desk in the end. Working in a plain folder becomes a
first-class thing **inside Projects**: a *working folder* (the Desk word, kept
as the zh "工作目录") that holds *sessions*. A Project stays "hand work to a
coordinator"; a working folder stays "sit down with Claude in a folder, no
worktree". Both are places in the Projects panel, both show the same session
UI, and `/desk`, `/api/v1/desk`, the `desk` flag and `internal/desk` go away.

Projects must keep opening on its own (standalone `tingly-project`). So the
absorbed part is split by one rule:

- **Generic** (works with just a `claude` on PATH): lives in `projects/` and
  `@tingly/projects`.
- **Tingly Box only** (profiles, model tiers, quota status line, the
  `tingly-box cc --resume` handoff, `@cc` session sharing): stays in Tingly
  Box and reaches Projects through an optional `Host` capability on the
  backend and a slot/prop on the frontend. Projects never learns what a
  profile or a quota is.

The earlier analysis (`desk-vs-projects.md`, 2026-10-13) argued against this on
three grounds. What changed: the engine split it asked for has since landed
(`agentboot/engine`, used by both), the UI already converges (#2064–#2080), and
the product call is now to have one place. Its worry about UX stays valid and
shapes the model below: a folder session must not make people learn projects,
worktrees or merging.

## What Desk has, and where each piece goes

| Desk piece | Today | Goes to |
|---|---|---|
| Session lifecycle: create, list, get, messages, send, respond, interrupt, archive | `internal/desk` on `remote/session` | **Projects**: same operations on the thread runtime (`Threads.Start/Send/Respond/Stop/Resolve`) |
| Turn engine, resident processes, approvals, subagents, background tasks | `agentboot/engine` | already shared; nothing to move |
| Transcript, composer, approval card, subagent cards | `@tingly/projects/transcript`, `Composer` | already shared |
| Session list, shell | `WorkspaceLayout`, `StatusGroups` | already shared |
| Working folders: saved shortcuts (browser localStorage) + recent folders API | `useDeskProjects`, `/desk/folders/recent` | **Projects**, server-side `workdirs` (see below) |
| Folder picker, new-session form | `FolderPicker`, `NewSessionView` | **library** (folder field already a picker in the message box, #2073) |
| Permission mode per session | `PermissionModeSelect`, `PUT …/permission-mode` | **library** (project already has `ThreadPermissionMode`; a session can override) |
| Background tasks panel, stop task, read output | `BackgroundTasksPanel`, `/tasks/:id/*` | **library + Projects routes**; threads get it too |
| Drafts, queued messages, attention, poll | `useDesk*` hooks | **library** (queue/draft already in `Composer`; attention folds into the waiting signal, #2063 step 2) |
| Usage per turn (`usage` entries) | converter in the engine | **Projects** keeps them on the session; the status line reads them |
| Profile (Claude Code settings per session) | `ProfileSelect`, `SetProfile`, `GetClaudeCodeSettingsPathForProfile` | **TB**, as a host capability + picker slot |
| Model tier (separate vs unified profile) | `ModelSelect`, `/scenario/claude_code/models` | **TB** slot; the generic part is `Thread.Model` |
| Status line (routed model, context, tokens, quota) | `StatusLine`, `/desk/sessions/:id/status` | **TB** slot below the box, TB route keyed by session id |
| Handoff to a terminal | `POST …/handoff` | generic `claude --resume` command in Projects; **TB** overrides with `tingly-box cc --resume` through the host |
| Sessions shared with IM `@cc` (`ChatID="web"`, same store) | `remote/session` | **decision needed**, see below |
| `desk` flag, `/desk` route, rail placement, `desk` i18n | TB | removed; everything sits under the `projects` flag and namespace |

## Data model

A folder session is a **Thread with no project**: `ProjectID` empty, `Worktree`
set to the folder, no branch. It reuses the thread table, the event table, the
engine wiring, `ThreadPanel` and the status groups, so the session page and
the project's threads are literally one component. Alternatives weighed:

1. *Thread without project* (chosen). Least new surface; one runtime, one UI.
   Cost: `ProjectID` becomes nullable and the project-scoped code (slots per
   project, report to the coordinator, merge/diff) must tolerate "no project".
2. *Implicit one-thread project per folder.* Keeps `ProjectID` required, but
   shows people projects, goals and a coordinator they did not ask for, which
   is the objection in the 2026-10-13 analysis.
3. *A separate `Session` table.* Clean, but a second copy of the thread
   runtime and UI, which is the duplication this whole effort removes.

A new small `Workdir` table (path, name, starred, last used) is the server-side
version of Desk's browser shortcuts, so they survive a browser change and the
sidebar can list them without a session. A folder is also remembered by
having sessions, as today ("recent means used").

Review/merge/diff do not apply to a folder session; the panel hides them
(there is no worktree to diff). A later "hand this session to a project"
creates a thread in a project with the same session id (TEP #2063 step 5).

## Backend

- New routes under `/api/v1/projects`: `workdirs` (list, add, remove, star) and
  `sessions` (create in a folder, list, get with events, send, respond, stop,
  archive, permission mode, tasks stop/output, handoff command). They are the
  thread routes with the project prefix dropped; handlers share code.
- `project.Host` gains **optional** capabilities (type-asserted, so existing
  hosts and the standalone binary keep compiling):
  - `Launcher`: per-session routing. `LaunchEnv(profile)` gives the env and
    `--settings` path, `CheckLaunch(profile, model)` validates a choice.
    Profiles are opaque strings to Projects.
  - `Handoffer`: the program that continues a session in a terminal
    (`HandoffLaunch(profile)` → argv). Projects adds `--resume`, the
    session's `--model` and the folder, quoted for PowerShell on Windows and
    a POSIX shell elsewhere.
  - No status capability: the status line is a TB route drawn in a UI slot.
- The pool is shared: threads and folder sessions run in one `pool.Pool`, so
  the 10-process limit is global, not per surface.
- Standalone: no capabilities, so no profile/model/status UI, and the handoff
  command is plain `claude --resume`.

## Frontend

- `ProjectsSidebar` gets a native **Working folders** section (today TB injects
  it through `sections`); the injection point stays for other hosts.
- New package pages: working-folder page (session list in `StatusGroups` + new
  session + open session) built on `WorkspaceLayout`; the session view is
  `ThreadPanel` with project-only actions hidden.
- Slots for TB under `ProjectsHost.session`: `launchPickers` (profile and
  model, in the new-session form and the session composer) and `statusLine`
  (under the composer). Handoff is a library action; TB only changes the
  command through `Handoffer`. Library code stays free of TB imports.
- Delete: `pages/desk`, `components/desk`, `services/deskApi`, `deskHandlers`
  mocks, `desk` i18n.

## Phases

Each is one PR (or a few), with a sub-TEP checklist, and Desk keeps working
until phase 4.

1. **Folder sessions in Projects, backend only.** Nullable `ProjectID`,
   `Workdir`, the routes above, tests ported from `internal/desk` (race test
   and the resident-process e2e come with them). No UI change.
2. **Working folders in the library UI.** Native sidebar section, folder page,
   new-session form, session view. Behind the existing `projects` flag; the TB
   nav shows the native section only while the `desk` flag is off, so the two
   can be compared. The background tasks panel, the Trajectory view and
   handoff move to phase 3 with the rest of the parity list.
3. **Session parity and TB capabilities.** Background tasks panel, Trajectory,
   handoff, expand-tools, drafts; `Launcher` (profile, tier), status line route
   + slot, handoff override. Parity checklist against Desk (every row above).
   Status:
   - [x] Background tasks: header chip with the running count, a panel with
     each task's output and stop; archive and handoff ask first while tasks run.
   - [x] Trajectory view and expand/collapse tools, remembered per browser.
   - [x] Continue in terminal: `POST …/handoff` releases the resident process
     and returns the command; TB's `Handoffer` launches `tingly-box cc|profile`.
   - [x] Profile and model per session (`Launcher`; standalone rejects a
     profile), pickers through the `session.launchPickers` slot.
   - [x] Status line: `GET /scenario/:scenario/route` resolves the route and
     quota for a profile and tier; `session.statusLine` slot renders it with
     the session's token usage.
   - [x] Drafts: what is typed but not sent is kept per thread in the browser
     (threads and sessions alike). Messages sent mid-turn are queued by the
     server, as for threads, so Desk's browser-side send queue is not ported.
4. **Cutover.** Done. Desk was an unreleased experiment, so nothing is
   carried over:
   - [x] No import: Desk's sessions stay in the remote session store and are
     not shown in Projects.
   - [x] The `desk` flag is gone with no migration: its toggle, gate and nav
     entry are deleted, and a leftover `desk` key in a config is ignored.
   - [x] `/desk` and `/api/v1/desk` are removed, not redirected.
   - [x] Deleted `internal/desk`, `module/desk`, the Desk pages, components,
     API client, mocks and strings; the profile/model pickers and the status
     line strings moved to `components/claudeCode` and `claudeSession.*`.
   - [x] `desk.md` marked retired (its §3 engine behaviour still applies),
     `projects-panel.md` rewritten to the current panel.
5. **After.**
   - [x] One waiting signal on the rail (#2063 step 2): the dot reads both the
     project list and the working folders' waiting counts.
   - [ ] Hand a session to a project, `@cc` visibility: only when asked for.

## Decisions

1. **`@cc` sharing** (decided). Desk sessions lived in the same store as IM
   `@cc` sessions (`ChatID="web"`). Projects owns its storage to stand alone,
   with no live sharing and no import; showing IM sessions in the panel can
   come back as a TB-only section later.
2. **Data model.** Chosen: thread without a project (above). Say if you want
   the one-thread-project alternative instead.
3. **Flag** (decided). The `desk` flag is removed as a whole and everything is under `projects`: no migration and no alias, since Desk never shipped. Its gate, nav entry and experimental toggle are deleted.
4. **Cutover timing.** Phase 4 only after phase 3 reaches parity, one release
   with both paths available if you want a fallback.
