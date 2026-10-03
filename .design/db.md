# Database Layer (`internal/db`)

Design notes for `internal/db`'s stores — SQLite via gorm, one shared
`*gorm.DB` owned by `StoreManager`. Performance is the first topic below;
other sections (schema, migrations, ...) can be added here as they come up.

## Performance

Status: `ProviderStore` and `APITokenStore` (read-cache fixes) and
`StatsStore`/`UsageStore` (transaction-merge fix) shipped as three
independent PRs off this investigation. Batched/debounced stats+usage
writes remain open — tracked below as the holistic follow-up.

### Why this matters

`internal/routing/pipeline_bench_test.go` benchmarks the real production
selection pipeline (`ServiceSelector` + `protocolserver.LoadBalancer`, wired
exactly as `internal/server/server.go` does). Profiling it showed
`ServiceSelector.Select` spending **83% of its own time** in one line:

```go
provider, err := s.config.GetProviderByUUID(result.Service.Provider)
```

That call chain — `config.Config.GetProviderByUUID` → `db.ProviderStore.GetByUUID`
→ `gorm.DB.First` → SQLite via cgo — accounted for **47.5% of the entire
benchmark's CPU time** (`cgocall` alone was 20.6%; `database/sql` locking/scan
machinery another ~25%). `ProviderStore` additionally guarded every method,
reads included, with a plain `sync.Mutex` — so even *read* traffic across
concurrent requests fully serialized on one lock, on top of paying for a disk
round-trip that returns data changing only on admin CRUD.

This isn't a one-off: every routed request resolves exactly one provider by
UUID, so this cost was paid **once per LLM request**, unconditionally. The
same shape turned up in `APITokenStore.ValidateToken` (auth middleware) and
in `StatsStore`/`UsageStore`'s per-request writes — see below.

### `ProviderStore`

`internal/db/provider_store.go`'s `ProviderStore` keeps a write-through
in-memory mirror of the `providers` table:

- `cache map[string]*ProviderRecord` + an `order []string` slice (map
  iteration is randomized in Go; `order` keeps `List`/`ListOAuth`/`ListEnabled`
  stable in insertion order, matching what SQLite returned before and what the
  provider table UI depends on).
- `mu` changed from `sync.Mutex` to `sync.RWMutex`.
- All **read** methods serve from the cache under `RLock` — no SQLite call,
  no exclusive lock.
- All **write** methods write to SQLite first (still the source of truth /
  durability boundary), then update the cache entry, under `Lock`. Writes
  mutate a copy and only swap it into the cache once the SQLite write
  succeeds (`writeThroughLocked`), so a failed write can't leave the cache
  holding unpersisted data.
- The cache is loaded once at construction. Both `NewProviderStore` and
  `StoreManager.initProviderStore` funnel through one `newProviderStoreOverDB`
  helper for this, since `StoreManager` builds `ProviderStore` over its own
  already-open connection rather than calling `NewProviderStore` — two
  separate init paths that previously had to be kept in sync by hand (a bug
  once, caught by `TestStoreManager_StoreOperations`).

Single-process, single-DB-connection deployment (`StoreManager` owns the one
`*gorm.DB` shared by every store) makes this store the sole writer of its
table, so the mirror can't drift from concurrent external writers.

**Result** (same benchmark, before → after):

| Benchmark | before | after | allocs before → after |
|---|---|---|---|
| `BenchmarkSelect_Plain` | 59.7 µs/op | 5.8 µs/op (**10.2×**) | 236 → 76 |
| `BenchmarkSelect_SmartMatch` | 61.5 µs/op | 10.5 µs/op (**5.9×**) | 252 → 92 |
| `BenchmarkSelect_SmartNoMatch` | 66.9 µs/op | 9.7 µs/op (**6.9×**) | 251 → 91 |
| `BenchmarkSelect_Affinity` | 65.7 µs/op | 8.3 µs/op (**7.9×**) | 252 → 86 |

Re-profiling after the fix: `GetProviderByUUID` no longer appears in the
profile's top nodes at all. The new dominant cost (~27% cum) is
`loadbalance.ServiceID.String()` / `fmt.Sprintf` inside `HealthFilter.Filter`
(called twice per request — the double active/health filtering noted in
`pipeline_bench_test.go`) — a separate, smaller inefficiency, out of scope here.

