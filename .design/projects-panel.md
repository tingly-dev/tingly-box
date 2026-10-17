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
