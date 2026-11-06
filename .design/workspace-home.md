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
- **IM binding** (host with IM, TEP #2143): a third section under the same shape — one line, "Bind a chat" (opens the
  IM settings view), a three-step guide (connect → message & grant access → bind) until the first binding exists, then
  the three newest bound chats (`ImChatCard`, newest `updated_at` first; opening one lands where the IM settings page's
  chat link does, via the shared `imDestination`). Without a host IM capability the section is absent, like the
  sidebar's IM row.
- **First-visit hint.** The hint is tied to the enablement of the Workspace power-up, not to page visits: enabling it
  (the gate's "Enable Workspace" dialog, or the Projects toggle in the experimental settings) calls
  `armWorkspaceImHint()` (`@tingly/projects/workspace-im-hint`), and the first render of the Workspace home after that
  shows the shared `CoachMark` (`@tingly/ui`, the rail's own tip style) beside the layout's IM entry — the sticky IM
  block at the sidebar's bottom (`IM_SIDEBAR_ANCHOR_ID` in `ProjectsSidebar`; the sidebar mounts lazily, so the page
  waits briefly for it). A layout without that entry falls back to the page's own IM section, scrolled into view.
  Showing it spends the arm, so it pops once per enablement. Any close of its "Got it" button is kept in localStorage
  (`workspace.imHintDone`): never again, not even when the feature is enabled again — enforced both when arming and on
  the page. While a hint is up but not dismissed, the rest of the browser visit stays quiet (`sessionStorage` flag),
  so a refresh does not nag. Only shown when the host has IM.

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
