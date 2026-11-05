# Workspace home

> Status: implemented. Frontend only (`frontend/packages/projects` + Tingly Box routing); no backend or API change.
> Context: [projects.md](./projects.md), [projects-placement.md](./projects-placement.md).

## What

Entering Workspace (the rail item, the Power-ups menu) opens `/workspace`, a page titled **Workspace**, instead of the
Projects list. It splits the entry in two and teaches each side:

- **Projects**: one line on what it is for, the primary action (New project), and either
  - a three-step guide when the user has no projects, or
  - their three most recently active project cards (the same `ProjectCard` as the list) and an "All projects" link.
- **Workdirs**: the same shape: one line, "Add working folder", then a three-step guide when there are no working
  folders, or the three most recently active folders (`WorkdirCard`) and "All workdirs".

The two sections are independent: a user with projects but no folders sees cards on one side and the guide on the other.

## Decisions

- **Always lands here.** No "remember last used" and no setting (ux-principles: smart defaults over toggles). The rail's
  `defaultPath` is `/workspace`; so is the Power-ups entry.
- **Existing pages unchanged.** `/projects`, `/projects/folder`, `/projects/:id`, `/projects/im` and the sidebar are as
  before. The home links to them: New project is `/projects?new=1`, Add working folder is `/projects/folder?add=1`,
  "All …" is the list / Workdir page. The rail stays lit on the home and on every page under Workspace.
- **Guide only while empty.** Education sits where the user would act (ux-principles #8) and leaves once it has been
  used; it is not a tour or a dismissable banner. Empty is judged per side from the shared project list
  (`useProjectList`), the one the sidebar already reads, so the page adds no request or stream of its own.
- **Recent = last activity.** Sorted by `updated_at`, newest first, three of each (`RECENT`). Not starred-first like
  the sidebar: the home answers "where was I", the list answers "what do I have".
- **No new route in the standalone Projects app.** `WorkspaceHomePage` is exported for hosts (`embed.ts`); only Tingly Box
  routes it (`/workspace`, behind the same Workspace power-up gate as the other Workspace routes).
- **Reuse.** `ProjectCard` moved out of `ProjectsPage` into `components/project/` so both pages share it. Strings are in
  the `projects` namespace (`workspace.*`, en and zh; Russian falls back to English like the rest of the package).

## Mock mode

`?mockData=newcomer` now also gives the Projects mock an empty project list, so the guide state can be previewed.

## Tests

`WorkspaceHomePage.test.tsx` (empty / populated / mixed / loading, recent cap, navigation targets),
`useActivityItems.workspace.test.tsx` (the rail opens `/workspace` and stays lit on every Workspace path),
`routes.contract.test.tsx` (`/workspace` resolves to its own page).
