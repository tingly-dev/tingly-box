# Routing limits: account concurrency & usage operators

Status: **implemented** (backend, API, UI, smart-routing positions). Code: `ai/limits.go`, `internal/limits/`, `internal/routing/{limits,stage_limits}.go`, `internal/protocolserver/limits.go`, `internal/server/{limits.go,module/limits/}`, frontend `src/components/limits/`. Research: [`.research/routing-concurrency-usage-operators.md`](../.research/routing-concurrency-usage-operators.md).
Related: [tier-routing.md](./tier-routing.md) (pipeline, breaker, affinity), [team.md](./team.md) (Team scopes),
[quota-semantics.md](./quota-semantics.md) (upstream quota), [usage-tracking.md](./usage-tracking.md),
[rule-storage.md](./rule-storage.md).

## 1. Goal and non-goals

Control routing by two signals that nothing in the gateway reacts to today:

1. **Account concurrency** — cap in-flight requests per upstream account (provider = one API key / OAuth
   login). A saturated account is skipped; if every eligible account is saturated the request is queued
   briefly, rejected with 429, or let through (configurable).
2. **Usage** — cap requests/tokens per sliding window and/or stop using an account whose upstream
   `ai/quota` is nearly exhausted; optionally *steer away early* ("soft" threshold) so traffic shifts
   before the hard stop.

Generic: usable in any rule / scope. Teams are the main consumer but the routing layer has no Team concept —
a Team scope is just rules (`team.md`), so limits compose with it for free (§5).

Non-goals: billing/credit enforcement for sharing keys (per-key budgets), cross-instance coordination,
per-user fairness inside a Team, replacing the breaker/health mechanisms (they stay the *reactive* layer;
limits are the *proactive* one).

## 2. Concepts

**Limits** (`ai.Limits`, one struct, JSON) — what an entity may do. Attached at two places:

| Attach point | Key | Meaning |
|---|---|---|
| `Provider.Limits` | `acct:<providerUUID>` | The account's *physical* cap. Shared by every rule, scope and Team that routes to it. |
| `Service.Limits` (inside a rule) | `svc:<ruleUUID>/<providerUUID>/<model>` | *This rule's share* of the account. In a Team scope: the Team's share. |

A candidate service is subject to **both** (all-or-nothing acquire).

```jsonc
{
  "max_concurrent": 4,        // in-flight requests; 0 = unlimited
  "window_seconds": 18000,    // sliding window for the two budgets below (default 3600 when a budget is set)
  "max_requests": 0,          // per window; 0 = unlimited
  "max_tokens": 0,            // input+output tokens per window; 0 = unlimited
  "soft_percent": 80,         // steer away at >= this % of any limit; 0 = off (only hard stop)
  "quota_percent": 95         // provider limits only: stop at >= this % of upstream ai/quota; 0 = off
}
```

**Pressure** = the largest used/limit ratio over all dimensions of all subjects of a candidate. A candidate is
`blocked` at ratio ≥ 1 (or quota ≥ `quota_percent`), `hot` at ratio ≥ `soft_percent`, otherwise `ok`.

**Overflow policy** — what to do when *every* candidate is blocked. A property of the pool decision, so it
is a **rule flag**, not part of Limits: `limit_overflow` ∈ `queue` (default) | `reject` | `ignore`, and
`limit_queue_timeout` (seconds, default 15). `queue` waits only if at least one candidate is blocked *purely
by concurrency* (a usage window will not free up in seconds; waiting there only delays the 429).
`reject` → HTTP 429 `rate_limit_error` + `Retry-After`. `ignore` → proceed as if no limits (counters still
move, caps may be exceeded) — the old "degrade, don't disappear" behaviour, opt-in.

**Operator** = a pipeline unit. Two are added:

- `LimitStage` (`internal/routing/stage_limits.go`) — *filter operator*. Position: after SmartRouting, before
  Affinity (§3). Removes blocked candidates, prefers `ok` over `hot`.
- `Permit` / `Lease` (`internal/limits`) — *lifecycle operator*. Acquired after the final pick, released when
  the request ends, re-acquired on failover.

And two smart-routing positions (`service_concurrency`, `service_usage`) so the same signals can be combined
with content conditions through the existing op UI (§7).

## 3. Where it plugs in

```
Health → SmartRouting → LimitStage → Affinity → LoadBalancer      (select)
                                         │
SimpleSelector.SelectService:  loop { Select; TryAcquire(permit) ; on race-loss exclude + retry }
                                         │
limitsMiddleware (per request):  defer lease.ReleaseAll()           (release; covers stream/cancel/panic)
failover (selectFallbackService): release failed attempt's permit, filter + acquire for the next one
trackUsage*(…):                  registry.Record(tokens)           (usage window)
```

