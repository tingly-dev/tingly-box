# Frontend Refactor 2026-09 — Plan & Survey Reports (merged)

> One-page contents: [Status & Staging Plan](#status--staging-plan) · [Routing UI follow-up](#routing-ui-follow-up-2026-09-23) · [Appendix: shared-infra](#appendix-shared-infra) · [Appendix: top-level-components](#appendix-top-level-components) · [Appendix: scenario](#appendix-scenario) · [Appendix: guardrails](#appendix-guardrails) · [Appendix: remaining-pages](#appendix-remaining-pages)

## Status & Staging Plan

> **Status: COMPLETE (2026-09-23).** P0–P6 all executed — 26 commits from `ac91bab70` (docs) to `8935bfed4`, 174 files, net −2,020 lines (≥3,000 lines of pure dead code removed; the additions are the extracted small modules). Verified per batch: `pnpm typecheck` (no new errors vs baseline), `pnpm build`, modulepreload list unchanged, vitest 278 passing. RulesPage + guardrails pages screenshot-verified via ui-preview mock mode.
>
> Deferred (assessed, intentionally not done): `ScenarioPageContext` relocation out of `pages/` (page-free today, zero bundle impact, 28-importer churn); SkillPage i18n (needs en/zh/ru key parity decision); App.tsx routing-widget extraction (cosmetic); ApiKeyTable/OAuthTable full table-shell merge (row rendering diverges; overflow-menu/delete-confirm hooks extracted instead); ModelTestPage in-app entry (product decision — back-links fixed to `navigate(-1)`); Codex statusline `.ps1` (scripts point at the real repo path but no ps1 exists in-repo yet).
>
> Known behavior changes (intentional, review-worthy): SystemLogViewer/AILogViewer no longer refetch on sort clicks; GroupsPage policy summaries use the rich shared formatter; Escape now closes ClaudeCode/OpenCode config modals uniformly; useCopyFeedback gained the execCommand fallback (secure contexts unaffected); ProbeDialog curl keeps loading-state through the debounce window.

Branch: `refactor/0922`. Baseline: `pnpm build` passes; `pnpm typecheck` clean except pre-existing WIP `src/services/api.errorHandling.test.ts` (untracked, not ours — ignore its errors). `frontend/package.json` has an unrelated pending dependency-bump diff — **do not touch package.json / pnpm-lock.yaml in refactor commits.**

Five area reports (all grep-verified) feed this plan:

- Appendix: shared-infra report — services/hooks/contexts/utils/layout/constants/types
- Appendix: top-level-components report — `components/*.tsx` top level
- Appendix: scenario report — `pages/scenario/`
- Appendix: guardrails report — guardrails pages + rule-card/nodes/flags/tier
- Appendix: remaining-pages report — remaining pages + component subdirs + App/vite

## Routing UI follow-up (2026-09-23)

The route graph and page header received a visual consistency pass after the structural refactor:

- Explicit `1M: On` and forced `Endpoint: Chat/Responses` choices use a soft blue selected state. `1M: Off` and `Endpoint: Auto` remain neutral. Page-header Plugins controls use the same selected treatment.
- Smart conditions and rule Plugins entries are configuration details, so they use a neutral hover-like background and the existing provider-name typography (`NODE_LAYER_STYLES.typography`, `body2`, regular weight, secondary text). Smart conditions show the field and comparison/value on separate lines so values remain readable at that size.
- Long plugin names truncate within their row instead of displacing the remove control; the tooltip includes the full name. The pinned Plugins card aligns to the top of the graph, including long Smart routes.
- Plugin rows retain their soft background with quieter 12px typography. Clicking a row opens the catalog at that plugin; the × removes any active plugin type without opening the editor.
- Direct and Smart mode buttons share 12px, semibold text and proportionate icons; the selected fill remains their state cue.

UX review: explicit choices are visible without making metadata compete with model names; concrete condition values remain inspectable; the Plugins card is near the first route row. Light/dark mock previews were checked on Claude Code and OpenAI SDK pages, including explicit 1M and Endpoint selections. `pnpm build:dev`, targeted `oxlint`, and a TypeScript check excluding the unrelated untracked `src/services/api.errorHandling.test.ts` passed; the full typecheck is blocked by that file's existing errors.

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

---

<a id="appendix-shared-infra"></a>

# Appendix: shared-infra — Shared Infrastructure (services / hooks / contexts / utils / layout / constants / types / styles)

Date: 2026-09-23. Scope: `frontend/src/{services,hooks,contexts,utils,layout,constants,types,i18n,styles}`. All "dead" claims verified by grepping the identifier across `frontend/src` (tests noted where they are the only consumer).

---

## 1. DEAD CODE

| # | Finding | Location | Evidence | Action | Risk |
|---|---------|----------|----------|--------|------|
| 1.1 | `service_providers.json` never referenced | `services/service_providers.json` | No import/fetch of the path anywhere in `frontend/` (excl. node_modules); `serviceProviders.ts` loads from `api.getProviderCatalogs()` instead | Delete file | Low |
| 1.2 | Entire `constants/index.ts` dead | `constants/index.ts:2-26` (`DEFAULT_RULE`, `DEFAULT_RULE_UUID`, `API_STYLES`, `SCENARIOS`, `NOTIFICATION_TYPES`) | Zero importers of `constants`/`constants/index`; the live `SCENARIOS` is `pages/scenario/scenarioRegistry.tsx:36` (name collision trap) | Delete file | Low |
| 1.3 | `notifyAuthFailure` never called → auth-prompt wiring dead | `services/authState.ts:21`; subscriber at `contexts/AuthContext.tsx:190` | Only reference to `authEvents` outside authState is the `onAuthFailure` subscription; nothing (not `openapi.ts`, not `api.ts`) ever emits. `AuthPromptDialog` can never open via a 401 | Either call `authEvents.notifyAuthFailure()` from `controlApi`'s 401 path, or delete the event system + dialog | Medium (latent bug: session-expiry prompt is unreachable) |
| 1.4 | ~33 unused `api.*` methods | `services/api.ts` | Word-grep of each name outside `api.ts`/`modelApi.ts`: zero call sites. Dead: `getHistory` (L110, also its `limit` param is ignored), `getAvailableVirtualModels`, `startServer`, `stopServer`, `restartServer`, `getAllRules`, `getScenarios`, `getScenarioIntFlag`, `setScenarioIntFlag`, `updateGuardrailsConfig`, `reloadGuardrailsConfig`, `probeModel`, `probeProvider`, `openAIChatCompletions`, `anthropicMessages` (modelApi.ts:67-74 — dead everywhere, not just the alias), `setUserToken`, `getUserToken`, `removeUserToken`, `setModelToken`, `removeModelToken`, `oauthProviders`, `oauthProviderConfig`, `getSkillLocation`, `scanIdes`, `listMCPClients`, `getMCPClient`, `createMCPClient`, `updateMCPClient`, `deleteMCPClient`, `reconnectMCPClient`, `getMCPInstallCommand`, `executeMCPTool`, `getAPIToken` | Delete in a single "retire unused API surface" PR; check git history first in case CLI/docs reference them | Low-Medium (large but mechanical; grep hits were unrelated identifiers/i18n keys) |
| 1.5 | useCustomModels dead surface | `hooks/useCustomModels.ts` | Only external consumers destructure `{customModels}` (ModelsPanel:83) and `{customModels, removeCustomModel, saveCustomModel, updateCustomModel}` (ModelSelectDialog:45). Dead: `CUSTOM_MODEL_UPDATE_EVENT` (L18), `getCustomModels` (L139), `getCustomModel` (L144), `isCustomModel` (L150 — ModelsPanel's `isCustomModel` comes from `utils/modelUtils.getModelTypeInfo`, not this hook), `loadCustomModelsFromStorage` (L155), `removeCustomModelFromStorage` (L159), `dispatchCustomModelUpdate` (L164), `listenForCustomModelUpdates` (L169); `saveCustomModelToStorage` is exported but only used internally | Strip the "backward compatibility" wrapper block | Low |
| 1.6 | useNewModels dead surface | `hooks/useNewModels.ts` | Consumers: ModelsPanel:87 `{newModels, clearNewModels}`, useProviderModels:27 `{detectAndStoreNewModels}`. Dead: `NEW_MODELS_UPDATE_EVENT` (L25), `getNewModels` (L97), `loadNewModelsFromStorage` (L102), `saveNewModelsToStorage` (L107), `removeNewModelsFromStorage` (L111), `dispatchNewModelsUpdate` (L116), `listenForNewModelsUpdates` (L121); also `version`/`refetch` unused by consumers | Same | Low |
| 1.7 | useRecentModels dead surface | `hooks/useRecentModels.ts` | Consumers: ModelSelectDialog:61 `{recentModels, lastProvider}`, ModelsPanel:86 `{recentModels}`, useModelSelection:13 `{addRecentModel}`. Dead: `RECENT_MODELS_UPDATE_EVENT` (L21), `getRecentModels` (L59), `clearRecentModels` (L64), `loadRecentModelsFromStorage` (L75), `saveRecentModelsToStorage` (L80), `removeRecentModelsFromStorage` (L84), `dispatchRecentModelsUpdate` (L89), `listenForRecentModelsUpdates` (L94) | Same | Low |
| 1.8 | useProviderModels dead surface | `hooks/useProviderModels.ts` | Consumers (ModelSelectDialog:46, ModelsPanel:84) use only `providerModels, refreshingProviders, fetchModels, refreshModels`. Dead: `MODEL_CACHE_EVENT` (L18), `PROVIDER_MODELS_UPDATE_EVENT` (L20, legacy string), `setModels` (L152), `removeModels` (L165, only referenced from its own test), `refetchAll` (L184), `isRefreshing` (L190 — ModelsPanel computes its own from `refreshingProviders`), `getModels` (L195) | Same (keep test-covered members only if the test is kept meaningful) | Low |
| 1.9 | ModelSelectContext probing/snackbar slice dead | `contexts/ModelSelectContext.tsx` | `addProbingModel`/`removeProbingModel`/`isModelProbing` are destructured only by `hooks/useModelSelection.ts:12`, which never calls them. `hideSnackbar` (L103, no-op) and the `snackbar: CLOSED_SNACKBAR` field are read by nobody. `showSnackbar` (L99) is a pure alias of `notify.show` | Delete `probingModels` state + `snackbar`/`hideSnackbar`; keep `showSnackbar` alias or rename to `notify` | Low |
| 1.10 | `hooks/useModelSelection.ts` effectively dead weight | `hooks/useModelSelection.ts:11-25` | Single consumer `components/ModelSelectDialog.tsx:60`. The callback only calls `onSelected` + `addRecentModel`; the four context values it destructures (L12) are unused | Inline the 2-line callback into ModelSelectDialog and delete the hook (with 1.9) | Low |
| 1.11 | `types/remoteGraph.ts` mostly dead | `types/remoteGraph.ts` | Only `AgentConfig` is imported (by `components/nodes/AgentConfigNode.tsx:1`). Dead: `RemoteGraph`, `RemoteConnection`, `RemoteAgentsListResponse`, `RemoteAgentResponse`, `RemoteAgentCreateRequest`, `RemoteAgentUpdateRequest`, `GuideAgent`, and its `ConfigProvider` (L38) which duplicates `components/RoutingGraphTypes.ts:5` | Reduce file to `AgentConfig` (+ what it needs) or move it and delete the rest | Low |
| 1.12 | Dead style/util exports | `styles/style.tsx:7,18` (`ToggleButtonGroupStyle`, `ToggleButtonStyle` — PascalCase twins of `styles/toggleStyles.tsx`, zero usages); `styles/common.ts:4` (`commonStyles`), `styles/common.ts:102` (`getStatusBgColor`); `utils/modelUtils.ts:79` (`filterModels`), `utils/modelUtils.ts:86` (`navigateToModelPage`); `utils/protocol.ts:70` (`isHttps`), `utils/protocol.ts:28` (`getApiProtocol` — only used internally by `getApiBaseUrl`); `constants/ideSources.ts:31` (`getIdeSourceIcon`); `constants/profileScenarios.ts:10` (`ProfileScenario` type) | Delete; delete all of `styles/style.tsx` once the two exports go | Low |

No commented-out code blocks of significance were found in scope (only prose comments).

---

## 2. DUPLICATION

| # | Finding | Locations | Evidence / Analysis | Suggested action | Risk |
|---|---------|-----------|---------------------|------------------|------|
| 2.1 | **Model-list hook family is one pattern × 3** | `hooks/useCustomModels.ts` (193 L), `hooks/useNewModels.ts` (142 L), `hooks/useRecentModels.ts` (116 L) | Identical architecture: module-level `createEventSystem` + `useLocalStorage` keyed store + listen→`refetch` effect + same-shaped save/remove/get + 5-8 dead "backward compatibility" wrappers each (see 1.5-1.7). ~60% of each file is boilerplate | Extract a factory, e.g. `createPersistedKeyedStore<T>(storageKey, eventName)` returning `{data, save, remove, get}`, or fold the event→refetch sync into `useLocalStorage` itself (it already syncs via the `storage` event for cross-tab). Keep thin domain wrappers | Medium |
| 2.2 | Model-selection hooks: overlap assessment | `useModelSelectDialog` / `useModelSelection` / `useProviderModels` / `useModelDescriptions` | **Not** duplicates of 2.1: `useProviderModels` is a server-backed in-memory cache (control-plane `/api/v2/provider-models`), `useModelDescriptions` fetches gateway `/v1/models` descriptions into a per-mount `useState` cache. Real overlaps: (a) `useModelDescriptions` is a second, unshared model-metadata cache — candidate to fold into the `useProviderModels` store or a module-level cache; (b) `useModelSelectDialog` mixes dialog state with rule persistence (`api.updateRule` + `buildRuleUpdatePayload`, L212-238) — persistence could move to the caller (`TemplatePage`, its only consumer); (c) `useModelSelection` is redundant (see 1.10) | Fold descriptions cache; slim `useModelSelectDialog` | Medium |
| 2.3 | **Dual hook instances for one dialog** | `components/ModelSelectDialog.tsx:46` and `components/model-select/ModelsPanel.tsx:84` both call `useProviderModels()`; same for `useCustomModels`/`useNewModels`/`useRecentModels` (ModelSelectDialog:45-46,61 vs ModelsPanel:83-88) | ModelsPanel is rendered *inside* ModelSelectDialog (ModelSelectDialog.tsx:227), so two independent state caches + two event listeners exist for the same data; updates propagate only via the event→refetch dance, duplicating memory and listener churn | Hoist the model stores to module-level singletons (the event system already exists) or provide them via `ModelSelectContext` so there is exactly one cache per dialog | Medium |
| 2.4 | useProviderDialog vs useProviderEditDialog | `hooks/useProviderDialog.tsx:253-303`, `hooks/useProviderEditDialog.tsx:38-118` | Not wholesale duplicates (add flow vs edit flow — both are widely used and worth keeping separate), but they duplicate: form-state + field-change handler (`providerFormData`/`handleFieldChange` vs `handleProviderFormChange`), submit handler shape, and above all the payload mapper: `buildProviderData` (useProviderDialog L253: `api_base_openai: fd.apiBaseOpenAI || undefined`…) vs `buildEditProviderPayload` (L42: `api_base_openai: fd.apiBaseOpenAI ?? ''`…) — same field list, subtly different empty-value handling, a drift hazard | Extract one shared `buildProviderPayload(fd, {emptyAs})` used by both | Low-Medium |
| 2.5 | Three response-envelope helpers | `services/openapi.ts:66` (`controlApi` → `{success:false,error}` or raw data), `services/api.ts:30` (`teamApiCall` → `{success,data,error:{message}}`), `services/botApi.ts:32` (`botAccessCall` → throws) | Documented as intentional (different caller contracts, comments at api.ts:26-29 and botApi.ts:5-15) | Leave contracts, but the duplicated client/headers/errorMessage scaffolding inside each could share one internal primitive | Low |
| 2.6 | Raw-fetch helpers duplicated | `services/api.ts:51` (`fetchUIAPI`, user-token auth), `services/modelApi.ts:31` (`modelAPI`, model-token auth), plus GUI-token fallback duplicated between `services/modelApi.ts:35-47` and `services/openapi.ts:27-36` | Both build `${getApiBaseUrl()}${path}`, inject `Authorization: Bearer`, parse JSON | Extract a shared `rawFetch(path, {token: 'user'|'model'|'none'})`; move the GUI-token fallback into one place | Low |
| 2.7 | Codex/DSH apply/preview quartet boilerplate | `services/api.ts:807-917` (`applyCodexConfig`, `getCodexConfigPreview`, `applyDshConfig`, `getDshConfigPreview`) + `oauthRefresh` (L757) + `getVersion` (L541) | Five near-identical blocks of getClient/getAuthHeaders/widen-`never`-error/unwrap/catch (~30 lines each) differing only in route, body and message | Extract `rawControlCall(route, body, failureLabel)` | Low |
| 2.8 | API-token wrap boilerplate | `services/api.ts:1320-1389` (`listAPITokens`, `getAPIToken`, `createAPIToken`, `deleteAPIToken`, `setAPITokenEnabled`) | Same `if (data?.success === false) return data; return {success: true, data}` wrapper five times | Small local helper | Low |
| 2.9 | usePagination single-consumer | `hooks/usePagination.ts` — only `components/model-select/ModelsPanel.tsx:148` | Not duplicated elsewhere (dashboard tables use MUI pagination), but the hook hard-codes `typeof item === 'string'` filtering and per-provider keyed state for one caller | Optional: inline into ModelsPanel or generalize the filter predicate | Low |

---

## 3. OVERSIZED FILES

| File | Lines | Sections | Clean split boundary | Risk |
|------|-------|----------|---------------------|------|
| `services/api.ts` | 1423 | status/server/token (L82-168) · rules (L171-235) · scenarios+flags (L238-305) · profiles/claude-config (L307-383) · guardrails (L385-498) · probe (L500-537) · version/shortcut/imagegen/health (L541-594) · model-gateway aliases (L596-612) · usage dashboard (L614-717) · oauth (L719-786) · config-apply claude/codex/opencode/dsh + codex import (L788-933) · skills (L935-1000) · ImBot settings + QR flows (L1002-1201) · user/model token mgmt (L1127-1147, 601-610) · system config (L1203-1214) · MCP runtime + tool exec (L1216-1313) · API tokens + teams (L1315-1420) | Yes — same pattern as the existing `botApi.ts`/`modelApi.ts` splits: move each section to `services/<domain>Api.ts` (e.g. `rulesApi`, `scenarioApi`, `guardrailsApi`, `imbotApi`, `mcpApi`, `usageApi`, `oauthApi`, `configApplyApi`, `skillApi`, `teamApi`) and keep `api.ts` as a re-exporting aggregate (or migrate callers incrementally). Suggested staging: (1) dead-method purge (1.4) first, (2) extract the four biggest leaves — guardrails (~115 L), MCP (~100 L), ImBot+QR (~200 L), usage (~105 L), (3) config-apply after 2.7 de-duplication | Medium (many call sites, but purely mechanical) |
| `constants/platformGuides.tsx` | 430 | Per-platform JSX onboarding guides + `PLATFORM_BRAND_ICONS`/`platformDisplayName`/`usePlatformGuide` | Under-threshold-adjacent; it is JSX content, not constants — if touched again, move to `components/bots/platformGuides.tsx` (constants/ importing components/icons + i18n is a layering smell) | Low |
| Others | — | `layout/ActivityBar.tsx` (359), `layout/Sidebar.tsx` (356), `hooks/useProviderDialog.tsx` (339), `hooks/useModelSelectDialog.tsx` (328), `services/serviceProviders.ts` (305), `services/botApi.ts` (305), `layout/useActivityItems.tsx` (304) | All under 400; no action needed | — |

---

## 4. CONVENTION VIOLATIONS

| # | Violation | Locations | Evidence | Action | Risk |
|---|-----------|-----------|----------|--------|------|
| 4.1 | `@mui/icons-material` direct imports | None found outside `components/icons/` (the sanctioned adapter) | grep across `frontend/src`: only `components/icons/tablerMui.tsx` (type import) and re-export in `components/icons/index.tsx:166` | None — convention holds | — |
| 4.2 | Local `<Snackbar>` instead of `useNotify` | `pages/VirtualModelsPage.tsx`, `pages/bots/PlatformBotPage.tsx`, `pages/bots/BotOverviewPage.tsx`, `pages/remote-agent/PlatformRemoteAgentPage.tsx` (outside the scoped dirs, found during verification) | grep `<Snackbar` | Migrate to `useNotify()`/`NotificationProvider` (bot pages predate `useBotToggle`'s unification, which already removed 3 such copies) | Low |
| 4.3 | Hand-rolled copy feedback instead of `useCopyFeedback` | In scope: none — `useFunctionPanelData.copyToClipboard` (hooks/useFunctionPanelData.ts:33) is a plain toast-only copy, acceptable. Out of scope, to sweep later: `components/OAuthDialog.tsx`, `components/ApiKeyTable.tsx`, `components/ModelRequestHeader.tsx`, `components/notify/BotNotifyGroup.tsx`, `components/bot/PairingCodePanel.tsx`, `components/bot/BotTable.tsx`, `components/bot/BotChatsButton.tsx`, `pages/SharingKeysPage.tsx`, `pages/guardrails/CredentialsPage.tsx`, `pages/scenario/components/SharingKeysDialog.tsx`, `pages/prompt/SkillPage.tsx` (per `components/CopyIconButton.tsx:29-33` comment, "byte-identical in three of" these) | Replace copied-flag `useState`+`setTimeout` with `useCopyFeedback` | Low |
| 4.4 | Re-implemented storage/visibility primitives | None in scope: the model-list hooks correctly build on `useLocalStorage`; `useProviderModels` uses `usePageVisibility`; `useSidebarCollapsed` mirrors ThemeContext's pattern deliberately (comment at `layout/useSidebarCollapsed.tsx:5`). `useRecentModels`/`useNewModels`' dead `loadXFromStorage` bypass `useLocalStorage` with raw `localStorage.getItem` (L75-78 / L102-105) — dies with 1.6/1.7 | None beyond 1.6/1.7 | — |
| 4.5 | Stub-indirection kept for compile compatibility | `hooks/useFunctionPanelData.ts:16` (`notification: CLOSED_NOTIFICATION` stub) and `contexts/ModelSelectContext.tsx:13` (`CLOSED_SNACKBAR`) — both comments say "kept so consumers keep compiling"; the `snackbar` field has no readers at all anymore (1.9) | Delete the stubs and their `NotificationState`/`SnackbarState` types together with 1.9 | Low |

---

## 5. OPTIMIZATION

| # | Finding | Locations | Evidence | Suggested action | Risk |
|---|---------|-----------|----------|------------------|------|
| 5.1 | Context values not memoized | `contexts/ModelSelectContext.tsx:124` (`const value = {...}` each render), `contexts/HealthContext.tsx:77` (inline object), `contexts/FeatureFlagsContext.tsx:72` (inline object). `ThemeContext` already does it right (`useMemo` at L82) | Any state tick in the provider re-renders every consumer: HealthContext polls every 30 s (`setChecking` twice per cycle, HealthContext.tsx:72), so all `useHealth` consumers re-render 2×/30s; ModelSelectContext's tab state re-renders both ModelSelectDialog and ModelsPanel subtrees on every tab switch | `useMemo` the value objects (HealthContext/FeatureFlags are trivial; ModelSelectContext benefits most) | Low |
| 5.2 | Duplicate per-component hook instances inside one dialog | See 2.3 — `useProviderModels` state exists twice per open dialog | Same fix as 2.3 (module-level store or context-provided) | Medium |
| 5.3 | `useModelDescriptions` cache is per-mount and unshared | `hooks/useModelDescriptions.ts:9-15` — `useState({})` cache dies with unmount; auto-fetch on mount (L61-65) re-hits the gateway `/v1/models` every time a ModelsPanel mounts; `providerUuid` parameter is accepted but ignored | Module-level cache or fold into the model store (2.2a); drop the unused param | Low |
| 5.4 | Code-splitting: no page-file exports leaked into eager code | Verified: eager `layout/useActivityItems.tsx:3` and `components/dashboard/AgentQuickNav.tsx:4` import only from `pages/scenario/scenarioRegistry` — the page-free module created for exactly this purpose (per `frontend/CLAUDE.md`). `App.tsx:28` static-imports `pages/Login`, the documented exception | None — convention holds | — |
| 5.5 | Shared context lives under `pages/**` | `pages/scenario/context/ScenarioPageContext.tsx` is imported by `components/CompactConfigCard.tsx:7` and `components/ProviderConfigCard.tsx:7` (component layer). The module itself is page-free (imports only `useFunctionPanelData`), so today's bundle impact is nil (those two cards are page-only), but it violates the "shared things don't live in pages/**" rule and invites future leaks | Move `ScenarioPageContext.tsx` (and arguably `pages/scenario/hooks/useScenarioPageInternal.ts`, imported by 12 sibling pages — that one is page-to-page, fine) out of `pages/` | Low |
| 5.6 | `useProviderQuota` is single-consumer | `hooks/useProviderQuota.ts` — only `pages/CredentialPage.tsx:51` (`fetchAllQuotas`/`batchFetchQuota`/`loading`/`errors` also unused externally) | Fine as-is; trim return surface when convenient | Low |
| 5.7 | `FeatureFlagsContext` fans 5 separate `useState` + 5 parallel API calls | `contexts/FeatureFlagsContext.tsx:32-52` | Could be one `useState<Flags>` object; consumers destructure anyway. Cosmetic | Low |

---

## Suggested staging (each step independently shippable)

1. **Purge dead exports** (1.2, 1.5-1.8, 1.11, 1.12 + `styles/style.tsx`) — one mechanical PR, low risk.
2. **Retire unused `api.*` methods** (1.4) after a git-history/CLI-usage sanity check.
3. **Fix or remove the auth-failure wiring** (1.3) — decide intent first; it is the only "dead" item that hides a probable bug.
4. **De-duplicate `api.ts` boilerplate** (2.6, 2.7, 2.8) — shrinks api.ts before splitting it.
5. **Split `services/api.ts`** per §3 boundaries, biggest leaves first.
6. **Unify the model-list hook family** (2.1) and give the dialog a single store instance (2.3/5.2), deleting `useModelSelection` and the ModelSelectContext dead slices (1.9, 1.10).
7. **Context `useMemo` pass** (5.1) and `ScenarioPageContext` relocation (5.5).
8. **Convention sweep outside scope** (4.2, 4.3) for the four Snackbar pages and the copy-feedback copies.

---

<a id="appendix-top-level-components"></a>

# Appendix: top-level-components — Top-Level components/*.tsx

Scope: the ~66 files directly in `frontend/src/components/` (subdirectories excluded — covered by other reviews). Every claim below was verified by grep across `frontend/src` or by direct file read on 2026-09-22.

---

## 1. DEAD CODE

No fully-dead files: every top-level component file is imported somewhere (checked by path and by exported-symbol grep, including relative imports).

| Finding | Location | Evidence | Action | Risk |
|---|---|---|---|---|
| Unreachable context Menu (dead UI) | [ModelRequestHeader.tsx:127](../frontend/src/components/ModelRequestHeader.tsx#L127), [158–160](../frontend/src/components/ModelRequestHeader.tsx#L158), [421–442](../frontend/src/components/ModelRequestHeader.tsx#L421) | `menuAnchor` is only ever set to `null` (`setMenuAnchor(null)` in `handleMenuClose`); no handler ever sets it, no `onContextMenu` exists. The Menu (and `handleSetWildcard`, "Custom model name"/"Cancel" items) can never open. | Delete the Menu, `menuAnchor`/`setMenuAnchor`, `handleMenuClose`, `handleSetWildcard` (~25 lines) — or wire it up if the right-click menu is still wanted. | Low |
| Hand-rolled "empty" Advanced accordion | [ProviderFormDialog.tsx:694–721](../frontend/src/components/ProviderFormDialog.tsx#L694) | `AccordionDetails` contains only an empty `<Stack spacing={2.5} />` with comment "Empty for now". Renders a clickable accordion that opens nothing. | Remove the Accordion + `advancedOpen` state until content exists. | Low |
| Unused exported prop types | [ConfirmDialog.tsx:13](../frontend/src/components/ConfirmDialog.tsx#L13) `ConfirmDialogProps`; [EmptyState.tsx](../frontend/src/components/EmptyState.tsx) `EmptyStateAction`; [PluginFeatures.tsx](../frontend/src/components/PluginFeatures.tsx) `PluginFeaturesProps`; [RegionBadge.tsx](../frontend/src/components/RegionBadge.tsx) `RegionBadgeProps` | Zero usages of each symbol outside its own file (scripted grep over all `.ts/.tsx` in src). | Un-export or delete. | Low |
| Dead exports duplicating utils | [SharingKeysTable.tsx:29–38](../frontend/src/components/SharingKeysTable.tsx#L29) `maskToken`, `formatDate` | Only importer (pages/SharingKeysPage.tsx, components/scenario/.../SharingKeysDialog.tsx) imports `SharingKeysTable` + `type SharingKey` — never `maskToken`/`formatDate`. Both also duplicate `utils/tokenUtils.ts:9 maskToken`. | Delete the exports (see §2 for the maskToken duplication). | Low |
| Unused type imports | [ApiKeyTable.tsx:3](../frontend/src/components/ApiKeyTable.tsx#L3), [OAuthTable.tsx:3](../frontend/src/components/OAuthTable.tsx#L3) — `import type {ExportFormat}` | Symbol appears only on the import line in each file. | Delete both import lines. | Low |

---

## 2. DUPLICATION

### 2.1 ApiKeyTable vs OAuthTable — the big one (verified line-by-line)

[ApiKeyTable.tsx](../frontend/src/components/ApiKeyTable.tsx) (680) and [OAuthTable.tsx](../frontend/src/components/OAuthTable.tsx) (664) share near-identical scaffolding, copy-pasted with only cell-level differences:

| Shared piece | ApiKeyTable | OAuthTable |
|---|---|---|
| `COLUMNS` array + `TABLE_MIN_WIDTH` + identical header/comment | 78–87 | 79–88 |
| `DeleteModalState` + delete handlers | 64–68, 179–197 | 59–63, 138–156 |
| `ModelListDialogState` + handlers | 70–73, 207–216 | 71–74, 183–192 |
| `moreMenu` state + open/close handlers | 114–130 | 120–136 |
| Copy Base64/JSONL `useCallback`s | 218–234 | 194–210 |
| TableContainer/Table/fixed-layout/head markup | 236–263 | 251–278 |
| Status Switch cell, Name tooltip cell (UUID tooltip), API Style cell, Proxy cell | 277–343, 387–403 | 294–344, 371–387 |
| Actions cell: Edit + Divider + Quota button + Models button + MoreVert (incl. identical `sx` blocks) | 405–481 | 389–465 |
| `ProviderQuotaDetailRow` fragment row | 484–493 | 467–476 |
| Overflow `Menu` (same anchor config; menu items differ by 2) | 497–554 | 481–562 |
| Delete confirmation Modal (identical Box sx, copy-pasted strings) | 631–669 | 563–601 |
| ModelListDialog wiring | 670–675 | 654–659 |

**Suggested action:** extract a `ProviderTableShell` (container + fixed-layout table + COLUMNS head) plus hooks `useRowOverflowMenu`, `useDeleteConfirm`; replace the hand-rolled confirm Modals with the existing `ConfirmDialog` (see 2.2). Estimated removal: ~300 lines of the 1,344 combined. **Risk: medium** (large mechanical diff, but both tables render in one page — CredentialPage — so it is easy to verify visually).

### 2.2 Hand-rolled confirm Modals vs existing ConfirmDialog

`components/ConfirmDialog.tsx` (with test, used by 13 other files incl. pages/SharingKeysPage, components/bot/BotTable, components/rule-card/dialogs) already implements exactly "title + description + cancel/confirm + loading spinner".

- [ApiKeyTable.tsx:631–669](../frontend/src/components/ApiKeyTable.tsx#L631) — delete-confirm Modal, raw `<Modal>` + centered Box.
- [OAuthTable.tsx:563–601](../frontend/src/components/OAuthTable.tsx#L563) — same delete Modal copy-pasted.
- [OAuthTable.tsx:602–653](../frontend/src/components/OAuthTable.tsx#L602) — refresh-token confirm Modal; `ConfirmDialog` supports `confirmColor`, `loading`, `confirmingLabel` — covers it.

**Action:** swap all three for `ConfirmDialog`. **Risk: low.**

### 2.3 SystemLogViewer vs AILogViewer — log-table shell

[SystemLogViewer.tsx](../frontend/src/components/SystemLogViewer.tsx) (472) and [AILogViewer.tsx](../frontend/src/components/AILogViewer.tsx) (447) duplicate:

- Toolbar: auto-refresh Switch + "Refresh now" Tooltip/IconButton + count caption — SystemLogViewer 183–236 vs AILogViewer 192–237 (same markup).
- `SortField`/`SortOrder` types + `sortField`/`sortOrder` state + `handleSort` — SystemLogViewer 40–41, 55–56, 169–178 vs AILogViewer 61–62, 102–103, 161–170 — **verbatim identical logic**.
- 5s `setInterval` auto-refresh effect — SystemLogViewer 162–167 vs AILogViewer 153–159.
- Client-side sort embedded in the fetch function (`loadLogs`/`loadRequests`), plus an effect that **re-fetches from the network on every sort change** even though sorting is client-side (SystemLogViewer 157–160, AILogViewer 148–151) — should be `useMemo` on already-fetched data.
- Expandable-row pattern (`<>`/`<Fragment>` + detail `TableRow` + `Collapse`), empty-state row, outer Stack layout (181, 191).

**Action:** extract `useAutoRefresh(fn, ms)` + `useSortedFetch` (or sort via memo) and a shared `LogTableShell` (toolbar + bordered scroll box + sticky-header table + empty/loading rows). **Risk: medium.**

### 2.4 Token masking — 4 implementations

| Implementation | Location |
|---|---|
| Canonical | [utils/tokenUtils.ts:9](../frontend/src/utils/tokenUtils.ts#L9) `maskToken` |
| Duplicate export (unused outside) | [SharingKeysTable.tsx:29](../frontend/src/components/SharingKeysTable.tsx#L29) |
| Local copy (different rule) | [OAuthDetailDialog.tsx:81–84](../frontend/src/components/OAuthDetailDialog.tsx#L81) |
| Local variant | [ApiKeyTable.tsx:199–205](../frontend/src/components/ApiKeyTable.tsx#L199) `formatTokenDisplay` |
| (cross-dir, flag only) | [pages/system/AccessControl.tsx:251](../frontend/src/pages/system/AccessControl.tsx#L251) local `maskToken` |

**Action:** consolidate on `utils/tokenUtils.maskToken` (parameterize prefix/suffix lengths). **Risk: low** (masking is display-only).

### 2.5 Date formatting — 4 local helpers in top-level components

`formatDate` [SharingKeysTable.tsx:35](../frontend/src/components/SharingKeysTable.tsx#L35), `formatDate` [OAuthDetailDialog.tsx:63](../frontend/src/components/OAuthDetailDialog.tsx#L63), `formatTimestamp` [SystemLogViewer.tsx:134](../frontend/src/components/SystemLogViewer.tsx#L134), `formatTime` [AILogViewer.tsx:82](../frontend/src/components/AILogViewer.tsx#L82) — all `new Date(x).toLocaleString()` + fallback. **Action:** one `formatDateTime` util. **Risk: low.**

### 2.6 OAuthDialog proxy block vs provider-form-dialog/ProxyUrlField

- [OAuthDialog.tsx:932–966](../frontend/src/components/OAuthDialog.tsx#L932) hand-rolls "Proxy URL TextField + Use quick proxy FormControlLabel/Checkbox" — near-identical markup to `components/provider-form-dialog/ProxyUrlField.tsx` (same props: `proxyUrl`, `globalProxyUrl`, `useGlobalProxy`, `onUseGlobalProxyChange`; same disabled/label logic).
- Global-proxy fetch duplicated: OAuthDialog 604–610 and ProviderFormDialog 122–126 both call `api.getConfig()` → `http_transport.global_proxy_url`.
- localStorage keys diverge: `oauth_use_global_proxy` (OAuthDialog:601) vs `provider_use_global_proxy` (ProviderFormDialog:294) — two persisted flags for the same concept.

**Action:** reuse `ProxyUrlField` in OAuthDialog; extract a `useGlobalProxy(persistKey)` hook. **Risk: medium** (OAuth flow behavior).

### 2.7 ConnectProviderDialog: duplicated CN/Global region groups

[ConnectProviderDialog.tsx:482–515](../frontend/src/components/ConnectProviderDialog.tsx#L482) and [517–550](../frontend/src/components/ConnectProviderDialog.tsx#L517) are the same 30-line block twice (RegionBadge header + count + CardGrid + ProviderCard map), differing only in badge/region. **Action:** `renderRegionGroup(region, badge, providers)`. **Risk: low.**

### 2.8 DialogTitle + close-IconButton header pattern

Hand-rolled in OAuthDialog (360–375 and 808–821), ConnectProviderDialog (586–596), ModelListDialog (48–53) — while `components/DialogHeader.tsx` exists but is only used by `components/prompt/skill/*` (verified: only importers are AddSkillLocationDialog, AutoDiscoveryDialog). **Action:** either adopt DialogHeader in these dialogs or delete it. **Risk: low.**

---

## 3. OVERSIZED FILES (>400 lines) — internal sections and split boundaries

| File | Lines | Sections | Split boundary / verdict |
|---|---|---|---|
| [OAuthDialog.tsx](../frontend/src/components/OAuthDialog.tsx) | 1002 | `OAuthProvider` type + `FALLBACK_OAUTH_PROVIDERS` catalog 26–114 · `OAuthAuthorizationDialog` (flow UI + polling + confirm/timeout sub-dialogs) 144–574 · wrapper state (proxy localStorage, session cleanup, auto-start) 576–793 · "direct mode" config screen JSX 794–993 | Highest-value split: (a) move `FALLBACK_OAUTH_PROVIDERS`+type to `oauthProviders.ts` — **ConnectProviderDialog.tsx:21 imports them from OAuthDialog, dragging the whole 1000-line dialog module into the picker's bundle** (same eager-import trap the frontend CLAUDE.md warns about); (b) `OAuthAuthorizationDialog` → own file; (c) polling (225–308) → `useOAuthPolling` hook; (d) proxy config (583–629 + 932–966) → `useGlobalProxy` + `ProxyUrlField` (§2.6). Risk: medium. |
| [ProviderFormDialog.tsx](../frontend/src/components/ProviderFormDialog.tsx) | 773 | init/reset mega-effect 178–303 (contains 8 `console.log`s) · protocol-slot state 112–113 + handlers 306–356 · name helpers 359–374 · verification 377–445 · submit 447–482 · JSX 513–770 | Boundaries: `useProtocolSlots(data, allProviders)` (init/seed/sync) and `useProviderVerification`; the empty Advanced accordion goes (§1); `commitProtocolState` (319–321) is a pass-through wrapper of `syncProtocolsToParent`, and `commitOpenAI`/`commitAnthropic` (342–343) are literally identical — collapse. Risk: medium. |
| [ApiKeyTable.tsx](../frontend/src/components/ApiKeyTable.tsx) | 680 | modals (token view 556–630, delete 631–669), overflow menu 497–554, table body 264–495 | Split = shared shell with OAuthTable (§2.1/2.2); token modal could use CopyIconButton (§4). Risk: medium. |
| [OAuthTable.tsx](../frontend/src/components/OAuthTable.tsx) | 664 | same as above + refresh modal 602–653, expiry helpers 212–249 | Same as above. Risk: medium. |
| [ConnectProviderDialog.tsx](../frontend/src/components/ConnectProviderDialog.tsx) | 618 | already decomposed into local subcomponents: `SectionHeader` 65–99, `ProviderCard` 101–240, `CardGrid` 242–276, `ProviderListContent` 279–568, dialog shell 570–616 | Boundary is clean: move `ProviderListContent` + the three subcomponents to `components/connect/` (it is also imported standalone for onboarding — verified via `ConnectAIDialogs`/`inline` prop); dedupe region groups (§2.7); rename local `CardGrid` (§5). Risk: low. |
| [UnifiedRoutingGraph.tsx](../frontend/src/components/UnifiedRoutingGraph.tsx) | 605 | styles/constants 33–67 · `StyledCard`/`GraphContainer`/`GraphRow` 118–162 · mode resolution + responses toggle 244–292 · `renderTierLayout` 310–368 · `renderSmartRules` 371–462 · `renderDefaultProviders` 465–483 · JSX 485–603 | Cohesive orchestrator that already delegates nodes/guides to `components/nodes` and `components/tier`. Only worthwhile cut: promote `renderTierLayout`/`renderSmartRules` closures to subcomponents to shrink the 20-prop component. Low priority. Risk: low. |
| [SystemLogViewer.tsx](../frontend/src/components/SystemLogViewer.tsx) | 472 | toolbar 183–293 · level filter/sort state 43–178 · table + expandable rows 294–468 | Shared log-table shell (§2.3); also fix Fragment-key bug (§5/§6 notes). Risk: medium. |
| [ModelRequestHeader.tsx](../frontend/src/components/ModelRequestHeader.tsx) | 447 | styled defs 27–78 · inline title edit 171–214 · toggles 284–364 · actions 369–401 · dead menu 421–442 | Shrink via dead-menu removal (§1); optional: extract `renderTitle`'s edit-mode block to `InlineEditableName`. Otherwise fine. Risk: low. |
| [AILogViewer.tsx](../frontend/src/components/AILogViewer.tsx) | 447 | same shape as SystemLogViewer + detail fetch cache 172–188 | Shared log-table shell (§2.3). Risk: medium. |
| [RuleCard.tsx](../frontend/src/components/RuleCard.tsx) | 405 | already fully decomposed into `rule-card/useRuleCardHooks`, `rule-card/dialogs`, `FlagCatalogDialog`, `SmartRuleCatalogDialog` | No action — this is what the others should look like. |

---

## 4. CONVENTION VIOLATIONS

| Rule | Result | Detail |
|---|---|---|
| No direct `@mui/icons-material` imports | **Clean** | grep over all top-level component files: zero hits; all icons come from `@/components/icons` / `BrandIcons` / `ProviderIcon`. |
| Copy feedback via `hooks/useCopyFeedback` | **1 violation** | [ApiKeyTable.tsx:610–618](../frontend/src/components/ApiKeyTable.tsx#L610) — token modal copies via raw `navigator.clipboard.writeText` with no feedback state and no `CopyIconButton`, even though `CopyIconButton` (which wraps `useCopyFeedback`) is used in OAuthDetailDialog:238/271, UpdatePanelDialog, PageLayout, ShortcutCard. Other copy sites (CodeBlock:60, OAuthDialog:310 copyUserCode — no feedback either but contextually OK) fine. |
| No local Snackbar | **Clean** | zero `<Snackbar>` in top-level components; `useNotify` used where toasts are needed (ProvidersCard, ShortcutCard). Inline `<Alert>` inside dialogs is in-form messaging, not a toast — not a violation. |

Also: [ProviderFormDialog.tsx](../frontend/src/components/ProviderFormDialog.tsx) contains the only `console.log` statements in top-level components (8 occurrences, lines 181–277, verbose debug logging of dialog init) — remove or gate behind `import.meta.env.DEV`.

Verified bug (not just style): [SystemLogViewer.tsx:362–363](../frontend/src/components/SystemLogViewer.tsx#L362) — `logs.map(... => (<>
<TableRow key={index}>…` — the key is on the inner TableRow, not on the list-item Fragment, so React will warn and reconciliation of expanded rows is index-fragile. AILogViewer does it correctly (`<Fragment key={req.request_id}>`, line 347).

---

## 5. CROSS-DIRECTORY FINDINGS (flagged, not deep-dived)

| Finding | Location | Note |
|---|---|---|
| Local `CardGrid` name collision | [ConnectProviderDialog.tsx:242](../frontend/src/components/ConnectProviderDialog.tsx#L242) defines a local `CardGrid` (plain CSS grid) shadowing the unrelated [components/CardGrid.tsx](../frontend/src/components/CardGrid.tsx) (MUI Grid + virtualization, used by HelpPage, System, DevelopPage, ExperimentalPage) | Rename the local one (`PickerCardGrid`) to avoid confusion. |
| Third copy of the fixed-layout table scaffolding | `components/bot/BotTable.tsx` comments (lines 26, 48–53, 165) explicitly say it "mirrors ApiKeyTable" | If the §2.1 shared table shell is built, evaluate adopting it in BotTable. |
| `maskToken` again outside components/ | [pages/system/AccessControl.tsx:251](../frontend/src/pages/system/AccessControl.tsx#L251) | Fold into the tokenUtils consolidation (§2.4). |
| OAuthDialog re-implements `provider-form-dialog/ProxyUrlField` | §2.6 | Prefer the shared field. |
| Eager-bundle import across component files | ConnectProviderDialog imports `FALLBACK_OAUTH_PROVIDERS` from OAuthDialog (§3 row 1) | Same hazard class as the frontend CLAUDE.md lazy-loading rule; extracting the catalog fixes it. |

---

## Suggested staging order

1. **Zero-risk sweeps** (§1, §4): delete dead ModelRequestHeader menu, empty Advanced accordion, unused exports/imports, ProviderFormDialog console.logs; fix SystemLogViewer Fragment key. Risk: low.
2. **ConfirmDialog + small util consolidation** (§2.2, §2.4, §2.5, §2.7): swap 3 hand-rolled Modals, one `maskToken`/`formatDateTime` util, one region-group helper. Risk: low.
3. **OAuthDialog split** (§3): extract provider catalog (fixes bundle import), `OAuthAuthorizationDialog`, polling hook. Risk: medium.
4. **Table shell** (§2.1/§2.3): shared provider-table scaffolding + log-table shell; optionally adopt in BotTable. Risk: medium.
5. **Proxy/global-proxy unification** (§2.6): `useGlobalProxy` hook + ProxyUrlField reuse. Risk: medium (behavioral, persisted localStorage keys).

---

<a id="appendix-scenario"></a>

# Appendix: scenario — pages/scenario/

Scope: all of `pages/scenario/` (~17,040 lines, 60 files). Every claim below was grep-verified on branch `refactor/0922`.

## Family sizes (wc -l)

| Block | Files | Lines |
|---|---|---|
| Config modals (9) | ClaudeCodeConfigModal 776, CodexConfigModal 785, ClaudeDesktopConfigModal 377, DshConfigModal 342, OpenCodeConfigModal 175, CursorConfigModal 125, XcodeConfigModal 108, VSCodeConfigModal 78, PiConfigModal 65 | ~2,831 |
| QuickConfig panels (3) | ClaudeCodeQuickConfig 1,268, CodexQuickConfig 387, DshQuickConfig 295 | ~1,950 |
| Use\*Page family (17) | UseClaudeCodePage 360 … UseAnthropicPage 54, + ClaudeCodeProfilePage 470, AgentOverviewPage 198 | ~2,480 |
| ImageGen area | ImageGenPlaygroundCard 2,501, ImageSliceDialog 1,428, SketchCanvasDialog 962, gallery 471, + chrome/types/session/tests | ~5,900 |

---

## 1. Config-modal family — heaviest duplication in the tree

### 1a. The shared skeleton

Every "big" modal (Codex, Dsh, ClaudeCode, OpenCode) repeats the same shell, character-for-character in several spots:

- **Dialog shell**: `maxWidth="lg" fullWidth`, `slotProps.paper.sx {borderRadius: 3, maxHeight: '90vh'}`, a close-guard `onClose` (CodexConfigModal.tsx:278-296, DshConfigModal.tsx:145-162, ClaudeCodeConfigModal.tsx:361-372, OpenCodeConfigModal.tsx:41-59).
- **DialogTitle**: h6 title + `text.secondary` subtitle + (reset IconButton) + Quick/Manual `Tabs` with the identical `sx={{ mt: 1, minHeight: 40, '& .MuiTabs-indicator': { height: 3 } }}` (Codex:297-332, Dsh:164-190, ClaudeCode:373-404).
- **Quick-tab spinner**: `{isConfigLoading && <Box sx={{display:'flex',justifyContent:'center',py:8}}><CircularProgress size={28}/></Box>}` — verbatim in Codex:443-447, Dsh:192-196, ClaudeCode:457-461.
- **DialogActions**: Close (outlined/inherit) + Apply (contained, `startIcon={isApplying ? <CircularProgress size={16} …/>}`, label `common.applying`/`scenarioPage.autoConfig`) — Codex:691-705, Dsh:323-337, OpenCode:155-170, ClaudeCode:716-740.
- **Hydration-on-open effect**: reset-on-close → `api.getApplied*Config()` → `mergeSaved*Prefs()` → `isConfigLoading` flag (Codex:98-125, Dsh:51-72, ClaudeCode:145-205). Same shape three times.
- **Debounced server preview**: 250 ms `setTimeout`, `cancelled` flag, `api.get*ConfigPreview(prefs…)`, placeholder fallback to `token` (Codex:156-180, Dsh:77-94). Same shape twice.

### 1b. The "Step N · Create or update \<file\>" manual sections — the big win

Nine near-identical JSX blocks: a heading, a platform `Tabs` (`JSON/TOML/YAML | Windows | Linux/macOS`, same `minHeight: 32` styles), and 2-3 `<CodeBlock>`s with per-tab `language`/`filename`/`maxHeight`:

| File | Sections | Lines each |
|---|---|---|
| ClaudeCodeConfigModal | settings.json (477-530), .claude.json (533-586), statusline (589-688) | ~55-100 |
| CodexConfigModal | config.toml (460-513), auth.json (522-582), catalog (586-647) | ~55-62 |
| DshConfigModal | settings.yaml (204-255), .credentials.yaml (257-313) | ~52-57 |
| OpenCodeConfigModal | opencode.json (91-152) | ~62 |

≈ **550 lines** that reduce to a single `ManualFileSection` component:
`<ManualFileSection step={1} title="~/.codex/config.toml" tabs={[{label:'TOML', language:'toml', code: configToml, filename:'…'}, {label:'Windows', language:'js', code: winScript, …}]}/>`.

The accompanying **script templates** are also copy-paste: the PowerShell here-doc / bash here-doc pairs (Codex:182-225 — six template strings; Dsh:96-126 — four) differ only in target path and payload. One `writeFileScripts(targetPath, content)` helper returning `{windows, unix}` removes them all. ClaudeCode's `generateNodeScript` (84-112) is a different but parallel mechanism, plus its statusline installers duplicate the node-download boilerplate twice (280-343).

### 1c. QuickConfig field rows — 3-way duplication

CodexQuickConfig `FieldRow` (203-273), DshQuickConfig inline rows (202-238, 248-288) and ClaudeCodeQuickConfig `FieldRow` (924-1020) are the same three-column row: label + info Tooltip (same `maxWidth: 280` two-caption rich tooltip), monospace key badge (same `px:0.75 py:0.25 borderRadius:0.75 bgcolor:'action.hover' fontSize:'0.72rem'` styles — literal `flex:'0 0 180px'`/`'0 0 320px'` duplicated 8 times across Codex/Dsh, and ClaudeCode already extracted its version as `CLAUDE_CONFIG_ROW_COLUMNS`/`CLAUDE_CONFIG_KEY_SX` which ClaudeCodeProfileOverrides reuses). Also each file defines its own `useLang()` hook (CodexQuickConfig.tsx:188, DshQuickConfig.tsx:155, ClaudeCodeQuickConfig.tsx:805) and its own `PREF_KEYS` + `mergeSaved*Prefs` with identical merge loops (Codex:51-60, Dsh:47-56). One shared `PrefsFieldRow`, `usePrefsLanguage()`, and a generic `mergeSavedPrefs<T>(keys, defaults, applied)` collapse ~400 lines.

### 1d. Instruction-block modals (Xcode, Cursor, ClaudeDesktop, Pi, VSCode)

- **Xcode vs Cursor**: after s/Xcode/Cursor/ the two files are **0 diff lines** (verified: `diff <(sed s/…/…/)` → 0). Both render numbered-step instruction boxes + `token.slice(0, 16)}...` + a Copy URL / Copy API Key `Stack` pair. ClaudeDesktopConfigModal:196-213 repeats the same Copy URL / Copy API Key pair a third time.
- **Pi vs VSCode**: same "link-button info dialog" shape, differing only in i18n keys and one extra button.
- **API-key truncation** `token.slice(0, 16)` appears at XcodeConfigModal.tsx:59, CursorConfigModal.tsx:88, ClaudeDesktopConfigModal.tsx:190.

Feasibility: **high**. Proposed ladder:
1. `CopyUrlKeyButtons({baseUrlPath, token, onCopy})` — trivial, zero behavior risk.
2. `InstructionSteps` / `StepBox` for the numbered-step boxes (Xcode 44-65, Cursor 73-94, ClaudeDesktop 151-193).
3. `ConfigModalShell` (Dialog + title/subtitle/reset/tabs + actions + close-guard).
4. `ManualFileSection` + `writeFileScripts`.
5. `useAppliedPrefs` hydration hook + `useDebouncedPreview`.
6. Optionally a data-driven `ConfigModal` that composes QuickConfig + Manual sections, keeping Codex's auth-mode radio and ClaudeCode's apply-result alert as `children`/slots.

Per-tool genuinely-unique parts that must stay: Codex auth-mode radio + OAuth provider picker (CodexConfigModal:338-442), ClaudeCode apply-result alert with created/updated/backup lists (411-455) and MODAL_TEXT bundle (44-81), ClaudeDesktop model add/delete editor (58-126), Cursor unreachable-URL warning (16-26, 60-72), Dsh reset IconButton (171-181).

### 1e. Inconsistencies found while diffing (fix during extraction)

- Close-guard drift: `shouldIgnoreDialogClose` (@/components/dialogClose) only blocks `backdropClick` (dialogClose.ts:3-5), but ClaudeCodeConfigModal:364 and OpenCodeConfigModal:44 inline-check `backdropClick || escapeKeyDown` — **Escape closes those two modals but not Codex/Dsh**. Probably unintended; pick one behavior.
- ClaudeCodeConfigModal statusline installers download from a **placeholder URL** `https://github.com/your-repo/tingly-statusline/...` (lines 281, 314) while the JSON tab links the real repo (627, 656). Copy-paste relic; likely broken for users.
- i18n: Xcode/Cursor/ClaudeDesktop modals are hardcoded English; Pi/VSCode/Codex/Dsh use `t()`.
- Apply-error i18n key drift: UseDshPage.tsx:59 uses `t('dshConfig.applyFailed')` while its modal uses the same key but the page otherwise lives under `scenarioPage.dsh.*`.
- ClaudeDesktopConfigModal.tsx:22 `import api from '@/services/api'` (default) vs named `import { api }` everywhere else.

---

## 2. Use\*Page family — boilerplate confirmed, extraction is cheap

All 16 pages follow one skeleton: `ScenarioPageModalProvider` → `useScenarioPageInternal(scenario)` → `PageLayout(loading, ScenarioPageSkeleton, notification)` → `CardGrid` → `UnifiedCard(title + Tooltip InfoIcon + rightAction Button)` → `ProviderConfigCard` → `TemplatePage` → one config modal → optional `ConnectAIDialogs`.

Verified duplication:
- **UseOpenAIPage vs UseAnthropicPage: 0 diff lines** after name substitution. UseCustomPage and UseEmbedPage differ only by `compact` / a `title` prop. These four are the same component.
- **UseCursorPage vs UseXcodePage: 0 diff lines** after name substitution.
- The `UnifiedCard` title block (`Box flex` + `<span>{name}</span>` + `Tooltip`+`IconButton`+`InfoIcon`) is repeated in **all 14 tool pages** (e.g. UseCodexPage.tsx:72-82, UseCursorPage.tsx:32-40, UsePiPage.tsx:43-50 …).
- The `connectAI = useProviderDialog(showNotification, {onProviderAdded: () => window.location.reload()})` + `<ConnectAIDialogs flow={connectAI}/>` pair is repeated in 7 pages (Codex 33-36/132, VSCode 32-34/103, Pi 31-33/98, OpenCode 32-34/151, Dsh 37-39/143, ClaudeCode 65-67/345).

What genuinely varies (and should be config, not code):
| knob | values |
|---|---|
| scenario id, title, baseUrlPath, tooltip key | per tool |
| header rightAction | plain Config button / AutoConfig / Dsh's two buttons / ClaudeCode's mode ToggleButtonGroup |
| AgentSetupCard | present on 7 pages with per-tool installCommand/installActions/onApply |
| apply handler | per-tool `api.apply*Config` + file-list extraction (Codex 38-62, Dsh 43-63, OpenCode 70-86) |
| context-1M plumbing | `pendingContext1MChange` state + `handleContext1MToggle` duplicated in Codex (36-67), ClaudeDesktop (27-35), ClaudeCode (68-75) |

**Recommendation**: a `ScenarioPage` component driven by a per-tool descriptor (extend `ScenarioDescriptor` in scenarioRegistry.tsx with `providerCard`, `setupCard`, `modal` fields). Registry already exists (scenarioRegistry.tsx:36-149) and is deliberately page-free — extend it, don't duplicate it. ClaudeCode (unified/separate mode machine, lines 78-216) and ImageGen/Team stay bespoke. Note ClaudeCodeProfilePage already demonstrates the "page as composition" pattern.

**Dead fetch found**: UseClaudeCodePage.tsx:49-54 destructures only `{showNotification, notification, copyToClipboard, baseUrl}` from `useScenarioPageInternal`, so the hook still fires `loadRules('claude_code')` (useScenarioPageInternal.ts:68-71) whose result is thrown away — the page keeps its own rules state (57-173). One wasted API call per mount + two parallel rule states conceptually.

---

## 3. ImageGen area — the 2,501-line card has clean seams

Already extracted and healthy: `ImageGenPlayground.types.ts` (shapes), `imageGenSession.ts` (pure logic, tested in imageGenSession.test.ts), `ImageGenPlayground.chrome.ts` (shared styling), session persistence via `utils/playgroundSession`.

Internal map of ImageGenPlaygroundCard.tsx and proposed cuts:

| Lines | Content | Extract to |
|---|---|---|
| 67-135 | constants + `readImageSize`/`fileToDataUrl` helpers | `imageGenConstants.ts` / utils |
| 140-227 | `ImportedImageCard` | own file |
| 228-312 | `RunSourceStrip` | own file |
| 313-337 | `PanelAction` | own file (or chrome.ts) |
| 338-473 | `ReferenceThumb` (thumb + drag handle + drop target + keyboard reorder) | `ReferenceImagesRow.tsx` together with the row handlers |
| 480-519 | **19 `useState` hooks** in the single component | split into `useImageGenRefs` (references/dnd), `useImageGenClipboard` (paste/drop/import 598-812), `useImageGenRuns` (runGeneration/retry/cancel/reuse 936-1133), `useImageGenLightbox` (1216-1260 memos + film) |
| 1275-2118 | render: request panel (prompt editor at 1475-1652) + results strip (1653-2118) incl. `renderRunActions` (1150) | `GenerationRunCard.tsx` |
| 2119-2391 | **Lightbox dialog (~270 lines, inline)** | `ImageGenLightbox.tsx` — cleanest single cut; it consumes only `lightboxFilm`/`showLightboxFrame` |
| 2392-2501 | mounted dialogs (gallery, ConfirmDialog, slice, prompt editor 2436-2488, sketch) | stays, or a `PlaygroundDialogs` wrapper |

Risk notes: the lightbox and the gallery dialog intentionally share `fullBleedDialogPaperSx` and `runImage()` — keep that contract. ImageSliceDialog (1,428) and SketchCanvasDialog (962) are self-contained (own state, only consume `SketchResult`/props; ViewAnglePopover/PoseLibraryPopover used **only** by SketchCanvasDialog) — they can be moved to a subfolder but don't need internal splitting; slice's crop-drag math (244-262) is pure and could gain tests if touched.

---

## 4. Dead / dead-ish code (grep-verified)

1. **CodexConfigModal session-import block** — `SHOW_CODEX_SESSION_IMPORT = false` (line 40) gates: the optional Step-3 JSX (649-687), the `handleSessionAction` flow (227-250), the nested confirm Dialog (706-780), plus state `sessionAction/isSubmitting/result/error/createBackup/autoUndoOnStop` (69-74). ~**200 lines** of permanently-hidden UI. Delete or move behind a real flag.
2. **Unused imports**: UseCodexPage.tsx:10-11 — `Dialog, DialogActions, DialogContent, DialogTitle, Typography, Alert` (MUI) and `RestartIcon` never referenced; UseClaudeDesktopPage.tsx:4-5 — same set (`Dialog…Typography, Alert`, `RestartIcon`) never referenced. Leftovers from an in-page restart dialog that moved into ClaudeDesktopConfigModal.
3. **`PROTOCOL_VALUES` export** (DshQuickConfig.tsx:28) — never imported anywhere; used only in-file. Drop the `export` or share it with the backend-mirror comment.
4. **`useRuleManagement` returned setters** `setRules`, `setNewlyCreatedRuleUuids` (useRuleManagement.ts:47-52) — no consumer outside the hook/hook-reexport. Narrow the return type.
5. **ClaudeCodeConfigModal statusline scripts with `your-repo` placeholder** (280-343) — shipped UI running a URL that can't work; dead-as-written (see 1e).
6. Duplicated constants that should have one home: `PI_REPO_URL` (UsePiPage.tsx:18 + PiConfigModal.tsx:10), `MARKETPLACE_URL`/`VSCODE_INSTALL_URL` (UseVSCodePage.tsx:18-19 + VSCodeConfigModal.tsx:10-11).

Not dead (checked): `ViewAnglePopover`, `PoseLibraryPopover` (SketchCanvasDialog), `TitleIconButtons`/`TemplatePageActions` (TemplatePage), `TeamKeyScopeAlert` (SharingKeysDialog), `Context1MChangeBanner` (3 modals), `claudeCodePrefsState` (ClaudeCodeConfigModal), all QuickConfig exports used by ClaudeCodeProfileOverrides.

---

## 5. Convention violations

- **`@mui/icons-material` direct imports: none** in pages/scenario (grep clean). ✔
- **Local Snackbar: none**; SharingKeysDialog/UseTeamPage use `useNotify`. ✔
- **Local copy-feedback: none** — `useCopyFeedback` used where needed (ImageGenPlaygroundCard, AgentSetupCard, SharingKeysDialog, UseTeamPage). ✔
- **Code-splitting**: no module outside `pages/scenario/` imports a page file. The registry extraction (scenarioRegistry.tsx) is done correctly. Two layering smells remain:
  - `src/components/ProviderConfigCard.tsx:17` and `src/components/CompactConfigCard.tsx:7` import `useScenarioPageModal` from `@/pages/scenario/context/…` — shared components depending on a `pages/` module (inverted layering; page-free today, but one careless `import` of a page into that context breaks lazy-loading per frontend/CLAUDE.md). Move `ScenarioPageContext` to `src/context/` or `src/components/scenario/`.
  - `src/pages/HelpPage.tsx:11` imports `TeamGuideDialog` from `pages/scenario/components/` — cross-page component import; moves TeamGuideDialog into HelpPage's chunk. Minor.

---

## Recommended refactor order (value ÷ risk)

| # | Block | Est. reduction | Risk | Why this order |
|---|---|---|---|---|
| 1 | **Delete dead code** (§4.1-4.2, §4.5): Codex session-import block, unused imports, placeholder statusline scripts | ~250 lines | Very low | Pure deletions; also removes a broken user-facing script |
| 2 | **ManualFileSection + writeFileScripts** across Codex/Dsh/ClaudeCode/OpenCode | ~500→~150 | Low | Mechanical, visually identical, single biggest line win in modals; fix close-guard drift + i18n in the same PR |
| 3 | **Use\*Page consolidation**: `ScenarioPageHeaderCard` (title+tooltip+rightAction) first, then merge the four identical SDK pages and Cursor/Xcode into one parameterized page fed by the registry | ~900→~250 | Low | 0-diff merges verified; registry pattern already exists |
| 4 | **CopyUrlKeyButtons + InstructionSteps** for Xcode/Cursor/ClaudeDesktop (and Pi/VSCode shells) | ~400 | Low | Small files, easy snapshot review |
| 5 | **ConfigModalShell + useAppliedPrefs + useDebouncedPreview** | ~350 | Medium | Touches hydration order — needs manual test of reopen-restores-prefs in all three big modals |
| 6 | **QuickConfig shared row/lang/merge** | ~400 | Medium | Cross-file style unification (grid vs flex columns) — verify row alignment against screenshots |
| 7 | **ImageGenPlaygroundCard split** (lightbox first, then run card, then hooks) | 2,501 → ~5 files ≤600 each | Medium-high | Pure moves, but the 19-state tangle means regression risk in drag/paste/preview paths; do last, one extraction per PR, keeping imageGenSession tests green |
| 8 | Move `ScenarioPageContext` out of `pages/`; decide TeamGuideDialog home | — | Low | Layering hygiene; bundle-affecting so verify `dist/index.html` modulepreload per frontend/CLAUDE.md |

Total realistic reduction: **~2,500-3,000 lines** plus a component vocabulary (`ConfigModalShell`, `ManualFileSection`, `CopyUrlKeyButtons`, `ScenarioPage`) that makes the next per-tool config modal a ~50-line descriptor instead of a 300-line copy.

---

<a id="appendix-guardrails"></a>

# Appendix: guardrails — Guardrails (pages + rule-card / nodes / flags / tier)

Scope reviewed: `frontend/src/pages/GuardrailsPage.tsx`, `frontend/src/pages/guardrails/*`, `frontend/src/components/rule-card/*`, `frontend/src/components/nodes/*`, `frontend/src/components/flags/*`, `frontend/src/components/tier/*`. Every claim below was grep-verified across `frontend/src`. Line numbers are current as of branch `refactor/0922`.

---

## 1. Oversized files

### 1.1 `pages/guardrails/RulesPage.tsx` — 2,756 lines (largest file in the app)

The entire page is **one component**: `GuardrailsRulesPage` spans lines 246–2756 (~2,510 lines). Internal map:

| Lines | Section |
|---|---|
| 62–157 | Types + constants: `PolicyGroup`, `GuardrailsPolicy`, `DisplayPolicy`, `RegistryPolicyEntry`, `EditorState`, `EditorListField`, `OversizedListField`, `PreparedEditorState`; `MAX_SUMMARY_VALUES/CHARS`, `MAX_INLINE_LIST_ITEMS/CHARS`, `OVERSIZED_LIST_PREVIEW_ITEMS`, `resourceAccessActionOptions`, `commandExecutionActionOptions`, `DEFAULT_GROUP_ID`, `PENDING_REGISTRY_INSTALLS_STORAGE_KEY` |
| 197–244 | localStorage helpers: `read/writePendingRegistryInstallIds`, `add/removePendingRegistryInstallId` |
| 247–296 | **28 `useState` hooks** |
| 298–427 | Pure helpers re-created inside the component every render: `splitLines`, `textListRows`, `joinLines`, `isOversizedList`, `prepareListField`, `summarizeValues`, `effectiveListValues`, `normalizeGroup`, `normalizePolicyGroups`, `ensureDefaultGroupMembership`, `toggleValue`, `update/append/removeTextListValue` |
| 428–531 | `buildBuiltinPayload` + **13 `useMemo`** (displayPolicies, per-tab policy lists, labels…) |
| 532–704 | More logic closures: `getEffectivePolicyState`, `policyNeedsEnableWithDefault/Disable`, `generatePolicyId`, `applyKindDefaults`, `buildSuggestedReason`, `buildPolicySummary`, `buildPolicyScope`, `sanitizeVerdictForKind`, `blurActiveElement` |
| 705–779 | `makeEditorState`, `makeEditorStateFromDraft` |
| 780–925 | `loadPolicies`, `loadRegistry` + **7 `useEffect`** (initial loads, localStorage sync, default-group bootstrap, `?policyId=` deep link, `newPolicyDraft` location-state handoff) |
| 927–1314 | 12 handlers (`openPolicyEditor`, `handleNewPolicy`, `buildPolicyPayload`, `handleSavePolicy`, `handleDuplicatePolicy`, `handleTogglePolicy`, `handleSetPoliciesEnabled` (~86 lines), `handleDeletePolicy`, `handleInstallRegistryPolicy`, `handleCloseEditor`, `handleConfirmClose`) |
| 1316–1501 | `renderPolicySection` (~186 lines) — policy list rows |
| 1503–1648 | `renderCompactListEditor` (~145 lines) — inline list editor used 5× in the dialog |
| 1649–1789 | `getScenarioPresentation` + `renderScenarioScopeSelector` (~87 lines) |
| 1790–1984 | Page JSX: status card, policies card with 3 tabs, Download Management / registry card (1884–1983) |
| 1985–2710 | **Editor Dialog (~725 lines)** incl. three near-identical "kind cards" (2008–2126) and the Advanced panel |
| 2711–2746 | Confirm-close dialog + delete-confirm dialog |

#### Recommended split plan for RulesPage.tsx

Create `frontend/src/pages/guardrails/rules/` (only the page imports these, so the `frontend/CLAUDE.md` "no nav-level exports from page files" rule is respected):

| New file | Moves from RulesPage.tsx | Approx. lines |
|---|---|---|
| `types.ts` | L62–157 all types + option constants + `DEFAULT_GROUP_ID` | ~95 |
| `pendingInstalls.ts` | L197–244 localStorage helpers (pure, unit-testable) | ~50 |
| `listField.ts` | L123–130 `EditorListField`/`OversizedListField` types + `splitLines/textListRows/joinLines/isOversizedList/prepareListField/summarizeValues` (currently re-created per render; module-level pure functions) | ~120 |
| `editorState.ts` | `applyKindDefaults` (581), `buildSuggestedReason` (608), `generatePolicyId` → generic `uniqueIdFromName(name, takenIds, fallback)` (553), `buildPolicyPayload` (966), `buildBuiltinPayload` (428), `sanitizeVerdictForKind` (689) | ~250 |
| `policyPresentation.ts` | `getEffectivePolicyState` (532), `policyNeeds*` (540/548), `buildPolicySummary` (651), `buildPolicyScope` (684) — shared with GroupsPage (see §2.1) | ~90 |
| `useGuardrailsConfig.ts` | `loadPolicies` (780), `loadRegistry` (819), the default-group bootstrap effect (851–881) — **shared hook with GroupsPage** | ~120 |
| `usePolicyEditor.ts` | editor `editorState`/`editorSnapshot`/`oversizedListFields`/dirty/confirm-close state machine; `makeEditorState` (705), `openPolicyEditor`, `handleCloseEditor`, `handleConfirmClose` | ~250 |
| `PolicyListSection.tsx` | `renderPolicySection` (1316–1501) as a real component (memoizable) | ~190 |
| `CompactListEditor.tsx` | `renderCompactListEditor` (1503–1648) as a real component | ~150 |
| `ScenarioScopeSelector.tsx` | `renderScenarioScopeSelector` (1702–1789) + `getScenarioPresentation` (1649) | ~95 |
| `PolicyKindCards.tsx` | the three duplicated kind-selection cards (2008–2126) → one card + `.map()` over kind metadata | ~120 (down from ~370) |
| `PolicyEditorDialog.tsx` | dialog L1985–2710, composed from the above | ~350 |
| `RegistryCard.tsx` | Download Management card L1884–1983 | ~100 |
| `RulesPage.tsx` (remainder) | state wiring + page layout | **~350–450** |

Also extract `PolicyConfirmDialog` (the confirm-close + delete dialogs, 2711–2746) — GroupsPage/CredentialsPage have the same confirm-dialog shape.

### 1.2 Other files > 500 lines

- `pages/guardrails/CredentialsPage.tsx` (818): two inline dialogs (credential editor ~line 600s, provider import ~line 640s). Split `CredentialEditorDialog.tsx` + `ImportProvidersDialog.tsx` → page ~350. **Risk: low.**
- `pages/guardrails/GroupsPage.tsx` (691): inline group dialog; share `useGuardrailsConfig` + confirm dialog. **Risk: low.**
- `components/rule-card/SmartRuleCatalogDialog.tsx` (658) and `FlagCatalogDialog.tsx` (555): both carry parallel `CATEGORY_META` / `CATEGORY_ORDER` / `categoryMeta` scaffolding (SmartRuleCatalogDialog:126–141, FlagCatalogDialog:69–83). Extract a shared `useCategoryGrouping(specs, order, meta)` util. **Risk: medium** (behavior-rich dialogs, no tests).
- `pages/GuardrailsPage.tsx` (558): split Import/Export dialogs into components → page ~300. **Risk: low.**
- `components/tier/diagrams.ts` (716): pure guide data; fine as-is.

---

## 2. Duplication (verified)

### 2.1 Across guardrails pages

- **`blurActiveElement` — 3 identical copies**: `RulesPage.tsx:698`, `GuardrailsPage.tsx:122`, `CredentialsPage.tsx:99`. Extract to `utils/dom.ts`. **Risk: low.**
- **Default-group bootstrap effect — near verbatim**: `RulesPage.tsx:851–881` vs `GroupsPage.tsx:177–208` (same `api.createGuardrailsGroup({ id: 'default', name: 'Default', enabled: true, severity: 'high' })`, same guard conditions, only toast text differs). Extract into the shared `useGuardrailsConfig` hook. **Risk: low.**
- **Slug-unique-id loop duplicated**: `generatePolicyId` (`RulesPage.tsx:553–580`) vs `generateGroupId` (`GroupsPage.tsx:137–154`) — identical normalize → dedupe-with-suffix loop. One shared helper. **Risk: low.**
- **Types duplicated**: `PolicyGroup` (`RulesPage.tsx:62` vs `GroupsPage.tsx:32`) and `GuardrailsPolicy` (`RulesPage.tsx:69` vs `GroupsPage.tsx:39`). Also `DEFAULT_GROUP_ID` at `RulesPage.tsx:194` and `GroupsPage.tsx:30`. Move to `pages/guardrails/types.ts`. **Risk: low.**
- **`buildPolicySummary` variants**: `RulesPage.tsx:651` (rich, uses `summarizeValues`) vs `GroupsPage.tsx:110` (cruder join). Same underlying `policy.match` shape; unify on the RulesPage version. **Risk: low.**
- **Local `actionMessage` alert pattern in all 5 pages** (see §4). Also `HistoryPage.tsx:82 compactList` and `:88 formatTimestamp` duplicate ideas found in `RulesPage` `summarizeValues` (346) and `components/SystemLogViewer.tsx:134 formatTimestamp`. **Risk: low.**

### 2.2 Within `components/rule-card`

- **Six near-identical export wrappers** in `utils.ts`: `exportRuleWithProviders` (367), `exportRuleAsBase64ToClipboard` (388), `exportRuleAsJsonlToClipboard` (404), `exportProvider` (426), `exportProviderAsBase64ToClipboard` (450), `exportProviderAsJsonlToClipboard` (467). All are `try { getContent; download/copy; notify success } catch { notify error }` around one of two content sources. Collapse to `exportContent({ source, dest: 'file' | 'clipboard', format })`. Two of the six are dead (§3). **Risk: low–medium.**
- **`copyToClipboard` hand-rolled fallback** (`utils.ts:593–611`, `document.execCommand('copy')` path) coexists with `hooks/useCopyFeedback.ts` and duplicates clipboard concerns (see §4). Note: `useCopyFeedback` has no non-secure-context fallback — either port the fallback into the hook once and delete the local copy, or accept `navigator.clipboard` only. **Risk: low.**
- **Catalog-dialog scaffolding**: `CATEGORY_META`/`CATEGORY_ORDER`/`categoryMeta` in `FlagCatalogDialog.tsx:69–83` vs `SmartRuleCatalogDialog.tsx:126–141`; both dialogs also re-implement search/scroll-to/jump-to-item plumbing. **Risk: medium.**
- **`FlagCatalogDialog.tsx:51–60` `flagToBool/flagToInt/flagToString/flagToServiceRef`** are thin typed wrappers over `flagHelpers.getFlagValue` — fold into `flagHelpers.ts` as generics. **Risk: low.**
- Cross-directory overlap is low: `components/flags/*` renders flag controls (used by `PluginFeatures.tsx`, and `HeadersEditor` also by `FlagCatalogDialog.tsx:38`); `components/nodes/*` renders graph nodes; `rule-card/utils.ts` is rule logic. No copy-pasted rendering found between them beyond the flag-key string constants in `RoutingGraphTypes`.

### 2.3 rule-card ↔ nodes

- `rule-card/timeRange.ts` is consumed by `nodes/SmartOpNode.tsx` and `SmartRuleCatalogDialog.tsx` — good shared module, no action.
- `rule-card/utils.ts:14 isWildcardModelName` is used by live `ModelRequestHeader.tsx` and by dead `ModelNode.tsx` (§3) — keep.

---

## 3. Dead code (grep-verified, zero references in `frontend/src`)

### 3.1 `components/nodes/` — 6 dead node files (~831 lines)

Exported by the barrel `components/nodes/index.tsx` but never imported anywhere:

| File | Lines | Evidence |
|---|---|---|
| `ModelNode.tsx` | 317 | no `ModelNode` import outside `components/nodes/`; `UnifiedRoutingGraph.tsx:12–21` imports the other 8 nodes but not this |
| `ConfigNode.tsx` (barrel name `CWDNode`) | 123 | `CWDNode` referenced nowhere outside the barrel |
| `RoutingModeNode.tsx` | 101 | no references |
| `PlatformNode.tsx` | 81 | no references |
| `AgentConfigNode.tsx` | 79 | no references |
| `DividerNode.tsx` | 78 | no references |
| `CrossNode.tsx` | 52 | no references |

Cascade: `StyledModelNode` (`nodes/styles.tsx:264`) is used **only** by dead `ModelNode.tsx` → also removable.

Live nodes for contrast (verified consumers): `ActionAddNode`, `ArrowNode`, `EntryNode`, `ServiceEntryNode`, `ServiceNode`, `SmartOpNode`, `TierNode` → `UnifiedRoutingGraph.tsx`; `ApiEntryNode`, `ChatNode`, `ImBotNode` → `notify/BotNotifyGroup.tsx:11`; `AtNode`, `AgentNode`, `BotModelNode`, `CCProfileNode`, `AccessNode` → `bot/RemoteControlGraph.tsx`; `NodeTooltip`/`ServiceNodeContent` used internally; `NodeContainer`/`graphRowStyles`/`getInactiveHatchSx` (styles.tsx) used by all three graphs. **Risk of removal: low** (but confirm no runtime string-based dynamic imports first — none found).

### 3.2 `components/rule-card/utils.ts` — 5 dead exports

Verified zero references outside `utils.ts` itself (including both test files):

- `serviceToConfigProvider` (L25)
- `normalizeSmartRoutingServices` (L72)
- `hasTierAssigned` (L115)
- `exportRuleWithProviders` (L367)
- `decodeBase64Export` (L571)

Also `RuleCard.tsx:111` calls `useRuleExport({ rule, showNotification })` and discards the result — the hook (and thus `exportRule*ToClipboard`) is wired but unused at that call site; verify before deleting the hook. **Risk: low.**

### 3.3 Likely-dead behaviors in RulesPage (verify at runtime / git history before removing)

- **`newPolicyDraft` location-state handoff** (`RulesPage.tsx:905–925` effect + `makeEditorStateFromDraft` at 748–779): `grep` finds **no producer** — nothing in `frontend/src` navigates to `/guardrails/rules` with `state.newPolicyDraft`. The comment at 1089 ("Duplicating now only creates a local draft") suggests this replaced a cross-page flow. ~65 lines.
- **`?policyId=` / `?ruleId=` deep-link effect** (`RulesPage.tsx:883–903`): no in-app link produces these params (could be for manual bookmarks; keep or confirm).

---

## 4. Convention violations

1. **`@mui/icons-material`: CLEAN.** No direct imports anywhere in scope; only the sanctioned type re-export inside `components/icons/` (`index.tsx:166`, `tablerMui.tsx:4`). All icon imports go through `@/components/icons`.
2. **Local Snackbar/Alert feedback instead of `hooks/useNotify`** — violates `frontend/CLAUDE.md`. The `{ type: 'success' | 'error'; text: string } | null` + `<Alert>` pattern is hand-rolled in **all 5 pages** (49+ set sites):
   - `RulesPage.tsx:251` (40+ `setActionMessage` calls, rendered inline at 1808, 1989)
   - `GuardrailsPage.tsx:55` (rendered at 247, 441, 489)
   - `CredentialsPage.tsx:82` and a second local channel `editorMessage` at `:95`
   - `GroupsPage.tsx:68` (16+ call sites)
   - `HistoryPage.tsx:124` (rendered at 204)
   
   Replace with `useNotify()` (global `NotificationProvider`); deletes ~5 state hooks and all inline `<Alert>` wiring. **Risk: low** (UX change: toast position/duration differs; do it as one mechanical sweep).
3. **Local copy feedback instead of `hooks/useCopyFeedback`**: `CredentialsPage.tsx:340–347` (`handleCopyAliasToken` → `navigator.clipboard.writeText` + success message state) and `rule-card/utils.ts:593–611` (`copyToClipboard` with execCommand fallback + notification callback). **Risk: low.**

---

## 5. State / performance (RulesPage)

- **Monolithic state = full-page re-render on every keystroke.** 28 `useState` in one component; the editor dialog is inline in the same render tree, so each keystroke in a dialog `TextField` re-runs all 13 `useMemo`s *and* re-renders the policy lists, registry card, and 3 tab panels. Biggest single win of the split: `PolicyEditorDialog` as a child component localizes editor state so typing no longer re-renders the page (and `React.memo` becomes possible for `PolicyListSection`/`RegistryCard`).
- **~30 closures re-created per render** (§1.1 lines 298–704). The pure ones (`listField.ts`, `editorState.ts`, `policyPresentation.ts`) should be module functions — removes both re-creation cost and the `useCallback` pressure after the split.
- **`renderX` functions cannot be memoized.** `renderPolicySection` (1316) maps over up to N policies and rebuilds row JSX on every render including `buildPolicySummary(policy)` per row (651, string work per row per keystroke). As a memoized component with `useMemo` per list this disappears.
- **Editor open-intent effects fight each other**: deep-link effect (883) depends on `[location.search, navigate, policies, scenarioOptions]` and the draft effect (905) on `[location.state, navigate, supportedScenarios]`; both call `navigate(..., { replace: true })` and set 6–8 states. Consolidate into a single "open intent" reducer inside `usePolicyEditor` to avoid double-open/close races.
- **Prop drilling after split**: keep it shallow — pass `editorState` + a `patch(partial)` dispatcher into `PolicyEditorDialog` rather than 15 individual setters; the per-row selection states (`selectedResourceRow/CommandTermRow/PatternRow`, 275–277) belong *inside* `PolicyEditorDialog`/`CompactListEditor`, not in the page.
- `handleSetPoliciesEnabled` (1147–1232) loops `api.updateGuardrailsPolicy` per policy; fine for current scale but wrap in `Promise.allSettled`-style batching if split touches it.

---

## Suggested execution order

1. **Dead code sweep** (§3): delete 6 dead node files + `StyledModelNode`, 5 dead `utils.ts` exports (~900 lines, near-zero risk). Verify `newPolicyDraft`/deep-link behaviors at runtime first.
2. **Convention sweep** (§4): `useNotify` across 5 pages, `useCopyFeedback` in CredentialsPage, shared `blurActiveElement`.
3. **Shared page modules** (§2.1): `pages/guardrails/types.ts`, `useGuardrailsConfig.ts`, `uniqueIdFromName`, confirm-dialog component — shrinks Groups/Rules pages together.
4. **RulesPage split** (§1.1 plan) — do after 3 so `usePolicyEditor` lands on the shared foundation.
5. **Catalog dialogs** (§2.2): shared category grouping + collapse export wrappers.

---

<a id="appendix-remaining-pages"></a>

# Appendix: remaining-pages — Remaining Pages & Component Subdirs (+ App / vite configs)

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
