# Project thread list: kanban view and a resizable top band

> Status: implemented. Touches `frontend/packages/projects` only; no backend or API change.
> Context: [projects-placement.md](./projects-placement.md) (where the Overview lives).

## What

A project's threads are listed by the **Overview**: in the side pane (default), a floating window, or as a
band at the top. Two additions:

1. **List | Kanban** in the Overview's Threads tab (side pane and window). Kanban draws the threads as
   columns by status, one outlined card per thread.
2. The **top band's** thread area can be stretched by dragging its lower edge.

## List | Kanban

- **Same board as the top band.** The band was already a kanban (one column per status). Its columns are
  now one component, `workspace/StatusColumns`, used by the band and by the pane's kanban view, so the two
  cannot look different. The list view is unchanged (`StatusGroups`).
- **Columns follow the thread statuses**, most urgent first: Waiting on you, Failed, Ready for review,
  Working, Idle, Resolved. Empty columns are left out. There is no "queued" status on a thread (queued
  messages are events, shown in the conversation), so Working is just Working. Resolved is a column in the
  pane; the band keeps showing only its count, as before.
- **Columns side by side, scrolling sideways, not stacked lanes.** The pane is 360 to 960px wide and
  full height, so real columns fit and each scrolls on its own. At the default 440px one and a half
  columns show and the rest scroll; widening the pane (it was already draggable) shows more.
  Alternative rejected: stacked lanes, which read as the list with boxes round the rows.
- **Cards are `ListRow outlined` + `stacked`**: title, then the subtitle (what it waits on, last report,
  branch), then commit/merged badges and the time. `stacked` is new and only the pane uses it: in a
  220px column the badges and time beside the title left it a letter or two. The band keeps the
  one-line card. The status mark is on the column heading (as in the band), not on every card.
- **Opening is unchanged**: a card calls the same `onOpen(thread id)` as a row; the open thread is the
  `selected` card (`aria-current="page"`), keyboard Enter/Space work (ListRow).
- **Persistence**: `tingly-project.overview.view` (`list` | `kanban`) in localStorage through the package's
  `readLocal/writeLocal`, like the pane width and placement. `uiPrefs` is Tingly Box's host-only store and
  the package must not depend on it. Default List.
- **No drag and drop.** A thread's status is the agent's (working, waiting, failed) or the result of an
  explicit action (resolve, reopen), not a free label, so dropping a card on a column would mean nothing.
- **The Workdirs folder sidebar has no kanban.** An earlier version added one there (stacked lanes). It
  was dropped: the request was about a project's threads, the folder tree already groups sessions per
  folder, and a second board with a different layout would be one more thing to learn.
- **i18n**: `overview.view.{label,list,kanban}`, `project.resizeBand`, in en and zh. The Projects package
  ships only en and zh (the host loads only those two into the `projects` namespace), so Russian falls
  back to English like every other Projects string.

## Resizing the top band

- **Handle on the edge that faces the content**: the band's lower edge, over the conversation. Drag down to
  grow, up to shrink; double-click resets; focus it and use ArrowDown/ArrowUp (Shift for bigger steps).
  The side pane's left-edge handle and this one are one component (`EdgeResizer` in
  `project/PaneResizer.tsx`), so look, behaviour, keyboard and a11y (`role="separator"`) cannot diverge.
- **Default unchanged**: until the user drags, the columns fit their content up to 152px of cards. The first
  drag starts from the height they have now (no jump). Once set, the height is fixed and the cards scroll
  inside it. Resetting (or dragging back to the default) returns to "fit the content".
- **Clamp**: at least 96px; at most 480px and never so tall that the conversation keeps less than 240px
  (`clampBand`, measured against the page column; the window until measured). A shorter window clamps the
  saved height for now and it comes back when there is room, like the pane's width.
- **Remembered per browser** (`tingly-project.overviewBandHeight`).
- **List vs kanban at the top**: the band is the kanban; it has no list mode, so the switch does not
  appear there. The pane's choice is independent.
- **Left navigation sidebar**: not resizable today and left that way. Its 200px width is a constant of the
  host shell (`frontend/src/layout`), shared by every activity panel, not Projects'; making it draggable is
  a host-wide layout change. The right side pane was already resizable.

## Files

`workspace/StatusColumns.tsx` (the board), `workspace/ViewSwitch.tsx` (switch and persistence),
`project/OverviewPane.tsx` (switch and view), `workspace/OverviewBand.tsx` (uses the board, owns the band
height), `project/PaneResizer.tsx` (`EdgeResizer`, `PaneResizer`, `BandResizer`, `clampBand`),
`ListRow.tsx` (`stacked`).
