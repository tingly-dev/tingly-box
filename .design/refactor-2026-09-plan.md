# Frontend Refactoring Plan — 2026-09

> **Status: COMPLETE (2026-09-23).** P0–P6 all executed — 26 commits from `ac91bab70` (docs) to `8935bfed4`, 174 files, net −2,020 lines (≥3,000 lines of pure dead code removed; the additions are the extracted small modules). Verified per batch: `pnpm typecheck` (no new errors vs baseline), `pnpm build`, modulepreload list unchanged, vitest 278 passing. RulesPage + guardrails pages screenshot-verified via ui-preview mock mode.
>
> Deferred (assessed, intentionally not done): `ScenarioPageContext` relocation out of `pages/` (page-free today, zero bundle impact, 28-importer churn); SkillPage i18n (needs en/zh/ru key parity decision); App.tsx routing-widget extraction (cosmetic); ApiKeyTable/OAuthTable full table-shell merge (row rendering diverges; overflow-menu/delete-confirm hooks extracted instead); ModelTestPage in-app entry (product decision — back-links fixed to `navigate(-1)`); Codex statusline `.ps1` (scripts point at the real repo path but no ps1 exists in-repo yet).
>
> Known behavior changes (intentional, review-worthy): SystemLogViewer/AILogViewer no longer refetch on sort clicks; GroupsPage policy summaries use the rich shared formatter; Escape now closes ClaudeCode/OpenCode config modals uniformly; useCopyFeedback gained the execCommand fallback (secure contexts unaffected); ProbeDialog curl keeps loading-state through the debounce window.

Branch: `refactor/0922`. Baseline: `pnpm build` passes; `pnpm typecheck` clean except pre-existing WIP `src/services/api.errorHandling.test.ts` (untracked, not ours — ignore its errors). `frontend/package.json` has an unrelated pending dependency-bump diff — **do not touch package.json / pnpm-lock.yaml in refactor commits.**

Five area reports (all grep-verified) feed this plan:

- [refactor-2026-09-shared-infra.md](refactor-2026-09-shared-infra.md) — services/hooks/contexts/utils/layout/constants/types
- [refactor-2026-09-top-level-components.md](refactor-2026-09-top-level-components.md) — `components/*.tsx` top level
- [refactor-2026-09-scenario.md](refactor-2026-09-scenario.md) — `pages/scenario/`
- [refactor-2026-09-guardrails.md](refactor-2026-09-guardrails.md) — guardrails pages + rule-card/nodes/flags/tier
- [refactor-2026-09-remaining-pages.md](refactor-2026-09-remaining-pages.md) — remaining pages + component subdirs + App/vite

## Global staging (risk ascending; each numbered phase = one or more independent commits)

### P0 — Dead-code sweep (cross-cutting, ~2,500+ lines, near-zero risk)

- `pages/mcp/MCPClientEditor.tsx` + `pages/mcp/localTypes.ts` (487+ lines, fully orphaned; localTypes duplicates live `types.ts` with drift)
- 6 dead node files in `components/nodes/` (ModelNode, ConfigNode, RoutingModeNode, PlatformNode, AgentConfigNode, DividerNode — ~831 lines) + `StyledModelNode` in styles.tsx
- `constants/index.ts` (whole file dead; live SCENARIOS lives in scenarioRegistry — collision trap), `services/service_providers.json`, `styles/style.tsx`
- Dead exports: `types/remoteGraph.ts` (keep AgentConfig), `utils/modelUtils.filterModels/navigateToModelPage`, `utils/protocol.isHttps`, `constants/ideSources.getIdeSourceIcon`, `constants/profileScenarios.ProfileScenario`, `styles/common.ts` dead entries, 5 dead exports in `rule-card/utils.ts`, dead compat surface of `useCustomModels/useNewModels/useRecentModels/useProviderModels`, `useModelSelection` (single trivial consumer), ModelSelectContext probing/snackbar slice, `useRuleManagement` unused setters, `PROTOCOL_VALUES`, `SharingKeysTable` duplicated mask helpers, unused prop-type exports, `ExportFormat` imports
- Codex modal `SHOW_CODEX_SESSION_IMPORT = false` gated ~200 lines; unused imports in UseCodexPage/UseClaudeDesktopPage; ModelRequestHeader unreachable context menu; ProviderFormDialog empty Advanced accordion
- `api.ts` ~33 zero-call-site methods — **own commit** for easy revert
- Kept (verified alive or decision-pending): `?policyId=` deep-link, ModelTestPage route, `notifyAuthFailure`/auth-prompt wiring (flagged as latent bug, out of refactor scope)

### P1 — Convention & micro-dedupe sweep

