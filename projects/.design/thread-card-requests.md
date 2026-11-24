# Answering a thread's requests from its card

A thread that waits on an approval or a question can be answered where the
threads are listed, without opening the thread. Applies to the project page's
Overview (list, vertical and horizontal kanban) and the top band's columns.

## States of a card

| Thread state | Subtitle | Row under the card | Click on the card |
|---|---|---|---|
| waiting, 1 approval pending | `Bash` + command (first line, mono) | `● Needs your approval   Review ›` | opens the thread |
| waiting, 1 question pending | the question | `● Has a question for you   Review ›` | opens the thread |
| waiting, several pending | first request, `+N more` | `● N requests need you   Review ›` | opens the thread |
| waiting, nothing answerable (failed worktree, a report that needs a reply, interrupted) | wait reason, as before | none | opens the thread |
| not waiting | as before | none | opens the thread |

Colour is only the dot (the waiting amber, same mark as the status heading).
The row is a nested outlined mini-card in the card, built on the shared
`DisclosureRow` (chevron, hover, focus ring, `aria-expanded`); it replaces the
wait reason under the title only because the reason paraphrases the request.

## The popover

`Review` opens a popover anchored to the row (not an inline expansion: board
columns are 220px and must not reflow while threads stream in). Contents:

- header: the thread's mark and title, and `Open thread` for the full context;
- one card per pending request, oldest first, scrollable. Each is the transcript's
  own `PendingRequestCard` (Allow / Always allow here / Deny with the whole
  command and `Details`; a question's options, free text and Decline), so the
  thread page and the card answer identically and share their drafts by request id.

Keyboard / aria: the row is a button with `aria-haspopup="dialog"` and a name
(`Needs your approval · Review`); the popover is `role="dialog"` named after the
thread; Escape closes and focus returns to the row. Clicks inside never reach
the card (the card is a button too).

## Settling

- Answered here: the card shows "Response sent", the thread frame moves it on and
  the request leaves the list.
- Answered elsewhere (another tab, the thread page, a chat): the stream frame
  that settles it removes it live. If it is answered here a moment too late the
  server returns 409; the request is dropped, the list re-read and the popover
  says "Already answered elsewhere".
- Everything settled: "Nothing is waiting on you here any more", closing itself
  after ~1.6s.
- Refused (non-409): the card keeps the answer and offers retry; the message is
  toasted like on the thread page.

## Data

The server is the authority on what is pending: `GET /threads/{id}/requests`
returns the thread's pending requests from its prompter (the same set
`/respond` accepts), as transcript events. The project's event stream only says
when to look again: `thread_event` frames of kind `approval` / `ask` add or remove
a request at once (a frame invalidates any read that began before it); a thread
becoming `waiting` is read once; a thread leaving `waiting` is cleared; a stream
reconnect re-reads every waiting thread. `ProjectPage` owns the store
(`useThreadRequestsStore`) and provides it to the cards through context, so a
page without it (a folder's session list) shows no requests.

## Notifications

A "waiting" notification for a project thread deep-links to
`/projects/:id?review=<thread>`. The page opens that card's popover when the
thread has pending requests, and opens the thread as a page when it does not
(failed worktree, or answered meanwhile). Failed and finished pings, folder
sessions and decision cards route as before.

## Not covered

- The coordinator's decision cards live in the project conversation and are
  already answerable there (`/events/{eid}/choose`).
- The conversation's "needs input" line for a thread, and folder sessions' list
  (`WorkdirPage`), still open the thread; the strip/popover is built to be reused
  there (`ThreadRequestsStrip` reads the store from context).
