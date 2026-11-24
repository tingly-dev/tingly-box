# Projects panel: projects and working folders in one place

> TEP: [#2063](https://github.com/tingly-dev/tingly-box/issues/2063). Status:
> implemented. Desk has been folded in (`projects-desk-absorption.md`); this
> note describes the panel as it is now.

## What the user sees

The rail's **Projects** item is where Claude Code works from the browser. Its
sidebar lists *places*, not pages:

- **Projects**: real projects (coordinator + threads), starred first, with
  the number of threads waiting on the user. The title opens the project
  list; "+" starts a new project.
- **Working folders**: plain folders (saved ones first, then any folder with a
  session), with the number of sessions waiting on the user. The title opens
  every folder's sessions; "+" adds a folder.

Opening a folder shows that folder's sessions, and "New session" starts there.
One flag, `projects`, turns the whole panel on.

## How it is built

- `@tingly/projects` exports `ProjectsSidebar`. It lists projects and working
  folders through the package's own API client and refreshes on a slow poll
  and whenever a page reports a change (`PROJECTS_CHANGED`, fired by
  `useProjectsHost()`'s `onProjectsChanged`). Hosts can add sections through
  the `sections` prop (title, link, "+" action, rows with icon / selected /
  attention / trailing content) and hide either built-in one.
- Tingly Box's layout has `ActivityItem.sidebar`: a render function that
  replaces the sidebar's rows, so a sidebar can list things instead of pages
  and still get the column, collapse, flyout and mobile drawer. `children`
  still decide which routes light the item up.
- `pages/projects/ProjectsNav.tsx` (lazy, like the Projects pages) renders
  `ProjectsSidebar` inside the host provider; `projectsHost.ts` is the host
  config both it and `ProjectsRoute` use.

## Step 3a: one workspace shell (TEP step 3)

Direction (YZ, 2026-10-15): folder sessions adopt Projects' interaction; generic UI
lives in `@tingly/projects`, Tingly Box-only parts (profile, model tiers,
status line, terminal handoff) come in through props/slots, and the
standalone tingly-project keeps building.

- `WorkspaceLayout` + `useWorkspacePane` (package): header, main area and a
  resizable side pane that becomes a drawer below `lg`. ProjectPage and the
  working-folder page are built on it.
- `StatusGroups` (package): rows grouped by status, most urgent first, with
  collapsed groups for finished work. The project Overview's thread list and
  a folder's session list are both this component.
- The working-folder page is that shell: the header names the folder (or
  "Working folders" when unscoped), the side pane lists its sessions in
  groups, and the main area shows the open session or the new-session form.
- The one deliberate difference from a project: a session opens in the main
  area, not in the side pane, because a folder session is the work surface
  itself (a project's main area is its coordinator conversation).

## Step 3b: one transcript

Project threads and folder sessions render with one transcript
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

### Questions (AskUserQuestion)

TEP: [#2098](https://github.com/tingly-dev/tingly-box/issues/2098). Status:
implemented.

A question the turn waits on is a `QuestionCard`, not a text box: each of the
call's questions with its header, its options as buttons (label and
description), a box for other words, then Answer and Decline.

- **One click when it can be.** A single question with a single choice goes
  out on the option's click; several questions, or a multi-select, wait for
  Answer, which stays off until every question has an answer.
- **The suggested option is marked.** Claude Code appends "(Recommended)" to
  the option it suggests; the card shows the label without it and a
  "Recommended" tag instead, and sends the label back unchanged.
- **Typed words stand in.** For a single choice, typing replaces the pick and
  a pick clears the words; for several, the words are added to the picks
  (joined by ", ", as Claude Code expects).
- **The answer reaches Claude.** Claude Code reads answers from the tool's
  input (`answers`: question text → answer). The respond endpoints take
  `answers` alongside `answer`; the thread runtime puts them into the input it
  hands back, and records one readable line per question as the settled
  answer. A bare `answer` goes to the first question, and declining sends no
  answers, so Claude sees the question as declined rather than answered
  with nothing.

Not done here: a card that keeps working on the suggested option while
unanswered. Claude Code's question blocks the turn, so that needs its own
mechanism (a timeout, or a thread-level decision outside the tool).

### Reading a thread at a glance

TEP: [#2098](https://github.com/tingly-dev/tingly-box/issues/2098). Status:
implemented.

- **Done is one click.** The thread header carries a resolve icon (a folder
  session's is Archive, still asking first while background tasks run); a
  resolved thread shows Reopen in the same place. The More menu no longer
  repeats them.
- **What is new since the last visit.** The panel remembers per browser when
  each thread was last open (`seen.<thread>`, the newest event's time) and
  draws a "New" line above the first block that arrived after it. The mark
  is read once per visit, so the line stays where it was while the reader
  catches up; a first visit, or nothing new, draws no line.
- **What the turn is doing, and for how long.** While a turn runs and no
  request waits, the line under the transcript names the current action from
  the last call (running a command, reading, editing, searching the web,
  waiting on a subagent, thinking) and the time since the last message to
  the agent, ticking each second. A stuck turn reads differently from a slow
  one without opening anything.
- **The agent's progress card.** Thread and folder session panels pin a
  read-only card above the scrolling conversation, in both chat and trajectory
  views. Claude's `TodoWrite` snapshots and successful `TaskCreate`,
  `TaskUpdate`, `TaskGet`, and `TaskList` calls are folded from transcript
  events; background execution tools are separate. The latest group shows its
  own count and opens while unfinished, and completed groups remain accessible
  through “Previous task groups”. Changing the current group resets the default
  expansion; the user can always expand or fold it. Long lists scroll inside
  the card.
- **Grouping is presentation, not a Claude task-list boundary.** A successful
  create after every task in the current group has completed starts a new group;
  an update to an existing ID remains with its original group. A `TodoWrite`
  replaces its snapshot; after completion, a disjoint snapshot starts a new
  group. Overlapping snapshots remain in the same group. This is a heuristic:
  edited or repeated text and shared task lists can make the real intent
  ambiguous. Empty lists clear the current visible list. Grouping ignores
  elapsed time and never writes back to Claude. When the initial transcript is
  paginated, the panel reads the thread's history separately to reconstruct
  progress; missing events or external changes cannot be inferred. The fold is
  recomputed when the loaded events change, not on unrelated renders.
- **A run of calls is one line.** Calls and thoughts with no reply between
  them are one run; from two calls on it folds into a line saying what it did
  ("Read 3 files, ran 2 commands, edited 1 file · 30s") with failures in red,
  and opens to the per-call rows. The run still going, or the one right before a
  request waiting on the user, stays open and settles shut when the turn moves
  on; "expand all" opens every fold. The fold carries its calls' ids while
  shut, so the trajectory still leads to them. `TodoWrite` calls are left out
  of runs: the todo list has its own card.

## Step 3c: one message box

`Composer` in the package is now the only message box: the project
conversation, chats, threads and folder sessions all write in it.

- It took Desk's behaviour: Enter sends on desktop, on a phone Enter adds a
  line and Ctrl/Cmd+Enter sends, an IME's Enter never sends, a send or a stop
  never runs twice.
- Two policies for the text, picked by the host: a streamed reply (chat,
  conversation) clears the box at once and puts a refused message back;
  `keepUntilSent` (threads, sessions) keeps the text until the send is
  accepted, so a reload mid-request loses nothing.
- While a turn runs, Stop sits in the box (a thread's Stop moved here from
  its header); `sendWhileStreaming` keeps Send for hosts that take messages
  mid-turn (a thread or session reads them after the turn).
- Tingly Box's profile and model pickers and its status line come in through
  `ProjectsHost.session` slots. Nothing TB-specific is in the package.

## Not yet

- Nothing open. The rail dot is one waiting signal (TEP step 2): it lights
  when a project thread or a working-folder session waits on the user.
