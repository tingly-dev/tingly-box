# Project thread list: kanban view and a resizable top band

> Status: implemented. Touches `frontend/packages/projects` only; no backend or API change.
> Context: [projects-placement.md](./projects-placement.md) (where the Overview lives).

## What

A project's threads are listed by the **Overview**: in the side pane (default), a floating window, or as a
band at the top. Two additions:

1. **List | Vertical kanban | Horizontal kanban** in the Overview's Threads tab (side pane and window). The
   kanbans draw the threads by status, one outlined card per thread, as columns or as lanes.
2. The **top band's** thread area can be stretched by dragging its lower edge.

## List | Vertical kanban | Horizontal kanban

The Threads tab's switch has three states, one icon each: List, Vertical kanban (status columns), Horizontal
kanban (status lanes). A single three-way switch rather than List | Kanban plus an orientation control:
the orientation would only mean something while Kanban is on, so a second control would be disabled or
hidden half the time (a mode picker inside a mode picker).

**Principle: the board scrolls in one direction only, vertically.** The pane is 360 to 960px wide and the
board takes exactly that width and grows downward; the pane's own scroll is the only one. No column or lane
has a scrollbar of its own and nothing scrolls sideways (the pane clips x). Two directions of scrolling
(an earlier version scrolled the columns sideways and each column vertically) hide cards off-screen in
both axes and make "what needs me" a hunt.

- **Same board data as the band**: same statuses in the same order (Waiting on you, Failed, Ready for
  review, Working, Idle, Resolved), empty ones left out, same card (`ThreadCard`, shared with the band's
  `StatusColumns`). There is no "queued" status (queued messages are events), so Working is just Working.
  Resolved is a column / lane here; the band shows only its count.
- **Vertical**: a CSS grid of columns, `repeat(auto-fill, minmax(min(220px, 100%), 1fr))`. It reflows with
  the width: one column at the default 440px, two at about 520, up to four at 960; the extra columns wrap
  onto a second row of columns instead of scrolling. Cards stack under their status heading, top-aligned
  so a short column does not stretch.
- **Horizontal**: a lane per status, stacked down the pane and divided by a hairline. Cards flow left to
  right inside the lane and wrap onto further lines (the same auto-fill grid), so a busy lane grows
  downward rather than scrolling sideways. Considered and rejected: a single line of cards per lane that
  scrolls sideways (a pane of 360 to 960px shows two or three cards, so it would scroll all the time, and
  vertical scroll would still be needed once there are enough lanes: two directions again). The heading sits
  above its cards in a narrow pane; once the pane is 640px or wider (a container query on the board, not
  the window) it moves to a 150px rail on the lane's left, which is where horizontal reads differently
  from vertical. In a narrow pane (one card per line) the two kanbans are close to one another, by
  nature; the lane dividers and cards-flow-in-rows are what remain.
- **Cards are `ListRow outlined` + `stacked`**: title, subtitle (what it waits on, last report, branch),
  then commit / merged badges and the time. The stacked footer wraps, so in a 220px column the badge
  and the time drop to two lines instead of overflowing. The band keeps the one-line card. The status
  mark is on the heading, not on every card.
- **Opening is unchanged**: a card calls the same `onOpen(thread id)` as a row; the open thread is the
  `selected` card (`aria-current="page"`), Enter/Space work (ListRow).
- **Persistence**: `tingly-project.overview.view` = `list` | `vertical` | `horizontal`, through the package's
  `readLocal/writeLocal` (`uiPrefs` is Tingly Box's host-only store; the package must not depend on it).
  Default List. The value `kanban`, stored while the board had a single form (the columns), reads as
  `vertical`.
- **No drag and drop.** A thread's status is the agent's (working, waiting, failed) or the result of an
  explicit action (resolve, reopen), not a free label, so dropping a card on a column would mean nothing.
- **The left Workdirs tree is unchanged: no kanban there.** An earlier iteration added a switch and stacked
  lanes to it; it was reverted on the user's call (the board belongs to a project's thread list, the tree
  keeps its folder-and-sessions look). `NavTree`, `ProjectsSidebar`, `WorkdirPage` and `StatusGroups` are
  identical to `main`; a test guards that the section has no switch.
- **i18n**: `overview.view.{label,list,vertical,horizontal}`, `project.resizeBand`, in en and zh. The
  Projects package ships only en and zh (the host loads only those two into the `projects` namespace), so
  Russian falls back to English like every other Projects string.

## The top band keeps its own layout

The band is a fixed, draggable height, so it cannot be the grow-downward board: `workspace/StatusColumns`
stays what it was (columns side by side, each scrolling inside, sideways only if the window is narrower
than its columns) and the pane's boards are `workspace/StatusBoard`. Only the card is shared. Applying the
one-direction principle to the band would mean choosing between wrapping columns (the band's height then
stops being the user's) and dropping the sideways scroll; left as is, a follow-up if it turns out to be a
problem. The switch does not appear there; the pane's choice is independent.

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
- **Left navigation sidebar**: not resizable today and left that way. Its 200px width is a constant of the
  host shell (`frontend/src/layout`), shared by every activity panel, not Projects'; making it draggable is
  a host-wide layout change. The right side pane was already resizable.

## Files

`workspace/StatusBoard.tsx` (the pane's vertical and horizontal boards), `workspace/StatusColumns.tsx` (the
band's columns, and `ThreadCard`), `workspace/ViewSwitch.tsx` (switch and persistence),
`project/OverviewPane.tsx` (switch and view), `workspace/OverviewBand.tsx` (uses the columns, owns the band
height), `project/PaneResizer.tsx` (`EdgeResizer`, `PaneResizer`, `BandResizer`, `clampBand`),
`ListRow.tsx` (`stacked`).
