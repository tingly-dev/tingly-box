# Mobile UI: a phone-native surface for Agent and the Projects workspace

> Status: **proposal** (TEP-2558, not implemented). Nothing here exists in the product yet.
> Prototype: `.design/mobile-prototype/index.html` (static, clickable, mock data; `?theme=dark`, `?frame=0`, deep-link states via `?s=tab,screen,thread,sheet,ptab`).
> Audit and mockups: `pr-assets/tep-mobile/` (`raw.githubusercontent.com/tingly-dev/tingly-box/pr-assets/tep-mobile/...`).

## The problem

The two things people most want from a phone are the **Agent** pages (is a client connected, copy a URL or key, glance at rules) and the **Projects workspace** (what needs me, answer an approval or a question, review a finished thread, send one instruction). Both are desktop UIs: a nav rail, nested cards, a board, a side pane, an info rail, a routing graph, three rows of composer pickers.

Shrinking them is not enough. The audit at 390×844 and 360×800 found no horizontal overflow, only desktop interaction on a phone: four stacked bars take ~28 % of a thread screen and the composer ~23 %; the board is behind a 30 px icon; the rule graph is 852 px wide inside a 268 px window; 16 of 18 visible controls on the workspace page and 35 of 36 on an Agent page are under 36 px; `.row-menu` / `.pin-star` exist only on hover. Visible area and interaction both change, so the phone gets its own style.

## What a phone is for (and not for)

| Phone: triage, respond, review, glance | Desktop only |
|---|---|
| Is anything blocked on me? Answer it | Compose a long brief, paste stack traces |
| Send one instruction to the coordinator or a thread | Drag/reorder rules, edit the routing graph |
| Read a thread's report and a change summary; resolve | Full diffs, side-by-side panes, floating windows |
| See that a client is connected; copy the URL / key | Provider/credential bulk editing, tables |
| Simple edits: change a rule's target model, enable/disable | Board orientations, layout/placement choices |

## Decisions

1. **A phone shell, not a squeezed desktop shell.** Below `sm` on a coarse pointer the app mounts a mobile shell with its own pages. Same URL space (so IM and push deep links open on either), same API client, stores, i18n and theme; different layout components. Desktop (≥ `sm`, or fine pointer) is untouched. A "use desktop UI" switch in 我的 overrides it (`?ui=mobile|desktop` for developers).
2. **Shared data and domain layer, separate presentation.** The mobile pages reuse the Projects data hooks and transcript model (`useProjectList`, `useProjectEvents`, `useTranscript`, `threadRequests`, `transcript/model`) and the generated API client. They do not reuse `ProjectPage`, `WorkspaceLayout`, `ThreadPanel`, `StatusBoard` or the rules graph. Anything the data hooks don't export yet is exported from `@tingly/projects` (`embed.ts`) first.
3. **Fallback tier.** A route with no mobile page renders its desktop page inside the mobile frame (compact top bar, bottom tabs), i.e. today's responsive behaviour. Mobile pages replace this fallback one route at a time, so the shell can ship before every page exists.
4. **Inbox-first.** The home tab is a cross-project **收件箱**: things that need an answer (approvals, questions) with inline actions, things ready to review, things running. The tab badge counts what needs an answer. It is built from the project list and each project's threads/events; an aggregate endpoint is an optimisation, not a prerequisite.
5. **Four tabs:** 收件箱 · 工作区 · 智能应用 · 我的 (Team, Credentials, Dashboard, Image, Extensions, preferences, notifications, the UI switch).
6. **Docked actions in the thumb zone.** An approval or question docks above the composer with 48 px buttons (allow / deny; recommended answer / refuse). The less common choices ("本线程都允许 Bash…", "其他选项…") open a sheet. A finished thread docks "标记为已解决 / 继续追问". Nothing needed to answer scrolls out of sight.
7. **One-line composer; settings behind a sheet.** Attach, field, send/stop. Profile, model, effort and permission live in the thread menu → settings sheet. The composer must follow the on-screen keyboard (`visualViewport`, `100dvh`), and the tab bar hides while it is open.
8. **Concise by default.** The transcript defaults to the concise view on phones (tool runs folded to one line); details are one tap away. Thread info (branch, changes, context, tokens) is a sheet, not a rail. Changes are a read-only file summary; full diffs stay on the desktop.
9. **Agent: status first.** A stat strip (requests, error rate, latency) and a log link, then connection values as tap-to-copy rows (the full value, wrapped), then the rules as a list; a rule opens a sheet with its IF-steps stacked vertically and "更换目标模型". Graph editing and reordering stay on desktop (the page says so).
10. **One surface level.** Grouped lists (a rounded container, hairline-separated rows) replace nested cards on phones, so content keeps ~358 of 390 px instead of ~268. This departs from the desktop "keep card nesting" preference and is called out as an open question in the TEP.

