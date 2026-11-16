# Projects: one visual language

Draft for review (no code yet). Canvas: [`projects-visual/`](./projects-visual/)
(a Tingly Design project; open it with `npx tingly-design open .design/projects-visual`).
Previews: [`images/projects-visual/`](./images/projects-visual/). Catalog of Projects
feedback: #2158. Builds on `ux-principles.md` P3, P4, P9 and `projects-ui.md`.

## Problem

The theme is shared with Tingly Box, which is right. What is not shared is how
Projects uses it: the same state looks different in each place it shows up, and
the page carries more hues than it has things to say (board B).

- *Running* is a blue dot in the sidebar, a cyan dot in list headings, a blue
  spinner on cards, a cyan spinner in the header, and the same blue as selection
  and the primary button.
- *Done* is a gray check, a green check, a filled green "Merged" pill or a
  dashed circle, depending on the surface.
- *Failed* has no place of its own: it is filed under "Waiting on you" (amber)
  except in task rows, where it is red.
- In code: 5 text sizes beyond the `fontSizes` tokens (13, 14, 15 px, `0.875rem`,
  `0.9375rem`), 6 border-radius values, status colours picked per call site
  (`getReadableAccent`, `toneColor`, `success.main`, `alpha(...)`).

## Direction (boards A, C, D, E, F)

1. **Quiet workbench.** Neutral by default; title, conversation and diff lead.
2. **Colour is for "needs you" and "failed".** Amber = blocked on the user, red =
   failed. Running, ready, done, merged are neutral and differ by mark and word.
   Colour sits on the mark (dot or icon) only; the word next to it, subtitles and
   banners stay neutral text, so a header or row never carries coloured text.
3. **Brand blue = selection + one primary action per region.** Never a status.
4. **One state, one look**, in every surface: sidebar, list, card, header, task.

| State | Mark | Word |
|---|---|---|
| needs you | amber filled dot | Waiting on you |
| running | neutral spinner | Working |
| ready | neutral ring | Ready (the Review button is the signal) |
| done | neutral check | Done / Merged |
| failed | red filled dot | Failed |

Type: title 14/500, body 14/400, meta 12, micro 11.5, code 12 mono. Radius: 12
(cards, panels, inputs), 8 (rows, buttons, selectors, tabs), 6 (pills), circle
(dots only). One list row everywhere: mark slot, title/subtitle, right meta.

## Folder page and info rail (board F)

The folder session page uses the same language as the thread page: the left nav
carries the state marks, the right info rail (Sessions, Environment, Changes,
Background tasks; see `projects-thread-rail.md`) uses the same card, row and
type scale, and its task rows use the same marks (`ThreadState`). The rail
adds no colours of its own: only "needs you" is amber, a failed task is red.

## Left nav tree (boards G and H)

Decided (YZ, 2026-10-23): Projects and working folders are one tree, drawn
with the same node: an icon that tells the type (stack for a project, folder for
a folder), the name, a fold caret after the name, and a rollup dot at the right
when folded (amber needs you, red failed, neutral running). Sessions sit under
the node lined up with its name, title 14/500, time and state mark at the end
(idle shows no mark). Sort needs-you, running, latest; the latest five (plus
anything running, waiting or open) show, the rest and archived ones sit behind
"Show N older…". Background tasks appear only while they run, under their
session. Fold state is remembered per browser and the open node starts open.
No tree/flat toggle.

Rejected: folder as a heavy two-line row (A), unindented sessions (B, first
choice, dropped once folders and projects had to look alike: without an icon
the folder level was hard to read), and removing the section titles (the "+"
entry points need them).

## Rollout (one PR per step, each with screenshots)

1. A single `ThreadState` component (mark + word, `compact` for dots only) in
   `@tingly/ui`, replacing `ThreadStatusChip`, `StatusIcon`, `StatusHeading`
   dots and the sidebar badge; add `failed` to the thread status the UI derives.
2. Colour budget: remove green/cyan/info from status call sites; `Merged`
   becomes a neutral pill.
3. Type and radius tokens (`fontSizes` extended to cover 14, one `radius` scale);
   replace literal values in `packages/projects`.
4. One `ListRow` primitive for sidebar, sessions, tasks, subagents, Overview.
5. Per-region primary action audit (header, thread card, permission card).

## Open questions

- Is "ready" worth its own mark, or should it fold into "done"? Drawn as a
  neutral ring so the Review button carries the signal.
- Failed threads are `waiting` with a reason today; step 1 needs a derived
  `failed` state (the list's `ThreadCounts` has none).
