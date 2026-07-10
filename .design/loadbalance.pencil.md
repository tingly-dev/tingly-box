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
  │  latency   ┈┈► latency percentiles │    candidates = IsAvailable(rule,svc) ┈┈►    │
  │  speed     ┈┈► tokens/sec          │    pick via within-tier sub-tactic ──────────┼──► horizontal
  │  capacity  ┈┈► ModelCapacity       │    Allow-claim ONLY the picked one           │    (recursion,
  │            (all read ServiceStats) │    claim fails (probe in flight) → re-pick   │     1 level)
  │                                    │  all buckets tripped → degrade to T0         │
  │  ⚠ G1 breaker-blind: a tripped    │  (client must see the real upstream error)   │
  │  peer is still selectable here     │                                              │
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
  │        ⚠ G1 not read by  │                            │            (ttft/capacity)│
  │            horizontal (②) │                            │                           │
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
                                      preview, health view/reset
  CLI   harness lb --example …       LBSimulator: drives the REAL Select + failover loop
        (13 self-checking scenarios)  against scripted fake upstreams, one fake clock moves
                                      breaker + health + affinity TTL together
  HTTP  X-Tingly-Debug-Routing: 1    response headers X-Tingly-Selected-* + routing_source
  Logs  routing_selected /           structured logrus + smart-routing memory sink
        smart_routing trace           (frontend system-log page)
```

## Known gaps

- **G1 — horizontal tactics are breaker-blind** (② and ③). Correctness is covered by
  per-request failover, but a slow-failing peer costs ~1/N of requests a full timeout
  until failover, and affinity drops pins the selector can immediately re-create.
  Agreed fix: breaker-availability filter (IsAvailable + degrade) at the
  `LoadBalancer.SelectService` seam for non-tier tactics + Allow-claim on the pick,
  reusing the tier two-phase helper. Regression test parked at
  `TestLBScenario_B_Flat_DeadPeerSelection_KnownGap` (t.Skip).
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
