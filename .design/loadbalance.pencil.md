# Load balancing — the whole subsystem (pencil)

One page for everything LB: the selection pipeline, the tactic engine, the three
runtime state stores and their feedback loops, and the diagnostic surfaces.
Deep dives live in `tier-routing.md` (tier/breaker/affinity semantics) and
`failover.pencil.md` (per-request retry); this page is the map that connects them.

Legend: `│`/`▼` = control flow · `┈┈►` = state read · `━━►` = state write ·
`⚠ Gn` = known gap (see bottom).

## ① Selection — one pipeline, four stages

Every protocol handler (Anthropic v1/Beta, OpenAI Chat/Responses/Embeddings/Images)
funnels through the same path; there is exactly one production route.

```
  client request (any endpoint)
        │
        ▼
  SimpleSelector.SelectService(c, scenario, rule, req)        ← facade (routing/simple.go)
        │
        ├── X-Tingly-Probe-Service: uuid:model ────────► pin that service, skip pipeline ✓
        ▼
  ServiceSelector.Select(ctx)                                 ← engine (routing/selector.go)
        │   candidates = rule.Services ∪ every SmartRouting[].Services   (dedup, ordered)
        ▼
  ┌─ stage pipeline ─ filter stages narrow the pool · terminal stages pick ──────────────┐
  │                                                                                       │
  │  1 HealthStage        filter    drop 429/auth-unhealthy services                      │
  │       │               ┈┈► HealthMonitor        degrade: none healthy → keep all       │
  │       ▼                                                                               │
  │  2 AffinityStage      terminal  honor session pin iff the strategy would pick it NOW  │
  │       │               ┈┈► AffinityStore (strict TTL)                                  │
  │       │               ┈┈► typ.IsAffinityEligible: breaker walk + tier + PromotionHold │
  │       ▼                                                                               │
  │  3 SmartRoutingStage  terminal  first rule whose ops all match → service subset       │
  │       │               ops read extracted RequestContext (model/tokens/agent-kind/…)   │
  │       │               processor ops (proxy_vision) mutate req, then BYPASS ↓ to 4     │
  │       │               subset of >1 → LB *within* the subset (same engine as 4)        │
  │       ▼                                                                               │
  │  4 LoadBalancerStage  terminal  the global fallback — always selects (or errors)      │
  │       └────────────► LoadBalancer.SelectService(rule│narrowed candidates)   see ②     │
  └───────────────────────────────────────────────────────────────────────────────────────┘
        │  validate pick: service active + provider resolvable + provider enabled
        │  (invalid → fall through to the next stage, not a hard error)
        ▼
  postProcess: (re)pin session ━━► AffinityStore          (unless the pin itself won)
        │
        ▼
  (provider, service) → prologue continues → dispatchWithPriorityFailover   see ③
```

## ② Tactic engine — shape decides the strategy

`LoadBalancer.SelectService` (server/load_balance.go) is the innermost engine,
shared by stage 4, smart-routing subsets, failover re-selection, and the admin API.

```
  LoadBalancer.SelectService(rule)
        │ active filter → health filter (degrade: none healthy → keep all) → 1-svc shortcut
        ▼
  rule.LBTactic.Instantiate()          ← the ONE seam turning config JSON into behavior
        │                                 unset/unknown/legacy("adaptive") → random
        ▼
  tactic.SelectService(tempRule)
        │
  ┌─────┴──────────────────────────────┬─────────────────────────────────────────────┐
  │ HORIZONTAL — one layer, N peers    │ VERTICAL — TierTactic (multi-layer)          │
  │                                    │                                              │
  │  random    ┈┈► Service.Weight      │  bucket by Service.Tier ascending (T0 first) │
  │  token     ┈┈► window tokens       │  per bucket:                                 │
  │  latency   ┈┈► latency percentiles │    PickBreakerAvailable:                     │
  │  speed     ┈┈► tokens/sec          │    pick via within-tier sub-tactic ──────────┼──► horizontal
  │  capacity  ┈┈► ModelCapacity       │                                              │    (recursion,
  │            (all read ServiceStats) │  all buckets tripped → degrade to T0         │     1 level)
  │                                    │  (client must see the real upstream error)   │
  │  breaker-aware via the same        │                                              │
  │  PickBreakerAvailable walk;        │  shared walk (typ.PickBreakerAvailable):     │
  │  none available → degrade to       │  IsAvailable filter (non-consuming) → pick   │
  │  unfiltered pick                   │  → Allow-claim ONLY the pick → re-pick on    │
  │                                    │  claim failure (probe already in flight)     │
  └────────────────────────────────────┴──────────────────────────────────────────────┘

  Config shapes (see tier-routing.md):  A single = 1×1 · B flat = 1×N (horizontal)
                                        C cascade = M×1 · D grid = M×N (vertical)
  "tier" is not a mode — it is the emergent shape of a multi-layer rule.
```

## ③ Runtime state — three stores, one feedback loop

Selection is stateless; all memory lives in three stores fed by dispatch outcomes.

