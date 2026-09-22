# Refactor Candidates — `frontend/src/pages/scenario/` (2026-09)

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
