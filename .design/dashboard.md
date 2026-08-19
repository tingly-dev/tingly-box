# Usage Analytics

This document describes the usage analytics subsystem end to end: the two product surfaces, metric semantics, API and storage paths, frontend data flow, shared table architecture, responsive behavior, and verification boundaries.

The two entry points answer different user questions:

- **Usage Dashboard** — “What is happening across this time range, and where should I investigate?”
- **Team Usage** — “Who consumed the shared access, and which models/providers account for that person’s usage — or the reverse, which accounts are behind a given model’s or provider’s usage?”

Primary frontend files:

- `frontend/src/pages/DashboardPage.tsx`
- `frontend/src/pages/UserUsagePage.tsx`
- `frontend/src/components/dashboard/`

Primary backend files:

- `internal/server/module/usage/`
- `internal/data/db/usage_record.go`
- `internal/data/db/usage_daily.go`

## Product surfaces and routes

### Usage Dashboard

Route: `/dashboard/:timeRange`, where `timeRange ∈ today | yesterday | 3d | 7d | 30d | 90d`. Invalid values fall back to `today`; `/overview` redirects into this route.

`today` and `yesterday` are hourly views backed by minute-level time series. The other ranges are daily views.

Three global filters apply to the stat cards, chart, request view, activity heatmap, and Usage by Model table:

- **Provider** — provider UUID, grouped by authentication type.
- **Model** — concrete model name.
- **Identity** — `user_id`; `admin` is the main account and the remaining options are sharing keys from `listAPITokens`.

The main analysis pane has three modes:

- **Summary** — stacked token trend.
- **By Request** — paginated request table for concrete diagnosis; only available for `today` and `yesterday`.
- **Activity** — fixed 365-day heatmap.

If a stale `requests` selection survives a route change into a daily range, `effectiveViewMode` renders Summary instead.

### Team Usage

Route: `/dashboard/users`.

Available ranges are `today | 7d | 30d | 90d`. Unlike Dashboard’s completed daily windows, Team Usage ends every range at the current time:

- `today`: local midnight → now.
- `7d` / `30d` / `90d`: local midnight of the first included day → now.

The page is a two-stage, full-width drill-down over one of three axes, chosen with a **By account / By model / By provider** toggle in the page header (default: By account):

1. **Roster table** — searchable, sortable, and paginated.
   - By account: registered users (roster is metadata-driven, from `listAPITokens`).
   - By model: every model with usage in range (roster is stats-driven, from `group_by=model`).
   - By provider: every provider with usage in range (roster is stats-driven, from `group_by=provider`).

   Model and provider rosters have no "registered" concept, so unused ones simply don't appear — unlike the account roster, which keeps registered-but-unused identities visible.
2. **Selected roster item detail** — identity context, four summary metrics, and the complementary breakdown table: model breakdown for a selected account; account breakdown for a selected model or provider. All three breakdown directions render through one shared `RosterBreakdownTable` component (`frontend/src/components/dashboard/RosterAxis.tsx`) — only the identity column(s), row key, and empty-state copy differ per direction.

Selecting a row updates the second stage and scrolls it into view. The tables remain full width so Cache/Input/Output values can be compared as columns instead of being compressed into prose or progress bars.

Account, model, and provider are genuinely orthogonal dimensions the backend already supports independently (`group_by` selects the axis; `user_id`/`model`/`provider` are independent filters — see API contract below), so the toggle exposes axes the backend already modeled rather than adding new backend surface. Each axis's search/sort/pagination/selection/detail-load lifecycle is driven by a shared `useRosterAxis` hook (same file), called once per axis — see "Roster-axis shared infrastructure" below. Each axis's state is fully independent — switching the toggle does not carry a filter typed for one axis over into another. All three rosters and all three detail panels load in parallel regardless of which axis is currently visible, so toggling never shows a loading spinner for already-fetched data; only a range change or manual refresh re-fetches.

