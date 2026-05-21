# Frontend UI Optimization Plan

This document tracks the planned UI improvement work for the Tingly-Box frontend. The goal is to make the product UI clearer, more consistent, easier to scan, and easier to extend without fighting one-off styling decisions.

## Goals

- Improve readability across management pages by normalizing typography.
- Create a predictable page layout pattern for titles, filters, actions, cards, tables, and empty states.
- Reduce unnecessary card nesting, hover elevation, and decorative visual noise.
- Flatten ordinary management surfaces so structure comes from spacing, borders, typography, and state color rather than stacked shadows.
- Keep new frontend work aligned with the project guideline: use MUI components and MUI icons for new UI.
- Preserve the existing product structure while making high-traffic pages feel more polished.

## Non-Goals

- This plan does not introduce new backend APIs.
- This plan does not regenerate swagger client code.
- This plan does not redesign the entire product brand or replace the app navigation model.
- This plan does not change business logic or data contracts.

## Current Progress

Last updated: 2026-05-21

| Phase | Status | Notes |
| --- | --- | --- |
| Phase 1: Establish The UI Baseline | Complete | Typography scale, shared `PageHeader`, shared spacing direction, and default flat surface treatment are in place. Dashboard, Overview, Credentials, and Provider List use the shared header pattern. |
| Phase 2: Refactor Shared Components | In progress | `UnifiedCard` sizing and default hover elevation are refactored. `Surface` has been introduced as the non-card grouping primitive. Table density work has started with dashboard usage tables, credential tables, and Provider List. Agent overview, Connect AI, OAuth provider picker, Onboard, and model-select cards now follow the flat hover model. Scenario routing graph nodes use a localized, theme-aware hover elevation because dense graph editing needs clearer target separation than ordinary content cards. |
| Phase 3: Improve Dashboard | Mostly complete | Dashboard header, StatCard side-stripe removal, StatCard hierarchy, skeleton loading, Quick Start flattening, chart wrapper flattening, and flatter dashboard surfaces are complete. Three-column layout can still be tuned with real usage data. |
| Phase 4: Improve Overview And Heatmap | Mostly complete | Overview uses the shared page header. Heatmap container nesting, metric typography, and small-screen horizontal scrolling have been improved. |
| Phase 5: Polish Navigation And Icon Consistency | In progress | Side-stripe/glow selected states have been removed from the main sidebar, zen sidebar, and activity bar. ActivityBar label width and icon consistency still need a full pass. |
| Phase 6: Improve Login, Empty States, And Error States | Not started | Login, shared empty states, and auth/error dialog normalization remain future work. |

## Flattening Direction

The preferred product UI direction is flat, quiet, and operational:

- Ordinary content surfaces use `border: 1px solid divider`, `background.paper`, and `boxShadow: none`.
- Shadows are reserved for floating UI such as dialogs, menus, popovers, temporary overlays, or deliberate emphasis.
- Non-clickable cards and panels must not translate, lift, or intensify on hover.
- Selection state should use background, icon color, text weight, and contrast. Avoid side stripes, glow effects, and decorative edge indicators.
- Page grouping should prefer `Surface` or unframed layout sections over stacking `Paper` inside `Card` inside another `Paper`.
- Tables should be compact and scan-friendly: about 44-48px row height, compact headers, and right-aligned numeric columns.
- Metric card labels should prefer Title Case over all caps. Reserve uppercase for short table headers, tiny badges, or system codes.
- Metric cards need a stable title area so wrapped labels do not push values out of alignment. Labels should clamp to two lines and values should use tabular numerals.
- Hover feedback on flat metric cards should be visible through stronger borders, tinted backgrounds, and icon state changes, without static shadows or hover lift.
- Hover and selected states need one shared interaction vocabulary. Navigation items should use the primary blue-tinted background for hover/selected states. Content cards and picker cards should use a consistent flat card state, ideally border-color plus a light tinted background, rather than mixing gray overlays, provider-colored overlays, shadows, and lift effects across modules.
- Form controls should avoid oversized pill radii unless the surrounding pattern is intentionally pill-based. Search fields and ordinary text inputs should use the same modest radius as standard settings inputs, so pages like Onboard and System feel like one product.
- Dense interactive graph nodes are an exception to the ordinary flat-card rule. For scenario model/provider/add nodes, hover may use a refined, theme-aware elevation because users are manipulating small graph objects and need stronger target separation. Keep the outer graph panel flat; only raise the active inner node with a primary border, subtle primary tint, text/icon emphasis, and a soft multi-layer shadow. Light themes should use low-alpha slate shadows; dark themes need a deeper shadow plus a faint primary edge so the card does not disappear into dark paper.

