# Projects: info rail (coordinator, threads, sessions) and sidebar status

Modelled on the side column of Claude's session page: a fixed column of small
cards, each answering one question, with the numbers shown instead of hidden
behind a popover.

## Thread info rail (`ThreadInfoRail`)

- Sits beside the conversation inside `ThreadPanel` when the panel is at least
  `RAIL_MIN_PANEL` (880px) wide; below that nothing changes (header chips and
  popovers stay). A folder session's main area is wide enough; a project's
  thread pane usually is not, until it is floated or docked wide. "Hide
  details" in the thread menu turns it off (`thread.rail`, per browser).
- Cards: Status (state, context fill, turns, last turn, last active; see
  "Status fields"), Environment (folder, base → branch, profile · model,
  permission mode, session id; read-only: the pickers stay at the message box),
  Usage (tokens in/out, cache read/write and hit rate), Changes (commits, files,
  uncommitted, Review), Background tasks (folder sessions: running rows with
  elapsed time and a stop button, finished ones behind a count). A card's fold
  state is remembered (`thread.rail.<id>`).
- On a folder page with a session open (viewport above the `lg` breakpoint), the
  Sessions list is the rail's first card (`railTop`, rail on from 640px wide and
  not hideable) and the separate side pane and its header toggle are gone: one
  right column. Narrow screens keep the drawer. Project thread panes are
  unchanged.
- Data is the `Thread` record (see "Status fields"); the rail makes no request of its own.
- With the rail on, the header's "N running" chip, the menu's Background tasks
  entry and the commits/uncommitted facts of the meta line drop out so nothing
  is shown twice. The Review button stays in the header: it is the action.
- Clicking a task row scrolls the conversation to the call that started it.
  Output and subagent detail still live in `BackgroundTasksPanel`.

## Coordinator status (the Overview's Status tab)

The project conversation had no status at all: only the Overview pane (threads,
library) sat beside it, and nothing told the user whether the coordinator was
working, how full its context was or what it had spent. The coordinator is a
Claude Code session like a thread or a folder session, so it gets the same
cards, not a column of its own.

- **Where:** a third tab, "Status", in the Overview next to Threads | Library
  (`OverviewPane`, `status` prop; no prop, no tab). The Overview already lives in
  the side pane, in a floating window, or as a band, and the tab comes with it:
  side pane and window show all three tabs; with the band, a "Status" button
  next to "Library" opens a floating window on that tab (`bandWindow`). There is
  no header toggle, no popover and no column beside the conversation. An earlier
  version had a rail beside the conversation; it was dropped because it took
  room from the conversation and duplicated the pane's place.
- **Cards:** the thread rail's Status and Usage cards (`StatusCard`,
  `UsageCard` in `infoRail.tsx`: one implementation for coordinator, project
  threads and folder sessions, including their fold state), plus Threads
  (working, waiting on you incl. failed, ready for review, resolved) and Model
  (configured, last reported, effort, session id), which only the coordinator
  has. The coordinator's queued-input count is an extra row of its Status card.
- **One data path:** `GET /projects/{id}` returns the main thread as
  `coordinator`, with the status of a thread. The row does not track it; the
  handler (`coordinatorView`) gives it `working` while the coordinator's queue
  runs a turn, `waiting` (with a reason) while an interrupted input needs a
  recovery decision, otherwise `idle`. The page therefore reads state, tokens and
  context from a `Thread` exactly as it does for any thread, with no
  coordinator-only rendering. `coordinator_queued` (inputs in state `pending`)
  is the one coordinator-only number. The page already re-reads the detail
  whenever the coordinator stops being busy, so the numbers refresh each turn.
- **Usage on the row:** like every thread, the coordinator's usage lives on its
  main `Thread` row (`Thread.AddUsage`), through the same `turnTally` the worker
  threads use; no new table.

## Status fields

One set of fields for the coordinator, project threads and folder sessions;
all live on `db.Thread`, set by `Thread.AddUsage` when a turn ends.

| Field | Meaning | Source |
| --- | --- | --- |
| `status` / `queued` | idle, working, waiting, ready, failed, resolved; waiting for a slot | thread store (threads, sessions); coordinator: derived from its queue and recovery state by the project detail handler |
| `turns` | completed turns | existing |
| `input_tokens`, `output_tokens` | summed over the turn's API calls; input includes cache reads and writes | `engine.TurnUsage` ("usage" entry, from Claude's `result` and its per-call assistant usage) |
| `cache_read_tokens`, `cache_write_tokens` | how much of the input the cache served / stored; hit rate = read / input | same |
| `context_tokens` | prompt size of the turn's last main-thread call (latest turn, not summed) | `TurnUsage.ContextTokens` |
| `context_window` | the model's window, 0 when Claude did not report it (then the meter shows tokens without a percentage) | `TurnUsage.ContextWindow` (from `modelUsage`) |
| `last_turn_ms`, `last_turn_at` | duration and end of the last turn | `TurnUsage.DurationMS`, turn end time |
| `reported_model`, `model` | model that answered / configured launch model | existing |
| `session_id` | Claude Code session (first 8 chars; full id in the tooltip) | existing |
| `permission_mode`, effort | read-only facts | existing |

- Counting rules: a failed or stopped coordinator turn still adds its tokens
  (they were spent); subagent calls count toward tokens but not toward the
  context fill. A turn that never reached the model leaves the context
  figures as they were.
- Context fill colours: primary below 70%, warning from 70%, error from 90%.
- Not available: cost. Claude Code prices every call at Anthropic list prices,
  which is wrong for routed profiles, so `engine.TurnUsage` leaves it out; a
  cost line needs the gateway's own pricing. Also not shown: running background
  tasks for the coordinator (it has none; its tools are Projects MCP only).
- Folder sessions in Tingly Box still have the host's status line under the
  message box (`sessionUsage`, read from "usage" events); the rail now carries
  the same numbers from the thread row, so they also work for project threads,
  whose "usage" events are not stored.
- Existing rows: the new columns default to 0 (migration is additive), so a
  thread shows context and cache only from its next turn on; its token totals
  are unchanged.

## Sidebar status (`ProjectsSidebar`)

- Project and folder rows say what runs and what waits ("2 running · 1 waiting")
  and carry a dot: amber when something waits, blue when work only runs.
- The open folder lists its latest live sessions under it (waiting, running,
  then recent; archived hidden; at most 3). One request, for the open folder
  only, read again when its thread counts change, so the sidebar adds no
  per-folder polling.
- Not done: a failed state. The list's `ThreadCounts` has none; a failed turn
  surfaces as `waiting` with a reason.