When the model axis is selected, "which accounts used this model" is filtered by **provider + model together**, not model name alone — the same model name can exist under more than one provider, and filtering on name alone would conflate them. The provider axis has no equivalent collision risk — `provider_uuid` alone is a stable key — so "which accounts used this provider" filters by `provider` only.

## Canonical metric semantics

The frontend must not treat the backend `total_tokens` field as the displayed grand total. The canonical display total is:

```text
display_total = input + output + cache_read
```

`getTotalTokens()` in `chartStyles.ts` owns this calculation.

The cache model has two separate values:

- `cache_read_tokens` — tokens served from cache; displayed as **Cache Read**.
- `cache_write_tokens` — tokens written into cache.

The critical invariant is:

```text
cache_write_tokens ⊂ total_input_tokens
```

Cache writes are already included in Input. They must never become an addend in a total, a fourth chart stack, or a fourth donut slice. Doing so double counts the same tokens. See `.design/stream-usage-tracking.md` for protocol normalization and billing details.

Cache writes appear only as attribution:

- Input tooltip/legend annotation: `incl. N written`.
- Cache Hit Rate subtitle: `N read · M written`.
- A Cache Write table column, only when at least one row has a non-zero value.

The cache hit rate is:

```text
cache_hit_rate = cache_read / (cache_read + input)
```

Because Input contains writes, a write correctly counts as a miss. The helper `getCacheHitRate()` owns the formula.

Vocabulary is deliberate:

- Use **Cache Read** for hits.
- Use **Cache Write** for creation/write cost.
- Do not use an ambiguous bare “Cache” label for a table dimension.

## API contract

Four gzip-compressed JSON endpoints live under `/api/v1/usage/`:

| Endpoint | Main consumers | Important parameters |
|---|---|---|
| `/stats` | Dashboard cards/model table; Team user/model tables | `group_by=model\|provider\|scenario\|rule\|user\|daily\|hourly`, time bounds, provider/model/scenario/rule/user/status filters, sort, limit ≤ 1000 |
| `/timeseries` | Dashboard Summary and Activity | `interval=minute\|hour\|day\|week`, time bounds, provider/model/scenario/user filters |
| `/records` | Dashboard By Request | time bounds, provider/model/scenario/user/status filters, `limit` ≤ 1000, `offset` |
| `/performance` | Dashboard Summary | time bounds and provider/model/scenario/user filters; successful requests only |

The stats response is `{ meta, data }`; `data` contains additive counts plus derived rates. The records response uses `meta: { total, limit, offset }` for the real filtered range.

`status` values stored by usage tracking are `success` or `error`, matching the By Request filter. The public query type also documents `partial`, so any new producer or filter must be checked against the values actually persisted.

All frontend time bounds are local ISO strings with an explicit timezone offset. The SQLite path stores and compares server-local time strings, so sending UTC-only bounds can shift the intended range.

### Surface-specific calls

Usage Dashboard:

- `/stats`, `group_by=model`, `limit=1000`.
- `/timeseries`, with minute or day interval.
- `/records`, initially limited to the most recent 500 when By Request is opened.
- `/performance`, for the complete filtered range while Summary is visible.
- `getProviders` and `listAPITokens` for filter metadata.

Team Usage:

- `listAPITokens({ limit: 500 })` for the registered-user roster.
- `/stats`, `group_by=user`, `limit=500` for user aggregates (account roster).
- `/stats`, `group_by=model`, `limit=1000` for the model roster.
- `/stats`, `group_by=provider`, `limit=200` for the provider roster.

  All four of the above (`loadRosters`) load together on mount, range change, and manual refresh, regardless of which axis is currently shown, so toggling between axes is instant.

- `/stats`, `group_by=model`, `user_id=<selection>`, `limit=1000` for the selected account’s model breakdown.
- `/stats`, `group_by=user`, `model=<selection>`, `provider=<selection>`, `limit=500` for the selected model’s account breakdown.
- `/stats`, `group_by=user`, `provider=<selection>`, `limit=500` for the selected provider’s account breakdown (no `model` filter — a provider spans every model under it).