Known flattening backlog:

- Card hover/selected states are not fully unified yet. Agent, Connect AI, Credential OAuth, Onboard provider, and model-select cards now use the primary flat hover model; routing graph nodes intentionally use localized hover elevation; remote-control bot cards and skill cards still need a focused pass.
- Input border radius is not fully normalized yet. Onboard search has been reduced, but search/filter fields across provider lists, tool lists, dialogs, and system pages should be checked as a group.
- Some graph/node components outside the scenario routing graph may still use shadows to describe graph objects. Review separately because graph nodes may need a selected/active affordance. Prefer a refined theme-aware elevation for dense editing nodes instead of a generic heavy card shadow or a muted gray overlay.
- Dialogs, menus, tooltips, and floating status indicators may keep elevation, but should be checked for excessive shadow strength.

## Phase 1: Establish The UI Baseline

| Priority | Task | Goal | Main Files | Acceptance Criteria |
| --- | --- | --- | --- | --- |
| P0 | Adjust global typography scale | Improve readability and reduce overuse of 10px/12px text | `frontend/src/theme/base.ts` | `body1` is around `1rem`, `body2` is around `0.875rem`, `caption` is not below `0.75rem`, and key pages have no obvious text overflow |
| P0 | Define a shared page header pattern | Make page titles, subtitles, filter controls, and action areas consistent | New or existing shared component under `frontend/src/components/` | Dashboard, Overview, Credentials, and Provider List use the same title size, subtitle treatment, spacing, and action layout |
| P1 | Normalize spacing tokens | Replace scattered local `gap`, `mb`, `p`, and `px` values with a predictable rhythm | `frontend/src/styles/common.ts` or theme helpers | Page padding, section gaps, card padding, and toolbar spacing follow a documented scale |
| P1 | Standardize surface treatment | Lower the stacked-card feeling and reserve shadows for meaningful elevation | Theme, `UnifiedCard`, common styles | Default cards do not jump or intensify on hover; shadows are used mainly for floating UI, dialogs, menus, and emphasis |
| P1 | Document flat surface rules | Make future modules follow the same flat visual model | `docs/frontend-ui-optimization-plan.md`, shared components | The plan states when to use borders, backgrounds, shadows, hover states, and selected states |

## Phase 2: Refactor Shared Components

