# Mobile session (thread) layout

> Status: **implemented** in this PR (2026-11-26), behind `usePhoneShell` (the
> phone shell; the desktop layout is unchanged). The design was approved
> (option A with B's per-call rows in the process sheet); implementation and the
> few deliberate deviations are in §10.
> The prototype is in `.design/mobile-prototype/` (`app4.js` + `session.css`; open
> `index.html?s=work,session&variant=A|B&theme=dark&w=390&h=844&frame=0`).
> Canvas: `.design/mobile-session/` (a Tingly Design project; open with
> `npx tingly-design open .design/mobile-session`); exported previews:
> `.design/images/mobile-session/*@2x.png` (01 session, 02 process sheet,
> 03/04 long text, 05 folder, 06 menu, 07 dark, 08 rationale).
> Screenshots: `library/outputs/tb-sign/mobile-session-{current,option}-*.png` (the
> design/prototype) and `mobile-impl-*.png` (the implementation), in the shared upload dir.
>
> This continues `.design/mobile-ui.md` decision 8 ("Concise by default") and
> rollout item 3 ("Workspace screens: … thread page with docked actions, sheets,
> keyboard handling"). It reuses the phone shell, `BottomSheet`, `GroupedList`,
> `DockedActions`, `useIsPhone` / `usePhoneShell` and the `mobile` tokens.

## The principle

> 移动端的目的就是为了简化方便，不是为了完全等效的体验，但是又能完成关键的工作。
>
> Mobile exists to be **simpler and handier**, not a full-equivalence experience
> of the desktop — but it must still get the **key work** done.

The phone must complete exactly these jobs, first-class and fast:

1. **Read the conversation** — the dialogue and the final report.
2. **Send / steer a message** — one message, an instruction, a nudge; stop a run.
3. **Approve or answer a blocking request** — without scrolling it away.
4. **See status and errors** — what the thread is doing, what failed.
5. **Stop a run** — the composer's stop is always there while a turn runs.
6. **Find a file or output** — the files the agent sent, the outputs it produced.

Everything else is **secondary**: at most one tap into a sheet, or explicitly
"open on desktop". When a desktop affordance would shrink the conversation, drop
or defer it rather than mirror it. This is why the session page has no project
band, no IM banner, no links strip, no detail toggle, no inline model/permission
pickers and no per-tool tallies in the collapsed run: each of those exists on the
desktop to give *more control or more structure at once*; the phone gives the
dialogue room and puts the rest one step away.

**Intentionally not equivalent on mobile** (entry point in the app kept where it
is cheap; otherwise the desktop is named):

| Desktop capability | On the phone |
|---|---|
| Full diff / PR file review, side-by-side panes | read-only change summary in the info sheet; full diff on desktop |
| Trajectory list/graph, "expand all tool calls" | the process/subagent sheets show what ran; the full trace is desktop |
| Tool input/output, raw payloads | process sheet (one tap), copy per section; never in the collapsed row |
| Branch / model / tokens / context / PRs | info sheet (read-only) |
| Model, effort, permission, auto-answer pickers | one settings icon in the composer → settings sheet |
| Rule / condition editing, drag-reorder, routing graph | view + change target in the rule sheet; editing/reorder desktop |
| Provider / credential / Team bulk editing, tables | desktop |
| Floating windows, placement, master-detail | one column only |
| Detailed/concise toggle, hover time stamp, expand-all | not shown; the phone is concise-only, time/copy are the message action |
| Checklist history, subagent full transcript inline | sheets |
| Image inpainting canvas, drag-to-reorder references | desktop |
| Full Library management (trees, uploads, zips) | preview a file; management desktop |

Destructive actions still confirm (archive/delete/restart) through the existing
confirmation sheet, because safety is not an "equivalence" feature.

## 0. The short version

On a 390×844 phone the current session page gives the **conversation 5–29 % of
the screen** (t-pool 29 %, t-cache 5 %, folder s-dark 6 %; at 360×800: 16 %, 4 %,
4 %). The rest is stacked chrome: three header bands, an expanded checklist, a
detail toggle, the links strip, five composer pickers and a quota line.

The proposal: **one compact session bar, a conversation-first transcript, a
one-line composer with its settings in a sheet, and the pending decision docked
above the composer**. Process (thinking, tool calls/results, subagent steps,
checklist history) folds to a 44 px line and opens one tap deep. In the
prototype this gives the transcript **57 %** of 390×844 with a request docked
(**80 %** once nothing is pending) and **54 %** at 360×800 — with every tappable
row at least 44 px (measured: 0 below 44 in both options).

Two options are compared below; the recommendation is **A (conversation-first)
with B's per-call rows inside the process sheet**, so nothing is hidden, only
one tap away.

## 1. Current state (real mock-mode screenshots)

Method: `frontend` dev server in mock mode (`USE_MOCK=true vite --mode mock`),
`?ui=mobile` to mount `MobileShell`, zh UI, 390×844 and 360×800, light and dark,
iPhone-class touch emulation. Three threads: `t-pool` (waiting on an approval),
`t-cache` (finished, report + code + links), and folder session `s-dark`
(checklist + subagent + background task + approval).

| Screen (data) | Viewport | Header stack | Transcript | Transcript share | Composer area† | Tab bar |
|---|---|---:|---:|---:|---:|---:|
| `t-pool` | 390×844 | 226 px | 243 px | **29 %** | 316 px | 59 px |
| `t-cache` | 390×844 | 226 px | 46 px | **5 %** | 513 px | 59 px |
| `s-dark` | 390×844 | 226 + 133 checklist | 51 px | **6 %** | 375 px | 59 px |
| `t-pool` | 360×800 | 279 px | 128 px | **16 %** | 334 px | 59 px |
| `t-cache` | 360×800 | 279 px | 32 px | **4 %** | — | 59 px |
| `s-dark` | 360×800 | 279 + 133 | 32 px | **4 %** | — | 59 px |

† everything between the transcript and the tab bar: the link strip, the
detail toggle, the composer and its pickers, the auto-answer row and the status
line. Measured with `getBoundingClientRect()` on the transcript scroller
(`overflow-y:auto`) and the tab bar.

Screenshots (same directory):

- `mobile-session-current-pool-390x844-{light,dark}.png` — approval thread: the
  only visible content is a stale `Bash` row and the `Allow Edit?` card; the
  user's brief and the assistant's reply are scrolled out.
- `mobile-session-current-cache-390x844-{light,dark}.png` — a finished thread:
  the transcript is a 46 px sliver; the links card (Issue/PR ×3) fills the middle.
- `mobile-session-current-session-390x844-{light,dark}.png` — folder session:
  the expanded progress card sits above a 51 px transcript, and the quota line
  wraps to three lines under the composer.
- `mobile-session-current-*-360x800-*.png` — worse: the IM banner wraps
  ("绑定聊天" on its own line), the header stack is 279 px, and the approval card
  is clipped by the fold.

### Problems, with evidence

| # | Problem | Evidence |
|---|---|---|
| P1 | **Three header bands.** Project title bar, "已绑定的 IM" banner, thread header + a meta line of status/model/badges. At 360 the IM row wraps, so the header is **279 px (35 %)**. | `pages/ProjectPage.tsx:436-481` (WorkspaceLayout header), `:471-475` (IM notice), `components/project/ThreadPanel.tsx:428-538` (thread header + meta). Screenshot `current-pool-360x800-light`. |
| P2 | **Composer is a 300–500 px stack.** Above/inside it: `MessageDetailControl` (详细/简洁), the `ComposerStrip` (Issue/PR card), the composer's `leftSlot` (profile/model, effort, permission, auto-answer), and `belowComposer` (quota status). | `ThreadPanel.tsx:596-672`; control at `:617`; strip at `:618`; pickers at `:620-641`; auto-answer `:635-638`; status `:670`. Screenshot `current-cache-390x844-light` (513 px). |
| P3 | **Checklist is above the scroll and open by default.** `open = choice ?? !finished`, so any unfinished list takes 130–200 px permanently; it also pushes the transcript down. | `ThreadPanel.tsx:573`; `transcript/Transcript.tsx:965-1022`, `:971`. Screenshot `current-session-390x844-light`. |
| P4 | **Detailed mode is the default** (phones included), and the detail rows are **24 px** tall inline disclosures, not 44. | `hooks/useMessageDetail.ts:17`; `components/common/Disclosure.tsx` inline `py:0.25`; `Transcript.tsx:204-214`. |
| P5 | **Per-message time is hover-only**; there is no message-level copy on this branch. Touch relies on time dividers. | `Transcript.tsx:699-715` (`display:none` outside `hover:hover`). The sibling branch's `.design/transcript-message-copy.md` adds a copy button (still a ~30 px icon) — see §6. |
| P6 | **No virtualization.** The latest `TAIL = 200` events load at once and every folded block renders; expansion/streaming re-renders the list. Only page-level "load earlier" exists. | `hooks/useTranscript.ts:7` (`TAIL = 200`), `:141,:154`; `ThreadPanel.tsx:582`. |
| P7 | **Process nests inside process.** A tool's input/output is a `Collapse` with nested pre; a subagent card nests its whole `BlockList` (its own tool rows, thoughts, requests) with left borders. | `Transcript.tsx:256-261` (tool detail), `:854-858` (subagent collapse), `:866-917` (AgentBody). |
| P8 | **The links strip is a tall card on a phone.** Each link is a 30 px row; three open links plus its header ≈ 150 px above the composer. | `components/project/ComposerStrip.tsx:90-105` (`ROW_H = 30`, header 44 on coarse). Screenshot `current-cache-390x844-light`. |
| P9 | **17–20 visible controls are under 44 px**: inline disclosure rows (24 px), the 42 px detail toggle, picker inputs (19 px), the code-block copy icon (~30 px), thread title copy (16 px). | Measured at 390×844: `t-pool` 18/35, `t-cache` 20/43, `s-dark` 19/37 under 44. Code copy: `components/Markdown.tsx` `IconButton size="small"`. |
| P10 | **The quota/status line wraps to three lines** under the composer (folder sessions). | `src/pages/projects/session/SessionStatusLine.tsx`, rendered via `ThreadPanel.tsx:670`. Screenshot `current-session-390x844-light`. |
| P11 | Long user briefs and tool outputs have no clamp/expand; a pasted brief occupies the whole transcript. | `conversationStyles.ts:10-18`; `Transcript.tsx:80-85`. |
| P12 | No horizontal page overflow (good): code blocks and tables scroll inside their box. Keep this invariant. | `conversationStyles.ts:36`; `components/Markdown.tsx` `pre { overflow-x:auto }`. |

## 2. What a phone session is for

| Phone: read, answer, send one thing | Desktop only |
|---|---|
| Read the dialogue (user ↔ agent) and the final report | Full diffs, side-by-side panes, the trajectory graph |
| Answer an approval or a question without scrolling it away | Editing rules/conditions, bulk provider work, drag/reorder |
| Check what is running, glance at progress/tokens | Long composition with many attachments, floating windows |
| Send one instruction / review a finished thread | The full subagent transcript side by side with the conversation |

The corollary: **the dialogue is the page; the process is reference material.**

## 3. Information hierarchy per segment

Two layers only:

1. **Dialogue layer** — user messages, assistant replies, files sent, errors, and
   the *one* request that is blocking. Always visible, never collapsed.
2. **Process layer** — thinking, tool calls/results, a run summary, subagent
   steps, settled requests, commands, system notes, checklist history. One line
   by default; opened on tap; raw payloads in a sheet.

| Segment (`transcript/model.ts`) | Phone default | Tap | Where the detail lives |
|---|---|---|---|
| `user` | bubble; **clamp at 10 lines** with the 展开/收起 row **inside the bubble's bottom edge** | expand | attachments as a row above; author/IM source as a caption |
| `assistant` (markdown) | reply; **clamp prose at 12 lines** with the 展开/收起 row **inside the block's bottom edge**; **code blocks have their own height cap + their own 展开 inside the code box**, horizontal scroll untouched | expand | — |
| `assistant` with files (`send_file`) | **one row**: up to 4 thumbnails + "N 个文件 · a 图 · b 文档"; a long caption clamps at 3 lines | file preview | file sheet / Library |
| `activity` · thinking | folded into the run line (or B: one "思考 · Ns" row) | expand | process sheet, full text |
| `activity` · tool call/result | **A:** one neutral run line "过程 · 3m" (no counts); **B:** one 44 px row per call | row | process sheet: counts, input/output per call, copy |
| `agent` (subagent) | one card: description + type/status (+ usage in B) | open | subagent sheet: steps + report; the full trace is desktop |
| `request` pending (approval/ask) | **docked above the composer**, 48 px allow/deny; "本线程都允许…" opens a sheet | — | sheet for extra options; the raw payload is behind "详情" |
| `request` settled | one quiet line: "允许 Edit · bench/results.md · 已允许" | row | process sheet / message action |
| `error` | one red line: "Bash 超时 · go test … · 120s"; long text clamps at 10 lines in its sheet | row | error sheet: exit code, output, retry |
| `command` (compact/clear) | one line: "会话已压缩 · 98k → 31k · 自动 · 12s"; a long summary clamps at 10 lines in its sheet | summary | compaction summary in a sheet |
| `system` / GitHub events | one quiet centred line / event note; long notes clamp at 6 lines | row | host note (already) |
| `custom` | host renderer, else one quiet line | — | host |
| checklist (`todoGroups`) | **one 44 px strip**: "进度 1/3 · 正在验证…" | expand (max 40 vh) | history in a sheet; full list in the progress sheet |
| working line / background tasks | one line under the last block | — | task panel (sheet) |
| time | time dividers (already) + exact time in the message meta | — | — |
| copy | per-message **right-aligned copy icon** (44 px) + code-block copy icon on the **right** of its footer/header | — | — |
| thread meta (branch, model, tokens, context) | **not on the page** | — | info sheet (the existing info rail content) |
| files/links/PRs of the thread | not on the page | — | files in the ⋯ sheet; PRs/branch in the info sheet |

Rules that decide the edge cases:

- **A request is never hidden.** It docks; while typing it folds to a 44 px
  "等待批准 …" line above the composer (still one tap from allow/deny).
- **A failed tool result is never silently folded** into a run summary: the run
  line carries a red "N failed" and stays one tap from the rows.
- **Information is never destroyed, only moved.** A folded *run* says only that
  it is process ("过程") and, if it fits, how long it took; the counts and the
  summary field are in the sheet, so the reader opens it to see what happened.
  A folded line that is itself one thing (a settled request, a command, an
  error) still names that thing. A run with a failure shows its status (a red
  dot / "有失败"), never silently disappears.
- **One level of expansion.** A 44 px row expands inline; its input/output opens
  in a sheet, not another accordion. Subagent steps open as a sheet/page, never
  nested inside the conversation transcript.
- **Long text is clamped, not destroyed.** Any text surface that can grow (a user
  brief, an assistant reply, an error, a command summary, a file caption) has a
  line/height cap — 10 lines for a user message, 12 for a reply, 6–10 for notes
  and sheets — with a 44 px 展开/收起 row; expanding is **in place**, and the
  per-message open state survives re-render/streaming. Short text gets no button
  (measured, not guessed). Copy always writes the original Markdown, never the
  clamped view. Code blocks are capped by height, not line-clamped, so their
  horizontal scroll and copy stay intact.
- **A file sent by the agent is not a message** (its text is a caption): one
  row, captions on the file itself (already the desktop rule).

## 4. Interaction details

**Touch targets.** Everything tappable is ≥ 44 px, primary/docked ≥ 48 px
(`mobile.tapMin` / `tapPrimary`). Concretely: inline `DisclosureRow` gets
`@media (pointer: coarse) { minHeight: 44 }` (the `ComposerStrip` header already
does this, `ComposerStrip.tsx:95`); the detail toggle is **not shown on a phone**
(concise is the only view; details open per row/sheet), so it costs nothing here;
the pickers are rows in the settings sheet, 44–56 px; the code-copy button is
44 px; the per-message action is 44 px. The visual glyph may stay 14–18 px — the
hit area is the row.

**Expand/collapse.** Tap the whole row; the chevron is decoration. Only one detail
open per run (opening another closes the previous) to keep the page short. The
row keeps its height while opening (no layout jump at the tap point). State is
per row while the page is open; "expand all" is a desktop tool and is not shown
on the phone.

**Gestures and scroll.** One vertical scroller (the transcript). No swipes, no
long-press-only actions (undiscoverable). Long-press on message text is the OS
selection — never intercepted. Horizontal panning exists only inside code blocks
and tables, with `overscroll-behavior-x: contain` and `-webkit-overflow-scrolling:
touch`, so a horizontal drag does not fight the vertical scroll or the back
gesture. Expanded blocks do not create nested vertical scrollers: tool output is
capped (20 lines) with "在面板打开"; the checklist expands within the page (max
40 vh) and pushes the transcript rather than scrolling inside itself.

**Time and copy.** Group time dividers stay (day/hour). Each message group has a
quiet meta row: the exact time on the left (user messages: on the right) and a
**right-aligned 44 px copy icon** — an icon, not a text button and not a `⋯`
menu. The copy writes the entry's raw Markdown via the sibling branch's
`messageCopyText()` (`.design/transcript-message-copy.md`), never the DOM. Code
blocks put the copy icon on the **right** of their footer (the expand/collapse
label is on the left); the desktop `CodeBlock` already puts the language left and
the copy icon right in its header (`components/Markdown.tsx`), so both surfaces
use the same icon and `useCopyFeedback` — the mobile change mirrors the desktop
rather than inventing a different control. Long-press still selects and copies
arbitrary text; a "locate in trajectory" action is desktop-only. On touch the
meta row is **visible** (the time is not hover-only), because a phone has no
divider for every message.

**Bottom safe area.** `viewport-fit=cover`; the composer's bottom padding is
`max(12px, env(safe-area-inset-bottom))`; the dock sits above the composer, and
sheets pin their actions above the home indicator. The session is a pushed
screen: the shell's tab bar is **hidden** on it (it comes back on Back), so the
composer owns the bottom edge. This is a change from today's always-on tab bar;
it is what the prototype shows and is called out as a decision.

**Keyboard.** The shell already tracks `visualViewport` and sets
`--phone-viewport-height` (`@tingly/ui/theme/phone`, `usePhoneShellBody`). On the
session page, when the keyboard opens: the tab bar and progress strip hide; the
full dock folds to the 44 px "等待批准 …" line; the composer stays directly above
the keyboard; the focused field scrolls into view. `100dvh` (never `100vh`) and
`overscroll-behavior: contain` prevent the iOS rubber-band showing the page
behind the keyboard.

**Long text and code (a component-level change).** A single reusable
`ClampedText` wrapper (in `transcript/`) is used by the user bubble, the
assistant renderer, the error line, command summaries, system notes and file
captions. It clamps by CSS line box (user 10 lines, assistant 12, notes/sheets
6–10, a file caption 3) and renders a 44 px `展开全文 / 收起` row **only when the
content actually overflows** (measured with `scrollHeight > clientHeight` after
layout/resize, so short messages get no button). The full text stays in the DOM
and in the entry's `content`; copy (`messageCopyText`) always writes the original
Markdown, never the clamped view.

**The control is the block's last row, inside it — not a sibling below the
block.** In the user bubble the row spans the bubble's bottom edge (it inherits
the bubble's bottom corners and a hairline); in the assistant block it is the
last row of the content with a hairline; in a code block the copy + expand row
sits inside the code box. A bottom **mask** fades the last visible line into the
row, so the truncated text and its control read as one unit. The row has its
own 44 px (measured body→row gap 8–9 px), so it **never overlaps the last full
line of text**; the faded line is the hint that there is more. Expanding is in
place: the row stays at the end of the full text, the transcript's `scrollTop`
is not touched, and the stick-to-bottom rule is not triggered. The open state is
keyed by the block/message id in the transcript (a `Set`/`Map`, not
component-local), so it survives re-render and streaming deltas; a reply that
starts short and grows gets its row when it crosses the threshold. The toggle is
a native button with `aria-expanded`/`aria-controls` (`展开全文` / `收起`), and
the clamped container keeps the full text for selection and assistive tech.

Code blocks are **height**-capped (≈ 210–240 px) with their own `展开代码 / 收起`
row inside the code box, not line-clamped: their `overflow-x: auto` and full-code
copy stay intact. The copy is an **icon on the right** of that row (the
expand/collapse label is on the left); desktop's `CodeBlock` already puts the
language left and the copy icon right in its header (`components/Markdown.tsx`),
so there is no desktop text button to change — and the mobile shell's phone
theme raises that same icon to 44 px (`@tingly/ui/theme/phone`), while the
desktop keeps its normal size. Tool output keeps its own 20-line cap in the
process sheet; a subagent report shows a 3-line preview in the card and the
full text (clamped at 10 lines with 展开) in its sheet. Assistant prose and a
code block are clamped independently, so a long reply with code never clips the
code's horizontal scroll.

Screenshots: `mobile-session-option-a-390x844-{light,dark}-long-user.png`,
`...-long-assistant.png`, `...-long-code.png` (collapsed, the row inside the
block) and the matching `-expanded.png`; `...-long-{user,assistant,code}-expanded-end.png`
show the 收起 row at the end, still inside the block/code box. Measured clamp:
user 264 px shown of 445, assistant 316 of 456, code 210 of 491; the row is
44 px with an 8–9 px gap after the clamped body (no overlap); a programmatic
toggle kept `scrollTop` at 944 → 944 (no jump).

**Streaming and waiting.** Auto-stick to the bottom only while the reader is at
the bottom (the existing `stick` ref, `ThreadPanel.tsx:574-580`); the "最新消息"
pill is already there (`:597-603`). A message written while a turn runs keeps its
"下一条消息会读到" note (`Transcript.tsx:87-91`).

**Accessibility.** Status never by colour alone (glyph + word + colour, already);
`aria-expanded` on disclosures; every icon button has an `aria-label`; the
expanded/collapsed state is in the label; reduced-motion is respected by the shell.

## 5. Performance (500+ messages)

Today: the latest `TAIL = 200` events load at once (`useTranscript.ts:7`), older
pages on demand (`:154`), a subagent's steps only when its card opens
(`:158-162`). Rendering is not windowed: `BlockList` maps every block, `Collapse
unmountOnExit` only helps closed subagents (`Transcript.tsx:256,854`), and the
transcript Markdown is not memoized (`transcript/Markdown.tsx`), so streaming
re-parses the growing reply.

Strategy, cheapest first:

1. **Fold before you render.** Concise-by-default on phones means an activity run
   is one row, not 50 tool rows; `Collapse`'s `unmountOnExit` already drops the
   children (`Transcript.tsx:256`). Keep subagent children unmounted until opened.
2. **`content-visibility: auto` + `contain-intrinsic-size` on each top-level
   block.** Offscreen blocks skip layout/paint while keeping scroll position and
   find-in-page; a one-line estimate (~44 px) is enough because folded blocks are
   short. This is a CSS-only win and works in current Chrome/Safari.
3. **Memoize the message subtree** by message id/content (so a streaming delta
   re-renders one block, not the list) and memoize `Markdown` (the host's version
   already is; the transcript's is not).
4. **Phone page size.** Keep the server pagination but lower `TAIL` to ~50 on a
   phone (or load 200 but render the last 50 + a "更早" button), so the first
   paint is small.
5. **Measure before virtualizing.** If 500 loaded blocks still miss 60 fps on a
   mid-range phone, add a measured virtualizer (`@tanstack/react-virtual`,
   dynamic sizes) to the transcript only. The design above (folded one-line rows,
   `content-visibility`) is chosen so this step is optional, not required.

Targets: first transcript paint < 100 ms after data on a mid-range phone; 60 fps
scroll; expansion opens within one frame. Measure with the Performance panel
(DOM node count, long tasks) on a 500-event fixture.

## 6. Two options (same data, same viewport, light/dark)

Both share the fixes that are not the subject of the choice: compact session bar
(project/IM/meta move to an info sheet), checklist as one line, one-line
composer with a settings sheet, docked request, 44 px targets, safe area,
keyboard handling, no tab bar. They differ in **how process is represented**.

### Option A — 极简对话流 (conversation-first)

- One run = one neutral 44 px line: "过程 · 3m" (no per-tool tallies); tap →
  the process sheet (counts, thinking, each call's input/output, copy).
- Subagent = one card line (name + type + status); tap → the subagent sheet.
- Checklist = one 44 px strip; tap → expand / sheet.
- Composer = one line; model/effort/permission/auto-answer in the settings sheet.
- A finished thread is nearly all dialogue (the "report + code" case).

Screenshots: `mobile-session-option-a-390x844-{light,dark}.png`,
`...-360x800-{light,dark}.png`; the process sheet
`...-a-390x844-{light,dark}-activity.png`; a scrolled view with the report,
code, settled request, compaction and error `...-a-390x844-light-scrolled.png`;
keyboard state `...-a-390x844-light-keyboard.png` (dock folded to one line).

Measured: transcript **480 px = 57 %** at 390×844, **436 = 54 %** at 360×800;
**676 = 80 %** at 390 once the request is resolved (docked removed); 0 of 18
visible controls below 44 px. A 4-call run occupies **44 px** (one line), and the
transcript content is **2247 px** tall (including the long-text fixtures).

### Option B — 保留结构但折叠 (structure kept, folded)

- Process stays visible as **one 44 px row per call** (思考 · Read · Grep · Edit ·
  Bash), each opens inline; the collapsed row names the tool and its target, and
  the result/count appears only in the opened row. A small "这一轮的过程 · 3m"
  heading groups them (no step/tool count).
- Subagent card shows the report preview (2 lines).
- Checklist is a card header (folded), expandable.

Screenshots: `mobile-session-option-b-390x844-{light,dark}.png`,
`...-360x800-{light,dark}.png`; inline expansion
`...-b-390x844-light-expanded.png`; scrolled `...-b-390x844-light-scrolled.png`.

Measured: transcript **480 px = 57 %** at 390×844, **436 = 54 %** at 360×800
(same chrome as A); **676 = 80 %** without the dock; 0 of 22 controls below
44 px. The same 4-call run occupies **272 px** (4 rows + heading), so its
transcript content is **2511 px** (+264 px over A): fewer messages fit without
scrolling.

### Comparison

| | A · conversation-first | B · structure kept |
|---|---|---|
| Transcript viewport share (390 / 360) | **57 % / 54 %** | **57 % / 54 %** (same chrome) |
| Without a docked request | **80 %** | **80 %** |
| Rows per 4-call run | 1 | 4 (+1 heading) |
| A 4-call run occupies | **44 px** | **272 px** |
| Transcript content height (this conversation) | 2247 px | 2511 px |
| Process at a glance | a neutral "过程" label (+ duration) | each call's name + target |
| To see one call's input/output | open the process sheet | tap that row |
| Info loss in the default view | that a run happened and how long; what it did is one tap away | none (each call is a line) |
| Risk | a reader may feel blind while a turn runs | process still occupies the page |
| Best for | triage/read/answer on a phone | following the work, reviewing a run |

## 7. Recommendation

**Ship A, with B inside it.** Default the phone to the conversation-first layout;
the process sheet that A opens is exactly B's per-call list (name, input, output,
copy), so nothing is lost, it is one tap away. This matches `.design/mobile-ui.md`
decision 8 ("Concise by default. Tool runs folded to one line; details are one
tap away") and the phone's job (read, answer, send one thing).

What is not on the default screen, and where it is found:

| Not shown by default | Found at |
|---|---|
| thinking text, tool input/output, per-tool counts | process sheet (per call), copy per section |
| subagent steps/report | subagent sheet; "open on its own" = today's `SubagentFocus` page |
| checklist history | progress sheet ("上一组任务") |
| settled approvals/answers | one-line row → process sheet |
| command/compaction summary | command row → summary sheet |
| thread branch/model/tokens/context, links/PRs | info sheet (the current rail's cards) |
| full diff, trajectory graph, rule editing | desktop (unchanged) |
| exact time | message meta row (visible on touch) / long-press selection |
| raw Markdown | message `⋯` copy (the sibling branch's `messageCopyText`) |

Cost (frontend only; no backend/API change; no new dependency):

| Slice | What changes | Files | Cost |
|---|---|---|---|
| Gate + default | `usePhoneShell()` decides the session layout; `?ui=mobile` already exists; default detail = concise on a phone | `ThreadPanel.tsx`, `useMessageDetail.ts`, `Transcript.tsx` | XS |
| Compact bar | hide the project/IM bands on the session page; bar = back, title + status, ⋯; thread details, subagents and files live in the ⋯ sheet, which names desktop-only depth | `ProjectPage.tsx`, `ThreadPanel.tsx` | S |
| Composer | one line (attach · settings · field · send/stop); pickers into the settings sheet; `ComposerStrip` + status line into the info/⋯ sheets; the detail toggle is dropped (concise-only); safe area; keyboard fold | `ThreadPanel.tsx`, `Composer.tsx`, new `SessionSettingsSheet` | M |
| Checklist strip | header-only 44 px on a phone; expand ≤ 40 vh; history sheet | `Transcript.tsx` `ChecklistCard`, `ThreadPanel.tsx:573` | S |
| Process folding (A) | one run line; `ActivityRun` concise; process sheet reusing `BottomSheet`/`GroupedList` | `Transcript.tsx`, new `ActivitySheet` | M |
| Per-call rows (B, inside the sheet) | the existing `ToolRow` with 44 px rows + input/output | `Transcript.tsx`, `Disclosure.tsx` | S |
| Subagent sheet | move `AgentBody` into a sheet; keep `SubagentFocus` for "on its own" | `Transcript.tsx`, `ThreadPanel.tsx` | M |
| Message meta row | time + a **right-aligned 44 px copy icon**; reuse `messageCopyText` (sibling branch); the desktop copy is the same right-corner icon | `Transcript.tsx`, `copyText.ts` | S |
| Long text clamp | **new reusable `ClampedText`** (line clamp + measured overflow + 44 px 展开/收起 inside the block, per-block state, `aria-expanded`); used by user/assistant/error/command/system/caption; code blocks get a height cap + 展开; **copy is a right-aligned icon in the footer (the desktop header already uses one)** and keeps the full original | `transcript/ClampedText.tsx` (new), `Transcript.tsx`, `conversationStyles.ts`, `components/Markdown.tsx` | S–M |
| Touch targets | coarse-pointer `minHeight: 44` on disclosure rows, pickers, code copy | `Disclosure.tsx`, `Markdown.tsx` | S |
| Performance | memoize message/Markdown; `content-visibility` per block; phone page size | `Transcript.tsx`, `transcript/Markdown.tsx`, `useTranscript.ts` | S–M |
| Tests | vitest with `matchMedia` (as `Composer.test.tsx`), plus the existing viewport script over 320–430 × short/regular | `frontend` tests | M |

Roughly **2 frontend days** for the S/M slices above, excluding a virtualizer.
CSS/layout-only work is the bar, checklist, composer layout, safe area and touch
targets; component work is the concise run line, the sheets, the message meta
row and the memoization.

## 8. Decision points

**Approved** with the recommended answers (LGTM, 2026-11-26): option A with B's
per-call rows inside the process sheet, plus the refinements above. The list is
kept for reference; re-open an item if it should differ.

1. **Default density on a phone:** conversation-first (A) or structure-kept (B)?
   (Recommend A, with B one tap deep.)
2. **Header:** may the session page drop the project title bar and the IM
   banner, moving that context into the ⋯ sheet? (Recommend yes; Back returns to
   the workspace, the project name is in the sheet.)
3. **Tab bar on a pushed session:** hide it (composer owns the bottom) or keep
   it? (Recommend hide; it returns on Back.)
4. **Composer settings:** all behind one settings icon → sheet (A and B both do
   this now), or any inline picker? (The principle says settings are secondary;
   recommend no inline pickers.)
5. **Checklist:** a 44 px sticky strip (A) or a folded card header (B)? And may
   its history move to a sheet? (Recommend strip + sheet.)
6. **Subagent:** a sheet (keeps the conversation) or a full page (today's
   `SubagentFocus`)? (Recommend sheet, with "open on its own" for the full page.)
7. **Per-message copy:** a visible right-aligned 44 px icon on every message
   group (recommended), or long-press selection only? (The icon is an icon, not
   a text button; long-press still selects.)
8. **Copy dependency:** build on the sibling `tp/add-copy-button-to-session-messa-*`
   branch (`messageCopyText`), or reimplement? (Recommend build on it; merge it
   first, then this layout.)
9. **Performance:** is `content-visibility` + pagination enough to ship, with
   virtualization only if measured? (Recommend yes.)
10. **Scope:** is this a Projects-only phone layout inside `MobileShell`
    (decision 8 / rollout 3), not a desktop change? (Recommend yes.)

## 9. Verification of this proposal

- The baseline and options were rendered in headless Chrome against the real
  frontend (mock mode) and the static prototype; sizes and target counts were
  measured with `getBoundingClientRect()` (the numbers above), light and dark,
  390×844 and 360×800.
- The prototype scripts pass `node --check` and render with no page errors
  (checked in Chromium).
- The implementation (this PR) passes `tsc --noEmit` in `frontend` and in the
  projects package, `oxlint`, `check-style-literals`, `check-api-arrays`, the
  projects vitest suite (952 tests, including the new ClampedText/phone tests),
  and the shell tests (9, run with the symlinked `node_modules` path allowed in
  a temporary config); details and screenshots in §10.

## 10. Implementation notes

The approved layout is implemented behind `usePhoneShell`; the desktop
(`ThreadPanel`, `Transcript`, `Composer`) keeps its existing rendering,
including the wide info rail and the project/folder header. In the phone shell:

- **Surface**: `ProjectPage`/`WorkdirPage` hide the page header and IM banner for
  an open session; `MobileShell` hides the tab bar (and activity switcher) while
  the URL has `thread=`/`session=`. `ThreadPanel` renders the compact bar (back,
  title, status, ⋯), the settings icon in the composer, the checklist strip and
  its sheet, and docks the live `PendingRequestCard` above the composer (the
  transcript omits its copy).
- **Transcript**: `Transcript` (`usePhoneShell`) renders a run as one neutral
  `过程 · duration` row that opens a `Dialog` (the phone theme's bottom sheet)
  with the per-call rows; a subagent card opens its body in a sheet; an error is
  one line → sheet; settled requests/commands stay one line. `shownBlocks` does
  not hide a settled request on the phone (the run row does not report it).
  Messages get a 44 px meta footer: exact time (short form) + right-side copy
  icon; copy reuses `messageCopyText` from #2811.
- **Long text**: `transcript/ClampedText.tsx` (new) is used by the user bubble
  (10 lines) and assistant replies (12); the 44 px 展开/收起 row is the block's
  last row, with the bottom mask, per-component open state, `aria-expanded`/
  `aria-controls`, and the full text in the DOM. The code block
  (`components/Markdown.tsx`) is height-capped at 210 px on the phone with its
  own 展开代码/收起代码 and a right-side copy icon; its horizontal scroll and
  full-code copy are unchanged. The same wrapper is ready for error/command/
  caption text (the error sheet uses it).
- **Touch/i18n/a11y**: `DisclosureRow` rows get 44 px on coarse pointers (the
  theme already raises MUI buttons/icon buttons/inputs); all new strings are in
  en/zh/ru (transcript locales + `phone.*` in the project locale).

Deliberate deviations from the design text:

1. **A reply containing a fenced code block is not outer-clamped**; its code
   block is capped and expanded on its own. An outer line clamp would hide the
   code block's own controls and could clip its horizontal scroll. Prose-only
   replies (the common long-text case) are clamped normally.
2. **`content-visibility` is used on the folded process/error/command rows, not
   on message blocks**: a skipped layout gives a clamp nothing to measure, so
   the message blocks opt out. Paging is the existing `TAIL = 200` transcript
   page; no virtualizer was added (report first, as agreed).
3. **The ⋯ menu is a MUI `Menu`** (thread details, copy ref, task, subagents,
   restart/resolve/archive, model/tokens and a "desktop-only" note), not a custom
   sheet; the thread details themselves open a `Dialog`. MUI menus/items already
   meet the phone theme's 44 px.
4. **Keyboard** is a real `visualViewport`/`100dvh` layout in the app; the
   headless screenshot is a 390×523 visible viewport (Chrome for Testing has no
   OS keyboard), so it demonstrates the composer/safe-area geometry, not the
   keyboard pixels.

Canvas: `.design/mobile-session/` (8 boards; previews `.design/images/mobile-session/`).
Screenshots: `mobile-impl-*` in `library/outputs/tb-sign/` — 390×844 and
360×800, light/dark: `pool` (docked approval), `checklist` (folder session),
`long` (clamped user/assistant), `code` (capped code + right copy),
`long-expanded` and `code-expanded` (in-place 收起), `menu`; plus 390 light/dark
sheets (`process`, `checklist-sheet`, `settings`, `subagent`, `rail`) and a
`keyboard` visible-viewport shot. The mock's `t-long` thread is a dev fixture for
the long-text states.
