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
  uncommitted, Review; later replaced by the PR cards, see below), Background tasks (folder sessions: running rows with
  elapsed time and a stop button, finished ones behind a count). A card's fold
  state is remembered (`thread.rail.<id>`).
- On a folder page with a session open (viewport above the `lg` breakpoint), the
  Sessions list is the rail's first card (`railTop`, rail on from 640px wide and
  not hideable) and the separate side pane and its header toggle are gone: one
  right column. Narrow screens keep the drawer. Project thread panes are
  unchanged.
- Data is the `Thread` record (see "Status fields"); the rail makes no request of its own.
- **Pull requests and Issues cards** (`ThreadPrsCard`, `ThreadIssuesCard`, after the host's cards): the PRs and the issues the thread opened or commented on through its tools, from `Thread.links`, one card per kind: `#12 title` as a link to GitHub, under it open / draft / merged / closed, for an open PR its checks and mergeability, and how old the reading is ("could not refresh" when the last refresh failed). Each card is absent when the thread has none of its kind. An open PR's row has a **Files changed** button that opens that PR's own diff (head against base: `projects/.design/github-tools.md`, "A PR's diff"), with **Merge…** for the PR of the thread's own branch. A branch with commits and no open PR has a row in the Pull requests card instead ("No PR yet · ahead of main · N commits · M files", **Merge…**). The Changes card is gone, and so is the Review button in the thread header and on the conversation's thread line; the diff belongs to a PR, so it is reached from the PR. The project's own **Issues & PRs** tab (below) lists every thread's links, each with its thread's title. The records and their refresh rules are in `projects/.design/github-tools.md`.
- **Issues & PRs tab** (`ProjectLinks`, beside Threads and Library in the
  project's overview pane): every link of the project's threads, newest
  first. It is drawn like the Threads tab, not like the rail: the same
  toolbar row (Open · N | Closed · N, the refresh-all button at its end),
  then quiet folding groups (Pull requests, Issues; the fold is remembered
  per project, as the Threads tab's) of `ListRow`s — kind/state icon in
  the mark, `#12 title` (link to GitHub) as the title, the state parts
  (checks, mergeability) then the thread's title (opens the thread) as the
  subtitle, the short reading age as the meta, refresh and details at the
  end. Only the list scrolls, inside the tab; the toolbar stays put. The
  scroll box is a plain block: the rail's cards in a flex column shrank to
  the pane (a clipping flex item has no min-content floor) and cut their own
  rows, so a long list could not be scrolled. Switching Open/Closed starts
  the other list at its top; a refresh or a thread event keeps the place.
- **Issues & PRs strip** (`ComposerStrip`, right above the message box): the
  same links as one line each (kind, `#12 title`, where it stands, Files
  changed / refresh / details), open PRs first, then open issues, then what
  finished. Every link is listed. The title row is a `DisclosureRow` (same chevron
  and header style as the thread-list groups) that folds the whole strip to
  that one row, which then shows the count, the open-PR count and a red dot
  when an open PR has failing checks or conflicts. The fold is remembered
  globally in the browser (`useGroupFolds('composer-strip')`, not per
  thread: it is a "this strip is in my way" preference, and a per-thread
  choice would reset on every thread switch); the default is open. Open, the
  list scrolls inside a cap of 5.5 rows, so many links never push the
  conversation out. The strip is there only when the thread has a link, so
  it is the short list worth seeing while typing. It no longer carries the folder and branch (the Environment card
  has them) nor the "answers itself at …" line (the question card says it).
- **Issues & PRs under list rows** (`ThreadLinkRows`, in `ThreadsView`'s
  list: a project's Overview and a folder's sessions): the list is where a
  thread's whole picture is read, so its links unfold under the subtitle as
  a vertical trail, one line each, in the strip's order (open PR, open issue,
  then what finished): kind/state icon, `#214 title` as a link to GitHub,
  and where it stands (open or draft, checks, mergeability), behind a thin
  guide line. Past three, and only when that hides two or more, the rest wait
  behind "Show N more", which opens in place. Clicks there never open the
  thread. Data is `Thread.links` of the thread list and its live events: no
  request of its own (refresh stays on the strip, the rail and the Issues &
  PRs tab).
- **Issues & PRs on cards** (`ThreadLinkPills`, `ThreadBadges links`, only on
  `ThreadCard`: the Overview's boards and the band's columns): a card is the
  compact mode, so its links fold into one pill for the first link in the
  same order (kind/state icon in its colour, `#214`, a red dot when an open
  item has failing checks or a conflict; the pill opens it on GitHub), the
  rest behind `+N`, whose card (hover, or click/tap on touch) lists every item
  with where it stands, each a link. Hover title and `aria-label` carry
  `repo#N title · state · checks`; clicks never open the thread. Same data,
  no request.
- **No commits pill in lists and on cards** (user's call, 2026-11-22): list
  rows, board and band cards, the status popovers and the conversation's
  thread lines show the links, Merged, the running-tasks count and the time,
  not how many commits a branch has: a count of commits says nothing about
  what to do next. The open thread's header keeps it (`ThreadBadges work`),
  and the rail's Pull requests card keeps "No PR yet · ahead of main · N
  commits · M files". Merged stays everywhere (a merge done from the Merge
  dialog has no PR to say so), except on a card whose leading pill is the
  merged PR of the thread's own branch, which already says it.
- **Auto-answer** (`AutoAnswerSelect`) is a picker in the message box's row
  of pickers, after the permission mode: both say how far the thread goes
  without the user, and that row is where the thread's behaviour is changed
  (the rail only reads it). Its label reads on its own ("Auto-answer in
  5 min"); the menu explains each choice. It sits outside the "applies next
  turn" hint of the model pickers: a waiting question is re-timed at once.
  Hidden once the thread is resolved. The setting itself:
  `projects/.design/projects.md` ("自动回答问题").
- With the rail on, the header's "N running" chip, the menu's Background tasks
  entry and the commits/uncommitted facts of the meta line drop out so nothing
  is shown twice.
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
| `input_tokens`, `output_tokens` | summed over the turn's API calls; input includes cache reads and writes | `engine.TurnUsage` ("usage" entry), from the stream's `result.usage` (per turn, final) |
| `cache_read_tokens`, `cache_write_tokens` | how much of the input the cache served / stored; hit rate = read / input | same |
| `context_tokens` | prompt size of the turn's last main-thread call (latest turn, not summed) | `TurnUsage.ContextTokens`, from the last main-thread `assistant` event's usage (input + cache read + cache write; those fields are final at message start) |
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
- Source of the numbers, and a trap: the usage entry is built from the stream's
  `result` line, but the agentboot pumps (`persistent_session.go`,
  `runner_execute.go`) used to treat it only as the turn boundary and never
  hand it on, so in real use no turn recorded usage while every test passed
  (`enginetest` sends a typed result straight to the sink). Both pumps now
  pass it on; `engine/usage_live_test.go` replays a real Claude Code 2.1.259
  capture through a fake resident process to keep that wired.
- What the converter relies on in a real stream: `assistant` usage is the
  `message_start` snapshot (output not final), `result.usage` is the turn's
  total, `result.modelUsage` / `total_cost_usd` are process-lifetime totals
  (only `contextWindow` and the model name are taken from them), and the
  per-call tally resets at each result so a resident process counts turns
  from zero.
- A failed result is always one transcript error entry (the converter's); a
  turn Claude starts by itself adds its own line only when it ended without a
  result (the process died).
- Existing rows: the new columns default to 0 (migration is additive), so a
  thread shows context and cache only from its next turn on; its token totals
  are unchanged.

## Quota card: a Tingly Box extension

What Claude Code's status line shows beside the model: how much quota is left.
Quota is a Tingly Box concept (`ai/quota`), so Projects stays out of it and
only offers a place for a host's own cards.

- **Extension point:** `SessionSlots.railCards({ thread, launch })` returns
  extra cards for a thread's rail and the coordinator's Status tab, after
  Usage. The pages call it and pass the result down as `railExtra` / `extra`;
  `launch` is `{ profile, model }`, how the thread's next turn launches (a
  session's own, a project thread's override or default, the coordinator's).
  `RailCard` and `Fact` are exported so the card folds and looks like the rest.
  The slot runs during render: return an element, keep requests inside it.
- **The card** is `frontend/src/pages/projects/session/QuotaCard.tsx`
  (strings: `claudeSession.quota*`). It reads the route a folder session's
  status line already reads (`GET /scenario/claude_code/route`; concurrent
  identical requests share one fetch): the routed provider and its windows
  ("60% left", a bar that shrinks as it is spent, reset time in the tooltip,
  `$12.40` for a balance), reusing the status line's colour and reset helpers.
  Refetched after each turn, kept with the thread it was asked for, and absent
  when there is no route or no quota. A folder session's own status line is
  unchanged; the card adds the same quota to project threads and the
  coordinator.
- **Left out on purpose:** Claude Code's `rate_limit_event` (5h / weekly on a
  Claude.ai login). Threads run through the gateway and nothing relays those
  headers, an OAuth provider's 5h / 7d already arrive through the route (they
  would show twice), and the event was never seen from a real subscription.
  Revisit for a standalone Projects on a direct login
  (`rate_limit_info.unifiedWindows.{five_hour,seven_day}`: `utilization` 0-1,
  `resetsAt` epoch seconds).
- **Not imported:** `ai/quota` would pull the whole `ai` module (openai-go,
  x/net, jwt) into the standalone `tingly-project` binary.

## Sidebar status (`ProjectsSidebar`)

- Project and folder rows say what runs and what waits ("2 running · 1 waiting")
  and carry a dot: amber when something waits, blue when work only runs.
- The open folder lists its latest live sessions under it (waiting, running,
  then recent; archived hidden; at most 3). One request, for the open folder
  only, read again when its thread counts change, so the sidebar adds no
  per-folder polling.
- Not done: a failed state. The list's `ThreadCounts` has none; a failed turn
  surfaces as `waiting` with a reason.

## Thread numbers

A thread is mentioned by its number in the project, `Thread.seq` (from 1,
stored, never reused; 0 for the coordinator, which shows none). IM already
used it (`[#3 Title]` tags, `@3 text`, `/session 3`); now the web and the
coordinator do too. Chosen over the 8-character id prefix: short, the same
name in every channel, and no new stored field.

- **Shown** as a muted `#3` before the title (`NumberedTitle`): list rows
  (project threads and folder sessions), board and band cards, the status
  popovers, the conversation's thread lines, the open thread's header.
- **Copied**: in the header the number is a button, and the thread menu has
  "Copy reference"; both copy `#3 Title` (`threadRef`, the IM tag) and say so.
- **Understood by the coordinator**: its thread table, `list_threads`,
  `get_thread` and `start_thread` results name a thread `#3 a1b2c3d4`; the
  thread tools take `#3` (also `#3 a1b2c3d4` or `#3 Title`, as copied) beside
  the id prefix (`findThread`: ids never start with `#`). The prompt tells it
  the user may name threads that way.
- A PR's `#214` and a thread's `#3` do not meet on screen: a PR number always
  sits behind its kind/state icon.