| Priority | Task | Goal | Main Files | Acceptance Criteria |
| --- | --- | --- | --- | --- |
| P0 | Refactor `UnifiedCard` sizing | Remove unpredictable percentage-based width and height presets | `frontend/src/components/UnifiedCard.tsx` | `small`, `medium`, `large`, `full`, and `header` no longer rely on `25%`, `50%`, or `100%` height defaults that depend on parent containers |
| P0 | Remove default hover elevation from content cards | Prevent ordinary content cards from feeling interactive when they are not | `UnifiedCard.tsx`, `frontend/src/styles/common.ts` | Non-clickable cards do not add heavy shadow or translate on hover |
| P1 | Add a `Surface` or `Section` primitive | Separate page grouping from card affordances | New shared component under `frontend/src/components/` | Pages can group content with spacing, dividers, or subtle backgrounds without wrapping every region in a Card/Paper |
| P1 | Normalize table density | Make data tables easier to scan in a management UI | `frontend/src/components/dashboard/ServiceStatsTable.tsx` and other table components as needed | Data rows are about 44-48px tall, table headers are compact and legible, numeric columns use right alignment consistently |
| P1 | Migrate repeated panels to `Surface` | Spread the flat model beyond Dashboard and Overview | High-traffic pages using repeated `Paper`/`UnifiedCard` wrappers | Provider List, System, Guardrails, and credential pages use flat surfaces unless a true card affordance is needed |
| P1 | Flatten credential management tables | Remove the large card wrapper and improve provider table scanning | `frontend/src/pages/CredentialPage.tsx`, `ApiKeyTable.tsx`, `OAuthTable.tsx` | Credentials uses `PageHeader` plus separate flat `Surface` sections, tables have compact headers/rows, and narrow screens scroll horizontally rather than compressing columns |
| P1 | Flatten interactive picker cards | Keep clickable grids clear without hover shadows or lift | `AgentOverviewPage.tsx`, `OAuthDialog.tsx` | Agent scenario cards and OAuth provider cards use border/background feedback on hover, with no box shadow or translate motion |
| P1 | Normalize hover and selected state tokens | Make interaction feedback predictable across navigation, cards, picker grids, and tables | Shared style helpers, `AgentOverviewPage.tsx`, `OAuthDialog.tsx`, provider/remote-control card grids | Navigation hover/selected states use a consistent primary-tinted treatment; clickable cards use one shared flat hover/selected treatment; provider-specific colors are reserved for icons, badges, or explicit status rather than changing the whole card behavior |
| P1 | Normalize input and card radius | Keep form fields and clickable cards visually related across pages | `BrowseProviders.tsx`, filter/search components, shared field styles | Search fields and ordinary text inputs use modest radii; cards stay at 8px or less unless a local design system explicitly requires more |

## Phase 3: Improve Dashboard

| Priority | Task | Goal | Main Files | Acceptance Criteria |
| --- | --- | --- | --- | --- |
| P0 | Rework Dashboard header | Make title, filters, auto-refresh, and refresh controls feel like one toolbar | `frontend/src/pages/DashboardPage.tsx` | Title/subtitle sit on the left, controls sit on the right, and small screens wrap without cramped controls |
| P0 | Remove StatCard side stripe | Avoid decorative side-stripe card styling | `frontend/src/components/dashboard/StatCard.tsx` | `StatCard` no longer uses a `::before` colored vertical stripe; state is communicated with icon tint, label, or subtle background instead |
| P1 | Tune StatCard hierarchy | Make metric cards easier to scan quickly | `StatCard.tsx` | Label, value, subtitle, and icon have clear hierarchy; values are prominent without crowding icons or labels; labels use Title Case, clamp to two lines, and values align across cards |
| P1 | Rebalance three-column dashboard layout | Avoid squeezing central charts and right-side model list | `DashboardPage.tsx` | On large screens the three columns feel stable; on medium screens quick nav hides or moves; the right column remains usable |
| P1 | Flatten Dashboard secondary panels | Align Quick Start, chart wrappers, model list, and table with the flat surface direction | `AgentQuickNav.tsx`, dashboard chart components, `DashboardPage.tsx`, `ServiceStatsTable.tsx` | Dashboard panels have no static shadow, no hover lift/glow, and use borders/spacing for structure |
| P2 | Add skeleton loading state | Replace centered spinner with page-shaped loading feedback | `DashboardPage.tsx` and dashboard components | Loading state shows skeleton cards, chart placeholders, and table/list placeholders |

## Phase 4: Improve Overview And Heatmap

| Priority | Task | Goal | Main Files | Acceptance Criteria |
| --- | --- | --- | --- | --- |
| P1 | Use the shared page header on Overview | Align Overview with Dashboard and other management pages | `frontend/src/pages/overview/OverviewPage.tsx` | Overview title, subtitle/date range, filters, and refresh action match the shared page header pattern |
| P1 | Simplify heatmap containers | Avoid Paper inside Paper and repeated borders | `OverviewPage.tsx`, `frontend/src/components/dashboard/TokenHeatmap.tsx` | Heatmap has one clear containing surface, not multiple nested surfaces with redundant borders |
| P1 | Improve heatmap metric typography | Make metric captions and values more readable | `TokenHeatmap.tsx` | Metric captions are at least 11-12px, metric values are at least 14px, and labels remain readable on desktop and tablet |
| P2 | Improve small-screen heatmap behavior | Prevent the heatmap from becoming too compressed | `TokenHeatmap.tsx` | Small screens support internal horizontal scrolling or an alternate compact layout instead of shrinking cells until labels become illegible |