- `useNotify()` adoption: local Snackbar in 4 pages (PlatformBotPage, BotOverviewPage, PlatformRemoteAgentPage, VirtualModelsPage); `{type,text}`+Alert pattern in 5 guardrails pages
- `useCopyFeedback`/CopyIconButton adoption: ApiKeyTable raw clipboard, SkillPage, CredentialsPage, rule-card/utils copyToClipboard (port execCommand fallback into useCopyFeedback first)
- `utils/datetime.ts`: 4× `toLocalISOString` + per-page range-window builders; Dashboard totals → existing `computeUsageSummary`
- Shared: `blurActiveElement` (3 copies), token-mask single source, date-format helper, `useProviderDialog`/`useProviderEditDialog` shared payload mapper, ConfirmDialog replacing 3 hand-rolled confirm modals, ProviderFormDialog console.logs + commitOpenAI/commitAnthropic collapse
- App.tsx cosmetics: normalize lazy-import suffixes; SystemLogViewer React-key bug fix

### P2 — services/api.ts split & de-dupe (shared infra structural)

- Extract `rawControlCall` primitive (5 near-identical call blocks); API-token wrap boilerplate ×5
- Split per-domain modules (guardrailsApi, mcpApi, imbotApi, usageApi, …) keeping `api.ts` as aggregate — same pattern as existing botApi/modelApi
- Model-list hook family: extract persisted-store factory for useCustomModels/useNewModels/useRecentModels; hoist dual cache instances in ModelSelectDialog+ModelsPanel; delete `useModelSelection`
- Context value memoization: ModelSelectContext, HealthContext, FeatureFlagsContext

### P3 — Top-level components consolidation

- OAuthDialog split: `FALLBACK_OAUTH_PROVIDERS` → own module (fixes eager-bundle trap), `OAuthAuthorizationDialog` → own file, polling → hook
- ApiKeyTable/OAuthTable shared table shell (+ BotTable as adopter), `useRowOverflowMenu`/`useDeleteConfirm`
- SystemLogViewer/AILogViewer: `useAutoRefresh` + sort-via-memo (fixes sort-refetch waste) + shared shell
- OAuthDialog proxy field → `provider-form-dialog/ProxyUrlField`; ConnectProviderDialog region-block dedupe; DialogHeader adoption

### P4 — pages/scenario consolidation (largest structural win)

1. `ManualFileSection` + `writeFileScripts` across Codex/Dsh/ClaudeCode/OpenCode modals (fix close-guard Escape drift + your-repo URL + i18n strays in same commit)
2. Use*Page merges via scenarioRegistry descriptors (0-diff pairs first: OpenAI/Anthropic, Cursor/Xcode)
3. `CopyUrlKeyButtons` + `InstructionSteps` (Xcode/Cursor/ClaudeDesktop, Pi/VSCode)
4. `ConfigModalShell` + `useAppliedPrefs` + `useDebouncedPreview`
5. QuickConfig shared row/useLang/merge helper
6. ImageGenPlaygroundCard split (lightbox → run card → hooks), move ImageSlice/SketchCanvas to subfolder
7. Relocate `ScenarioPageContext` out of `pages/`; TeamGuideDialog import home

### P5 — Guardrails structural split

- Shared modules: `rules/types.ts`, `useGuardrailsConfig`, `uniqueIdFromName`, `policyPresentation` (shrinks Rules+Groups together)
- RulesPage split per report §1.1 (PolicyEditorDialog first — fixes keystroke full-page re-render; then PolicyKindCards `.map()`, PolicyListSection, CompactListEditor, ScenarioScopeSelector, RegistryCard)
- rule-card export-wrapper collapse (6 → 1); catalog-dialog scaffolding dedupe

### P6 — Remaining pages structural

- `useBotList` hook for bot trio; UserUsage/Dashboard/SkillPage splits; SkillPage i18n; bench `useDebouncedCurl` lift; barrel trims; `pages/mcp` cleanup (types.ts remains single source)

## Verification protocol (every phase)

1. `pnpm typecheck` — no new errors vs baseline (ignore pre-existing api.errorHandling.test.ts errors)
2. `pnpm build` — success; for code-splitting-sensitive phases (P3/P4), compare `dist/index.html` modulepreload list
3. Re-grep any "dead" claim before deleting
4. UI-visible changes (P1 notify swap, P4 modals): `ui-preview` screenshot sanity check
5. One logical block per commit; never include package.json/pnpm-lock.yaml

## Decision-pending items (defaults chosen conservatively, revisit later)

| Item | Default |
|---|---|
| ModelTestPage: no in-app entry, back-links to nonexistent `/api-keys` | Keep route; fix back-links to a valid page in P1; surface to user for product decision |
| `?policyId=` deep-link (no in-app producer) | Keep (manual bookmarks plausible) |
| `notifyAuthFailure` wiring never fires (401 prompt can never open) | Out of refactor scope; flagged |
| SkillPage/UserPage/ServerToolPage/CredentialPage zero-i18n | i18n only for SkillPage in P6 (needs locale keys); others deferred |
