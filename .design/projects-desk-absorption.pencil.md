# Projects absorbs Desk — Pencil Graph

Visual companion to [`projects-desk-absorption.md`](./projects-desk-absorption.md),
which stays the authority on decisions and status.

> **Superseded in part** by [projects-sessions.md](./projects-sessions.md): the
> Workdir table and the `/api/v1/projects/sessions` and `/api/v1/projects/workdirs`
> routes drawn below are gone. A working folder is a `type=folder` project, its
> sessions are threads, and they are addressed as `/api/v1/threads/:id/*`. This file shows what changed
across the stack: where things lived before, where they live now, how Tingly
Box plugs its own parts in, and what happens to old Desk data at cutover.

Contents:

- Before / after (map)
- Data model: a session is a thread without a project
- One runtime for threads and sessions
- Host capabilities: what Projects asks the host, and the standalone answer
- UI slots: where Tingly Box draws inside the library
- Cutover
- The rail's waiting dot
- What moved where (file map)
- PR chain

## Before / after

```
BEFORE                                              AFTER
──────                                              ─────
rail ─┬─ Projects (flag: projects)                  rail ── Projects (flag: projects)
      │    projects ── coordinator ── threads               │
      │                                                     ├─ Projects        coordinator + threads
      └─ Desk (flag: desk)                                  └─ Working folders sessions in a plain folder
           folders ── sessions                                    (was Desk)

TB backend                                          TB backend
  internal/desk  ───────┐                             internal/server/module/projects
  module/desk           │  two runtimes,                Host: env, Launcher, Handoffer
  remote/session store ─┘  two stores                     │
  projects/ (embedded) ──┐                                ▼
                         │                          projects/ (embedded, also standalone)
TB frontend              │                            one store, one runtime, one pool
  pages/desk, components/desk, deskApi              TB frontend
  @tingly/projects pages ┘                            @tingly/projects pages
                                                      + ProjectsHost.session slots
                                                        (profile/model pickers, status line)
```

## Data model: a session is a thread without a project

```
Thread
├── ProjectID      "p-123"  → project thread (branch, worktree, merge, report)
│                  ""       → folder session (no branch, no merge, no report)
├── Worktree       project: the thread's git worktree
│                  session: the folder itself
├── SessionID      Claude Code's own session id (resume key, handoff key)
├── Profile        session only; opaque to Projects, named by the host
├── Model          what the owner picked; "" = profile / host default
├── PermissionMode per thread; a session's only mode
└── Turns          >0 → next turn runs `claude --resume`

Workdir (new)      path, created_at, last_used_at
                   a folder kept in the sidebar even with no session
```

## One runtime for threads and sessions

```
POST /projects/sessions                  POST /projects/:id/threads/...
   (folder, prompt, profile, model)         (coordinator or user)
              │                                       │
              ▼                                       ▼
        Threads.StartSession ──────────► Threads.enqueue ◄──── Threads.Send
                                               │
                                               ▼
                                        begin(threadID)
                          ┌────────────────────┴────────────────────┐
                    session (no project)                      project thread
                    launchEnv: host Launcher                  ThreadEnv + project
                    (profile → env, settings)                 settings file
                          └────────────────────┬────────────────────┘
                                               ▼
                               agentboot/engine.Start (shared pool, max 10)
                                               │ entries
                                               ▼
                               threadStore.Append → ThreadEvent
                          session: keeps subagents, task + usage events
                          project: main conversation only, report to coordinator
```

## Host capabilities

Projects type-asserts each optional capability on the host it was given. A
host that lacks one gets the standalone behaviour, so `tingly-project` keeps
building and running unchanged.

```
                   projects.Host (required)
                   ├─ ClientFor / Models      coordinator model
                   └─ ThreadEnv               gateway env + default model
                            │
         ┌──────────────────┼───────────────────────┐
         ▼                                          ▼
   Launcher (optional)                         Handoffer (optional)
   LaunchEnv(profile)  → env, --settings       HandoffLaunch(profile) → argv
   CheckLaunch(profile, model) → error
         │                                          │
 ┌───────┴────────────┐                   ┌─────────┴──────────────┐
 Tingly Box            standalone          Tingly Box               standalone
 profile → its CC      profile ≠ "" →      ['<tingly-box>', 'cc']   ['claude']
 settings file;        ErrNoProfiles       or [.., 'profile', p]
 model ∈ tier aliases  (400); model
                       passed as is        Projects adds --resume <sid>,
                                           --model <m>, the folder, and quotes
                                           for the shell: cd … && …  (POSIX)
                                           Set-Location …; & …   (Windows)
```