Why *after* SmartRouting: it already narrowed to the content partition; limits then act inside the chosen
scope. Running before it would let a saturated partition silently fall through to the base pool.
Why *before* Affinity: a pinned account that is blocked must not be honoured (`ContainsService` fails →
pin declined → re-selected; the re-lock follows the diversion, TTL bounds it). Why a *stage* rather than a
tactic: it composes with every tactic including `tier` (T0 saturated → narrowed set contains only T1 → tier
tactic picks T1; T0 resumes the moment a slot frees — no breaker, no oscillation).

Stage is read-only (`Assess`); consumption happens once, after the terminal pick, in `SimpleSelector`, so
diagnostics that drive `ServiceSelector.Select` directly (LB simulator) never consume permits. The probe-pin
bypass skips limits.

**Atomicity.** Assess and acquire are separate, so two requests can both see the last free slot. `TryAcquire`
re-checks under the registry lock; the loser adds the service to `ctx.LimitExcluded` and re-runs selection
(bounded by number of services; queue wait budget is shared across retries).

**Failover** bypasses the pipeline (`selectFallbackService`). It now (a) releases the failed attempt's permit,
(b) filters the remaining services with the same assessment (blocked removed, ok before hot, no queueing —
the client is already waiting), (c) acquires for the chosen one; none left → normal "failover exhausted".

## 4. State, lifecycle, failure modes

State is **process-local, in memory** (`limits.Registry`), like breaker/health/affinity. One mutex guards all
counters; operations are O(#subjects) and are dwarfed by LLM latency.

- **In-flight counter**: incremented on acquire, decremented by `Permit.Release` (idempotent). A request
  holds one permit per attempt; `Lease` (stored in the gin context) owns them and `limitsMiddleware` releases
  everything on handler return. Handlers are synchronous with the stream, so return == stream fully drained
  or client gone (upstream calls use the request context). `defer` covers panics.
- **Leak safety net**: every permit records `acquiredAt`; a sweeper (every 30 s) force-releases permits older
  than `max_hold` (default 2 h, a bug/hang guard, not a feature) and logs a warning. A late `Release` on a
  reaped permit is a no-op (no double decrement).
- **Cancelled / failed / streaming requests**: permit released at handler return in all cases; the usage
  window already counted the request at admission, tokens are added at completion (partial streams record
  what was produced). Queued waiters leave on request-context cancel.
- **Restart**: in-flight resets to 0 (their connections died with the process — correct). Usage windows are
  **warm-started** from `usage_records` (bucketed by timestamp) so a restart does not hand out a fresh
  budget. A window that was never configured has nothing to seed.
- **Config reload / edits**: specs are read from the live provider/service on every assessment; only the
  counters are keyed state. Raising/lowering a cap applies immediately; changing `window_seconds` rebuilds
  that key's window (re-seeded from DB on the next sweep tick is *not* done — it restarts empty; acceptable,
  documented).
- **Multiple instances**: caps are per instance (effective cap = N × instances). Sharing counters would need
  an external store; the `Registry` is the single seam where that could be injected. Not built (YAGNI; the
  breaker has the same property by decision in `tier-routing.md`).
- **Token budgets are eventually enforced**: tokens are known only at completion, so concurrent in-flight
  requests can overshoot `max_tokens` by their own size. Request budgets are exact (counted at admission).
- **Sliding window**: 60 buckets per window (bucket = window/60, ≥1 s). Avoids the tumbling-window burst at
  rollover that `ServiceStats` has. `Retry-After` = time until the oldest non-empty bucket expires (or 1 s
  for pure concurrency).
- **Unknown ≠ zero**: missing upstream quota data never blocks (same rule as `service_quota`).
- **Queue fairness**: wake-ups are a broadcast on any release; waiters re-assess. Not strictly FIFO
  (acceptable at a 15 s default timeout).

## 5. Teams

No Team-specific code in routing. Concretely:

- Account owner sets `Provider.Limits` once ("this Claude login: 4 concurrent, quota ≤ 95%").
  All Teams / scenarios sharing the account respect it.
- Team admin (or owner) sets `Service.Limits` on the Team rule's service rows ("Team A gets at most 2
  concurrent / 1M tokens per 5 h on this account"). Because the Team scope is the rule's scenario
  (`team:<uuid>`), the share is Team-scoped and survives Sharing-Key moves.
- Overflow policy is per rule, so each Team picks queue vs reject.
- `usage_records.team_id` is untouched; the warm start for `svc:` keys filters by `rule_uuid`.

Open (not built): one aggregate cap across *all* a Team's rules/models (needs a scope-level limit record,
e.g. on the Team row, with key `scope:<scenario>`; the Registry already supports arbitrary keys, only the
resolver and a Team-page editor are missing).

## 6. Config / API / UI surface

- `ai.Provider.Limits *Limits` → additive `providers.limits` JSON column (`serializer:json`).
- `loadbalance.Service.Limits *ai.Limits` → inside the existing `rules.services` JSON, no migration.
- Rule flags `limit_overflow`, `limit_queue_timeout` in `flag_registry.go` (registry-driven UI).
- `GET /api/v1/limits/status` → live state per limited provider/service (in-flight, window usage, pressure,
  state). Backed by `Registry.Stat`. Used for badges.
