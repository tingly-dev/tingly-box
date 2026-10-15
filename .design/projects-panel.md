# Projects panel: Desk and Projects in one place

> TEP: [#2063](https://github.com/tingly-dev/tingly-box/issues/2063), step 1.
> Written alongside the change; later steps extend this note.

## What the user sees

The rail's **Projects** item is where Claude Code works from the browser. Its
sidebar lists *places*, not pages:

- **Projects**: real projects (coordinator + threads), starred first, with
  the number of threads waiting on the user. The title opens the project
  list; "+" starts a new project.
- **Working folders**: Desk's plain folders (saved shortcuts first, then any
  folder with an open session), with the number of sessions waiting on the
  user. The title opens Desk with every folder; "+" adds a folder.

Opening a folder opens Desk scoped to it: Desk's own list shows only that
folder's sessions, and "New session" starts there. Desk no longer sits under
Remote (it never used a bot). `/desk` URLs are unchanged.

The two experimental flags stay separate. The rail item shows when either is
on, and its sidebar shows only the sections whose flag is on.

## How it is built

- `@tingly/projects` exports `ProjectsSidebar`. It lists projects through the
  package's own API client and refreshes on a slow poll and whenever a page
  reports a change (`PROJECTS_CHANGED`, fired by `useProjectsHost()`'s
  `onProjectsChanged`). Hosts add sections through the `sections` prop
  (title, link, "+" action, rows with icon / selected / attention / trailing
  content) and can hide the projects section (`showProjects`). The package
  has no notion of Desk or folders.
- Tingly Box's layout gained `ActivityItem.sidebar`: a render function that
  replaces the sidebar's rows, so a sidebar can list things instead of pages
  and still get the column, collapse, flyout and mobile drawer. `children`
  still decide which routes light the item up.
- `pages/projects/ProjectsNav.tsx` (lazy, like the Projects pages) renders
  `ProjectsSidebar` and builds the Working folders section from Desk:
  `useDeskWorkdirs` (folders + waiting counts) and `AddProjectDialog`.
  `projectsHost.ts` is the host config both it and `ProjectsRoute` use.
- Desk and the sidebar are mounted apart. Desk publishes each session list it
  loads (`DESK_SESSIONS_EVENT`), so counts move with the page; the sidebar
  polls slowly otherwise. Folder shortcuts are synced between the two
  `useDeskProjects` instances by `DESK_FOLDERS_EVENT`.

## Step 3a: one workspace shell (TEP step 3)

Direction (YZ, 2026-10-15): Desk adopts Projects' interaction; generic UI
lives in `@tingly/projects`, Tingly Box-only parts (profile, model tiers,
status line, terminal handoff) come in through props/slots, and the
standalone tingly-project keeps building.

- `WorkspaceLayout` + `useWorkspacePane` (package): header, main area and a
  resizable side pane that becomes a drawer below `lg`. ProjectPage is built
  on it; so is Desk.
- `StatusGroups` (package): rows grouped by status, most urgent first, with
  collapsed groups for finished work. The project Overview's thread list and
  Desk's session list are both this component.
- Desk's page is now that shell: the header names the working directory (or
  "Working folders" when unscoped), the side pane lists its sessions in
  groups (waiting on you, working, failed, done, archived), and the main
  area shows the open session or the new-session form. Desk's own left
  sidebar (`DeskSidebar`) is gone; the Projects sidebar picks the directory.
- The one deliberate difference from a project: a session opens in the main
  area, not in the side pane, because a Desk session is the work surface
  itself (a project's main area is its coordinator conversation).

## Step 3b: one transcript

A project thread now renders with the transcript Desk uses
(`@tingly/projects/transcript`), so both read the same way: tool calls as quiet
one-line rows with their result one click away, a settled approval folded into
its call ("Allowed"), the pending approval as one card, subagent cards, and
highlighted code.

- Threads keep their own event format (`ThreadEvent`). `threadTranscript`
  (in the package, next to `ThreadPanel`) converts it: calls pair with their
  results in order (thread events carry no call id), a settled request gets
  the answer entry the transcript expects, an expired one reads as not
  answered, and a report that repeats the last reply shows once.
- Two generic extensions on the transcript, both off unless the host passes
  them: `TranscriptEntry.author` names who sent a message (a thread's
  coordinator), and `rememberLabel` adds a second "allow" that also allows the
  tool from now on (a thread's "Always allow Bash here"); `onRespond` then gets
  `remember: true`.
- No server change: the API and event format are untouched (TEP step 4).

Next (3c): Desk's Composer (queue, drafts, IME) moves into the package and the
thread uses it, with TB-only controls (profile, model, status line) as slots.

## Not yet

- The rail dot still reflects Projects only (TEP step 2: one waiting signal).
- Desk's session list and the Projects thread list are still two components
  (TEP step 3).
