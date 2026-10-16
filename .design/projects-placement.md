# Projects: where the Overview and a thread live

> Status: implemented for the project page (`ProjectPage`). Desk's session page keeps the one side pane.
> Wireframes: `projects-ui` canvas in tingly-design (`projects/tingly-projects-ui`).

## The problem

The project page had one side pane. The Overview (threads by status, Library) and an
open thread took turns in it, so looking at a thread meant losing the picture of
everything else, and nothing could sit next to a thread.

## Decision

Two choices, kept apart, in the header's Layout menu and remembered per browser:

| Surface | Choices |
|---|---|
| Overview | side pane (default), floating window, top bar |
| Thread | side pane (default), floating window |

- **Floating window**: drag by the title bar, resize from the corner, minimise to a pill
  in the corner (with a dot when something waits on the user), dock back from the title
  bar. Position and size are remembered. It sits over the page and takes no room from the
  conversation.
- **Overview in a window** is independent of the thread: opening a thread there opens it
  in the side pane or its own window; the Overview stays.
- **Overview as a top bar**: one column per status, what needs the user first; collapsed
  it is one line of counts. The conversation and the open thread split below it, so the
  thread always sits in the lower split and its placement choice is greyed out with the
  reason. The Library moves to its own window from the bar.
- The header's Overview button opens and closes whichever surface the Overview is on.
- Narrow screens keep the single drawer; the choices are hidden there.

## Why not

- A per-surface mode picker inside each panel: two orthogonal choices read better as one
  small menu than as modes scattered across panels.
- Letting the thread float while the Overview is on top: the band is for the overall
  picture, the thread belongs under it; two floating things over a split cost more than
  they give.

## Open

- Floating surfaces are not yet available on Desk's session page.
- Whether the window's position should follow the project (it is per browser now).
