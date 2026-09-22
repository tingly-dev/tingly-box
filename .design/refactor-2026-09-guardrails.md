# Guardrails Frontend — Refactoring Candidates (2026-09)

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
