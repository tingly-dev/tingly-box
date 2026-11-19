# Frontend refactor — inventory & phased plan (2026-11)

Scope: `frontend/src` and `frontend/packages/{ui,projects,vision,mannequin}`. Goal: clean dead code, merge duplicate implementations, move misplaced code — **behaviour and UI unchanged**. Each phase is one small, independently reviewable PR; low risk first.

This is the second pass. The first one (`.design/refactor-frontend-2026-09.md`, P0–P6, completed 2026-09-23) removed ~3,000 lines of dead code and split the oversized scenario / guardrails pages. Its "Deferred" list is still open and is folded into the phases below. Everything here was re-measured against `main` at `f5235a0f5`, not copied from the old survey.

## How the inventory was produced (reproducible)

- **Unused imports / locals**: `tsc --noEmit --noUnusedLocals` (the repo does not enable it; 90 hits at the start).
- **Unreachable files**: a TypeScript-API import graph over `tsconfig.json`'s file set (static `import` / `export … from`, dynamic `import()`, `import()` types; `@/` and `@tingly/*` resolved), iterated to a fixpoint, tests not counted as importers. Entry roots: `src/main.tsx`, `*.d.ts`, and the `@tingly/*` package entry files.
- **Unused exports**: per-symbol import usage from the same graph, then a word-grep across `src`, `packages`, `scripts` to confirm zero external references. Most hits are types or "exported but only used inside the file"; only exports with *no use at all* (not even in the defining file) are listed as dead.
- **Duplicates**: top-level function/arrow bodies hashed after whitespace/comment/name normalisation (identical bodies), plus same-name functions in different files, then read by hand.
- **Placement**: for each file under `components|hooks|utils|services|contexts|constants|types|styles`, the set of importing "areas"; and every import that points *into* `pages/` from non-page code.

Not used: knip / ts-prune (not installed, installing was out of scope). Phase 2a below proposes adding `knip` as a dev tool so this stays enforced.

## Baseline (before any change)

`pnpm gen:api` → `pnpm typecheck` clean; `pnpm lint` = oxlint warnings (all in `ImageGenPlaygroundCard.tsx`, pre-existing) + `style literals: ok`; `pnpm test` = 1577 tests pass, **16 test *files* fail to load** with `Denied ID …/@lobehub/icons-static-svg/…svg?url` (vite `fs.allow` when `node_modules` is symlinked from the main checkout — environmental, see project memory `ui-preview-env`). Two timing-sensitive `MorePage` tests failed in one of three full runs and passed in isolation and in the other two (flaky under load).

## 1. Dead code

### 1.1 Done in phase 1 (zero references anywhere, typecheck/build unchanged)