### `APITokenStore`

`ValidateToken` is called by `AuthMiddleware.ModelAuthMiddleware` on every
request bearing a `tb-share-`-prefixed multi-tenant API token — the same
"every request, data barely ever changes" shape as `ProviderStore.GetByUUID`.
A benchmark was written first (`internal/middleware/auth_bench_test.go`,
wired against a real `db.APITokenStore`) and profiled before changing anything:

| Benchmark | before | after | change |
|---|---|---|---|
| `BenchmarkAPITokenStore_ValidateToken` (isolated store call) | 30.0 µs/op, 99 allocs | 119 ns/op, 1 alloc | **~251×** |
| `BenchmarkModelAuthMiddleware_APIToken` (full gin middleware chain) | 103.8 µs/op, 182 allocs | 55.4 µs/op, 80 allocs | **1.87×** |

The fix mirrors `ProviderStore`: a write-through `map[string]*APITokenRecord`
cache keyed by `TokenID`, `RWMutex`. Reads (`ValidateToken`, `GetToken`,
`ListTokens`) hit the cache; writes hit SQLite first, then update the cache
entry under the same write lock — so a revoked token stops validating the
instant `RevokeToken` returns. `CleanupExpiredTokens` (a bulk, predicate-based
delete) just reloads the whole cache from SQLite afterward rather than
re-deriving its `WHERE` clause in Go — not hot path, so the O(n) reload cost
doesn't matter.

**Why the full-middleware number only improved 1.87× despite `ValidateToken`
itself improving 251×**: the remaining cost is `APITokenStore.UpdateLastUsed`
— a genuine SQLite **write** on every authenticated request (fire-and-forget,
so it doesn't block the response, but it's real DB load), not a read this
caching pattern touches.

**`UpdateLastUsed`: debounce the write instead of caching a read.**
`last_used_at` only needs display-level freshness (token admin UI), not
per-request precision. `UpdateLastUsed` now persists at most once per
`defaultLastUsedDebounce` (1 minute) per token — checked cheaply under
`RLock` against the cached `LastUsedAt` before ever touching SQLite. No new
cache or flusher: the debounce state *is* the existing cache entry.

| Benchmark | cached only | + debounced `UpdateLastUsed` | change |
|---|---|---|---|
| `BenchmarkModelAuthMiddleware_APIToken` | 55.4 µs/op, 80 allocs | 1.39 µs/op, 10 allocs | **~40×** (≈75× vs. the original baseline) |

### `StatsStore` + `UsageStore`

An audit of every `internal/db/*_store.go` (see below) turned up two stores
on the request hot path despite looking like background bookkeeping:
`StatsStore.UpdateFromService` and `UsageStore.RecordUsage`, called
synchronously once per completed LLM request from
`ProtocolHandler.trackUsageWithTokenUsage` / `trackUsageFromContext`
(`internal/protocolserver/usage_tracking.go`). Unlike `ProviderStore` and
`APITokenStore`, both are genuine per-request **writes** with no prior read —
the read-cache pattern above doesn't apply.

| Benchmark | Result |
|---|---|
| `BenchmarkStatsStore_UpdateFromService` (upsert) | ~80 µs/op, 129 allocs |
| `BenchmarkUsageStore_RecordUsage` (insert) | ~307 µs/op, 117 allocs |
| `BenchmarkStatsAndUsage_Combined` (both, separate stores) | ~440 µs/op, 246 allocs |

pprof showed both dominated by gorm's implicit transaction wrapper
(`BEGIN`/`COMMIT`) around each write; `UsageStore.RecordUsage` costs ~4×
`StatsStore.UpdateFromService` because `usage_records` has 6 indexed columns
to maintain per insert against `service_stats`' single composite primary key.

Two directions were considered:

- **`PRAGMA synchronous=NORMAL`** — investigated, implemented, benchmarked,
  and **reverted**: read back as `1` (NORMAL) both before and after the
  change. The driver (`mattn/go-sqlite3`) already forces `synchronous=NORMAL`
  whenever `_journal_mode=WAL` is set (every store here already sets that),
  so the DSN change was a no-op — confirmed the pragma itself matters
  (isolated raw-driver test: 373 µs/op FULL vs. 26 µs/op NORMAL, 14×) but
  that headroom was never available here. Lesson: read back the actual
  `PRAGMA` value before crediting a DSN change with a win.

