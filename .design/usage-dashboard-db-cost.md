# Usage dashboard: database cost

What the dashboard asks of `usage_records`, where the time went, and what was
changed. Companion to `usage-analytics.md` (product/data flow) and `db.md`
(store-level performance). Response shapes of `/usage/*` are unchanged except
one noted below.

## Measured

`internal/db/usage_dashboard_bench_test.go` seeds 300k records (100 days, ~3k
a day, skewed providers/models/users/agents, chronological ids like
production) and replays what the dashboard requests with its real windows
(local-midnight start, tomorrow-midnight end). Median of 7 runs, one machine,
SQLite via cgo; `queries` = gorm statements.

```
TB_DASH_REPORT=1 go test ./internal/db -run TestDashboardReport -v
go test ./internal/db -run '^$' -bench DashboardRead -benchtime 5x
```

| case | before | after | queries before → after |
|---|---|---|---|
| `GET /stats group_by=scenario` 30d (agent filter options, every load) | 157 ms | 18 ms | 1 → 3 |
| `GET /timeseries day` 365d (activity heatmap) | 72 ms | 31 ms | **531 → 3** |
| `GET /timeseries day` 30d + agent filter | 64 ms | 7 ms | 1 → 3 |
| `GET /usage/performance` 30d | 573 ms | 241 ms (0 ms on a cache hit) | 1 → 1 |
| `GET /stats group_by=model` 30d + agent filter | 152 ms | 101 ms | 1 → 1 |
| Summary tab load, 30d (stats + series + agents + performance) | 777 ms | 318 ms | 8 → 10 |
| unchanged: stats model/provider/user 30–90d, timeseries 30–90d, minute/today, records | 22–51 ms | same | 1–3 |
| cold first query after upgrade / fresh DB (90d window) | 690 ms | 1360 ms | 181 → 68 |

`INSERT` into `usage_records` (the proxy hot path): ~293 → ~254 µs/op
(`BenchmarkUsageStore_RecordUsage`, 6×5000, median).

## Decisions

**Quiet days were re-aggregated on every request.** `usage_daily` only had
rows for days with traffic, so "which days are missing" re-listed every empty
day and rebuilt it (delete + `INSERT … SELECT`, two statements each) per
request — 531 statements for the 365-day heatmap. A completed day with no
traffic now gets a zero-count marker row (`emptyDayMarker`); readers filter
`request_count > 0`. A day aggregated once — empty or not — is not revisited, so
a row inserted later with an old timestamp (import/backfill) is not picked up
until `DeleteOlderThan` or a schema rebuild clears the day. Missing days are built per contiguous run of ≤7 days with
one grouped scan, so the write lock is held for a week of data at a time, not
the window.

**Scenario is a companion table, not a `usage_daily` dimension.** Crossing
scenario with provider × model × user multiplied `usage_daily` rows ~3.6× on
the benchmark data and made the always-run model/provider/user queries 2.5×
slower to speed up one query. `usage_daily_scenario(date, scenario, sums)` is
a few rows a day and is written in the same transaction as the matching
`usage_daily` days. It serves `group_by=scenario` and the lone-scenario
filter; scenario combined with provider/model/user still scans raw rows.
Because days aggregated before the table existed would never reach it,
`ensureUsageDailySchema` drops `usage_daily` (derived data) when the companion
table is missing; it refills lazily. This is the cold-start cost above: two
grouped passes instead of one.

**Equality filters are kept off the indexes (`+col = ?`).** SQLite had no
`sqlite_stat1`, so it preferred any equality term over the timestamp range:
`status = 'success'` (95% of rows) or `scenario = …` walked the whole index
across all history and filtered by timestamp row by row. Writing the column as
`+col` removes the term from index consideration, so the range index decides
deterministically (pinned by `TestRangeQueriesUseTimestampIndex`). This is
`withPinnedRangeFilters`, used only where the whole range is read anyway
(aggregates, time series, percentiles). `GetRecords` — newest-first with a
`LIMIT` — keeps index-eligible filters (`withColumnFilters`) so a rare filter
over a wide window can still use `idx_provider_model`/`idx_user`/… `ANALYZE`
was rejected: it is a guess that goes stale as the table grows.

**Unused `usage_records` indexes dropped**: `idx_timestamp_scenario`,
`idx_scenario`, `idx_usage_records_status`,
`idx_usage_records_total_tokens`. Nothing reads through them, and each is paid
on every proxied request's INSERT. They are dropped from existing databases in
`migrateUsageTables` (AutoMigrate never removes). `idx_provider_model`,
`idx_user`, `idx_team`, `idx_rule` (rename), `idx_trace_id` stay; the first
three serve only `GetRecords` filters and are the next candidates if write
cost matters.

**Performance summary** streams four ints per row instead of materializing a
`UsageRecord` (gorm reflection dominated) and is cached for 15 s per
`(window, filters)`, with the window bounds truncated to the TTL so the
"today" window (ends at the current second) still hits — it re-reads every successful request in the window and
the dashboard re-asks it identically on reload/filter toggle/auto-refresh.
Percentiles do not move visibly in 15 s. Deleting or re-attributing records
resets the cache. Counts/sums are deliberately *not* cached: they are cheap
after the rollup, and the live "today" view should follow traffic.