- UI: provider edit dialogs (API-key and OAuth) get an "Account limits" section with a live status line;
  each service node in the rule graph gets a "Limits" action (dialog with live status); the two overflow
  flags appear in the rule Plugins card via the flag registry; the smart-rule catalog lists the two new
  positions. The rule update payload carries `limits` through (the UpdateRule PUT is full-replace — dropping
  the field there would silently wipe limits on any UI save). Status is polled every 4 s only while one of the
  dialogs is open.
- Observability: structured logs `routing_limit_*` (diverted/queued/rejected/ignored, waited ms), debug
  header `X-Tingly-Limits` when `X-Tingly-Debug-Routing: 1`, status API above. OTel metrics: not added.

## 7. Smart-routing positions

`service_concurrency` (ops `util_ge/le/gt/lt`, % of cap, tightest service of the rule) and `service_usage`
(same ops, pressure % of window budgets). Seeded into `RequestContext` the same way as `service_capacity`
/ `service_quota`; pass when no data. Purpose: "if the pool is ≥ 80% busy route long-context requests to
the overflow partition" — content-aware use of the same numbers. Hard enforcement stays in `LimitStage`.

## 7a. Implementation notes / decisions made while building

- **Registry is one mutex, no sharding.** Hot-path cost is a few map lookups; contention is invisible next to
  LLM latency. Concurrency proven with `-race` tests (200 goroutines never exceed the cap).
- **Quota lookups are cached 10 s and run outside the registry lock** (the lookup is a store read).
- **`Service.Limits.quota_percent` is ignored** — quota is an account property; the service share only
  honours concurrency/usage/soft.
- **Status API names** are `LimitStatusEntry` / `LimitsStatusResponse` (a plain `StatusResponse` collides
  with the server-status schema in the generated OpenAPI).
- **Probe-pin (`X-Tingly-Probe-Service`) bypasses limits**; `count_tokens` goes through selection and
  therefore takes a permit for its (short) lifetime — acceptable, noted as a risk.
- **Builtin providers are read-only** in the provider API, so account limits cannot be set on them; use a
  service-level limit there.
- **Warm start** (`warmStartLimits`) seeds windows at server construction from `usage_records` aggregated
  per minute; it does not re-run when limits are edited later (windows start accruing from the edit).
- **Smart-routing positions** use `util_*` ops for `service_concurrency` and `pct_*` ops for
  `service_usage`; both read the tightest *limited* service of the smart rule and pass when none is limited.

## 8. Decisions and alternatives considered

| Decision | Alternative | Why not |
|---|---|---|
| New filter **stage** + lifecycle permit | Implement only as smart-routing ops | Ops choose a partition from aggregated numbers; they cannot exclude one account, hold a permit, queue, or protect the failover path. Kept as §7 complement. |
| Concurrency = **in-flight** counter | Reuse `ModelCapacity` seats (sessions via affinity) | Seats count pinned *sessions*, need affinity on, and say nothing about simultaneous requests. Both stay; `service_capacity` unchanged. |
| Sliding bucket window, own counters | Reuse `ServiceStats` windows | Tumbling (burst at rollover), per provider+model not per account, tied to Service pointer lifetime; one extra struct is simpler than bending it. Source of truth for warm start is still `usage_records`. |
| Limits on **Provider** + **Service** | Rule flags only / new limits table | Account cap is a property of the credential (shared across scopes). Service is the existing per-rule knob holder (Tier, ModelCapacity), JSON-persisted, no migration. A table adds joins for no query benefit. |
| Overflow policy as rule flag | Per-limit policy | The decision is about the *pool*; flags give rule-level scope and registry-driven UI for free. |
| Default overflow = **queue** (15 s) then 429 | `ignore` (degrade) | A configured cap that silently doesn't hold is worse than a short wait; coding agents retry 429 anyway. `ignore` available per rule. |
| `soft_percent` default off | Always steer at 80% | With `tier` (direct + fallback) users want to fill T0 to its cap before spilling. Steering is opt-in. |
| Release in a middleware `defer` | Release in usage tracking | Usage tracking is not called on every path (early returns, panics); the defer is. |
| Re-run selection on acquire race | Acquire inside the stage | Stage must stay side-effect free (simulator, diagnostics, affinity may still reject the pick). |
| Quota ceiling via existing `QuotaProvider` | New fetch path | Reuse; stored data, no upstream call on the hot path. |

## 9. Test matrix

`internal/limits`: acquire/release, idempotent release, all-or-nothing across subjects, race (-race, many
goroutines never exceed cap), sliding window roll-off with fake clock, soft/hard classification, reaper,
waiter wake-up/timeout/cancel, retry-after. `internal/routing`: stage filter (ok>hot>blocked, unlimited
candidates, tier composition, affinity pin declined), policies (queue wakes, queue timeout→error, reject,
ignore), selector acquire/exclude loop. `protocolserver`: lease middleware releases on normal/panic/cancel,
failover releases + re-acquires, 429 mapping. `db`: provider `limits` round trip, warm-start query.
