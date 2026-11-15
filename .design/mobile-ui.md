# Mobile UI: a phone-native surface for Agent and the Projects workspace

> Status: **in progress** (TEP-2558, issue #2558). Rolled out as stacked PRs, all behind an explicit opt-in (`?ui=mobile`, or 界面 in Settings); nothing changes by default. Implemented so far: tokens, `useIsPhone`, the shell preference, the phone shell with the 更多 page, the interface picker, and the sub-page switcher. Not yet: the workspace screens, the Agent pages, the viewport script (see Rollout).
> Prototype: `.design/mobile-prototype/index.html` (static, clickable, mock data; `?theme=dark`, `?frame=0`, deep-link states via `?s=tab,screen,thread,sheet,ptab`).
> Audit and mockups: `pr-assets/tep-mobile/` (`raw.githubusercontent.com/tingly-dev/tingly-box/pr-assets/tep-mobile/...`).

## The problem

The workspace is what people use on a phone most; the **Agent** pages come second. The two things people want from a phone are the **Agent** pages (is a client connected, copy a URL or key, glance at rules) and the **Projects workspace** (what needs me, answer an approval or a question, review a finished thread, send one instruction). Both are desktop UIs: a nav rail, nested cards, a board, a side pane, an info rail, a routing graph, three rows of composer pickers.

Shrinking them is not enough. The audit at 390×844 and 360×800 found no horizontal overflow, only desktop interaction on a phone: four stacked bars take ~28 % of a thread screen and the composer ~23 %; the board is behind a 30 px icon; the rule graph is 852 px wide inside a 268 px window; 16 of 18 visible controls on the workspace page and 35 of 36 on an Agent page are under 36 px; `.row-menu` / `.pin-star` exist only on hover. Visible area and interaction both change, so the phone gets its own style.

## What a phone is for (and not for)

| Phone: triage, respond, review, glance | Desktop only |
|---|---|
| Is anything blocked on me? Answer it | Compose a long brief, paste stack traces |
| Send one instruction to the coordinator or a thread | Drag/reorder rules, edit the routing graph |
| Read a thread's report and a change summary; resolve | Full diffs, side-by-side panes, floating windows |
| See that a client is connected; copy the URL / key | Provider/credential bulk editing, tables |
| Occasional management: copy/enable a team key, see a provider's status, re-authorise; change a rule's target model, enable/disable | Board orientations, layout/placement choices |

## Decisions

1. **A phone shell, not a squeezed desktop shell.** Below `sm` on a coarse pointer the app mounts a mobile shell with its own pages. Same URL space (so IM and push deep links open on either), same API client, stores, i18n and theme; different layout components. Desktop (≥ `sm`, or fine pointer) is untouched. A 界面 choice (桌面版 / 手机版) in Settings and the preferences menu, plus a one-tap "使用桌面版界面" in 更多, overrides it (`?ui=mobile|desktop` for developers).
2. **Shared data and domain layer, separate presentation.** The mobile pages reuse the Projects data hooks and transcript model (`useProjectList`, `useProjectEvents`, `useTranscript`, `threadRequests`, `transcript/model`) and the generated API client. They do not reuse `ProjectPage`, `WorkspaceLayout`, `ThreadPanel`, `StatusBoard` or the rules graph. Anything the data hooks don't export yet is exported from `@tingly/projects` (`embed.ts`) first.
3. **Fallback tier.** A route with no mobile page renders its desktop page inside the mobile frame (compact top bar, bottom tabs), i.e. today's responsive behaviour. Mobile pages replace this fallback one route at a time, so the shell can ship before every page exists.
4. **Workspace-first home.** The workspace is the main phone use, so the home tab is the workspace: **需要你回答** (approvals, questions, with inline buttons) first, then folded **可以 review** and **进行中** groups (the shared Disclosure pattern), then 项目 and 工作目录. It is cross-project, built from the project list and each project's threads/events (an aggregate endpoint is an optimisation, not a prerequisite). The tab badge counts what needs an answer. There is no separate inbox tab.
5. **Three tabs: 工作区 · 智能应用 · 更多.** **更多** is the management entry for everything that is not the workspace or Agent: 团队, 凭证, Image, 仪表盘, 扩展, IM 机器人, then preferences (主题与语言, 通知, 帮助) and the "使用桌面版界面" switch. Team and Credentials get small phone pages (keys: copy / enable / create; providers: status / re-authorise / enable / test); the rest use the fallback tier at first. Management is occasional, so it is two taps from anywhere (更多 → page), never hidden.
6. **Docked actions in the thumb zone.** An approval or question docks above the composer with 48 px buttons (allow / deny; recommended answer / refuse). The less common choices ("本线程都允许 Bash…", "其他选项…") open a sheet. A finished thread docks "标记为已解决 / 继续追问". Nothing needed to answer scrolls out of sight.
7. **One-line composer; settings behind a sheet.** Attach, field, send/stop. Profile, model, effort and permission live in the thread menu → settings sheet. The composer must follow the on-screen keyboard (`visualViewport`, `100dvh`), and the tab bar hides while it is open.
8. **Concise by default.** The transcript defaults to the concise view on phones (tool runs folded to one line); details are one tap away. Thread info (branch, changes, context, tokens) is a sheet, not a rail. Changes are a read-only file summary; full diffs stay on the desktop.
9. **Agent: status first.** A stat strip (requests, error rate, latency) and a log link, then connection values as tap-to-copy rows (the full value, wrapped), then the rules as a list; a rule opens a sheet with its IF-steps stacked vertically and "更换目标模型". Graph editing and reordering stay on desktop (the page says so).
11. **Sub-pages: a switcher bar, not a sidebar.** The desktop secondary sidebar (Agent's Claude Code, profiles, Codex, OpenCode…; Team's teams; Image's playground, profiles, library; Dashboard, Power-ups, System pages) has no room on a phone, and without it only the default page of each activity was reachable. Every activity with two or more pages gets a 44 px bar above the page naming the page you are on (icon, name, the activity as a caption, a chevron); tapping it opens a bottom sheet listing every page the desktop sidebar lists, in the same groups, with subtitles (a profile's id) and a check on the current one. Choosing a page navigates and closes the sheet. It is built from the same `ActivityItem.children` as the desktop sidebar, so a new desktop route appears on the phone by itself (`SubPageSwitcher.test.tsx` walks every activity and asserts each page is in the sheet). Hidden agents stay hidden, as on desktop outside edit mode; "add" actions (new Claude Code profile, new Team) are not pages and stay desktop-only for now. The Workspace has its own screens and no switcher.
10. **One surface level.** Grouped lists (a rounded container, hairline-separated rows) replace nested cards on phones, so content keeps ~358 of 390 px instead of ~268. This departs from the desktop "keep card nesting" preference and is called out as an open question in the TEP.

## Sizes

Phones range from 320×568 (small, short) to 430×932 and beyond, with foldables and tablets above 600 px. The shell is fluid, not tuned to one size; there are no pixel-perfect device frames. The prototype (`?w=&h=`) and the viewport script check the matrix 320×568, 360×640, 375×667, 390×844, 412×915, 430×932, 844×390 (landscape) and 768×1024.

| Class | Rule | What changes |
|---|---|---|
| compact | width < 360 | gutters 12, 26 px large title, tighter buttons; nothing else |
| regular | 360–427 | the reference layout |
| large | 428–599 | gutters 20, 32 px large title; content still one column (a wider column, never a second one) |
| short | height < 700 | large titles shrink and drop the subtitle; dock, composer and tab bar tighten; the transcript keeps ≥ 40 % |
| wide | width ≥ 600 (foldable, tablet portrait, landscape phone) | tab bar becomes a left rail; **master-detail**: list left (340 px), thread or project right; no Back button; sheets become centred dialogs |
| flat | height < 500 | approval dock goes inline (question next to the buttons), composer and bars compact |

Rules that hold at every size: nothing overflows horizontally (long values wrap; code blocks scroll inside their own box), the approval buttons and composer always stay on screen, targets stay ≥ 44 px, and text never goes below 12 px. Sizes are decided with CSS media/container queries (and `100dvh`), not user-agent sniffing; the shell switch only decides *which* shell, the size class decides how it lays out.

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

0. Foundations: mobile tokens, `useIsPhone`, the shell switch (an explicit `?ui=mobile`; `auto` stays desktop until `MOBILE_AUTO` is turned on in `src/shell/uiShell.ts`), the viewport screenshot script. No change by default. *Tokens (`@tingly/ui/theme/mobile`), `useIsPhone` and the switch (`src/shell/uiShell.ts`) are in; the script is next.*
1. Shell: bottom tabs (工作区 · 智能应用 · 更多), the 更多 entry with the UI switch, the fallback tier. *In (`src/shell/`): `Layout` mounts `MobileShell` when `?ui=mobile`; tabs come from the same activity list as the desktop rail; `/more` lists the other activities, System, preferences, Help and the desktop switch. The interface can also be chosen from the desktop side: a 界面 row (桌面版 / 手机版) in System › Preferences and in the rail's preferences menu, stored per browser (`ui.shell`, not synced: a phone and a laptop want different answers). `自动` is offered only once `MOBILE_AUTO` turns on. The tab dot is boolean for now; the count arrives with the workspace home. Activities with several pages (Agent, Team, Image, Dashboard, Power-ups, System) show the sub-page switcher bar above the page (decision 11), so every desktop sub-route is reachable.*
2. Workspace home: needs-you with inline approve/answer, folded groups, projects and folders, new-task sheet.
3. Workspace screens: project (对话 / 线程 / 文件库), folder session, thread page with docked actions, sheets, keyboard handling, master-detail at ≥ 600.
4. Review: report + change summary, resolve, follow-up.
5. Agent and 更多: Agent list, status, copy rows, rule sheet; Team and Credentials phone pages.
6. Sweep and install: remaining routes, web app manifest and icons. Push notifications are a separate decision.

## Testing

- A viewport script (Playwright against `pnpm dev:mock`; Chrome setup as in the `ui-preview` skill) walks the phone routes over the size matrix above, light and dark, and asserts: no horizontal page overflow, interactive elements ≥ 44 px, thread transcript ≥ 60 % of the viewport with no dock open; it saves the PNGs for the PR. It also runs a keyboard-open state (emulated `visualViewport`).
- vitest component tests with a `matchMedia` stub (as `Composer.test.tsx` does) for the shell switch and each mobile page's behaviours (inline approve, sheet open/close on Back).
- Desktop regression: 1440×900 screenshots before and after each phase; the desktop bundle must not grow (mobile pages are lazy chunks).

## Open questions

The list in TEP-2558 is authoritative; at the time of writing: tablet portrait (proposed: master-detail at ≥ 600), how a phone reaches the box (LAN / tunnel / IM only, HTTPS for install), Web Push, rule editing depth on phones, and whether the grouped-list surface should replace nested cards on phones.
