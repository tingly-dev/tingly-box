# Usage Dashboard

How the usage dashboard works today, end to end: the API contract, the backend
query path, and the frontend architecture. `frontend/src/pages/DashboardPage.tsx`
is the page; the view components live in `frontend/src/components/dashboard/`.

## Page structure

Route: `/dashboard/:timeRange` with `timeRange ∈ today | yesterday | 3d | 7d |
30d | 90d` (invalid values fall back to `today`; `/overview` redirects here).
`today` / `yesterday` are "hourly ranges" (minute-interval time series), the
rest use day intervals.

Three global filters live in the page header and apply to every view:

- **Provider** — provider uuid, grouped by auth type in the dropdown.
- **Model** — model name.
- **Identity** — `user_id`; `admin` is the main account, other entries are
  sharing keys (from `listAPITokens`, deduped by `user_id`).

The chart pane has three modes (`viewMode`): **Summary** (trend chart),
**By Request** (per-request table + charts, hourly ranges only), and
**Activity** (fixed 12-month heatmap). A stale `requests` selection carried
into a daily range renders as `summary` (`effectiveViewMode`).

## API contract

Three gzip-compressed GET endpoints under `/api/v1/usage/`
(`internal/server/module/usage/`):

| Endpoint | Used for | Notes |
|---|---|---|
| `/stats` | stat cards, Usage by Model table | `group_by=model`, `limit` ≤ 1000 (dashboard requests the max: card totals are summed client-side from the groups, so a low limit would under-count) |
| `/timeseries` | Summary charts, Activity heatmap | `interval=minute\|day`; filters `provider` / `model` / `user_id` |
| `/records` | By Request | `limit` ≤ 1000, `offset`, plus `status` / `provider` / `model` / `user_id` / `scenario` filters, all pushed down to SQL; response `meta: { total, limit, offset }` carries the real range total |

`status` values in the store are exactly `success` or `error`, so the
server-side equality filter matches the UI's Success/Error toggle 1:1.

Time bounds are sent as local ISO strings with timezone offset
(`toLocalISOString`) because the backend stores local time.

The dashboard components declare these response shapes as **local
interfaces**, not types generated from `openapi.json`. That is why
`cache_write_tokens` could ship ahead of a codegen run — `openapi.json`
currently lags the Go models by that one field. Anything that does consume
the generated SDK needs `task codegen` first.

## Backend query path

- **`usage_daily` pre-aggregation** (`internal/data/db/usage_daily.go`): one
  row per `(UTC day, provider_uuid, model, user_id)` with additive sums.
  Backfill is lazy (first query needing a completed day aggregates it, ≥1h
  after UTC midnight); which days exist is read from the table itself, not
  cached. `GetAggregatedStats` (group_by ∈ model/provider/user/daily, no
  scenario/rule/status filter) and `GetTimeSeries` (interval=day, filters ⊆
  provider/model/user) spanning ≥2 complete days run partial-day raw scans at
  the edges and `usage_daily` in the middle; anything else, and any
  aggregation error, falls back to the raw scan. `DeleteOlderThan` purges
  matching daily rows so boundary days re-aggregate. Steady-state cost of the
  daily path is flat in record count (measured ~25x faster than raw at ~200k
  records / 90 days).
- **Concurrent reads**: `UsageStore.mu` is a `sync.RWMutex`; queries take the
  read lock, so parallel dashboard requests don't queue behind each other or
  behind proxy usage writes.
- **`GetRecords`** pages with `LIMIT/OFFSET` ordered by `timestamp DESC` and
  skips the `COUNT(*)` scan when the returned page is short (the total is then
  `offset + len(records)` for free).

## Frontend data flow