The account roster intentionally starts from registered tokens, then left-joins usage by `user_id`. A registered but unused or disabled sharing key therefore remains visible with zero usage. Usage associated with an unknown/unregistered identity is not added as a synthetic account-roster row, but it does still surface inside a model’s or provider’s account-breakdown table (those tables render exactly what `group_by=user` returns, with no roster join), labeled by its raw `user_id` when it has no matching token.

The model and provider rosters have no equivalent "registered" concept — each is simply every `group_by=model`/`group_by=provider` bucket with usage in range.

### Contract maintenance note

The frontend transport uses the generated OpenAPI client, while dashboard view components keep small local interfaces (`AggregatedStat`, `TimeSeriesData`, `UsageRecord`) for rendering.

The runtime handlers and Swagger contract accept `user_id` consistently for `/timeseries`, `/records`, and `/performance`. Contract changes start from the backend model and route configuration, followed by `task codegen`; do not hand-edit `openapi.json` or `frontend/src/client/schema.d.ts`.

## Backend query and aggregation path

### Raw records

`usage_records` stores one row per request, including provider, model, scenario, rule, `user_id`, request/input/output/cache token counts, status, streaming state, latency, and timestamps.

`GetRecords`:

- filters in SQL;
- orders by `timestamp DESC`;
- pages with `LIMIT/OFFSET`;
- skips `COUNT(*)` when a returned page is short, deriving total as `offset + len(records)`.

### Daily pre-aggregation

`usage_daily` stores one row per:

```text
(UTC day, provider_uuid, model, user_id)
```

It contains additive sums needed to reconstruct request counts, token totals, errors, streaming rate, and average latency. Completed days are lazily backfilled on the first eligible query, at least one hour after UTC midnight.

`GetAggregatedStats` can use the merged daily path for:

- `group_by=model`
- `group_by=provider`
- `group_by=user`
- `group_by=daily`

provided there is a bounded multi-day window and no scenario/rule/status filter. Complete middle days come from `usage_daily`; partial edge days come from `usage_records`. The additive buckets are merged before error, streaming, and latency rates are derived.

`GetTimeSeries(interval=day)` uses the same completed-day/edge-day strategy when its filters are limited to provider/model/user. Unsupported shapes or an aggregation failure fall back to a raw scan.

The steady-state query cost of the daily path is effectively flat in raw record count; the original benchmark measured roughly 25× faster reads around 200k records over 90 days.

All usage reads take `UsageStore.mu.RLock()`, so concurrent dashboard requests do not serialize behind one another.

## Usage Dashboard frontend data flow

All asynchronous loaders use monotonic request sequence refs. A response is discarded when a newer request was issued while it was in flight. New loaders must follow the same rule.

### Metadata and filters

Provider and API-token metadata load once on mount and again on manual refresh. They do not reload on every filter or auto-refresh tick.

Provider and model options derived from stats use a snapshot pattern:

- Snapshot provider options only while Provider is `all`.
- Snapshot model options only while Model is `all`.

Deriving options continuously from already-filtered stats would collapse a dropdown to the selected value. Selections are reset only when their backing metadata disappears, not when a valid filter combination happens to return no data.

### Main load

`loadData` fetches stats and time series concurrently. After a successful load, it publishes a fresh `recordsParams` object containing the exact time window and filters. `RequestsView` keys its fetch to that object, so records load once per dashboard refresh instead of once per individual dependency change.

Manual refresh reloads metadata, stats, time series, request data, and the heatmap. Optional auto-refresh repeats the data load every 60 seconds and bumps the heatmap refresh key.

### Stat cards

Card totals are summed from the full `group_by=model` result. The dashboard requests the maximum stats limit because a truncated model list would under-count every card.

Cards show:

- Total Requests
- Total Tokens (`input + output + cache_read`)
- Cache Hit Rate, with read/write attribution
- Error Rate
- Streamed Rate

## Dashboard analysis views

### Summary

`HourlyTokenHistoryChart` is used for today/yesterday; `DailyTokenHistoryChart` for all other ranges.

The stack order is Cache Read → Input → Output. Cache Write is carried in each data point for tooltips but is never rendered as a fourth series.

