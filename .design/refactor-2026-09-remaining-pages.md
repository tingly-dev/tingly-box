# Frontend Refactoring Candidates — Remaining Pages & Component Subdirs

Date: 2026-09-23 · Scope: pages not covered by the earlier refactor pass (Dashboard, UserUsage, Skill, bench, bots, mcp, system, remote-agent, notify, servertool, plus components/{bot,dashboard,probe,notify,model-select,credential,provider-form-dialog,cloud,paste-detect,prompt}), App.tsx, vite configs. Every claim below was grep-verified across `frontend/src`.

---

## 1. DEAD CODE

### 1.1 `pages/mcp/MCPClientEditor.tsx` — fully orphaned (HIGH confidence, low risk)
- 487 lines; `grep -rn MCPClientEditor` across the whole frontend repo returns only the file itself. Not routed, not imported, no dynamic `import()` reference.
- Its only dependency, `pages/mcp/localTypes.ts`, is imported **nowhere else** — deleting the editor orphans it too.
- `localTypes.ts` also **duplicates** `MCPSourceConfig` / connection / auth types from `pages/mcp/types.ts` (the live copy, imported by `pages/servertool/ServerToolPage.tsx:27`). Deleting both files removes a type-drift hazard (the two `MCPSourceConfig` definitions already differ: `types.ts` allows `transport: 'advisor'`).
- **Action:** delete `pages/mcp/MCPClientEditor.tsx` + `pages/mcp/localTypes.ts`. Risk: LOW.