All loaders guard against out-of-order responses with a monotonic sequence
ref (`requestSeq` / `recordsSeq` / heatmap's own): a response is discarded if
a newer request was issued while it was in flight. Any new fetch added to the
dashboard should follow the same pattern.

- **Filter metadata** (providers, API tokens) loads once on mount and on
  manual refresh — not per filter change or auto-refresh tick.
- **`loadData`** (per filter/range change) fetches stats + timeseries in
  parallel, then publishes `recordsParams` — a fresh object holding the time
  window *and* the current filters. The requests view keys off that object,
  so records are fetched exactly once per dashboard load.
- **Auto-refresh** (60s, opt-in toggle) reruns `loadData` and bumps
  `heatmapRefresh`; the records view refreshes transitively via the new
  `recordsParams` object.

### Filter dropdown options — snapshot pattern

Provider/Model options are derived from stats, but only **snapshotted while
that filter is unselected** (`selectedProvider === 'all'` /
`selectedModel === 'all'`). Deriving them live from the already-filtered stats
would collapse the menu to the current selection and force a clear-filters
round-trip to switch. The Identity dropdown is metadata-driven and always
complete.

Selections are auto-reset only when they disappear from **metadata** (a
deleted provider / sharing key) — never based on the filtered stats being
empty, which would wipe valid sibling filters whenever a combination simply
has no data.

## Views

### Summary

`HourlyTokenHistoryChart` (minute interval) for today/yesterday,
`DailyTokenHistoryChart` otherwise, full width. (A ranked "Models by Token
Usage" side panel used to sit next to the chart; it was removed as a
near-duplicate of the "Usage by Model" table below — both read the same
`group_by=model` stats — so the table is now the sole per-model view.)

Three stacked series: **Cache Read**, **Input**, **Output**. Cache *writes*
are deliberately not a fourth series — see the cache read/write section below.

### By Request (`RequestsView.tsx`)

The page always fetches a **sample**: the most recent 500 records for the
current window + filters, plus the server's `meta.total`.

- **Sample covers the range** (`total ≤ 500`): the table filters (status) and
  paginates client-side — zero extra requests.
- **Range exceeds the sample**: the table switches to server-side paging —
  each page is fetched with `limit=rowsPerPage, offset=page*rowsPerPage`, the
  Success/Error toggle becomes a server-side `status` filter, and the
  pagination count is that query's `meta.total`. The page resets when filters
  or the range start change, but not on auto-refresh ticks (only `end_time`
  moves there).

The Token Breakdown / Latency charts always compute over the sample; when the
sample is partial, a caption says so explicitly ("Charts reflect the most
recent N of M requests") instead of letting capped data pass as the range.

### Activity (`DashboardHeatmapSection.tsx` + `TokenHeatmap.tsx`)

A fixed 365-day, GitHub-style contribution grid (~52 week columns, which also
fits the wide pane). It deliberately ignores the range selector — an info
tooltip states this — but shares all three filters, refetching on any change.
Days are filled client-side so every day in the window has a cell; color
levels use p25/p50/p75 quantiles of active days (a linear value/max scale
collapses to one shade when every day has traffic). First load shows a
skeleton (an empty `dailyData` otherwise renders the "No activity" state
before data arrives).

Layout facts that keep the grid stable:

- Cell size is responsive: a `ResizeObserver` divides the pane width by the
  week count, clamped to 10–16px; below the minimum the grid scrolls
  horizontally and auto-scrolls to the most recent weeks.
- The day-label column is a **fixed** `DAY_LABEL_WIDTH - CELL_GAP` px wide —
  the same constant the cell-size math subtracts — so the computed grid fits
  the measured pane exactly. A `max-content` column can differ by a few px and
  leave a phantom scrollbar that flickers on resize.
- Cell hover feedback is outline/opacity only. **Do not add a hover
  `transform` to cells**: transformed bounds of edge cells extend the
  scrollable overflow of the `overflowX: auto` container, making the scrollbar
  flicker and the grid jump under the cursor.

## Cache reads vs cache writes

Caching is two costs, not one. Reads are the discount; writes are billed at a
premium (Anthropic `cache_creation`, OpenAI `cache_write_tokens` since
gpt-5.6). The API exposes both: `cache_read_tokens` is reads only,
`cache_write_tokens` is writes.

**`cache_write_tokens` is a SUBSET of `total_input_tokens`, never an addend.**
This is the one thing to get right here — the backend folds the write cost
into input on purpose (see `.design/stream-usage-tracking.md` §2). So cache
writes must never become a fourth stacked series, a fourth donut slice, or a
term in any total; doing so double counts the same tokens and inflates every
figure on the page.

They surface as annotations on the Input figure instead:

- Chart tooltip: `Input Tokens: 12,570 (incl. 1,508 written)`
- Token Breakdown donut: `incl. 107K written` under the Input legend entry
- Cache Hit Rate card subtitle: `40M read · 4.8M written`
- Own column (`Cache Write`) in Usage by Model and the By Request table

Vocabulary: every previously-"Cache" label is now **Cache Read**, so the word
means one thing. The write column and the "written" annotations render only
when there is a non-zero write in the data — pre-gpt-5.6 models and Azure
never report writes, so an always-on column would be a permanent wall of
zeros. `chartStyles.ts` owns both readings of that policy: `hasCacheWrites`
for the tables, `formatCacheBreakdown` for the stat cards. A third surface
should read from there rather than re-derive it.

The Cache Hit Rate formula is unchanged: `read / (read + input)`. Since input
already contains the writes, a write correctly counts as a miss.

## Invariants / gotchas

- Backend timestamps are bound as server-local time strings by the SQLite
  driver and compared lexicographically; query bounds are converted via
  `.In(time.Local)` to match. Per-day aggregation scans pad the range by ±2h
  (`dstScanPad`) and guard with exact `date(timestamp) = ?`.
- `usage_daily` has no scenario/rule/status dimension — queries filtering on
  those always use the raw table. Extend the schema (and bump the rebuild
  condition in `ensureUsageDailySchema`) if those filters ever need the fast
  path.
- **Adding a summed column to `usage_daily` does not need a table DROP.**
  `usage_daily` and `usage_records` migrate together, so a column that is new
  in the aggregate is new in the source too — every historical record
  contributes 0, and AutoMigrate's zero-fill is already the correct value.
  Dropping would discard correct aggregates and force a full re-aggregation to
  reach the same zeros (`TestUpgradeAddingSummedColumnKeepsAggregates`). The
  rebuild in `ensureUsageDailySchema` is reserved for a layout change like v2's
  `user_id` dimension, or for a column whose SOURCE already carries historical
  non-zero data.
- Equivalence between the merged path and the raw path is locked in by
  `internal/data/db/usage_daily_test.go`; if you change either side, keep
  those tests green.
- On the frontend, parse `YYYY-MM-DD` strings as local time
  (`new Date(\`${d}T00:00:00\`)`) — a bare date string parses as UTC midnight
  and shifts the weekday/day in timezones behind UTC.
- `middleware.Gzip()` is JSON-only; never attach it to streaming/SSE routes.
