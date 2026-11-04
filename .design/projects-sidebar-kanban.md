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

## Resizing the top band

"Top" is the Overview placement `top` (see [projects-placement.md](./projects-placement.md)): `OverviewBand`,
one column per status under the page header. Its thread area can now be stretched.

- **Handle on the edge that faces the content**: the band's lower edge, over the conversation. Drag down to
  grow, up to shrink; double-click resets; focus it and use ArrowDown/ArrowUp (Shift for bigger steps).
  The side pane's left-edge handle (`PaneResizer`) and this one are the same component (`EdgeResizer`
  in `project/PaneResizer.tsx`), so look, behaviour, keyboard and a11y (`role="separator"`) cannot diverge.
- **Default is unchanged**: until the user drags, the columns fit their content up to 152px of cards, as
  before. The first drag starts from the height they have now (no jump). Once set, the height is fixed and
  the cards scroll inside it. Resetting (or dragging back to the default) returns to "fit the content".
- **Clamp**: at least 96px; at most 480px and never so tall that the conversation keeps less than 240px
  (`clampBand`, measured against the page column, the window until measured). A shorter window clamps the
  saved height for now and it comes back when there is room, like the pane's width.
- **Remembered per browser** (`tingly-project.overviewBandHeight`), like the placement, the open/closed
  state of the band and the pane width.
- **Lists vs kanban**: the band is already the board (one column per status) and has no list mode, so
  there is nothing to switch; the sidebar's List | Kanban choice is independent. The sidebar kanban's
  lanes are not resizable (they are as tall as their cards and the sidebar scrolls).
- **Left sidebar**: not resizable today and left that way. Its 200px width is a constant of the host shell
  (`frontend/src/layout`) shared by every activity panel, not Projects'; making it draggable is a
  host-wide layout change. The right side pane was already resizable.