Response Performance also lives in Summary because it describes the selected range as a whole rather than an individual request. It comes from `/performance` and excludes failed requests. When the viewport can preserve a readable chart (1280px and wider), it shares one analysis row with the token trend: Response Performance acts as a compact fixed-width index (320px) on the left, while the trend receives the remaining width on the right. Narrower screens stack the two cards in that same order. This keeps percentile summary and time-series detail at the same information level without dedicating a separate full row to either one:

- **TTFT** — P10/P50/P90/P95/P99 over streamed requests with a first content token.
- **TPS** — per-request output throughput, derived as `(output_tokens - 1) / ((latency_ms - ttft_ms) / 1000)`, the inverse of TPOT. The first token belongs to TTFT, so N output tokens contain N−1 decode intervals. The API returns P10/P50/P90/P95/P99; the UI emphasizes P10 through P95 and intentionally hides TPS P99. This is not aggregate server throughput across concurrent requests.
- **Latency** — P10/P50/P90/P95/P99 over successful requests with a positive latency.

Percentile rows descend top-to-bottom as P99 → P95 → P90 → P50 → P10; metric columns run left-to-right as TTFT → TPS → Latency. This transposed matrix keeps the performance card narrow while preserving globally aligned comparisons. The backend calculates all five percentiles for every metric, but the frontend intentionally hides TPS P99 because the fastest 1% is easily dominated by short-request timing artifacts and has little operational value; TPS P95 remains the stable upper-tail reference. TTFT and Latency retain P99. All values have equal visual weight and use the same type scale. TPS values omit a repeated unit because the metric name already defines it.

The frontend treats absent or non-finite percentile fields as unavailable (`—`) so a rolling upgrade against an older backend schema does not crash the dashboard while P95 is being introduced.

### By Request

The dashboard first loads the most recent 500 records plus `meta.total`.

- If `total ≤ 500`, status filtering and pagination remain client-side.
- If `total > 500`, the table switches to server paging and pushes status, limit, and offset into SQL.

By Request intentionally contains only the paginated request table. Token Breakdown was removed because its input/output/cache totals duplicate the stat cards, summary trend, and request columns. The request table exposes derived TPS per row for concrete diagnosis.

The page resets request pagination when filters or the range start change, but not when auto-refresh only advances the range end.

### Activity

`DashboardHeatmapSection` renders a fixed trailing 365-day window. It ignores the route range selector but shares Provider, Model, and Identity filters. An info tooltip explains the fixed window.

The client fills missing dates so every day has a cell. Color levels use p25/p50/p75 quantiles of active days; a linear value/max scale would collapse most cells into one shade when traffic is skewed.

Layout invariants:

- A `ResizeObserver` derives responsive cell size, clamped to 10–16px.
- The day-label column uses the same fixed width subtracted by grid sizing.
- Narrow grids scroll horizontally and initially reveal the most recent weeks.
- Cell hover may change outline/opacity, but must not use `transform`; a transformed edge cell expands scrollable bounds and causes scrollbar flicker.

### Usage by Model

`ServiceStatsTable` adds Provider and Model identity columns, then delegates all metric columns to the shared usage metric components. It paginates locally.

Dashboard preserves its existing display conventions:

- Requests use locale grouping (`1,842`).
- Cache Hit and Error Rate use two decimal places.
- It does not show a Total column.

## Team Usage frontend data flow

`loadRosters` fetches the token roster, `group_by=user` stats, `group_by=model` stats, and `group_by=provider` stats concurrently — all four, regardless of which axis is currently toggled on. It synthesizes the primary `admin` account, filters `admin` out of sharing-key metadata, then maps every registered identity to a `UserUsageRow`. This roster fetch stays page-specific (not part of the shared hook below) because how each roster is fetched genuinely differs: the account roster is left-joined against registered tokens, model/provider rosters are pure stats reads.