## Phase 5: Polish Navigation And Icon Consistency

| Priority | Task | Goal | Main Files | Acceptance Criteria |
| --- | --- | --- | --- | --- |
| P1 | Review ActivityBar width and labels | Reduce awkward truncation in nav labels | `frontend/src/layout/constants.ts`, `frontend/src/layout/ZenActivityBar.tsx` | Common English and Chinese labels fit cleanly or have deliberate tooltip/label behavior |
| P1 | Normalize selected navigation states | Reduce side stripe and glow effects in nav | `frontend/src/layout/Sidebar.tsx`, `frontend/src/layout/ZenActivityBar.tsx` | Selected state is communicated by background, text weight, and icon color with less decorative edge styling |
| P1 | Flatten nav surfaces | Keep navigation quiet and consistent with dashboard surfaces | `Sidebar.tsx`, `ZenSidebar.tsx`, `ZenActivityBar.tsx` | No selected-state glow or colored side stripe; inactive labels avoid awkward truncation |
| P2 | Prefer MUI icons for new UI work | Align with repository frontend guideline | New or refactored frontend components | New UI components use MUI icons unless there is an existing local brand icon requirement |

## Phase 6: Improve Login, Empty States, And Error States

| Priority | Task | Goal | Main Files | Acceptance Criteria |
| --- | --- | --- | --- | --- |
| P2 | Improve Login first screen | Make the entry point feel more trustworthy and product-specific | `frontend/src/pages/Login.tsx` | Login screen includes product identity, clear token explanation, and a calmer card/surface treatment |
| P2 | Introduce a shared `EmptyState` component | Avoid one-off empty states across pages | New shared component under `frontend/src/components/` | Dashboard, tables, provider list, and guardrails pages can reuse a common empty-state vocabulary |
| P2 | Normalize error and disconnect dialogs | Make high-stakes states clearer and more consistent | `frontend/src/App.tsx`, `frontend/src/contexts/AuthContext.tsx`, related pages | Dialog title, body, icon, and action hierarchy are consistent across auth/session/server failure states |

## Recommended Development Order

1. Update `frontend/src/theme/base.ts` typography.
2. Add or refactor a shared `PageHeader`.
3. Refactor `UnifiedCard` sizing and default hover behavior.
4. Rework Dashboard header, spacing, and toolbar layout.
5. Refactor `StatCard` and remove the colored side stripe.
6. Align Overview and `TokenHeatmap` with the shared layout system.
7. Polish navigation selected states and icon consistency.
8. Improve Login, empty states, and error dialogs.

## Verification Checklist

Run these checks after each phase:

```bash
cd frontend
pnpm typecheck
pnpm lint
pnpm dev
```

Manually inspect at least these routes:

```text
/login
/dashboard/7d
/overview
/provider-list
/system
/guardrails
```

Check at least these viewport widths:

```text
390px mobile
768px tablet
1280px desktop
```

For each route and viewport, verify:

- Text is readable and does not overlap.
- Buttons and filter controls wrap without clipping.
- Tables and charts remain scannable.
- Loading, empty, error, and success states have consistent hierarchy.
- The page does not rely on decorative card hover effects for non-interactive content.

## Implementation Notes

- For API-related frontend work, keep using generated swagger clients. If a UI improvement needs unsupported backend data, add a placeholder frontend function and document the backend/API follow-up separately.
- For new frontend components, use MUI components and MUI icons.
- Avoid broad visual rewrites while touching functional pages. Keep each phase small enough to review and verify independently.
- Prefer shared primitives and theme tokens over repeated `sx` values when the same pattern appears across multiple pages.