Handoff always releases the resident process first (two processes on one
Claude session file corrupt it) and refuses while a turn runs (409).

## UI slots

```
┌─ WorkdirPage (library) ───────────────────────────────────────────────┐
│ header: folder name + path                     [New session]  [≡]     │
│ ┌─ ThreadPanel (library, shared with project threads) ──┐ ┌─ side ──┐ │
│ │ title · status · folder      [N running] [⋮ menu]     │ │ groups: │ │
│ │                               │            ├ trajectory│ │ waiting │ │
│ │ transcript / trajectory       │            ├ expand    │ │ working │ │
│ │                               │            ├ tasks     │ │ idle    │ │
│ │                    tasks popover (output, stop)        │ │ resolved│ │
│ │ ┌─ Composer ─────────────────────────────────────────┐ │ └─────────┘ │
│ │ │ ▸slot launchPickers  [profile ▾][model ▾]  ◄─ TB   │ │             │
│ │ │  [permission ▾]  (library)                         │ │             │
│ │ │  draft kept per thread in this browser             │ │             │
│ │ └────────────────────────────────────────────────────┘ │             │
│ │ ▸slot statusLine  route · context · tokens · quota ◄─ TB│             │
│ └─────────────────────────────────────────────────────────┘            │
└────────────────────────────────────────────────────────────────────────┘
standalone: both slots empty → permission picker only, no status line
```

`ProjectsHost.session = { launchPickers, statusLine }`; Tingly Box fills them
in `pages/projects/session/sessionSlots.tsx`. The status line reads
`GET /scenario/:scenario/route?profile&tier` plus the session's usage events.

## Cutover

Desk never shipped, so nothing is carried over.

```
config        a leftover `desk` key is ignored; only `projects` turns the panel on
sessions      Desk's sessions stay in the remote session store, not shown
browser       /desk and /api/v1/desk are gone, no redirect
code          internal/desk, module/desk, the Desk pages, API client, mocks
              and desk.* strings deleted
```

## The rail's waiting dot

```
useProjectsWaiting (poll 30s, on PROJECTS_CHANGED, on tab focus)
        │
        ├─ GET /api/v1/projects           any project.threads.waiting > 0 ─┐
        └─ GET /api/v1/projects/workdirs  any workdir.waiting > 0 ─────────┤
                                                                           ▼
                                                     dot on "Projects" + tooltip
                                                     "Claude is waiting on you"
before: only the first request, so a session stuck on an approval left it dark
```

## What moved where

```
Desk (deleted)                                   now
──────────────                                   ───
internal/desk/service.go  sessions, turns        projects/internal/project/sessions.go, threads.go
  profile/model checks, claudeEnv                internal/server/module/projects/launch.go (Launcher)
  Handoff                                        project/launch.go + TB HandoffCommand
  tasks.go                                       project/sessions.go BackgroundTask, task output
internal/server/module/desk  /api/v1/desk/*      projects/internal/server/module/session  /api/v1/projects/sessions/*
  status route                                   module/scenario  GET /scenario/:scenario/route
pages/desk/DeskPage                              @tingly/projects pages/WorkdirPage
components/desk/SessionView                      @tingly/projects ThreadPanel (session mode)
  BackgroundTasksPanel, deskUtils tasks          components/workdir/BackgroundTasksPanel, backgroundTasks.ts
  NewSessionView, FolderPicker, PermissionMode   components/workdir/*
  ProfileSelect, ModelSelect                     TB components/claudeCode/* (via launchPickers slot)
  StatusLine                                     TB claudeCode/RouteStatusLine + session/SessionStatusLine
  useDeskDrafts                                  ThreadPanel draft per thread (utils/storage)
  useDeskQueues                                  server queue (messages sent mid-turn wait for the turn)
desk i18n (desk.*)                               projects.* (library) + claudeSession.* (TB)
flag desk                                        removed (flag projects)
```

## PR chain

```
#2082 design ─► #2087 sessions backend ─► #2088 working-folders UI
                                            │
                     #2094 waiting dot ◄─ #2093 cutover ◄─ #2092 parity + host capabilities

each branch has the previous ones merged in; review and merge left to right
```