Everything downstream of "here is a roster array" — search, sort, pagination, selection (with fallback to the first visible row when the current selection is filtered out or the roster changes), and the race-guarded load of that selection's "detail" data — is identical across axes and is driven by one shared hook, `useRosterAxis` (`frontend/src/components/dashboard/RosterAxis.tsx`), called once per axis:

```ts
const accountAxis = useRosterAxis({ roster: rows, getKey: accountKey, loadDetail: fetchModelsForAccount, ... })
const modelAxis = useRosterAxis({ roster: modelRoster, getKey: getModelKey, loadDetail: fetchAccountsForModel, ... })
const providerAxis = useRosterAxis({ roster: providerRoster, getKey: getProviderKey, loadDetail: fetchAccountsForProvider, ... })
```

`getKey`/`nameOf`/`searchText` and the three `loadDetail` functions (`fetchModelsForAccount`, `fetchAccountsForModel`, `fetchAccountsForProvider`) are stable module-level functions, not closures — so the hook's internal effects never see a changing function reference and never re-fire on an unrelated render. A model is keyed by `provider_uuid + model` together (`getModelKey`), not model name alone, because the same model name can exist under more than one provider; a provider is keyed by `provider_uuid` alone (`getProviderKey`) — it has no equivalent collision risk.

Each `useRosterAxis` call owns one axis's full state independently — its own search/sort/page/selection/detail/detailLoading, and its own internal sequence ref guarding the detail load against out-of-order responses. A filter typed while viewing one axis never silently applies once the user switches to another (see `.design/ux-principles.md` #12, scope side effects to the current surface). `rowsPerPage` is the one piece of state shared across axes (a display-density preference, not a filter), passed into all three hook calls. `activeAxis` (`viewMode === 'account' ? accountAxis : ...`) reads the fields common to every axis (`search`, `sortField`, `sortDirection`, `page`, `handleSort`, `detailLoading`, `visibleRows.length`, `detail.length`) without a three-way ternary at each call site; axis-specific fields (`selected`, `pagedRows`) are read off the specific `accountAxis`/`modelAxis`/`providerAxis` in whichever branch renders that axis's rows. Changing range, or a roster change, reloads all three detail queries; detail for axes not currently shown loads in the background so switching the toggle is instant once it resolves.

Per-axis summary tiles and totals are derived from whichever roster backs the active axis (`rows` for accounts, the model roster for models, the provider roster for providers) via one shared reduction (`computeUsageSummary`, same file), so the "Total tokens / Cache hit rate / Requests / Errors" tiles always describe what the visible roster table shows — only the first tile (`Registered users` / `Models used` / `Providers used`, with a “N active” / “across N providers” / “across N models” hint respectively) is axis-specific. “Active” for accounts means `request_count > 0`.

### Registered users table (By account)

The user identity cell is page-specific. All numeric columns come from the shared usage metric definition:

```text
Requests → Total → Cache Read → [Cache Write] → Cache Hit
         → Input → Output → Error Rate
```

The table:

- defaults to displayed Total descending;
- allows sorting by every visible metric and by user name;
- searches display name and `user_id`;
- paginates locally;
- shows Cache Write only when any roster row reports writes;
- highlights the selected row and exposes the model drill-down via the arrow.

### Model roster table (By model)

Mirrors the account roster: Provider and Model identity columns, then the same shared metric column sequence, same sort/search/pagination behavior. Unlike the account roster it is not left-joined against anything — a row exists only if that `(provider, model)` pair has usage in range, so there is no zero-row/disabled-row concept on this axis.

### Provider roster table (By provider)

Same shape again, but with a single Provider identity column (no second column, since provider is the whole axis) — otherwise identical behavior to the model roster table.

### Selected roster item detail

