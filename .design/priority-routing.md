# Priority-Based Service Routing

Status: shipped (v1) on branch `claude/priority-service-routing-dIQfX`.
Tracking commits: `a362ca3`, `5221109`.

## Why

Existing routing in tingly-box treats every service in a rule as an equal
peer. `random`, `token_based`, `latency_based`, `speed_based`, `adaptive`,
`capacity_based` — all of them spread load across services. That is the
right default for "I have several equivalent providers, share the load".

It is the wrong default for the other very common shape:

> "Use account A. If A is broken, use account B. Once A is healthy
> again, go back to A."

People with two Anthropic accounts (one primary, one as backup), or a
cheap-and-fast model with an expensive fallback, or any home-rolled
"direct + fallback" setup, were stuck:

- They could either include the backup in the pool (and have it pick up
  random traffic), or
- Mark it inactive and manually flip it on when the primary failed.

Neither is acceptable. We need first-class **direct + fallback**.

A second, related gap: when an upstream service starts failing, nothing
in the box notices. Every request keeps trying the same broken service
until the user disables it. We need a lightweight **circuit breaker** so
the next request automatically uses the backup, and the next-after-next
returns to the primary once it recovers.

## What this is not

- **Not** a cross-request load balancer rebalance — existing tactics stay
  intact and remain the default.
- **Not** a request-level retry loop. When a request fails mid-flight,
  the client still sees the error. Failover happens on the **next**
  request after the breaker trips. (Mid-request retry is parked as v2 —
  see "Future work".)
- **Not** a global health system. The breaker is process-local and rule-
  scoped through the service-id key; we deliberately avoided Redis-level
  shared state to keep the deployment story simple.

## How

### Two new concepts

1. **`Service.Priority int`** — a per-service number inside a rule.
   Higher = tried first. `0` is "unset" and sinks to the bottom.
2. **`TacticPriority`** — a new `Tactic` value. When selected, the rule
   ranks services by `Priority` and picks the highest tier whose circuit
   breaker is closed.

### Selection algorithm

```
SelectService(rule):
  active = rule.GetActiveServices()
  buckets = group services by Priority, sorted descending (0 last)
  for each bucket (highest priority first):
    candidates = services in bucket whose breaker allows a request
    if candidates is non-empty:
      return WithinTierTactic.pick(candidates)   # default: random
  // every tier is tripped — fall back to the top bucket regardless
  // so the upstream-error path can surface a real upstream error.
  return WithinTierTactic.pick(top bucket)
```

Three properties fall out:

- **Distinct priorities ⇒ pure failover.** Service at priority 10 is
  used until it fails; service at priority 5 takes over; once the 10's
  breaker closes, the next request snaps back to it.
- **Tied priorities ⇒ load sharing within a tier.** Two services at
  priority 10 share traffic via the sub-tactic (random by default).
- **All tiers tripped ⇒ degrade, don't disappear.** Picking nothing
  would let the caller bypass the real upstream error message; picking
  the top tier guarantees the client sees the actual provider's 5xx /
  rate-limit text.

### Recovery

The breaker is a three-state machine (`Closed → Open → HalfOpen`):

- **Closed** — normal. `Allow()` returns true, failures are counted.
- **Open** — too many consecutive failures (`FailureThreshold`, default
  3). `Allow()` returns false. After `OpenDuration` (default 30 s) the
  next `Allow()` call lazily flips to HalfOpen.
- **HalfOpen** — exactly one probe is permitted. Success → Closed,
  failure → Open with a fresh timer.

Recovery requires **no separate scheduler**. Selection re-evaluates the
priority list every request, and the breaker's lazy state transition
admits one probe naturally. Active probing was considered and rejected
for v1 — for hot rules it's redundant, and for cold rules there is no
one to serve anyway.

### Wiring failures to the breaker

`ProtocolRecorder` already sees every success and failure of every
upstream call. We added two lines:

- `RecordResponse(provider, model)` → `RecordServiceSuccess(serviceID)`
- `RecordError(err)` → `RecordServiceFailure(serviceID)`

The `serviceID` is computed from `provider.UUID + ":" + model`, matching
the format `Service.ServiceID()` produces, so the breaker registry's
keys line up exactly with the selection pool. **Zero changes** to the
dispatch hot path were required.

### Frontend UX

A clickable badge on the top-left corner of each provider node:

- Shows the current priority (`1`, `2`, …, `–` when unset).
- Click → small popover with a number input. Setting `0` clears.
- The badge is sized to overlap the corner, matching the existing
  badge convention used elsewhere (e.g. SmartOpNode index badges).
- The provider list re-sorts descending by priority as the user edits.