- **Merge the two writes into one transaction** (`db.RecordRequestOutcome`,
  `internal/db/record_outcome.go`) — implemented. `UpdateFromService` and
  `RecordUsage` were split into a pure record-builder and the DB write, so
  `RecordRequestOutcome` builds both records (outside any lock) and saves
  them inside one `gorm.DB.Transaction(...)`, assuming both stores share the
  same `*gorm.DB` (always true via `StoreManager`).

  | Benchmark | separate transactions | merged transaction | change |
  |---|---|---|---|
  | `BenchmarkStatsAndUsage_Combined` vs. `_RecordRequestOutcome` | ~440 µs/op, 246 allocs | ~420 µs/op, 235 allocs | **~4–8%**, 11 fewer allocs/op |

  Confirmed via pprof the merge is real (`gorm.(*DB).Commit` appears once
  per call, not twice), but the win is modest: most of the ~440 µs isn't
  transaction-wrapper overhead, it's the two per-row statements themselves
  — removing one `COMMIT` doesn't remove either statement. Kept anyway: the
  win is real, and it adds a correctness improvement — the stats and usage
  rows for one request can no longer partially persist
  (`TestRecordRequestOutcome_AtomicRollback`).

**Still open**: batching/debouncing multiple requests' stats and usage
writes into fewer, larger transactions — the actual lever for the remaining
per-statement cost, at the cost of async durability (a crash loses whatever's
still buffered) and added complexity (buffer, flush trigger, backpressure).

**Fixed in passing**: `NewUsageStore` never created the `db` subdirectory
`constant.GetDBFile` expects (every other `NewXStore` constructor does).
Never hit in production since `StoreManager` builds `UsageStore` directly
over its own already-open `*gorm.DB` — surfaced when
`stats_usage_bench_test.go` became `NewUsageStore`'s first-ever caller.

### Audit: is your store on a hot path?

Every store under `internal/db/` follows the same shape: a GORM-backed
SQLite table, one `sync.Mutex` (not `RWMutex`) per store, every method —
reads included — taking the exclusive lock and issuing a query. Whether
that matters depends on whether the store is read/written on the **request
hot path** vs. an **admin/UI path** or the lower-QPS **IM-bot/remote-control
message path**:

| Store | Hot-path? | Where | Status |
|---|---|---|---|
| `ProviderStore` | Yes (read) | `ServiceSelector.Select` resolves a provider every request | Fixed — read cache |
| `APITokenStore` | Yes (read + write) | `AuthMiddleware.ModelAuthMiddleware` validates every API-token request, updates `last_used_at` | Fixed — read cache + debounced write |
| `StatsStore`, `UsageStore` | Yes (write) | `usage_tracking.go` persists stats + a usage row every completed request | Partially fixed — merged transaction; batching still open |
| `ImBotSettingsStore` | No | Bot lifecycle events, admin REST/CLI only | Not touched — genuinely low frequency |
| `ModelStore` | No | Admin "list/refresh models" endpoints, OAuth completion, CLI | Not touched — already `RWMutex`, admin-paced |
| `TaskStore` | Dead code | Zero callers; `internal/task` also has no external production callers (see `backend-organization.md`), so it is not a live replacement subsystem | Not touched — separate cleanup ticket |
| `ToolConfigStore` | Effectively unreachable | `Config.GetToolConfig` is a different, in-memory-only implementation that never touches this store | Not touched — separate cleanup ticket |
| `RemoteChatStore`, `RemoteSessionStore`, `BotAccessStore` | IM-bot message path | `remote/control/remoteagent/handler_message.go` et al. | Not touched — real traffic but chat-message-rate, not LLM-API-rate; no app-level mutex today |

### Recommended approach for future stores

1. **Classify first**: is this store read on the per-request hot path? If
   not, don't bother caching — `sync.Mutex` → `sync.RWMutex` is enough, and
   only worth doing if profiling shows contention.
2. **For hot-path reads**: write-through cache keyed by the natural lookup
   key, `RWMutex`, writes go to SQLite first then update the cache under the
   same write lock. Don't invent a TTL/invalidation scheme — these stores
   are single-writer-per-process, so "cache is only ever mutated by this
   store's own write methods" is sufficient.