The identity title/subtitle/status-chip block is factored into its own `RosterDetailHeader` component (in `UserUsagePage.tsx`, not shared — the three axes' identity shapes are genuinely different: account has display name + `user_id` + enabled/primary chip + last-used/joined lines; model has name + provider name + an account-count chip; provider has name only + an account-count chip, with no secondary identity line since a provider has no natural parent to show). It was pulled out specifically because inlining it left the page dominated by one large three-way ternary block.

Four summary cells below it show Input, Output, Cache Read, and Cache Hit Rate, sourced from whichever roster row is selected on the active axis. Cache Write is annotated beneath Input because it is a subset.

The breakdown table below that adds the complementary identity column(s) — Provider + Model for an account's model breakdown, a single User column (display name over the raw `user_id`, resolved from the token roster when known) for a model's or provider's account breakdown — then uses the same shared metric sequence as the roster table. All three directions render through one shared `RosterBreakdownTable` component (`components/dashboard/RosterAxis.tsx`); callers pass an `identityColumns` array (one column for accounts, two for account→model) and a `rowKey`, so the table markup exists once rather than being duplicated per direction. It requests up to 1000/500 rows respectively and does not add a second pagination control; the table body has a bounded, internally scrollable height.

Backend model results are requested with `sort_by=total_tokens`, whose stored meaning is input + output. The displayed Total includes cache reads. If ordering must exactly follow displayed Total, sort the returned rows by `getTotalTokens()` on the client or add a distinct backend sort contract—do not silently redefine `total_tokens`.

Manual refresh reloads both rosters and both aggregate queries (`loadRosters`). Selected-item detail for each axis reloads when that axis's selection, the roster backing it, or the range changes. If refresh is expanded to reload detail explicitly, keep the independent sequence guards (one per axis).

## Shared metric table architecture

The shared layer intentionally owns metric columns, not the entire table. Dashboard and Team Usage have different identity columns, selection behavior, sorting, pagination, localization, and empty states; forcing those into one generic table component would couple unrelated UX.

### `RosterAxis.tsx` — roster-axis shared infrastructure

Team Usage's three axes (account/model/provider) share a genuinely identical *state* shape even though their *rendering* differs, so this file draws the line at state, not markup: it owns `MetricRow` (the common row shape), `filterAndSort`/`computeUsageSummary` (pure functions), the `useRosterAxis` hook (search/sort/page/selection/detail-load lifecycle for one axis), and `RosterBreakdownTable` (the one "which X used this Y" table shape, parameterized by identity columns). It does not know about accounts, models, or providers specifically — `UserUsagePage.tsx` supplies the axis-specific `getKey`/`nameOf`/`searchText`/`loadDetail` functions and the axis-specific identity columns/headers. A 4th roster axis (the backend already supports `group_by=scenario`/`group_by=rule`) would reuse this file entirely and only add a fourth `useRosterAxis` call plus its own roster fetch and column/header rendering.

Exported via `components/dashboard/index.ts` like the rest of this directory's shared pieces.

### `usageMetricColumns.ts`

Owns:

- `UsageMetricKey`
- the canonical column order
- default English labels
- optional Total and Cache Write insertion
- the minimal `UsageMetricSource` shape

Pure definitions live in `.ts`, separate from React exports, so Vite Fast Refresh keeps a stable component boundary.

### `UsageMetricCells.tsx`

`UsageMetricHeaderCells` renders the metric headers.

`UsageMetricValueCells` owns:

- compact token formatting;
- displayed Total via `getTotalTokens`;
- Cache Hit via `getCacheHitRate`;
- conditional Cache Write;
- error-rate color threshold;
- configurable request formatter and decimal precision.

Callers choose presentation-only differences:

- Team tables: compact requests, one decimal.
- Dashboard model table: locale-grouped requests, two decimals.

### Column-count rule

Empty rows and spacer rows must derive `colSpan` from the actual rendered column list (identity columns + `getUsageMetricColumns(...)` + the trailing selection-arrow column), never a hand-written arithmetic special case. Team Usage's primary-column arrays include every header cell — including non-sortable ones, like the model roster's leading Provider column (a `{ kind: 'label' }` entry, versus `{ kind: 'sort' }` for sortable ones) — specifically so `colSpan = primaryColumns.length + 1` stays correct without an axis-specific `+1`/`+2` branch.

## Responsive and interaction behavior

Both Team tables have a concrete minimum width. On narrow screens, `TableContainer` owns horizontal scrolling; the document itself must not gain a horizontal scrollbar. This scopes the side effect to the table surface.

The Team user and model tables are stacked rather than split into narrow side-by-side cards. This preserves direct column comparison at desktop widths and keeps the selection → detail relationship explicit.

Loading states use skeletons rather than briefly rendering empty-state copy. Registered-but-unused users intentionally render as zero rows, not as missing data.

## Mock data and verification

Mock usage endpoints live in `frontend/src/mocks/handlers.ts`. They must include:

- multiple registered identities, including disabled and unused users;
- model groups from more than one provider;
- non-zero Cache Read and Cache Write values;
- enough variation to exercise sorting and responsive widths.

`group_by=user` with a `model` or `provider` filter (the model/provider axes' account-breakdown queries) is mock-served by deterministically scaling/dropping the base user list per `(scope, user)` hash, where scope is `model:<name>` or `provider:<uuid>` — so different models/providers return different, sortable account mixes instead of one fixed list reused everywhere, and some accounts legitimately drop out of some scopes, exercising the "not every account used every model/provider" case. `group_by=model` and `group_by=provider` both derive from one unscaled `modelUsageBase` array in the handler — the provider branch reduces it by `provider_uuid` at request time — so there is a single source of truth for the mock figures instead of a hand-maintained sum that can drift out of sync.

Shared column ordering is covered by `frontend/src/components/dashboard/usageMetricColumns.test.ts`. Cache hit and read/write formatting invariants are covered by `frontend/src/components/dashboard/cacheBreakdown.test.ts`.

For frontend changes, verify at minimum:

```bash
pnpm -C frontend exec vitest run \
  src/components/dashboard/usageMetricColumns.test.ts \
  src/components/dashboard/cacheBreakdown.test.ts
pnpm -C frontend exec oxlint <changed-files>
pnpm -C frontend build
```

Use the repository `ui-preview` workflow to inspect:

- `/dashboard/7d`
- `/dashboard/users`
- a narrow viewport where table overflow remains internal.

## Invariants and extension checklist

- Keep `cache_write_tokens` inside Input and out of every total.
- Derive displayed Total with `getTotalTokens()`.
- Derive Cache Hit with `getCacheHitRate()`.
- Use `hasCacheWrites()` for conditional write columns.
- Extend `usageMetricColumns.ts` when adding a shared metric; do not add the same column independently to each table.
- Preserve caller-specific precision and request formatting.
- Use request sequence guards for new asynchronous loads.
- Keep filter option snapshots independent of filtered result emptiness.
- Keep the account roster metadata-driven (left-joined against `listAPITokens`); the model roster is stats-driven and has no equivalent registered/unused concept.
- Filter a single model by provider + model together, never model name alone — names can collide across providers. A single provider has no such collision risk and filters by `provider_uuid` alone.
- Keep each axis's search/sort/pagination state independent; a filter typed on one axis must not silently apply to another.
- A new roster axis (e.g. by scenario) should be built on the shared `useRosterAxis` hook and `RosterBreakdownTable` component (`components/dashboard/RosterAxis.tsx`) rather than re-implementing the search/sort/page/selection/detail-load lifecycle or the breakdown table markup per axis.
- Never hand-derive one mock aggregation from another (e.g. provider sums from per-model figures) as a separately maintained literal — compute it from the one source array at request time.
- Parse `YYYY-MM-DD` as local midnight (`new Date(\`${date}T00:00:00\`)`), not bare UTC-parsed dates.
- `usage_daily` has no scenario/rule/status dimension; those filters require raw scans unless the aggregate schema is extended.
- Adding a new summed column to both `usage_records` and `usage_daily` does not require dropping `usage_daily`; historical source rows contribute the migrated zero value.
- A true aggregate layout change must update the schema-rebuild condition and preserve merged/raw equivalence tests in `../internal/db/usage_daily_test.go`.
- `middleware.Gzip()` is JSON-only; never attach it to streaming/SSE routes.
- API additions start from backend models and Swagger definitions, followed by `task codegen`; generated files are never hand-edited.