## Behaviour changes to know about

- `group_by=scenario` now returns `provider_uuid`/`provider_name`/`model`/
  `user_id` empty. The raw path used to return an arbitrary row's values there
  (bare SQLite columns). The frontend reads `scenario`/`key` only.
- `GetPerformanceSummary` results can be up to 15 s old.
- Per-minute/hour raw scans (today, yesterday) are unchanged: still O(one
  day) and ~22 ms at 12k records.

## Not done

- Incrementally maintaining today's rollup at write time (extra upsert on the
  proxy hot path) or an hourly rollup (removes the remaining raw edge scans).
- A covering index for the raw edge scans.
- Batched/debounced usage writes (tracked in `db.md`).


## Hourly rollup (finest fold)

Providers that price by time of day (peak/valley) need usage per hour, so the
finest rollup is now **one hour**, not one day. Same 300k-record benchmark,
median of 7 runs (`usage_dashboard_bench_test.go`; rows with `hour` or
`today` are new cases):

| case | day-only rollup | + hourly rollup |
|---|---|---|
| `timeseries hour` 30d (peak/valley view, 720 buckets) | 191 ms | 39–44 ms |
| `timeseries hour` 7d | 56 ms | 9–10 ms |
| `timeseries hour` 30d, one provider | 93 ms | 26 ms |
| `stats group_by=hourly` 7d | 70 ms | 10 ms |
| `stats group_by=model` today | 23 ms | 4 ms |
| `stats group_by=provider` last 24 h (unaligned) | 22 ms | 4.6 ms |
| `stats group_by=model` 30d | 31 ms | 12 ms |
| `timeseries day` 30d / 90d / 365d | 22 / 29 / 31 ms | 6 / 14 / 16 ms |
| `stats group_by=scenario` 30d | 18 ms | 2.3 ms |
| Summary tab load, 30d | 335 ms | 253–265 ms (rest is the percentile scan) |
| cold first query, 90d window | 1.36 s | 2.77 s (153 statements) |

Unchanged: minute series, `group_by=model` + agent filter (needs
provider/model *and* scenario, so raw), records, percentiles.

Layout (all keyed by SQLite's UTC bucketing of the stored timestamp):

```
usage_hourly            (hour, provider, model, user)   finest
usage_hourly_scenario   (hour, scenario)
usage_daily             (date, provider, model, user)   = sum of the day's hours
usage_daily_scenario    (date, scenario)
```

A query window is covered by up to five segments (`planRollupSegments`):
raw partial leading hour → hours up to the first day boundary → whole days
(only when ≥ 2 of them) → hours after the last day boundary → raw trailing
partial hour. The raw edge scans that used to be up to a day long each are now
under an hour, which is where the "today"/"30d" gains come from.

Decisions:

- **Days are summed from hours, never from raw.** Totals are then identical
  whichever tiers serve a window, and a late row cannot make a day disagree
  with its hours. A day is built in one transaction that first builds its
  missing hours (`buildBuckets`), so a concurrent retention delete cannot
  interleave.
- **An hour is built once it has ended plus `hourAggGrace` (10 min).** Records
  are stamped when written (`RecordUsage`), so only a stalled write can land
  in an already-built hour; like days before it, a bucket is not revisited
  (`AggregateToDaily` rebuilds a day on demand). Day eligibility follows its
  last hour, so the old 1 h day grace is gone.
- **Empty hours get a marker row** (`request_count = 0`), as empty days
  already did; readers filter `request_count > 0`.
- **Hour keys are UTC.** Whole-hour offsets align exactly; for half-hour zones
  (India, Nepal, Lord Howe) the two partial hours at the window ends are read
  raw, which stays correct but is not rollup-served. A peak/valley partition
  defined in a zone is therefore a sum of `interval=hour` buckets by the
  caller; there is no new endpoint, so API shapes are unchanged.
- **`interval=hour` and `group_by=hourly` are served from the hour tier** and
  never from days (`planRollupSegments(..., hourOnly=true)`).

Costs to know:

- **Size.** At this synthetic density (~125 requests per hour across ~70
  provider×model×user groups) `usage_hourly` holds 151k rows for 300k records
  (day tier: 17k). Hour rows fold only when several requests share a
  (hour, provider, model, user); at low traffic the table is about as big as
  `usage_records` but then raw scans are cheap anyway. Retention deletes
  rollup rows together with records.
- **Cold start doubles** (the hour build precedes the day derivation) and is
  paid once per window, in runs of ≤ 7 days.
- **Mixed-offset timestamps.** `usage_records.timestamp` is text with the
  writer's UTC offset and ranges compare lexicographically; the build pads the
  scan by `dstScanPad` (2 h) for DST. A row written with a timestamp in
  another zone than the server's (an import) can be missed by the build and by
  raw scans alike; production writes use `time.Now()`.