## Style

- Type: 16 px body (also avoids iOS input zoom), 14 secondary, 12 caption minimum, 30 px large titles on tab roots, 17 px compact titles on pushed screens.
- Targets: ≥ 44 px everywhere, 48 px for primary and for docked buttons; rows ≥ 60 px with two lines.
- Status never relies on colour alone: glyph + word + colour (waiting, ready, running).
- Gutters 16 px; sheets with a 24 px top radius; segmented control (not underlined tabs) for 对话 / 线程 / 文件库.
- Colours, status colours and radii come from the existing theme and `@tingly/ui` tokens; the mobile kit adds tokens (`tapMin`, `tapPrimary`, `bottomNavHeight`, `sheetRadius`, safe-area helpers) there, so the strict style-literal guard still holds.
- No hover: anything hover-only becomes visible (the `LazyTree` `@media (hover: none)` pattern); tooltips are off on touch.
- Real routes: the browser/OS Back pops a screen; sheets push a history entry so Back closes them.
- Safe areas (`viewport-fit=cover`, `env(safe-area-inset-*)`), `theme-color`, `overscroll-behavior`, reduced-motion respected.

## Mobile kit (`@tingly/ui`)

`BottomTabs`, `LargeTitleHeader`, `CompactHeader`, `GroupedList` / `ListRow`, `DockedActions`, `BottomSheet`, `SegmentedControl`, `Toast`, plus `useIsPhone`. Built on MUI primitives, tested with a `matchMedia` stub.

## Rollout

0. Foundations: mobile tokens, `useIsPhone`, the shell switch (behind `?ui=mobile` and an experimental flag), the viewport screenshot script. No change by default.
1. Shell: bottom tabs, 我的 (with the UI switch), the fallback tier.
2. Inbox + approvals/questions in place.
3. Workspace: project list, project (对话 / 线程 / 文件库), thread page with docked actions, sheets, keyboard handling.
4. Review: report + change summary, resolve, follow-up.
5. Agent: list, status, copy rows, rule sheet.
6. Sweep and install: remaining routes, web app manifest and icons. Push notifications are a separate decision.

## Testing

- A viewport script (Playwright against `pnpm dev:mock`; Chrome setup as in the `ui-preview` skill) walks the phone routes at 390×844 and 360×800, light and dark, and asserts: no horizontal page overflow, interactive elements ≥ 44 px, thread transcript ≥ 60 % of the viewport with no dock open; it saves the PNGs for the PR. It also runs a keyboard-open state (emulated `visualViewport`).
- vitest component tests with a `matchMedia` stub (as `Composer.test.tsx` does) for the shell switch and each mobile page's behaviours (inline approve, sheet open/close on Back).
- Desktop regression: 1440×900 screenshots before and after each phase; the desktop bundle must not grow (mobile pages are lazy chunks).

## Open questions

The list in TEP-2558 is authoritative; at the time of writing: bottom-tab slots, tablet portrait, how a phone reaches the box (LAN / tunnel / IM only, HTTPS for install), Web Push, rule editing depth on phones, and whether the grouped-list surface should replace nested cards on phones.