```
                     dispatch outcome (per attempt, in the failover loop)
                     ┌──────────────────────┬───────────────────────────┐
             success │ RecordServiceSuccess │ reportHealthStatus        │ usage tracking
             failure │ RecordServiceFailure │ (status-classified)       │ (tokens/latency/
                     ▼                      ▼                           ▼  TTFT/speed)
  ┌───────────────────────────┬────────────────────────────┬───────────────────────────┐
  │ BreakerStore              │ HealthMonitor              │ ServiceStats              │
  │ scope: (ruleUUID, svc)    │ scope: global per svc      │ scope: global per svc     │
  │ signal: binary ✓/✗       │ signal: by status class    │ signal: usage metrics     │
  │                           │                            │                           │
  │ 3✗ → Open                │ 429  → rate-limit window   │ rolling windows +         │
  │ 30s → HalfOpen (1 probe,  │ 401/403 → instant unhealthy│ percentiles               │
  │   stale-reclaimed after   │ 5xx  → 3-strike            │                           │
  │   another OpenDuration)   │                            │                           │
  │ 3✓ → Closed              │                            │                           │
  │ PromotionHold 60s         │                            │                           │
  │        ┊                  │        ┊                   │        ┊                  │
  │        ┈┈► TierTactic     │        ┈┈► HealthStage (1) │        ┈┈► token/latency/ │
  │        ┈┈► IsAffinity-    │                            │            speed tactics  │
  │            Eligible (2)   │                            │        ┈┈► smart ops      │
  │        ┈┈► horizontal     │                            │            (ttft/capacity)│
  │            pick (②)       │                            │                           │
  └───────────────────────────┴────────────────────────────┴───────────────────────────┘

  AffinityStore (rule-scoped, session → service pin, strict TTL — no sliding renewal)
     written by postProcess (①) · read by AffinityStage (2) · validity delegated to
     the breaker walk, so a pin never outlives what the strategy would pick.

  Failover (failover.pencil.md) closes the loop: a retryable attempt records ✗ for
  THAT candidate, re-selects (tier walk or LoadBalancer.SelectService), and retries —
  so the next request's selection already sees the updated breaker/health state.
```

## ④ Diagnostic & admin surfaces — all traverse the real path

```
  REST  /api/v1/load-balancer/*      LoadBalancerAPI → LoadBalancerEngine (interface slice
        rules/stats/health/tactic     of LoadBalancer): summary, stats CRUD, current-service
                                      preview, health view/reset; rules/:id/health includes
                                      per-service breaker_state + breaker_retry_in_seconds
  REST  /api/v1/requests             Requests view: per-request summary now carries
                                      failover_hops + failover_path ("A → B"), folded from
                                      the failover loop's stage=failover_retry events
  CLI   harness lb --example …       LBSimulator: drives the REAL Select + failover loop
        (13 self-checking scenarios)  against scripted fake upstreams, one fake clock moves
                                      breaker + health + affinity TTL together
  HTTP  X-Tingly-Debug-Routing: 1    response headers X-Tingly-Selected-* + routing_source
  Logs  routing_selected /           structured logrus + smart-routing memory sink
        smart_routing trace           (frontend system-log page)
```

## Known gaps

- ~~**G1 — horizontal tactics are breaker-blind**~~ **RESOLVED**: the
  `LoadBalancer.SelectService` seam now runs `typ.PickBreakerAvailable` (IsAvailable
  filter → pick → Allow-claim, degrade to unfiltered when nothing is available) for
  non-tier tactics; TierTactic shares the same helper per bucket. Regression:
  `TestLBScenario_B_Flat_DeadPeerExcludedAndSingleProbe`.
- **G3 — affinity is global-scope**: pins are per rule, not per smart-routing subset
  (`selector.go` TODO; the `smart_rule` scope plumbing exists but the store keying doesn't).

## Map to code

| Piece | Where |
| --- | --- |
| Facade + probe pin + debug headers | `internal/server/routing/simple.go` |
| Pipeline engine + validation + re-pin | `internal/server/routing/selector.go` |
| Stages 1–4 | `internal/server/routing/stage_{health,affinity,smart_routing,load_balancer}.go` |
| Tactic engine + health degrade | `internal/server/load_balance.go` |
| Tactics + IsAffinityEligible | `internal/typ/tactics.go` |
| Breaker + store (+ stale reclaim) | `internal/loadbalance/breaker.go` |
| Health monitor / filter | `internal/loadbalance/health_monitor.go`, `internal/typ/health_filter.go` |
| Service stats | `internal/loadbalance/load_balancing.go` |
| Affinity store | `internal/server/affinity/affinity.go` |
| Failover loop + gate | `internal/server/failover_dispatch.go` |
| Smart-routing evaluator | `internal/smart_routing/` (README there) |
| Admin REST | `internal/server/load_balance_handler.go` |
| Simulator / scenario harness | `internal/server/load_balance_simulator.go`, `lb_scenario_test.go`, `cli/harness lb` |