3. **Watch for bypass construction**: `StoreManager` builds several stores
   via struct literals instead of calling each store's `NewXStore`
   constructor. Any cached store needs a shared `newXStoreOverDB(db, dbPath)`
   helper that both `NewXStore` and `StoreManager.initXStore` call, so the
   cache-init wiring can't drift between two hand-duplicated copies.
4. **Write the benchmark first**, against the real production wiring, and
   profile before touching any code — don't reason about the win from first
   principles; measure it.

## Process model: one process opens tingly.db

Status: shipped with the fix for #1912.

Every store assumes it is the **sole writer of its table** (that is what
makes the write-through caches above sound). The deployment invariant behind
it is stronger than "one `StoreManager` per process": **only the server
process should have tingly.db open at all.** SQLite in WAL mode coordinates
concurrent processes through `fcntl` locks and an mmapped `tingly.db-shm`
index, and both are exactly what Docker Desktop's host-directory bind mounts
(virtiofs / gRPC-FUSE on macOS and Windows) fail to honour across processes.

The invariant was being broken by accident. `cli/tingly-box/main.go` built an
`AppManager` for **every** subcommand before dispatching — and
`appconfig.NewAppConfig` → `config.NewConfig` → `db.NewStoreManager` opens the
database and runs every store's `AutoMigrate` (plus the deprecated-table
`DROP`, legacy-JSON imports and config migrations). So `tingly-box version`,
which Docker's `HEALTHCHECK` ran every 30s for the life of the container, was
a second, short-lived writer racing the server. A user on Docker Desktop for
macOS with a bind-mounted data directory hit `database disk image is
malformed` in `usage_records` / `idx_provider_model` right after an upgrade,
when both processes were also migrating schema (#1912); the pre-upgrade
backup passed `integrity_check`, and nothing in `internal/db` had changed
between the two releases.

Rules that follow:

1. **A CLI process opens the database only when a command asks for it.**
   `app.AppManager` builds `AppConfig` lazily, on the first
   `AppConfig()` / `GetGlobalConfig()` call; `ConfigDir()` and
   `GetRuntimeServerPort()` never do. Commands that only report on the
   binary (`version`, `mcp-builtin`) take no `*app.AppManager` at all, and
   commands that talk to a running server (`stop`, `log`, `open`) use the
   lock, the port file and `appconfig.UserTokenFromFile`.
   `TestCommandsThatMustNotOpenTheDatabase` (`cli/tingly-box/main_test.go`)
   asserts no database appears after running them. The per-command table
   is in `.design/cli-entry-semantics.md`.
2. **Container health checks probe the server over HTTP**
   (`/api/v1/info/health`, unauthenticated), never a CLI subcommand — the
   diagnostic should traverse the real path (`.design/ux-principles.md`),
   and a CLI process is a database client.
3. **Docs steer Docker Desktop users to a named volume** (`docs/docker.md`).
   Linux bind mounts are local directories and are fine.

Not done, deliberately: a startup `PRAGMA quick_check`. It is O(database)
on every boot, and it would report a corruption it cannot prevent; the
prevention is the process model above. The other CLI subcommands
(`status`, `provider …`, `rule …`, `quota …`) still open the database — that
is what they are for — and remain fine on any filesystem SQLite supports;
against a live server on a Docker Desktop bind mount they carry the same
risk the health check did, which the docs now say.

## GORM feature adoption

Status: **evaluation only** — nothing in this section is implemented yet.
After the performance rounds above, the remaining question is where the
stores still hand-roll something GORM already does. Every claim below was
checked against gorm v1.31.2 / gorm.io/driver/sqlite v1.6.0 with throwaway
probe tests and benchmarks (deleted after the numbers were taken), not read
off the GORM docs.

The short version: **one clear win (`serializer:json`), two small ones
(query `Scopes`, an N+1 fix), and four features worth explicitly *not*
adopting.** The "not" list matters as much as the "yes" list — the stores
hand-roll several things on purpose, and that intent isn't obvious from the
code.

### Yes: `serializer:json` for the JSON-in-TEXT columns

Eight columns across five stores hold JSON in a `string` field, marshalled
and unmarshalled by hand at **27 call sites**:

| Store | Columns | Sites |
|---|---|---|
| `provider_store.go` | `tags`, `oauth_extra_fields`, `vmodel_detail`, `credential` | 14 |
| `imbot_settings_store.go` | `auth_config`, `bash_allowlist` | 6 |
| `provider_model.go` | `models` | 4 |
| `remote_chat_store.go` | `project_history` | 2 |
| `bot_access_store.go` | `event_filter` | 1 |

GORM's `serializer:json` tag replaces all of it — the field becomes its real
type (`[]string`, `map[string]string`, `*typ.VModelDetail`) and GORM
marshals on write / unmarshals on read:

```go
Tags []string `gorm:"column:tags;type:text;serializer:json"`
```

The bigger win isn't line count, it's **error handling**. All 14 of
`provider_store.go`'s sites discard the JSON error, in three flavours: two
bare unmarshals with no check at all (`:93`, `:107`), two that check and then
silently fall through leaving the field nil (`:112`, `:119`), and ten writes
that swallow it with `_` (`:163`, `:174`, `:180`, `:185`, `:214`, `:230`,
`:238`, `:252`, `:504`, `:529`). A provider whose tags fail to marshal is
silently persisted with no tags. Under the serializer, the failure surfaces
through the normal `.Error` path.

**Reads migrate cleanly — verified.** The concern is legacy rows: the
current code writes `""` for "absent", which is not valid JSON.
`JSONSerializer.Scan` guards on `len(bytes) > 0`, so a probe reading
existing `""` rows through a serializer model returned a nil slice with no
error. No backfill needed.

**Writes change the on-disk representation — verified, and this is the trap.**
Probe results writing through the serializer:

| Go value | column before | column after |
|---|---|---|
| `nil` | `""` | `NULL` |
| `[]string{}` | `""` | `"[]"` |
| `[]string{"x"}` | `"[\"x\"]"` | `"[\"x\"]"` |

So **every predicate over a serialized column has to be re-audited** — but
re-audited, not reflexively rewritten. `ModelStore.GetAllProviders`
(`provider_model.go:225`) is the one that looked broken:

```go
ms.db.Where("models <> ''").Find(&records)
```

On inspection it survives the encoding change untouched. SQL's three-valued
logic excludes `NULL` from `<> ''` already (comparing `NULL` to anything
yields `NULL`, not true), and `NULL` is exactly the new spelling of "absent"
— so the rows it drops are the rows it should drop. A stored-but-empty
`"[]"` still matches, as it did before. Verified by running both
`models <> ''` and `models IS NOT NULL AND models <> ''` against a table
holding all four values: identical result sets.

The check that genuinely does break is the **Go-side** one. After decoding,
a stored `"[]"` and a never-written column are both an empty slice, so
`record.Models == ""` (`GetProviderInfo`) can no longer tell them apart and
needs a different discriminator.

**Scope it to the four AutoMigrate-owned stores.** `bot_access_store.go`
declares its tables with hand-written DDL where the JSON columns are
`JSON NOT NULL DEFAULT '{}'`. `JSONSerializer.Value` returns SQL `NULL` for a
nil value unless `NOT NULL` is in the *GORM tag* — and here the constraint
lives in the DDL, which GORM can't see. A nil `Config` would hit the NOT NULL
constraint at write time. Leave `bot_access_store`'s `rawJSONOrObject` helper
alone; it exists for this reason.

#### Outcome

Done for **`provider_store`** (tags, oauth_extra_fields, vmodel_detail,
credential), **`provider_model`** (models) and **`imbot_settings_store`**
(auth_config, bash_allowlist). Three things came out of doing it that the
evaluation above did not anticipate:

**1. It is a hot-path performance fix, not just a tidy-up.**
`ProviderStore.GetByUUID` is served from the write-through cache, so its cost
is whatever `toProvider()` does — which included decoding the JSON columns on
every call, i.e. once per routed LLM request. Removing that:

| Benchmark | before | after |
|---|---|---|
| `GetByUUID_Tagged` | 1637–1901 ns/op, 11 allocs | 369–386 ns/op, 2 allocs (**~4.5×**) |
| `GetByUUID_OAuth` | 2671–2922 ns/op, 21 allocs | 718–838 ns/op, 5 allocs (**~3.5×**) |
| `GetByUUID_NoJSON` (control) | ~253 ns/op | ~245 ns/op |

**2. `Updates(map[string]any)` silently bypasses serializers.** GORM applies
a field's serializer only when the value comes off the *model*. A map value
reaches the driver raw: passing a `[]string` fails with SQLite's
"row value misused", and — worse — an empty `[]string` **writes NULL with no
error at all**. Every store whose write path used `Updates(map)` on a column
being converted had to move to read-modify-write (`provider_model.saveModels`,
`imbot_settings_store.UpdateSettings`). This is the single biggest trap in
this migration: the failure is silent in exactly the case (empty collection)
that a test is least likely to cover.

**3. The string encoding was providing cache isolation by accident.**
Unmarshalling allocated a fresh value per read, so no caller shared state
with a cached record. `ProviderStore` caches `*ProviderRecord`, and the OAuth
callback handler mutates `provider.OAuthDetail.ExtraFields` in place on a
provider it read from the store — which, with typed fields, would write
straight into the cache and persist even if the following `UpdateProvider`
failed. Explicit clone helpers restore the isolation; `TestProviderStore_CacheIsolation`
pins it. **Any cached store converted in future needs the same treatment.**

#### Not done: `remote_chat_store.project_history`

Deliberately left as hand-marshalled, reversing the evaluation above.

The serializer turns a decode failure into a failed *query*. For provider
config that is the right trade — silently loading a provider with no
credentials is worse than a loud error. For `project_history` it is not:
`decodeProjectHistory` deliberately logs and degrades to empty, and the
column holds a best-effort MRU path list, not anything the bot needs to
function.

Hard-failing the read would also make the row **unrecoverable through the
store**. `GetChat` gates the IM-bot message path, and every write path
(`GetOrCreateChat` → `mutate` → `UpdateChat`) reads before it writes — so a
column that fails to decode could no longer be overwritten by the next
`BindProject`, where today it is simply replaced. Trading that for the
removal of two helper functions is a bad deal.

### Yes: `Scopes` for the repeated usage filter blocks

`usage_record.go` rebuilds the same time-range + filter-map `WHERE` block
four times — `rawAggBuckets` (`:324`), `rawTimeSeries` (`:456`),
`GetRecords` (`:531`), `GetPerformanceSummary` (`:585`). A pair of
`func(*gorm.DB) *gorm.DB` scopes collapses them.

It also closes a latent hole. All four do:

```go
for key, value := range filters {
    q = q.Where(key+" = ?", value)
}
```

The map *key* is interpolated into SQL. This is **not** exploitable today —
every caller in `internal/server/module/usage/handler.go` builds the map with
hardcoded keys (`provider_uuid`, `model`, `scenario`, `status`, `user_id`)
and only takes the *value* from the request. But the store has no way to
enforce that, and the next caller won't know. A scope that whitelists the
allowed columns makes it structurally safe rather than safe-by-convention.

### Yes: the `ListGroupActors` N+1 — but with `IN`, not associations

`bot_access_store.go:646-655` issues two queries per binding inside the loop
(one `remote_actors` lookup, one `group_actor_permissions` lookup), so
listing a group with 20 actors costs 41 queries. Two batched `Find`s with
`IN (...)` plus a group-by in Go fixes it.

**Do not reach for `Preload`/associations for this.** It would mean adding
`has-many`/`belongs-to` tags to records whose keys are composite —
`group_actor_permissions` is keyed on `(group_id, actor_id, capability,
action)` and referenced by a composite foreign key. Composite-key
associations are the weakest part of GORM's association support, and the
tags would have to agree with DDL that GORM doesn't generate. The batched
query is less code and no new coupling.

### No: `PrepareStmt` — measured, no win

The obvious candidate for the "batching still open" item above. Benchmarked
both ways on the real `RecordRequestOutcome` write path and a
`GetRecords` read, 300 iterations × 3 runs:

| Benchmark | `PrepareStmt: false` | `PrepareStmt: true` |
|---|---|---|
| stats+usage write | 365–396 µs/op, 235 allocs | 357–370 µs/op, 250 allocs |
| `GetRecords` read | 990 µs–1.12 ms/op, 2217 allocs | 986 µs–1.04 ms/op, 2151 allocs |

Both deltas sit inside run-to-run variance, and the write path *gains* 15
allocs/op. Consistent with the profiling above: the cost is statement
execution and index maintenance, not SQL parsing, so caching the parse
buys nothing. It would also add a per-connection statement cache that has to
be invalidated across migrations. Not worth it.

### No: replacing `bot_access_store`'s hand-written DDL with AutoMigrate

The nine `CREATE TABLE` statements in `migrateBotAccessTables`
(`bot_access_store.go:153`) carry things AutoMigrate cannot produce on
SQLite:

- `CHECK(effect IN ('allow','deny'))` on five tables
- `CHECK((direct_chat_id IS NOT NULL) != (group_id IS NOT NULL))` — the
  route XOR invariant
- a **composite** foreign key: `FOREIGN KEY(group_id, actor_id) REFERENCES
  remote_group_actors(group_id, actor_id) ON DELETE CASCADE`
- `ON DELETE CASCADE` throughout

SQLite cannot `ALTER TABLE ADD CONSTRAINT`, so these can only be declared at
`CREATE TABLE` time. Converting to AutoMigrate would silently drop the
database-level enforcement of the access model's invariants. Keep the DDL.

### No: the generics API (`gorm.G[T]`)

v1.31.2 ships `gorm.G[T](db).Where(...).Find(ctx)`, which gives type-safe
results and a mandatory `context.Context`. But the existing code already gets
its type safety from `Find(&typedSlice)`, so the only real gain is the forced
ctx — and adopting it means rewriting every call site in the package. Revisit
only if the context work below happens anyway.

### No: soft delete (`gorm.DeletedAt`)

Nothing here wants tombstones. `usage_records` has an explicit retention
path (`DeleteOlderThan`), and the access tables depend on `ON DELETE CASCADE`
actually removing rows — soft delete would break the cascade semantics the
DDL relies on.

### Adjacent, bigger than "ORM features": context propagation

Worth recording because it kept surfacing during this evaluation.
`bot_access_store` threads `context.Context` into GORM at 37 call sites.
**Every other store in the package ignores context entirely** — 0
`WithContext` calls across `api_token_store`, `imbot_settings_store`,
`provider_model`, `provider_store`, `remote_chat_store`,
`remote_session_store`, `service_stat`, `usage_daily`, `usage_record`. A
cancelled or timed-out HTTP request cannot cancel the DB work it started.

This is the highest-value item found, but it is not a local refactor: it
changes public method signatures across the package and every caller. It
belongs in its own ticket, not bundled with a codec change.

### Suggested order

All three are done. Context propagation (below) is deliberately left for
its own change.

1. ~~`serializer:json`, one store per commit, each re-auditing that store's
   `WHERE` clauses over the converted columns.~~ Done for `provider_store`,
   `provider_model` and `imbot_settings_store`; `remote_chat_store`
   deliberately skipped. See **Outcome** above.
2. ~~`usage_record` filter scopes + the column whitelist.~~ Done. Six
   rebuilt-by-hand `WHERE` blocks (four in `usage_record.go`, two in
   `usage_daily.go`) collapsed to `withinTimeRange` + `withColumnFilters`,
   with the typed `UsageStatsQuery` and the map form rendering to the same
   columns. Filter keys are validated against a closed column set — a
   smaller one for `usage_daily`, which has fewer dimensions — and an
   unknown key is an **error, not a dropped filter**: dropping one widens
   the result set, and for `user_id` that means returning another user's
   records. Keys are also sorted, so the generated SQL no longer varies
   with Go's randomized map iteration.
3. ~~The `ListGroupActors` `IN` batching.~~ Done: 2N+2 → 4 queries,
   measured 42 → 4 at N=20. The test asserts the count via a counting gorm
   logger rather than trusting the shape by inspection, so a future
   per-actor query is caught. Associations were not used, for the
   composite-key reasons above.

Not recommended as part of this: removing the manual `CreatedAt`/`UpdatedAt`
assignments. GORM does fill them (probed: `Create` sets both; `Updates(map)`,
`Update(col, val)` and `OnConflict{UpdateAll: true}` all bump `updated_at`
and the last preserves `created_at`) — so roughly 15 assignments are indeed
redundant. But `OnConflict` with an explicit `clause.AssignmentColumns` list
does **not** auto-bump (probed), which is why `bot_access_store.go:226` must
keep listing `updated_at` by hand; and `remote_chat_store.normalizeChat`
deliberately stamps UTC where GORM would use local time. The cleanup is
small, the ways to get it subtly wrong are not.
