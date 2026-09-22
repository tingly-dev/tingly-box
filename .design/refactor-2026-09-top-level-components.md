# Refactoring Candidates — frontend/src/components (top-level files only)

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