| Item | Evidence |
|---|---|
| `pages/notify/NotifyPage.tsx`, `pages/remote-agent/RemoteAgentPage.tsx`, `pages/servertool/ServerToolPage.tsx` (4-line `Navigate`) | No import; `routes/appRoutes.tsx:203,212-215,232` redirect `/notify`, `/remote-agent*`, `/tools/servertool`. `git log` shows they were still being edited by unrelated refactors (#2382, #2420, #2445) just to keep compiling — that churn is the cost of leaving them. `.design/notify-api-guide.md` already said "could be deleted". |
| `components/bot/BotChatsButton.tsx`, `components/bot/useStableBotOrder.ts` | Only importers were the deleted pages. |
| `components/ToolCard.tsx`, `ToolFilterBar.tsx`, `ClientConfigStatusChip.tsx` | No importer. |
| `pages/prompt/CommandPage.tsx` + `pages/prompt/index.ts` | Not routed (`appRoutes.tsx:64` only lazy-loads `SkillPage`); barrel unused. |
| `components/prompt/skill/{SkillDetailDialog,SkillFilterBar,SkillListDialog,SkillLocationCard}.tsx`, `components/prompt/index.ts`, `components/prompt/skill/index.ts` | `SkillPage` imports the other skill dialogs directly and renders its own columns; these four were left over from before the 3-column page. |
| `components/rule-card/index.ts`, `components/tier/index.ts` | Barrels nobody imports (callers deep-import). |
| `pages/scenario/components/TemplatePage.constants.ts` | Zero references. |
| `theme/base.ts`, `theme/types.ts` (re-export shims) | 0 importers (other `src/theme/*` shims are used, see 3.4). |
| `utils/timeAgo.ts` | Dead; `packages/projects/src/utils/format.ts` has the live `timeAgo`. |
| `packages/projects/src/components/im/ImChatCard.tsx` | No importer. |
| 48 unused import declarations in 38 files | `tsc --noUnusedLocals` TS6133/6192/6196/6198 on import specifiers only (e.g. `App.tsx` `IconButton/Paper/Stack/useVersion`, `useActivityItems.tsx` three calendar icons, `rule-card/dialogs.tsx` `Alert/Box/Typography/ConfigRecord`). |
| `styles/toggleStyles.tsx`: `toggleButtonStyle`, `switchBaseStyle`, `toggleButtonCompactStyle`, `switchSmallStyle`, `switchSuccessStyle`, `switchWarningStyle` | Only `toggleButtonGroupStyle` and `switchControlLabelStyle` are imported (`DecisionTryCard`, `AxisPrimitives`, `DashboardFilterBar`). |
| `types/bot.ts`: `AuthorizationDecision`, `CategoryLabels`, `AuthTypeLabels`, `maskSecret`, `getAuthDisplayValue`, `REMOTE_AGENT_SCENARIO`, `isRemoteAgentMounted`, `NotifyRoute`, `notifyRoutes`, `isNotifyMounted` | Zero references (all were consumed by the deleted pages). `bot-arch.md` / `notify-api-guide.md` updated. |
| `getProvidersByStyle` (`services/serviceProviders.ts`), `getImageProfile` (`pages/image/profiles/imageProfileStore.ts`), `buildPolicyScope` (`pages/guardrails/rules/policyPresentation.ts`), `useAllProjectEvents` (`packages/projects/src/hooks/useProjectEvents.ts`) | Defined, never used (not even in their own file). |

Net: 23 files deleted (2,048 lines — mostly `ToolCard`, the skill dialogs, `NotifyPage`, `RemoteAgentPage`, `BotChatsButton`) and 40 files edited (imports, `toggleStyles`, `types/bot.ts`, four dead functions).

### 1.2 Phase 1b (this stack's second PR): non-import dead locals, wrapper, artefacts

Done: ~40 unused locals removed by hand (state pairs whose value was never read — `authorizing`, `message`, `cloudPresetId`, `showTokenModal`… — unused props destructured in `TemplatePage`, the `ToggleRow` component, the orphaned `handlePasteFromClipboard`), the self-importing `TokenHistoryChart.tsx` wrapper (callers resolve `./TokenHistoryChart` to the directory index now), tracked `tsconfig.tsbuildinfo` / `package.json.md5` (+ `.gitignore` entry), the stale vendoring comment, and `noUnusedLocals: true` in `tsconfig.json` so it stays at zero. `TemplatePageProps` still declares `showCreateRuleButton` / `showImportButton`; callers may pass them, so the interface is left for a later cleanup. Still open from this table: unused dependencies / ESLint stack (needs a lockfile rewrite, see below) and the product-call items.

Original findings:

| Item | Evidence | Risk |
|---|---|---|
| ~40 unused **non-import** locals | `tsc --noUnusedLocals`: unused `useState` pairs and values in `scenario/ClaudeCodeProfilePage.tsx:81-83` (`showTokenModal`, `token`), `scenario/components/TemplatePage.tsx:71-91,229` (`showCreateRuleButton`, `showImportButton`, `newlyCreatedRuleUuids`, `isLoading`, `cloudPresetId`), `ModelSelectDialog.tsx:52-77` (`customModels`, `recentModels`, `isInitialized`), `OAuthDialog.tsx:47,50`, `BotNotifyGroup.tsx:251-252`, `ImageGenPlaygroundCard.tsx:369` (`handlePasteFromClipboard`), `AgentPage.tsx:153` (`ToggleRow`) … | Low, but each needs a look: some are hook calls (removing changes hook order/side effects) or half-finished features. Do per file, not by script. Then enable `noUnusedLocals` in `tsconfig.json` (see 2a). |
| `components/dashboard/TokenHistoryChart.tsx` is a self-importing wrapper | Line 5 `from './TokenHistoryChart'` resolves to **itself** (file shadows the `TokenHistoryChart/` directory), the `TokenHistoryChart` component has no user (`DashboardPage.tsx:6-7,279-281` uses Daily/Hourly directly) and `TokenHistoryChart/index.ts` is unreachable. | Low. Done. |
| Test-only modules | `packages/mannequin/src/landmarks.ts` (only `landmarks.test.ts`), `packages/projects/src/components/project/messageDetail.ts` (only tests), `packages/ui/src/theme/tone.ts` (its only consumer was the deleted `ToolCard`). | Needs a product call: unfinished feature vs dead. Don't delete without owner answer. |
| `BrandIcons.tsx:202` `Zai`; unused predefined icons in `packages/projects/src/components/icons/index.tsx:67-95` (`DriveFileMove`, `FolderOff`, `PostAdd`, `MemoryOff`, `HandStop`, `NewFolder`) | No reference. | Icon catalogues are intentionally a palette; low value, drop only if touching the file. |
| ~200 exports used only inside their own file (mostly `*Props` / option types, a few constants like `ActivityBar.tsx:35-36` `*_TIP_KEY`, `serviceProviders.ts` `getServiceProviders`/`subscribeToProviders`) | export graph | Cosmetic; let `knip` flag them going forward rather than hand-editing. |
| `react-transition-group` + `@types/react-transition-group` in `frontend/package.json` | No `import` anywhere in `src`/`packages` (MUI brings its own copy). | Low, but needs `pnpm install --lockfile-only` to rewrite `pnpm-lock.yaml`. **Not done**: running `pnpm install` was refused by this environment's permission classifier and the lockfile must not be hand-edited. Whoever can run it: drop `react-transition-group`, `@types/react-transition-group`, and (with `eslint.config.js`) `@eslint/js`, `eslint`, `eslint-plugin-react-hooks`, `eslint-plugin-react-refresh`, `typescript-eslint`, `globals`; `frontend/README.md` is the stock Vite-template text about ESLint and can go with it. |
| Tracked build artefacts: `frontend/tsconfig.tsbuildinfo` (35 KB), `frontend/package.json.md5` | No reader of either in repo/Taskfile/CI (`tsbuildinfo` is not in `.gitignore`). | Low. |
| Unused ESLint stack | `frontend/eslint.config.js` + 6 `eslint*` devDependencies; `pnpm lint` runs `oxlint` and no workflow/Taskfile calls `eslint`. | Low; confirm nobody runs it from an editor integration. |
| Stale comment | `packages/projects/src/theme/index.ts:1-3` says the theme is "vendored under frontend/vendor/tingly-ui … `task ui:sync`"; neither exists any more (`@tingly/ui` is a workspace package). | None. |
| Unused i18n keys | Not measured: 3 locales × per-area files (`.design/i18n.md`). Deleted pages/dialogs leave keys behind (e.g. `notify.*`, skill list dialog strings). | Needs a key-usage tool; propose a separate phase (4). |

## 2. Duplicated implementations

Ordered by value ÷ risk. "Same" = identical after normalisation; "diverged" = intentionally or accidentally different, merge needs a behaviour decision.

| # | Duplicate | Evidence | Verdict |
|---|---|---|---|
| D1 | `useCopyFeedback` twice | `packages/ui/src/hooks/useCopyFeedback.ts` (has `onCopied`, `reset`, `resetMs`; no unmount cleanup, no `catch` on the clipboard promise) vs `packages/projects/src/hooks/useCopyFeedback.ts` (unmount cleanup, ignores clipboard errors; used by `ChatPage`, `Markdown`, `infoRail`, `LinkDetails`). `src/hooks/useCopyFeedback.ts` is a shim to the ui one (`frontend/CLAUDE.md` tells people to use it). | **Done (phase 2b)**: one hook in `@tingly/ui` (ui API + timer cleanup + swallowed rejected write + `reset` clears the timer; `copy` is now async). Projects imports it from `@tingly/ui`; the projects copy is deleted; tests added. Tiny behaviour change: no state update after unmount, no unhandled rejection, a second click restarts the 2 s window. |
| D2 | `ConfirmDialog` in Projects | `packages/projects/src/components/ConfirmDialog.tsx` is a 15-line adapter over `@tingly/ui` `ConfirmDialog` (maps `message→description`, `danger→confirmColor`, closes on confirm). | Not a duplicate of logic; keep, but see 3.5 (adapter belongs next to its callers or `@tingly/ui` should grow the `danger`/close-on-confirm props). |
| D3 | `maskToken` three ways | `utils/tokenUtils.ts:9` (first/last 12 + `********`), `utils/sharingKey.ts:2` (prefix…last 4), inline in `pages/system/AccessControl.tsx:254`; plus `maskSecret` (deleted). Same name, **different output**. | Diverged — rename by purpose (`maskApiToken`, `maskSharingKey`), pull the `AccessControl` inline one into `tokenUtils`. Do not unify output (UI would change). UX principle 3 (one word, one meaning) applies. |
| D4 | `fmtLatency` | `components/dashboard/PerformanceSummary.tsx:32` = `RequestsView.tsx:64` (identical). | **Done (2b)**: both used the same logic as `formatLatency` in `probe/runProbe.ts`; moved to `utils/formatLatency.ts` (runProbe re-exports it). |
| D5 | `useLang` ×3 | `CodexQuickConfig.tsx:237` = `DshQuickConfig.tsx:157` (identical), `ClaudeCodeQuickConfig` has a variant. | **Done (2b)**: `hooks/useAppLanguage.ts`; all three `useLang` copies replaced (the `resolveLanguage(i18n.language)` call sites that also use `i18n` for other things are left). |
| D6 | `FieldRow` | `ClaudeCodeQuickConfig.tsx:969` vs `CodexQuickConfig.tsx:298` (different props: `oneMTooltip` vs `unsetLabel`). | Diverged; old survey (`refactor-frontend-2026-09.md` §1c "QuickConfig field rows — 3-way duplication") still open. Extract a shared row with both optional props. |
| D7 | IndexedDB helpers | `pages/image/imageWorkspaceDb.ts` and `utils/playgroundSession.ts` both define `openDb`, `flush`, the best-effort/no-IDB fallback; both headers explain the same rationale. | Same pattern → one small `utils/idbStore.ts`; each keeps its schema. Medium risk (data persistence) — needs tests with fake-indexeddb or careful manual check. |
| D8 | `localStorage` read/write helpers | `packages/projects/src/utils/storage.ts` (`readLocal`/`writeLocal`), `pages/image/prompt/instructionStarters.ts:7-10` (same names), `hooks/useSearchHistory.ts:9` & `utils/sliceParamsStore.ts:10` (`readAll`), `hooks/useOneTimeTip.ts:7` (`read`), `usePlacement.ts:4`. `hooks/useLocalStorage.ts` already exists. | Replace the hand-rolled try/catch readers with `useLocalStorage` / one `safeStorage` util. Low risk each; do per file with tests. |
| D9 | `formatTokens` / `formatDuration` inside Projects | `transcript/model.ts:665` vs `utils/format.ts:4` (`formatTokens`); `transcript/Transcript.tsx:101` vs `components/workdir/BackgroundTasksPanel.tsx:91` (`formatDuration`; the latter is `export`ed but unused outside). `transcript/` is also published as `@tingly/projects/transcript` and must not import app `utils/`. | Check output equality with a table test, then move both to `transcript/format.ts` (re-exported by `utils/format.ts`). |
| D10 | `ThemeModeProvider` / `NotificationProvider` | Host: `src/contexts/ThemeContext.tsx`, `src/contexts/NotificationContext.tsx`. Standalone Projects app: `packages/projects/src/contexts/ThemeModeContext.tsx`, `hooks/useNotify.tsx`. | Partly intentional: the standalone app has no host. Both themes already come from `@tingly/ui`. Candidate: move the generic provider (mode state + `prefers-color-scheme`) into `@tingly/ui`, keep storage key + i18n coupling as parameters. Only worth it if a third consumer appears; park. |
| D11 | Snippet builders | `ImageGenQuickStartDialog.tsx:46` and `DecisionsGuideModal.tsx:49` both `buildSnippet(lang, …)` producing curl/python/js samples; `components/probe/` and the config modals have a third and fourth. | Diverged (different endpoints/params). Extract a `CodeSampleTabs` + small per-language template helpers; medium effort, no behaviour change. |
| D12 | `CardGrid` | `components/CardGrid.tsx` (exported, 100 lines) vs local `CardGrid` in `components/ConnectProviderDialog.tsx:254`. | Diverged names only; check whether `ConnectProviderDialog` can use the shared one. |
| D13 | `ModelCard` | `components/model-select/ModelCard.tsx` vs local one in `pages/ModelTestPage.tsx:34`. | Different purposes (select vs test); leave, rename the local one `ModelTestCard`. |
| D14 | `unwrap` | `services/openapi.ts:59` vs `packages/projects/src/client/index.ts:89`. | Different clients (host vs Projects); leave. |
| D15 | Deferred from 2026-09 | `ApiKeyTable` vs `OAuthTable` shells, `SystemLogViewer` vs `AILogViewer`, 4 date-format helpers in top-level components, hand-rolled confirm modals vs `ConfirmDialog` (`refactor-frontend-2026-09.md` §2.1–2.8). | Re-measure per item before scheduling; some landed in P3. |

Also: `LegacyRemoteAgentRedirect` and `LegacyRemoteCoderRedirect` in `routes/appRoutes.tsx` had identical bodies — **done (2b)**, now one `LegacyBotsRedirect`.

## 3. Misplaced code / directory ownership

### 3.1 Layering inversions (non-page code importing `pages/`)

`src/components`, `src/layout`, `src/shell` import **from `pages/`**:

- `components/ProviderConfigCard.tsx`, `components/CompactConfigCard.tsx` → `pages/scenario/context/ScenarioPageContext` (28 importers; deferred in 2026-09 "zero bundle impact, 28-importer churn").
- `components/dashboard/{DashboardFilterBar,AgentQuickNav}.tsx`, `layout/{Layout,PowerUpsMenu,useActivityItems}.tsx` → `pages/scenario/scenarioRegistry` (already page-free by design, see `frontend/CLAUDE.md` code-splitting section).
- `layout/Layout.tsx`, `shell/MobileShell.tsx` → `pages/scenario/lastAgent`.
- `layout/useActivityItems.tsx` → `pages/mcp/{profileStore,types}`, `pages/image/profiles/imageProfileStore`, `pages/image/components/{ThumbImage,imageThumbnails}`, `pages/projects/ProjectsNav`.

These are the "nav-level shared state lives in a page folder" pattern the CLAUDE.md warns about; it is currently safe (modules are page-free) but the *location* says otherwise. Proposal: a `src/features/` (or `src/domain/`) layer for cross-cutting feature state — `scenario/{registry,lastAgent,context}`, `mcp/profileStore`, `image/profiles` — that `pages/`, `layout/` and `components/` all import; `pages/` then only contains routed screens and their private parts. Mechanical move + import rewrite; guard with an oxlint/`no-restricted-imports` rule (`components|layout|shell|hooks|utils` must not import `@/pages/*`).

### 3.2 Page-owned code sitting in shared folders

Files whose **only** importer area is a single page — colocate them with it (or keep shared and say why):

- Scenario: `components/{ApiKeyModal,CreateSharingKeyButton,DuplicateDialog,dialogClose}`, `components/limits/TeamLimitsDialog`, `components/hooks/useScrollToNewRule` (a `components/hooks/` dir with one file), `hooks/{useFunctionPanelData,useModelSelectDialog,useSharingKeyActions,useTeamSharingKeys}`, `utils/{copyName,providers}`.
- Credential: `components/{ApiKeyTable,OAuthTable}`, `components/credential/sortCredentials`, `hooks/useProviderProxy`.
- Image: `utils/{playgroundSession,promptFile,sliceParamsStore}`.
- Help: `components/{ProvidersCard,ShortcutCard}`. System: `components/GlobalExperimentalFeatures`. VirtualModels: `components/VirtualModelsTable`. Prompt/skills: `components/prompt/skill/{DeleteSkillLocationDialog,skillGrouping}`. Bots: `components/remote-control/CollapsibleGuide`, `hooks/useBotToggle`. Projects page: `components/claudeCode/{ModelSelect,ProfileSelect,RouteStatusLine}`.

`components/` has 114 entries at its top level; after moving the single-use ones it should hold only genuinely shared components (cards, dialogs, badges, graph).

### 3.3 Page code used by other pages

`pages/scenario/{components,hooks,context}` is a shared library in disguise: `TemplatePage`, `ScenarioCardHeader`, `AgentAccessSection`, `AgentStatusRow`, `ScenarioPageSkeleton`, `useRuleManagement`, `useScenarioPageInternal`, `useAgentObservation` are imported by `pages/image/{ImageApiPage,IdeaToPromptPage,ImagePlaygroundPage,ImageProfilePage,…}` and `pages/mcp/MCPProfilesPage`; `pages/image/components/ImageModelPicker` imports `useTemplatePageRules`; `src/mocks/image/*` import page files. Same remedy as 3.1 (a shared `features/` or `components/scenario/` home), and mocks should import from the shared module, not a page.

### 3.4 `src/theme`, `src/styles`, `src/constants`

- `src/theme/{controls,elevation,fonts,options,status,index,palettes/light}.ts` are one-line re-exports of `@tingly/ui/theme/*` ("old paths only re-export", `CLAUDE.md`). Importers: `@/theme/fonts` 117 files, `status` 9, `elevation` 5, others 1–3. `components/{CoachMark,ConfirmDialog,ConfirmPopover,CopyIconButton,DialogHeader,EmptyState,PageHeader,PageLayout,Surface}.tsx`, `constants/zIndex.ts`, `hooks/useCopyFeedback.ts`, `utils/clipboard.ts` are the same kind of shim (3–28 importers each; `PageLayout` 28, `CopyIconButton` 18, `ConfirmDialog` 15). `components/icons` is a shim by design (202 importers, documented convention) — keep.
  Proposal: codemod imports to `@tingly/ui/...` and delete the shims in one mechanical PR, *except* `@/components/icons` and `@/theme/{brand,dataviz,routeGraph}` (host-only). Keep `CLAUDE.md` ("old `src/` paths only re-export") in sync.
- `src/styles/{textStyles,toggleStyles}.tsx` are theme-adjacent style tokens living outside `theme/`; `components/dashboard/chartStyles.ts` and `components/RoutingGraphTypes.ts` likewise. Move `toggleStyles` to `@tingly/ui/theme/` (it only depends on `radius`) and `chartStyles` next to `src/theme/dataviz.ts`.
- `src/constants/{ideSources,platformGuides,profileScenarios,defaults}` mix data with JSX (`platformGuides.tsx`); fine, but `platformGuides.tsx` belongs under `components/bot/` (its only consumers).
- Colours: the strict style-literals lint already forbids new literals outside theme modules, so no colour migration is pending (checked: `pnpm lint` → `style literals: ok`).

### 3.5 Package boundaries

- Direction is clean: no file in `packages/*` imports `@/…` or the host `src`; `src` imports `@tingly/ui` (238 sites) and only the documented entry points of `@tingly/projects`.
- `packages/ui/src/index.ts` is the package entry but hosts import both the barrel and deep paths (`@tingly/ui/components/…`, `/theme/…`, `/hooks/…`) — pick one style (deep for tree-shake-friendly `sideEffects:false`, barrel for the few primitives) and document it in `packages/ui`.
- `packages/ui/src/theme/palettes/{claude,dark,ds}.ts` export constants (`claudePrimary*`, `darkBg*`, `darkInput*`, `dsSecondary*`) used only inside their own files — un-export.
- `packages/projects/src/embed.ts` re-exports `Composer`, `StatusGroups`/`StatusHeading`, `WorkspaceLayout`/`useWorkspacePane`, `timeAgo` that no host file appears to import (checked by grep of `from '@tingly/projects'` sites, not by the graph tool — re-verify before trimming); trim to what the host uses, or document them as the public API.
- Projects' own `ConfirmDialog` adapter (D2), `Sidebar`/`Layout` (host vs standalone, different by design) stay.
- `@mui/icons-material` is a devDependency used for one type (`SvgIconComponent`, `ui/src/icons/tablerMui.tsx:4`, `icons/index.tsx:178`) — fine, but make it a type-only dependency comment so nobody imports icons from it.

### 3.6 Naming / structure nits

- `src/pages` mixes `FooPage.tsx` at the top level with `foo/` directories of the same feature (`UserUsagePage.tsx` + `userUsage/`, `GuardrailsPage.tsx` + `guardrails/`, `DashboardPage`, `CredentialPage`…). Move each shell into its folder (`pages/guardrails/GuardrailsPage.tsx`); `routes/appRoutes.tsx` lazy paths change in one place.
- `components/credential/{LazyQuotaRawResponseDialog,QuotaRawResponseDialog}` both export a component named `QuotaRawResponseDialog` — rename the lazy wrapper.
- `pages/mcp/types.ts` has 9 unused exports (constants + aliases) — trim during phase 3.

## 4. Phased plan (one PR each, review-sized)

| Phase | PR | Content | Risk | Verify |
|---|---|---|---|---|
| **1** (this PR) | Remove unreachable code | §1.1 | none: typecheck, lint, tests, vite build identical | `gen:api` → `typecheck` → `lint` → `test` (16 pre-existing env failures unchanged) → `vite build` |
| 1b | Remaining dead code | §1.2: unused locals (per file), `TokenHistoryChart` wrapper, unused deps, tracked artefacts (+ `.gitignore`), stale comments, eslint decision; **add `knip` + `noUnusedLocals`** so it stays clean | low | typecheck; lockfile diff reviewed; build; modulepreload list in `dist/index.html` unchanged |
| 2a | Tooling guard | `knip` config, `no-restricted-imports` for `@/pages` from non-pages, unused-export report in CI (non-blocking first) | none | lint |
| 2b | Pure-same duplicates | D4, D5, `Legacy*Redirect`, D1 (`useCopyFeedback` → ui) | low | tests for the helper; copy button manual check |
| 2c | Diverged duplicates | D3 rename, D6 `FieldRow`, D8 storage helpers, D9 formatters (table tests first) | low–medium | unit tests; screenshots of Claude Code / Codex config modals (expect none changed) |
| 2d | Larger dedupe | D7 IndexedDB util, D11 code-sample component, D15 deferred table/log shells | medium | needs tests/screenshots; split further if big |
| 3a | Delete shims | §3.4 codemod to `@tingly/ui/*`, remove `src/theme` + component shims; update `CLAUDE.md` | low (mechanical, typecheck-proven) | typecheck, tests, build |
| 3b | Colocate single-use code | §3.2, one PR per page area (scenario, credential, image, rest) | low | typecheck; lazy-chunk check (`CLAUDE.md` code-splitting section) |
| 3c | Feature layer | §3.1/3.3: `features/` for scenario/mcp/image shared state + the import-ban rule; move `ScenarioPageContext` (the 2026-09 deferred item) | medium (large import churn, chunking) | typecheck, tests, `pnpm build` + `modulepreload` unchanged |
| 3d | Page folders / naming | §3.6 | low | typecheck, route contract test |
| 4 | i18n key cleanup | unused keys per locale with a usage scanner; keep en/zh/ru parity | low, noisy | i18n parity test + app smoke in mock mode |
| 5 | Package boundaries | §3.5 trims, `ThemeModeProvider` question (D10) | low | typecheck of every package |

Each phase ends with `pnpm gen:api && pnpm typecheck && pnpm lint && pnpm test` and, for anything touching imports of lazy pages, `pnpm build` + a check that `dist/index.html`'s `modulepreload` list did not grow. Phases that can change pixels (2c, 2d) include before/after mock-mode screenshots (`ui-preview` skill) in the PR.

## 5. Notes for whoever continues

- `pnpm typecheck` is only clean after `pnpm gen:api`; `tsconfig.json` includes `packages/*/src`, but running `tsc` *inside* `packages/projects` reports unrelated `process` type errors in `*.test.ts` (missing `@types/node` there) — not part of the repo's check.
- Do the work in an isolated checkout with real per-entry `node_modules` symlinks (project memory `ui-preview-env`); the 16 "Denied ID" test files are environmental there.
- `frontend/CLAUDE.md` has two lines that go stale if phase 3a lands ("old `src/` paths only re-export", the `@/theme/*` mentions); update them in that PR.
