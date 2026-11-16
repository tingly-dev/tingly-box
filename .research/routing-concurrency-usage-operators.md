# Research: what exists for concurrency / usage routing control

Date: 2026-11-03. Input to [`.design/routing-limits.md`](../.design/routing-limits.md).
Question: tingly-box should route by (1) per-account concurrency caps and (2) model/account
usage windows and quota. What already exists, what can be reused, what is missing?

## 1. The routing pipeline (where an "operator" can plug in)

Selection is a staged pipeline, `internal/routing/selector.go`:

```
initialCandidateServices(rule)          base ∪ every smart partition's services
 → HealthStage      (429 / auth-error windows from loadbalance.HealthMonitor)
 → SmartRoutingStage(content/op match → narrows to a partition; may run processors)
 → AffinityStage    (session pin; only honoured if the pinned service is still selectable)
 → LoadBalancerStage(terminal: tactic picks — random / speed / capacity / tier …)
```

- `SelectionStage.Evaluate(ctx, candidates) (narrowed, final, err)` — a stage narrows or picks.
  Narrowing must allocate a fresh slice. An empty narrowing is repaired by the driver
  ("degrade, don't disappear": fall back to the rule's active services so the client sees the
  real upstream error). `err != nil` aborts the pipeline.
- `routing.SimpleSelector.SelectService` is the production facade (probe-pin bypass, gin
  plumbing, debug headers). Handlers reach it through `ph.selectService*` in
  `internal/protocolserver/tracing.go` (7 call sites: anthropic message, count_tokens,
  openai chat/responses/embeddings/image/image-edit/decisions).
- Mid-request failover (`protocolserver/failover_dispatch.go`) **does not re-run the pipeline**:
  `selectFallbackService` re-picks from `rule.GetActiveServices()` minus the tried set, using the
  tier tactic or the load balancer. Anything that must hold per attempt (limits!) has to be
  re-applied there.
- Statefulness already in the pipeline: `loadbalance.BreakerStore` (rule-scoped, in-memory),
  `HealthMonitor` (service-scoped, in-memory), `AffinityStore` (in-memory TTL map). All are
  process-local; none is persisted. This is the accepted pattern (`.design/tier-routing.md`:
  "deliberately avoided Redis-level shared state").
- Time goes through `internal/clock` (`clock.Now`, `clock.SetClock`) so tests/simulator share one
  fake clock.

## 2. "Operators" that already exist: smart-routing ops

`internal/routing/smartrouting` (`op.go`, `type.go`, `routing.go`): a smart rule is
`[]SmartOp + []Service`; `SmartOp = Position + Operation + Value`; first rule whose ops all match
selects a *partition* of services. Positions include the two that are closest to this task:

| Position | Data | Notes |
|---|---|---|
| `service_capacity` | seat utilisation % = `AffinityStore.CountByService / Service.ModelCapacity` | "seat" = **sessions pinned** to a service, not in-flight requests. Needs affinity enabled. |
| `service_quota` | tightest `ai/quota` window used% (`Pct(WindowKindLimit)`) of the services' providers | Optional `QuotaProvider`; unknown → op passes (never read unknown as 0%/100%). Reads stored data, never triggers an upstream fetch. |
| `service_ttft` | rolling TTFT stats | |

Pattern worth copying: data is *seeded into `RequestContext` before evaluation*
(`collectAllCapacityInfo`, `collectAllQuotaInfo`), ops are pure comparisons, registry in `op.go`
doubles as UI metadata. Limitation: ops are *predicates on a rule's service set* (they
pick a partition for the whole set, tightest/avg aggregated) — they cannot say "exclude only
account A from the candidates", cannot hold a permit, and cannot wait.

## 3. Usage / token accounting that exists

- `ai.TokenUsage` normalised per request (`.design/usage-tracking.md`); tracked at request end by
  `ProtocolHandler.trackUsageFromContext` / `trackUsageWithTokenUsage`
  (`protocolserver/usage_tracking.go`) → `updateServiceStats` (in-memory) + OTel + DB.
