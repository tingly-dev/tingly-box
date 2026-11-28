# Time and timezones

One rule, split three ways by what a time value *means*. Getting this wrong is
not cosmetic: #2797 (empty dashboard right after local midnight) came from
mixing all three.

## The three kinds of time

- **Instant** — a point in time: request timestamps, event `created_at`,
  `expires_at`, `last_used`. Store as UTC, compare as UTC, serialize as RFC 3339
  with `Z`/offset. No exceptions.
- **Business day** — "today/yesterday", daily rollups, day-quota reset, daily
  extrema. This is *not* an instant; it is a calendar day in a specific zone.
  Always carry an **explicit** zone (the user's or a configured one). Never
  assume UTC, never use `time.Local`.
- **Display** — the outermost layer only. Convert to the browser/`Intl` zone
  when rendering; keep it out of storage and comparison.

"Everything in UTC" is right for instants and wrong for business days: a UTC
"today" resets a UTC+8 user's daily quota at 08:00 local.

## The failure this norm prevents (#2797)

Three implicit assumptions compounded:

1. Writes used `time.Now()` — the **server's local** offset.
2. SQLite stores `time.Time` as TEXT and range predicates compare it
   **lexicographically**, so differing offsets misorder even when instants are
   in order.
3. The dashboard's window came from the **browser's** zone.

It looked fine only when server TZ == browser TZ. A `TZ=UTC` container plus a
`+08:00` browser dropped every row after local midnight. The catch-all lesson:
**the server's local zone must never be an implicit input.**

## Rules

- Normalize every persisted instant to UTC at the write path, through the
  store helper — not ad-hoc `time.Now()` in business code.
- **No bare `>=`/`<=`/`<`/`>` on a TEXT time column.** Use `unixepoch(col)` for
  comparisons/ordering (see `ai/quota`), or store an INTEGER epoch. If TEXT is
  kept, write one format only: `…+00:00`.
- `_loc=UTC` in the SQLite DSN does **not** fix binding: mattn/go-sqlite3
  formats a bound `time.Time` with that value's own Location; `_loc` only
  affects how values are parsed back on scan.
- Compute business-day boundaries from an explicit zone and thread that choice
  through grouping (rollups included). `date(timestamp)` is UTC; that is not the
  user's day.
- APIs emit RFC 3339 with `Z`/offset; never a bare `YYYY-MM-DD HH:MM:SS`.
- Change the time contract only with an **idempotent, in-place** normalization
  migration over legacy rows; never a destructive rewrite.

## Where this is enforced today

- [internal/db/usage_record.go](../internal/db/usage_record.go):
  `prepareUsageRecord` (UTC writes), `withinTimeRange` (UTC bounds),
  `normalizeUsageTimestampsToUTC` (legacy migration).
- [internal/db/usage_rollup_plan.go](../internal/db/usage_rollup_plan.go):
  rollup bucket keys are UTC.
- [internal/server/module/usage/handler.go](../internal/server/module/usage/handler.go):
  timestamps serialized as UTC RFC 3339.
- [ai/quota/store.go](../ai/quota/store.go): `unixepoch(fetched_at)` range and
  order predicates — the pattern to copy for TEXT time columns.
- [ai/quota/history_compaction.go](../ai/quota/history_compaction.go): daily
  extrema grouped by an explicit `time.Local` day (a business-day choice, kept
  self-consistent on both read and write).

## Follow-ups (not in #2801)

- CI guards: forbid `time.Local` in grouping/boundary code and bare comparisons
  on TEXT time columns; assert timestamps in API responses carry `Z`/offset; run
  key packages under a `TZ=UTC / Asia/Shanghai / America/Los_Angeles` matrix.
- Per-zone day bucketing for multi-day usage (`tz` threaded through the rollup
  tiers), so `interval=day` follows the user's day.
- Audit residuals: `thread_events.created_at` (write UTC like `updated_at`),
  `ai/quota` `expires_at < time.Now()` (use `unixepoch`), `bot_capabilities`
  seed rows (`CURRENT_TIMESTAMP` vs `…+00:00`).
