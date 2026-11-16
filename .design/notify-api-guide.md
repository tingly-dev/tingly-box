# IM Notify API guide

> Status: implemented. Refs [#2443](https://github.com/tingly-dev/tingly-box/issues/2443)
> (IM association needs better guidance — the empty state in step 2 is the part
> of that this guide covers). Builds on
> [`bot-interaction-api.md`](bot-interaction-api.md) (the API itself) and the
> chat/group aliases (#2364).

## What was wrong

The old guide was a drawer of prose and two near-identical JSON blocks.

- **Layering.** Opened from the bot's Notify dialog, the right-hand drawer
  (MUI z-index 1200) slid in *under* the dialog (1300): two overlapping layers
  and a dimmed page.
- **Clipping.** Code blocks scrolled sideways inside a 440px drawer, so the
  `target` line and the `//` comments were cut off.
- **UUID hunt.** Step 3 told the reader to find an internal target UUID in a
  tooltip elsewhere on the page.

## Design

The guide is organised around the one decision the reader owns — *where to
deliver* — and generates everything else (ux-principles #1, #5, #8, #11).

1. **Turn Notify on** — shown as state (check mark, or a warning when off), not
   instructions.
2. **Pick where to deliver** — one dropdown of the bot's chats and groups,
   labelled `platform · alias · kind · id` (alias falls back to the group's
   platform name). Blocked targets are hidden; targets not yet allowed to
   receive stay listed, marked, and disable Send test. With several Notify bots
   the list is grouped by bot, so the bot id is never asked for separately. An
   empty list says exactly how a chat appears (send the bot a message; pair
   first when the bot requires it).
3. **Send it** — one sample with tabs (curl / JSON / JavaScript / Python),
   built from the picked target, the gateway's real base URL and
   `$TINGLY_TOKEN`; a **Send test** button sends exactly that request through
   `api.notifyBot` and shows `Delivered · 120ms` or the error.

Under the steps, **Request fields** replaces the `//` comments: name, type,
required/optional, one line. The status codes sit under it.

### Decisions

- **Token never printed.** Samples read `$TINGLY_TOKEN` (env var). The guide
  says it is the web UI's own user token (no new credential, see
  `bot-interaction-api.md` §3.1). Send test uses the web UI's token itself.
- **Single top layer.** On the Bots page the guide *replaces the dialog body*
  (back arrow in the title) instead of opening a second surface. The generic
  `GuideDrawer` also now sits above modals (`zIndex.modal + 1`) so any other
  guide opened from inside a dialog (e.g. the bot setup dialog) cannot hide
  behind it.
- **Width-agnostic.** `NotifyGuide` uses a CSS container query: one column
  below 760px of *its own* width (drawer, phone), two columns (steps + fields |
  code) in the wide dialog. Code blocks use `wrap`, so nothing is clipped or
  needs sideways scrolling. `GuideDrawer` accepts an optional `width`.
- **Shared target loading.** `notifyTargets.ts` owns listing chats/groups
  (previously inline in `BotNotifyGroup`) plus the label format, so the Notify
  card and the guide cannot disagree. `NotifyTarget` gained `alias`, and the
  chat node now shows the alias too.
- **Sample message** is a harmless "Hello from Tingly Box" test, not a fake
  build failure, because Send test delivers it to a real chat.

## Not done / follow-ups

- `pages/notify/NotifyPage.tsx` is no longer routed (`/notify` redirects to
  `/bots/overview`); it was only updated so it still compiles with the new
  props. It could be deleted.
- `level` is passed through as a hint (`Meta["level"]`); no platform renders it
  differently yet, so the guide calls it a severity hint.
- Interact (`/interact`, `/wait`) has no guide; this one covers one-way notify.
