# Workspace home

> Status: implemented. Frontend only (`frontend/packages/projects` + Tingly Box routing); no backend or API change.
> Context: [projects.md](./projects.md), [projects-placement.md](./projects-placement.md).

## What

Entering Workspace (the rail item, the Power-ups menu) opens `/workspace`, a page titled **Workspace**, instead of the
Projects list. Top to bottom it is: the **notifications control** (in the page header), **Needs you** (what waits on the
user, across everything), then Projects, Workdirs and one line of IM bindings. Projects and Workdirs split the entry in
two and teach each side:

- **Projects**: one line on what it is for, the primary action (New project), and either
  - a three-step guide when the user has no projects, or
  - their three most recently active project cards (the same `ProjectCard` as the list) and an "All projects" link.
- **Workdirs**: the same shape: one line, "Add working folder", then a three-step guide when there are no working
  folders, or the three most recently active folders (`WorkdirCard`) and "All workdirs".
- **IM binding** (host with IM, TEP #2143): a third part. Until the first binding exists it is a section of the same
  shape — one line, "Bind a chat" (opens the IM settings view) and the three-step guide (connect → message & grant
  access → bind). Once there are bindings it collapses to **one line** (a bordered row, like the other cards): the count
  and the newest chats with where each is bound ("Alice (ops) → Payments API latency · …"), and a "Manage ›" that opens
  the IM settings view. The per-chat cards are gone from the home (the IM page has them); the full list was the part
  that repeated another page. Without a host IM capability the part is absent, like the sidebar's IM row.
- **Needs you** (`components/workspace/NeedsYou.tsx`, `hooks/useNeedsYou.ts`). The first block under the header, shown
  only while something waits: threads whose status is `waiting` or `failed` in any project or folder (the same set
  the sidebar's waiting signal counts), waiting first, then most recently updated. Each thread is a nested card: a
  header row (state mark, title, "· project", the line that says what is asked, **Open ›**) that folds open into the
  thread's own approval or question card (`RequestCards`, the same `PendingRequestCard` a thread's card opens in a
  popover, #2544) answered in place — allow, deny, pick an answer. Folding is the shared `DisclosureRow`; until the user
  touches a row the first one that has something to answer is open and the rest are one line each. Threads that need
  the user but have no answerable request (a failed turn, a report that wants a reply) are a plain row; Open takes the
  user to the thread, to the same place a notification would (`threadLocation`, shared with the notification engine).
  Three rows show; "Show N more" reveals the rest.
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
  the `projects` namespace (`workspace.*`, en, zh and ru; other languages fall back to English).

### Where the data comes from

No new API. The shared project list (`useProjectList`, one all-projects stream) says which projects have threads waiting
or failed (its counts); only those projects' threads are read (`GET /projects/{id}/threads`) and then follow the stream's
`thread` frames (a frame upserts or removes the row). The all-projects stream does not carry transcript entries, so a
waiting thread's requests are read (`GET /threads/{id}/requests`) when it starts waiting and again on each of its
frames, and everything is re-read when the stream reconnects. `useThreadRequestsStore` (the store the project page
uses) holds them; it gained `reread(threadId)` for this.

## Notifications control

The browser-notification switch used to be a settings card at the bottom of the home page. It is now one quiet control
in the page header's actions (`notifications/NotificationControl.tsx`), the first thing on the page; the standalone
app keeps its Settings card (`NotificationSettingsCard`) on the same state (`notifications/useNotifications.ts`).

**On by default.** The app-level preference is on unless the user turned it off (`notificationsPreferred()`: stored
value is not `'0'`). The browser's permission is separate and cannot be granted without a click, so the control offers
it; nothing calls `requestPermission` on load. The engine runs when preference and permission are both there
(`notificationsEnabled()`); `syncNotifications()` starts or stops it, and the control follows the browser's own
permission change (`navigator.permissions` `change`), so allowing the site in the browser's settings turns it on without
a reload.

The control is "Notifications" plus the one next step, by status:

| status | when | shows |
|---|---|---|
| `on` | preferred and granted | bell, switch on; the tooltip on the label says what pings and when (hidden tab only) |
| `off` | the user turned it off | muted bell, switch off (turning it on asks the browser first if needed) |
| `ask` | preferred, permission not asked yet | **Enable** — the click opens the browser's dialog; if it is dismissed the control stays here |
| `blocked` | the browser refuses | "Blocked by browser" and **How to fix**, a popover saying to allow Notifications in the site settings |
| `unsupported` | no Notification API | the word "Not supported here", no action |

The engine's behaviour is unchanged: hidden tab only, the finished-turn cooldown, one ping per thread or card, click
deep link (`?review=`).

## Mock mode

`?mockData=newcomer` now also gives the Projects mock an empty project list, so the guide state can be previewed.

## Tests

`WorkspaceHomePage.test.tsx` (empty / populated / mixed / loading, recent cap, navigation targets, the one-line IM
summary, the notifications control above every section), `NeedsYou.test.tsx` (which threads, read only for projects with
something waiting, answering in place, open/fold, navigation, the cap), `NotificationControl.test.tsx` (each status, the
click-only permission request), `notifications/engine.test.ts` (default on, permission arriving later, `threadLocation`),
`useActivityItems.workspace.test.tsx` (the rail opens `/workspace` and stays lit on every Workspace path),
`routes.contract.test.tsx` (`/workspace` resolves to its own page).