### 1.2 `pages/ModelTestPage.tsx` — routed orphan with broken back-links (medium risk, decide intent)
- Routed at `/model-test/:providerUuid` (App.tsx:283), but **nothing in `src/` links to `/model-test`** — no `navigate`, `<Link>`, or href anywhere outside App.tsx and the page itself. Reachable only by typing the URL.
- Its two back buttons navigate to `/api-keys` (ModelTestPage.tsx:225, 248) — that route does not exist; the catch-all silently bounces to `/agent`.
- **Action:** either wire it into a provider row/menu action (it's a decent surface, backed by the well-maintained `components/probe/` stack) or retire the page + route. Risk: MEDIUM (product decision).

### 1.3 Unused barrel exports (low risk, cosmetic)
- `components/bot/index.ts`: `RemoteControlGraph` and `BotAuthForm` have zero external consumers (each is used only via relative import inside `components/bot/`). `BotPlatformSelector` IS used externally? No — only `BotConfigDialog.tsx:2` (relative). External barrel consumers are `BotTable`, `BotConfigDialog`, `BotAccessDialog`, `PlatformPicker`, `useBotModelDialog` only.
- `components/dashboard/index.ts`: `TokenHeatmap`, `computeShare`, `UsageMetricHeaderCells`, `filterAndSort`, `TOKEN_COLORS` have zero external consumers (TokenHeatmap is used internally by `DashboardHeatmapSection.tsx`).
- **Action:** trim barrel exports or leave as documented internal API; do not expand them. Risk: LOW.

---

## 2. DUPLICATION

### 2.1 The bot-page trio hand-rolls the same loader/actions/snackbar (best refactor candidate)
Three near-line-identical copies of: snackbar state + `showNotification` + `loadBotSettings` (`getImBotSettingsList` → `enrichBotsWithCapabilities` → identical error toasts) + `handleBotRestart` + `handleDeleteBot`:

| Concern | pages/bots/PlatformBotPage.tsx | pages/bots/BotOverviewPage.tsx | pages/remote-agent/PlatformRemoteAgentPage.tsx |
|---|---|---|---|
| snackbar state | 43–52 | 42–49 | 59–67 |
| loadBots | 63–78 | 52–67 | 76–90 |
| restart | 111–127 | 145–158 | 136–152 |
| delete | 129–141 | 162–170 | 154–166 |
| `<Snackbar>` render | 210–223 | 253 | 324–337 |

- `hooks/useBotToggle.ts` already exists for the toggle op ("Toggle uses the shared useBotToggle hook… restart/delete keep the page's own Snackbar" — PlatformBotPage.tsx:107–109). The comment acknowledges the gap.
- **Action:** extract `hooks/useBotList.ts` (load + restart + delete + notify) reusing `useNotify`; delete all four local `<Snackbar>` blocks. ~120 lines removed and one toast UX (global provider vs bottom-right MUI snackbar) unified. Risk: LOW–MEDIUM (toast placement changes slightly — verify against UX principles; global toasts are the sanctioned pattern per `frontend/CLAUDE.md`).

### 2.2 Local `<Snackbar>` instead of `useNotify()` — convention violation, 4 pages
`pages/bots/PlatformBotPage.tsx:210`, `pages/bots/BotOverviewPage.tsx:253`, `pages/remote-agent/PlatformRemoteAgentPage.tsx:324`, `pages/VirtualModelsPage.tsx:87` (VirtualModels also hand-wires state at 15–22). See 2.1 for the fix; VirtualModelsPage is a standalone swap to `useNotify`. Risk: LOW.

### 2.3 `toLocalISOString` copy-pasted 4x
- `pages/DashboardPage.tsx:68–79`, `pages/UserUsagePage.tsx:92–99`, `pages/QuotaHistoryPage.tsx:35–43` (identical), `components/dashboard/DashboardHeatmapSection.tsx:17` (same logic, different name shape).
- Related: the three pages each also re-implement range-window building (`TIME_RANGE_CONFIG`/`buildTimeParams` in Dashboard 57–195, `RANGE_DAYS`/`buildTimeParams` in UserUsage 85–111, `RANGE_MINUTES`/`buildTimeRange` in QuotaHistory 26–50).
- **Action:** extract `utils/datetime.ts` (`toLocalISOString` at minimum; optionally a `localRangeWindow(range)` helper). Risk: LOW.

### 2.4 Dashboard stat-total math duplicates `computeUsageSummary`
`DashboardPage.tsx:387–405` hand-rolls the same reduce-aggregates (requests/input/output/cache/errors/rates) that `components/dashboard/RosterAxis.tsx#computeUsageSummary` already provides and UserUsagePage consumes (UserUsagePage.tsx:482–496). **Action:** have DashboardPage call `computeUsageSummary(stats)`. Risk: LOW.

### 2.5 Bench vs ProbeDialog debounced-curl
`useDebouncedCurl` (`pages/bench/BenchPage.tsx:34–56`) duplicates ProbeDialog's debounced `buildProbeCurl` regeneration (`components/probe/ProbeDialog.tsx:209–240`). Otherwise bench reuses `runProbe`/`ResultSections`/`probeConfig` properly — this is the only seams-left copy. **Action:** lift a `useDebouncedCurl(request)` into `components/probe/`. Risk: LOW.

### 2.6 Verified NOT duplicated (no action)
- **pages/bots platform pages:** all 9 (Telegram…Slack) are 3-line files calling `createPlatformBotPage` — the factory pattern is applied uniformly.
- **components/bot vs components/notify:** `BotNotifyGroup.tsx` (630 lines) operates on chat *targets* (notify/confirm probes, pairing) via `useChatProbe`; it shares only the intended pieces (`components/nodes`, `services/botApi`, `types/bot`). `RemoteAgentBotCard` + `BotCard` share styling via `botCardStyles.ts` as designed. No merge needed.
- **MCP editors:** `MCPSourceEditor.tsx` (controlled form) is the only live editor once 1.1 is deleted; `MCPLocalMode`/`MCPRegisteredServers`/`AgentInstallCard` consume it without re-implementing scaffolding.
- **BotPlatformSelector vs PlatformPicker:** different UX (dialog dropdown vs nav tile grid), both used.
- **useChatProbe vs useModelTestProbe:** distinct purposes (chat-capability runner vs dialog-state lift).

---

## 3. OVERSIZED FILES

### 3.1 `pages/UserUsagePage.tsx` (1115 lines)
Already well-factored around `useRosterAxis` (the three-axis account/model/provider design is good). Remaining split boundaries, in dependency-safe order:
1. **Module helpers + loaders (61–199)** → `pages/userUsage/userUsageModel.ts` (types, `RANGE_DAYS`, `toLocalISOString`→shared util, key/name/searchText fns, the three `fetch*For*` loaders — they're deliberately module-level for hook stability, so they move cleanly).
2. **`RosterDetailHeader` (223–264)** → own file next to the page.
3. **Data loading (269–347)** → `useUserUsageData(range)` hook returning `{tokens, userStats, modelRoster, providerRoster, rows, loading, refreshing, error, reload}`.
4. **Roster table (706–984)** → `RosterTable` taking the active axis + columns; the three `pagedRows.map` branches (820–925) share ~80% shape and can take an identity-cell render prop.
5. **Detail section (991–1112)** → `RosterDetailSection` (breakdown card + top list + empty state).
Risk: LOW; page is behavior-stable, pure extraction.

### 3.2 `pages/prompt/SkillPage.tsx` (1014 lines) — worst internal quality of the three
1. **Three-column layout is fully inline (391–981):** LocationsColumn (393–507), SkillsColumn (510–774), DetailColumn (777–979) → three components; the Paper column shell sx is repeated 3x (394–402, 511–519, 778–786).
2. **Copy-pasted skill `<ListItem>`** inside SkillsColumn: grouped branch (650–708) vs flat branch (717–769) render the identical row; only path trimming differs. Extract `SkillListItem`.
3. **Grouped renderer is an inline IIFE** (615–714) — move `groupSkillsIntelligently` call to a `useMemo` and render normally.
4. **Zero i18n:** no `useTranslation`, every string hardcoded English ("Skill Management", "Add Location", …) — inconsistent with every other in-scope page (AccessControl, System, NotifyPage, SharingKeys, VirtualModels all translate).
5. **Raw clipboard + `utils/notify`** (117–119, 266–276): `navigator.clipboard.writeText` + toast instead of `useCopyFeedback`/`CopyIconButton`; `notify.show` wrapper instead of `useNotify`.
Risk: LOW–MEDIUM (i18n keys need to be added to all locale files; extraction is mechanical).

### 3.3 `pages/DashboardPage.tsx` (804 lines)
Mostly healthy — charts/tables/heatmap already live in `components/dashboard/`. Remaining:
1. **Data layer (112–503)** → `useDashboardData(timeRange)` hook: params building, seq-guarded `loadData`/`loadRecords`, filter-option loading, auto-refresh, option-snapshot effects. ~390 lines out.
2. **`headerActions` filter bar (504–658)** → `DashboardFilterBar` component (three `<FormControl><Select>` blocks share shape; identity dropdown with grouped ListSubheader is the only special one).
3. Stat-card row (677–732) is declarative and fine where it is once totals come from `computeUsageSummary` (see 2.4).
Risk: LOW.

---

## 4. CONVENTION VIOLATIONS (checked across `frontend/src`)

| Convention | Status | Evidence |
|---|---|---|
| No direct `@mui/icons-material` imports | **PASS** | Only type-only imports in `components/icons/tablerMui.tsx:4` and `index.tsx:166` (sanctioned); zero value imports elsewhere. |
| `hooks/useCopyFeedback` not hand-rolled | **MOSTLY PASS** | One violation: `pages/prompt/SkillPage.tsx:266–276` raw `navigator.clipboard` + toast. AccessControl, CopyIconButton, CodeBlock, SkillDetailDialog, ProbeDialog all use the hook. |
| `useNotify`, no local Snackbar | **FAIL (4 pages)** | PlatformBotPage:210, BotOverviewPage:253, PlatformRemoteAgentPage:324, VirtualModelsPage:87 (see 2.1/2.2). SkillPage/NotifyPage/BotNotifyGroup import `utils/notify` directly (SkillPage:47, NotifyPage:13) — acceptable store use, but components should prefer `useNotify`. |
| Lazy page imports (Login only static) | **PASS** | App.tsx:28 is the only static page import; eager imports from `pages/` are page-free modules only (`scenario/scenarioRegistry`, `scenario/context/ScenarioPageContext`) — complies with the code-splitting rule. |
| vite manualChunks/optimizeDeps sync | **PASS** | `vite.config.ts:56–111` and `vite.config.wails.ts:30–74` match (MUI-only vendor chunk, no recharts/d3 rule, same `optimizeDeps`), with cross-referencing comments. No drift. |
| i18n via `t()` | **PARTIAL** | SkillPage, UserPage, ServerToolPage, CredentialPage, ModelTestPage have zero `useTranslation` — all hardcoded English. May be deliberate for experimental pages; flag for a decision. |

---

## 5. APP.TSX

Clean overall: lazy discipline is perfect, redirects are commented, catch-all exists. Improvable without behavior change:
1. **Inconsistent `.tsx` suffix in lazy imports** — `./pages/SharingKeysPage.tsx` (:35), `./pages/system/System.tsx` (:55), `./pages/system/AccessControl.tsx` (:56) vs extensionless everywhere else. Normalize to extensionless.
2. **Inline routing widgets** — `AppDialogs` (:99–145), `OnboardingGate` (:154–184), `LegacyBotSectionRedirect` (:190–194), `LegacyRemoteCoderRedirect` (:198–201), `RouteFallback` (:92–96) could move to `components/routing/` (or a small `routing/gates.tsx`) to shrink App.tsx to the route table. Low priority — they're documented and small.
3. **Repeated `ExperimentalFeatureGate` wrapper** — 9 routes repeat the same element-wrap pattern (:235, 285–286, 318–328). A tiny `<GatedRoute feature="mcp" element={...} />` would declutter, but is cosmetic.
4. Route table itself: ordering, redirects, and back-compat paths are in good shape; nothing dead found in the table itself (every lazy const is referenced).

---

## Suggested order of attack

| # | Item | Effort | Risk | Payoff |
|---|---|---|---|---|
| 1 | Delete `MCPClientEditor.tsx` + `localTypes.ts` (1.1) | 15 min | LOW | −600 lines, kills a type-drift duplicate |
| 2 | `utils/datetime.ts` for `toLocalISOString` (2.3) | 30 min | LOW | 4 copies → 1 |
| 3 | Local Snackbar → `useNotify` in VirtualModelsPage + bot trio (2.1/2.2, incl. `useBotList` hook) | 2–3 h | LOW–MED | −150 lines, unified toasts |
| 4 | DashboardPage → `computeUsageSummary` (2.4) | 30 min | LOW | removes duplicate aggregate math |
| 5 | ModelTestPage: wire in or retire (1.2) | decision + 1 h | MED | removes orphan + broken `/api-keys` links |
| 6 | SkillPage split + i18n (3.2) | 1 day | LOW–MED | −~400 lines, i18n parity |
| 7 | UserUsagePage / DashboardPage hook+component extraction (3.1/3.3) | 1 day | LOW | maintainability |
| 8 | Barrel trims (1.3), bench `useDebouncedCurl` lift (2.5), App.tsx cosmetics (5) | 1–2 h | LOW | polish |
