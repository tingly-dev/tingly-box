# Transcript message copy

Reading a reply in the transcript and wanting its Markdown (to paste into an
issue, a doc, another agent) used to mean selecting the rendered text by hand:
formatting is lost, code blocks come out mangled, and a selection can drag in
the hover time. Each message now has a copy button that puts its **own source
text** on the clipboard.

## What is copied, per block

`transcript/copyText.ts` (`messageCopyText()`, pure) decides from the folded
`TranscriptBlock`. The source is the entry's `content` as the engine wrote it,
never the DOM, never the rendered Markdown.

| Block | Source text | Copy button |
|---|---|---|
| user message | `TranscriptEntry.content` — the user's words (`buildBlocks` in `transcript/model.ts`) | yes |
| assistant reply | `TranscriptEntry.content` — the raw Markdown | yes |
| assistant `send_file` (attachments) | `content` is the file's caption, not a reply | no |
| activity · thinking | `ThinkingStep.text` (`model.ts` `buildBlocks`, `case 'thinking'`) | no |
| activity · tool call / result | `ToolStep.input` (structured) and `ToolStep.result` (as returned) | no |
| request / question | `content` is the tool name or question title; the real payload is structured | no |
| error | engine text | no |
| command | `CommandPayload` (figures + optional summary), not a message | no |
| system / custom | host lifecycle notes, rendered by the host | no |
| agent (subagent) | prompt/report are shown in the card; the run is not a conversation message | no |

Only conversation messages (`user`, `assistant`) get the action: they are the
blocks that carry the hover time, they are what a reader copies, and the rest
are steps/notes/cards whose text is not Markdown. A thinking step or a tool's
Input/Output can still be selected by hand in its expanded section. This keeps
the action row from repeating on every row of a long run.

What is **not** copied: the host's `context` line (its own words, per
`TranscriptEntry.context`), `author`, attachments, `timestamp`, and anything
the UI adds (labels, outcome tags, the time). An empty/whitespace-only message,
or a file sent without a caption, yields `undefined` and gets no button.

## Where the button lives

The hover stamp and the copy button share one action row in the gap above the
message's corner (`MessageActions` in `transcript/Transcript.tsx`). It is
absolute-positioned, so it never moves the reading surface, and its bottom edge
is **flush** with the message's top edge: a margin there drops the parent's
`:hover` while the pointer travels from the text to the button, hiding the row
before it can be clicked (measured with a browser probe: the 2px gap gave
`opacity: 0; pointer-events: none` mid-travel and the click did nothing). On
pointer devices the row is quiet until the pointer is on the message **or the
copy button has focus** (`:focus-within`), which is what makes it reachable by
Tab. Touch screens have no hover, so there the copy button stays visible while
the exact time remains pointer-only (as before). The button keeps a 24px hit
area. The row uses the theme's tokens and the same `IconButton`/`Tooltip`
chrome as other icon actions; there are no literal colours or sizes outside
`fontSizes`.

## Feedback and i18n

`useCopyFeedback` (`@tingly/ui`) now exposes `failed` alongside `copied` (and
an `onFailed` callback); existing callers are unchanged. The button swaps to a
check for ~2s on success and to an error outline when the clipboard refuses
(permissions, insecure context), and its tooltip and `aria-label` name the
state. Strings are `projects.transcript.copyMessage` / `copyMessageDone` /
`copyMessageFailed` in the transcript locales (en/zh/ru).

## Keyboard and selection

The button is a native `<button>` (MUI `IconButton`): Enter/Space activate it,
the MUI focus ring is visible, and `aria-label` names it. It does not intercept
text selection in the message body; the action row sits above the content.

## Streaming

Copying is allowed while a reply streams: the click copies the text received so
far (the component re-renders with the growing `content`, and the handler takes
the value at click time). No buffering or debouncing.

## Tests

- `transcript/copyText.test.ts` — per-type extraction, exact raw Markdown,
  context/author/attachments/time excluded, empty and file messages.
- `transcript/Transcript.copy.test.tsx` — buttons only on user/assistant,
  clipboard gets the exact source text, success and failure states, native
  focusable button.
- `packages/ui/src/hooks/useCopyFeedback.test.tsx` — failure flag/onFailed.
