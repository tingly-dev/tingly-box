# Projects sidebar: kanban view for sessions

> Status: implemented. Touches `frontend/packages/projects` only; no backend or API change.

## What

The Workdirs section of the Projects sidebar lists each working folder with its sessions (`NavTree`).
Its header now has a two-button switch, **List | Kanban**. Kanban replaces the folder tree with one board
of every folder's sessions, grouped by status. It is still the sidebar: same width, same section, same
open/selected behaviour.

## Decisions

- **One style source.** The board is not a new look. It is `StatusGroups` (the folder page's and Overview's
  grouped list) with a new `outlined` prop that draws each row with `ListRow`'s existing `outlined` card
  look (the one `OverviewBand` already uses). Groups, labels (`session.group.*`), collapsed defaults
  (Resolved/Archived fold) and the row text (`folder · wait reason`, time) come from
  `workdir/sessionRows.ts`, which `WorkdirPage` now shares, so the two surfaces cannot drift.
  The status mark sits on the lane heading (as in `OverviewBand`); cards do not repeat it.
- **Stacked lanes, not horizontal columns.** The sidebar is 200px; side-by-side columns would show one
  column at a time and hide what waits. Vertical lanes, most urgent first (waiting, failed, working,
  idle, resolved, archived), show everything in one scroll. Rejected: horizontally scrolling columns.
- **One board across folders, not one per folder.** Kanban's value is "what needs me across everything".
  Each card names its folder in the subtitle; the tree stays available in List view. Cost: the board
  loads the sessions of every folder (the tree only loads open folders); it refetches a folder when its
  counts change, like the tree.
- **Remembered per browser** in localStorage via the package's `readLocal/writeLocal`
  (`tingly-project.nav.sessionsView`), like the folder fold state. `uiPrefs` belongs to the Tingly Box
  host, and the Projects package must not depend on it. Default is List, so nothing changes for
  existing users.
- **Open behaviour.** A card is a `ListRow` button (click, Enter, Space) that navigates to the same
  `?session=` URL the tree row links to (`sessionHref`) and calls `onNavigate`. The open session is the
  `selected` card (`aria-current="page"`). Cards are buttons, not anchors, so open-in-new-tab is not
  available (same as the Overview rows).
- **No drag and drop.** A thread's status comes from the agent (working, waiting, failed) or from
  explicit actions (resolve, reopen); it is not a free label. Dropping a card on a lane would have no
  meaning and would suggest a capability that does not exist.
- **Left out:** background-task child rows (shown under a working session in the tree) and the per-folder
  "Show older" cap. The sidebar tree has no unread state today, so none is added.
- **i18n.** `sidebar.view.{label,list,kanban}` and `sidebar.noSessions` in en and zh. The Projects
  package ships only en and zh (the host adds only those two bundles to the `projects` namespace);
  Russian falls back to English like every other Projects string.

## Files

`components/NavKanban.tsx` (board and loader), `components/workdir/sessionRows.ts` (shared groups and
rows), `components/workspace/StatusGroups.tsx` (`outlined`), `components/ProjectsSidebar.tsx` (switch,
persistence, `ProjectsSidebarSection.extra` slot), `components/NavTree.tsx` (`sessionHref`).