The design choice here is **implicit mode activation**: assigning any
service a priority > 0 flips the rule's `lb_tactic` to `priority` on
save. No separate tactic-selector UI is exposed. Clearing every priority
back to 0 leaves the previous tactic intact (the auto-save now
round-trips `lb_tactic`, which it didn't before).

Why implicit: tactic concepts are jargon for most users. "Set priorities
on services" is concrete and matches a real intent ("I want this one
first"). The tactic switch is plumbing — it shouldn't be a separate
question.

## Value

| Audience | Value delivered |
|---|---|
| Users with multiple equivalent accounts | First-class failover. Set priority 10 on the main, 5 on the backup, walk away. |
| Users running cost-tiered providers | Same model with a cheap-then-expensive cascade. |
| Operators of any production rule | Free circuit breaking. Even users on the existing tactics get failure isolation as soon as they assign a single priority. |
| Anyone debugging | The recorder now reports per-service success/failure into the breaker, and the breaker state is exposed via `BreakerStore.Snapshot()` for future surfacing. |

The feature is **additive**: rules without explicit priorities behave
exactly as before; the new tactic is opt-in via the UI.

## Comparison to claude-code-hub

A separate project (`ding113/claude-code-hub`) ships similar capability;
we deliberately did **not** clone its design.

| Their design | Ours | Why we differ |
|---|---|---|
| Priority is global across the provider pool. | Priority is scoped to a rule's services. | Our rules already segment requests by model/scenario, so the rule is the natural priority boundary. Global priorities would conflict with our rule isolation. |
| Numeric priority + cost multiplier + weighted random inside a tier. | Numeric priority + sub-tactic (default random). | Rules typically hold ≤ 5 services. A user-tunable sub-tactic gives flexibility without forcing a second config field on everybody. |
| Per-user-group priority overrides. | Not implemented. | Users already express user-group-specific routing by having separate rules. Adding overrides would duplicate that mechanism. |
| Redis-shared breaker state. | In-memory, process-local. | Single-instance is the dominant deployment shape. Redis can be added later with no model changes — `BreakerStore` is an interface boundary. |
| Active probing scheduler for half-open. | Lazy half-open on next request. | Strictly simpler. Hot rules don't need it; cold rules don't matter. |
| Mid-stream retry across providers using deferred finalization. | Not implemented in v1. | Touches every dispatch path; v2. |
| 32 KiB "fake 200" body sniffing. | Not implemented. | Niche; revisit if we see real cases. |
| Dispatch simulator UI. | Not implemented. | Excellent idea, separable feature, v2. |

## Future work

1. **Mid-request failover (v2)** — wrap `forwarding.Forward*` returns
   with a "try next priority tier" loop, gated on error class:
   - retryable: ECONNREFUSED, ETIMEDOUT, 429, 5xx
   - not retryable: 4xx (non-429), content-filter, client disconnect
2. **Streaming pre-first-delta failover (v3)** — detect that the
   upstream stream has not yet emitted a real content event, and on
   disconnect rewire to the next tier without writing anything to the
   client.
3. **Active half-open probing** — opt-in goroutine that probes Open
   breakers on a cadence, useful for cold rules.
4. **Per-rule breaker thresholds** — currently process-wide defaults.
   Could be surfaced as a Rule-level config block.
5. **Failover decision log + UI** — the recorder already sees every
   success/failure attribution; we can surface "request X went to
   service A → fell back to B" in the system log page.
6. **Dispatch simulator** — read-only "explain plan" that shows which
   service a hypothetical request would land on. Particularly useful
   in combination with smart routing.

## File map

Backend
- `internal/loadbalance/load_balancing.go` — `Service.Priority`,
  `TacticPriority` enum.
- `internal/loadbalance/breaker.go` — three-state breaker + store.
- `internal/loadbalance/breaker_test.go`
- `internal/typ/tactics.go` — `PriorityParams`, `PriorityTactic`,
  `groupServicesByPriority`.
- `internal/typ/priority_tactic_test.go`
- `internal/server/protocol_recording.go` — recorder → breaker bridge.

Frontend
- `frontend/src/components/RoutingGraphTypes.ts` — `ConfigProvider.priority`, `ConfigRecord.lbTactic`.
- `frontend/src/components/rule-card/utils.ts` — `pickLbTactic`,
  `hasPriorityAssigned`.
- `frontend/src/components/rule-card/useRuleCardHooks.ts` — autosave
  round-trips `lb_tactic` and `priority`.
- `frontend/src/components/RuleCard.tsx` — `handleProviderPriorityChange`.
- `frontend/src/components/RoutingGraph.tsx` — priority-sorted list,
  prop plumbing.
- `frontend/src/components/nodes/ProviderNode.tsx` — `PriorityBadge`
  component overlapping the top-left corner.
