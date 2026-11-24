# Transcript time markers

Long conversations (a coordinator chat over days, a thread that ran for hours) had no sense of *when*: every message looked like it was said just now. This adds quiet separators where time passes, and an exact time on hover. Nothing is added to a message's own header.

## Data (checked first)

No backend change is needed. Everything the transcript shows is a `ThreadEvent` row (`projects/internal/db/models.go`), and each carries `created_at` (immutable) and `updated_at` (moves on every write, used only as the catch-up cursor) over REST (`/threads/{id}/events`) and over SSE (`thread_event` frames carry the whole row).

| Surface | Source | Timestamp |
|---|---|---|
| Thread transcript, folder session | `ThreadEvent` → `threadTranscript()` → `TranscriptEntry.timestamp` | `created_at`, every kind (user, assistant, tool, task, approval/ask, error, command) |
| Coordinator chat (project conversation) | the main thread's `ThreadEvent`s, same adapter, same `<Transcript>` | `created_at` |
| Thread reports to the coordinator (`thread_update`), suggestions, decisions, `thread_started` | `ThreadEvent` kinds that pass through as custom entries | `created_at` |
| Subagent steps | `ThreadEvent` with `parent` | `created_at` |
| Standalone chat `Message` / `Conversation` | outside the transcript component | `created_at` exists; not rendered by `<Transcript>`, so out of scope here |

The folded model keeps a time for every block (`blockTime()`: a message's, an activity run's first step's, an agent call's). Nothing is inferred from array order. The only entries without a time are ones a host builds locally; they are skipped (no marker, no effect on neighbours).

## Design

A marker is a centred, quiet divider placed *between* blocks, never per message:

```
  ───────────── Tue, Nov 17 · 9:00 AM ─────────────      day, with its time (first block)
   [user]    Kick off the payments latency work…
   [agent]   Traces point at two hot spots…
  ────────────────── 2:20 PM ──────────────────          hour: silence of 1h or more
   [user]    Any news from the load test?
  ─────────── Yesterday · 8:30 AM ───────────            day, after a silence
   [user]    Good. Keep the pool thread going…
  ─────────────────── 8:10 PM ───────────────────
  ───────────────────── Today ─────────────────────      day crossed mid-flow (23:50 → 00:10): no clock
```

Hover on a message (pointer devices) shows its exact time above its corner, in the gap between messages, so nothing moves:

```
                                  Nov 19, 2026, 8:05:00 AM
                         ┌──────────────────────────────────┐
                         │ Morning. Where did the pool …    │
                         └──────────────────────────────────┘
```

### Thresholds

Decided in `transcript/timeMarks.ts` (`timeMarks()`, pure), over the blocks that are actually drawn, in display order. A marker goes before block *i* when its time is valid and any of:

| Rule | Condition | Shows |
|---|---|---|
| First | first timed block | day + time |
| New day | local calendar date differs from the previous block's | day, plus time only if also a silence |
| Silence | `time − previous ≥ 1 h` (`GAP_MS`) | time (plus day if the date changed) |
| Checkpoint | `time − last marker ≥ 3 h` (`CHECKPOINT_MS`) | time |

The checkpoint exists for agents that work for hours without a pause (tool calls every few minutes never trip the silence rule). One marker per boundary; a boundary never gets two.

### Formatting (one shared formatter)

All in `timeMarks.ts`, using `Intl` with the UI language (en/zh/ru need no extra strings):

- Day: `Today` / `Yesterday` (`Intl.RelativeTimeFormat`, `numeric: auto`: 今天/昨天, Сегодня/Вчера), then `Tue, Nov 17`, with the year only when it is not this year's.
- Time of day: the locale's own style (`2:32 PM`, `14:32`).
- Exact (hover): date + time to the second. Full (tooltip, `aria-label`): `Wednesday, November 18, 2026 at 8:30 AM`.
- Days are compared on the viewer's **local calendar date**, not by dividing elapsed hours, so a 23/25-hour DST day is one day. Gaps use elapsed milliseconds, which DST does not distort. "Now" is `serverNow()` (the corrected clock the "3m ago" labels already use). A re-render at local midnight turns "Today" into "Yesterday" on a page left open.
- Tags the runtime rejects (`en-US@posix`) fall back to the default locale instead of throwing.

### Stability (streaming, paging, scrolling)

- A marker depends only on the blocks *before* it, and `created_at` never changes, so events appended at the end (streaming, SSE, `?since=` catch-up) never move or remove an existing marker. Covered by a test that grows a transcript and compares the earlier dividers.
- Earlier history loaded above (`Load earlier`) moves the *leading* marker to the new first block when it is the same day, which is correct (the day starts there); content below keeps its place and the existing load-earlier scroll preservation applies. Markers have fixed height and no async content, so they cannot cause late layout shifts.
- A block a host chooses not to draw (a custom entry whose renderer returns nothing) is ignored for timing, so a marker never stands over nothing. Concise mode, which hides blocks, is handled by computing over the drawn blocks.
- Markers are not drawn inside a subagent's own work (`BlockList` has no `timeMarked`); only the conversation has them.
- When "New since last visit" lands on the same block, the time marker is drawn first, then "New".

### Consistency

One component (`TimeDivider` in `transcript/Transcript.tsx`) used by `<Transcript>`, which every conversation surface already renders: thread panel (project threads and folder sessions) and the coordinator chat. No host passes anything; hosts that embed the transcript get it. The library has no conversation, so nothing to do there.

### Accessibility

`role="separator"` with `aria-label` = the full date and time (the visible text is the short form; colour is not the only cue, there is text and a rule). Visible text is a `<time dateTime>`. The per-message hover stamp is `aria-hidden` (a pointer affordance); assistive tech gets the separators, which carry the full time.

### Density, print/export, mobile

- Collapsed/expanded and Concise/Detailed: markers follow what is drawn; tool expansion does not change them.
- Print/export: the dividers are ordinary text in the DOM, so selection/copy and print include them. There is no transcript export today; one would reuse `timeMarks()` + the formatters to emit the same lines.
- Mobile shell: a divider is one centred line and works at any width. Hover stamps are guarded by `@media (hover: hover)`, so touch screens rely on the dividers and carry no hidden affordance.

## Decisions and alternatives

- **Dividers over per-message stamps.** A stamp on every message is dense and mostly redundant inside a burst; the hover stamp keeps exact times one hover away.
- **Gap-based, not hour-bucket.** "Every time the clock hour changes" would put a divider between 14:58 and 15:02. A silence threshold marks real pauses.
- **Intl, not new strings.** en/zh/ru and any future locale for free; no translation to keep in sync.
- **Deferred: sticky day label while scrolling.** Not cheap here: the conversation pane and thread panel scroll differently, the checklist card is already sticky at the top, and stacked sticky dividers would need a measured offset. Dividers every day already say where you are within a screen or two.
- **Deferred: keyboard access to per-message time.** The messages are not focusable; making each a tab stop would cost more than it gives. Separators cover the screen-reader path.

## Tests

`timeMarks.test.ts` (thresholds on both sides of each boundary, midnight mid-flow, DST day in `America/New_York`, invalid times, prefix stability, leading marker moving when older history loads, labels in en/zh/ru, bad locale tag, year rule) and `Transcript.timeMarks.test.tsx` (day and hour dividers in place, accessible names, dividers staying put when events stream in, no marker over an undrawn block, hover stamp).
