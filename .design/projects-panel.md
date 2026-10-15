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

## Not yet

- The rail dot still reflects Projects only (TEP step 2: one waiting signal).
- Desk's session list and the Projects thread list are still two components
  (TEP step 3).