- `loadbalance.ServiceStats`: per-`Service` **tumbling** window (`TimeWindow` secs, default 300)
  with request / token counters, TTFT, latency, TPS; hydrated from `service_stats` table at boot.
  Tumbling windows reset to zero at the boundary → a cap built on them would let a burst through
  right after rollover. Also keyed by `Service` pointer, i.e. per provider+model, not per account.
- `db.UsageRecord` (`usage_records`): one row per request with `provider_uuid`, `model`,
  `rule_uuid`, `scenario`, `team_id`, `user_id`, tokens, status, timestamp; indexed. Exact source of
  truth for any window, survives restarts. Too heavy for a per-request read, ideal for warm start.
- `ai/quota` (`.design/quota-semantics.md`): upstream quota windows per provider
  (`ProviderUsage.Pct(WindowKindLimit)`, `RecoversAt`), refreshed in the background; the gateway
  must only consume `Kind=limit` windows for automated decisions (balances need a top-up, not time).
  This *is* "remaining budget" for subscription accounts (Anthropic 5h/7d, Codex, GLM…). Reuse as is.
- `tactics.go` `capacity_based`: weights by `Service.ModelCapacity` — static weights, no live load.

## 4. Concurrency control that exists: none for upstream

- `internal/middleware/ratelimit.go`: IP-based auth-failure limiter. Unrelated.
- `ModelCapacity` / `default_model_capacity`: seats for **sessions** (affinity count), not in-flight.
- Breaker/health react *after* failures (429s) — reactive, one request has to fail first.
- No in-flight counter anywhere. No request-lifetime hook exists besides tracing middleware
  (`tracingMiddleware`, wraps every model request) and the usage-tracking call at the end of each
  handler. Handlers are synchronous: gin handler returns only after a stream is fully drained or the
  client disconnects (upstream calls use the request context), so "handler return" is a reliable
  release point; a `defer` in a middleware also covers panics.

## 5. Teams

`.design/team.md`: Sharing Key → exactly one Team; `/tingly/team` is rewritten by
`teamScopeMiddleware` to rule scenario `team` (default team) or `team:<teamUUID>`. A Team's routing
is therefore *just ordinary rules in a scope*, with their own `Services`. `team_id` is stamped on
`usage_records`. So a limit configured on a rule's service is automatically "this Team's share of
that account", and a limit configured on the provider is "the account's physical cap, shared by all
Teams / scenarios". No new Team concept is needed in the routing layer.

## 6. Config / persistence facts that constrain the design

- `loadbalance.Service` is persisted inside the `rules.services` JSON column → adding a field needs
  **no schema migration** (`.design/rule-storage.md`). `Tier` and `ModelCapacity` are precedents for
  per-service routing knobs; the UI already edits them per service row.
- `ai.Provider` is persisted via `db.ProviderRecord` (explicit columns; additive JSON columns with
  `serializer:json` have precedent: `Credential`, `VModelDetail`) → one additive column.
- Per-rule scalar knobs are rule flags (`typ.RuleFlags` + `flag_registry.go`, UI is registry-driven).
- Selection errors are currently all mapped to HTTP 400 by the handlers; a gateway-originated
  429 for "all accounts at cap" needs a typed error and a small mapping.
- Process-local state is acceptable; the system is typically a single process with local SQLite.

## 7. Gaps → what the design has to add

1. A live **in-flight counter per account** with acquire/release tied to the request lifetime
   (incl. streaming, cancel, panic, failover attempts) — nothing exists.
2. A **sliding** usage window (requests/tokens) per account/service, warm-started from
   `usage_records` — `ServiceStats` is tumbling and per provider+model.
3. A **candidate filter** stage (exclude/deprioritise saturated accounts), because ops can only
   choose partitions; plus an **overflow policy** (queue / reject / ignore) — the existing
   "degrade" fallback is the wrong default for a hard cap.
4. Failover must respect the same limits (it bypasses the pipeline).
5. Observability of the live state (admin API + UI badges), since nothing currently exposes it.
6. Optional: expose the new signals as smart-routing positions so users can combine them with
   content conditions using the existing op UI.
