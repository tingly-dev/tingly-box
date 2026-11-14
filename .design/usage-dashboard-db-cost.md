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
`request_count > 0`. Missing days are built per contiguous run of ≤7 days with
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
deterministically (pinned by `TestRangeQueriesUseTimestampIndex`). `ANALYZE`
was rejected: it is a guess that goes stale as the table grows.

**Unused `usage_records` indexes dropped**: `idx_timestamp_scenario`,
`idx_scenario`, `idx_usage_records_status`,
`idx_usage_records_total_tokens`. Nothing reads through them, and each is paid
on every proxied request's INSERT. They are dropped from existing databases in
`migrateUsageTables` (AutoMigrate never removes). `idx_provider_model`,
`idx_user`, `idx_team`, `idx_rule` (rename), `idx_trace_id` stay; the first
three are now unused by the stats filters too and are the next candidates if
write cost matters.

**Performance summary** streams four ints per row instead of materializing a
`UsageRecord` (gorm reflection dominated) and is cached for 15 s per
`(window, filters)` — it re-reads every successful request in the window and
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
