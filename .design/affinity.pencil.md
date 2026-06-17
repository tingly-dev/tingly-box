# Affinity Flow Design

## Overview

Session affinity pins a client session to the service it first landed on. This document describes the complete flow of how affinity works in tingly-box, including the strict TTL behavior.

## Configuration

`rule.Flags.SessionAffinity` - TTL in seconds (e.g., 3600 = 1 hour)

When affinity is enabled:

- First request: locks to selected service, sets TTL
- TTL expires: session re-enters selection, may get different service
- Within TTL: session stays pinned to the same service

## Complete Request Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         REQUEST FLOW (完整版)                                │
└─────────────────────────────────────────────────────────────────────────────┘

Request 1 at T=0 (第一次请求, 无锁)
─────────────────────────────────────┘

1. SELECTOR.SELECT(ctx)
   │
   ▼
2. AffinityStage.Evaluate()
   │
   ├── store.Get(ruleUUID, sessionID) → nil (无锁)
   │
   ▼
3. → 返回 nil, false (pass through)
   │
   ▼
4. LoadBalancerStage
   │
   ├── 选择 service: svc-a
   │
   ▼
5. result = Result(svc-a, "load_balancer")
   │
   ▼
6. postProcess(result, ctx)
   │
   ├── 检查: result.Source == "affinity"? → NO (是 "load_balancer")
   │
   ├── 检查: ttl := GetEffectiveAffinity(rule) → 3600 秒
   │
   ▼
   ┌─────────────────────────────────────────────────────────────────┐
   │  ⭐ 第一次锁定                                                  │
   │  affinityStore.Set(..., AffinityEntry{                         │
   │      Service: svc-a,                                            │
   │      LockedAt: T=0,                                             │
   │      ExpiresAt: T=0 + 3600 = T=3600                            │
   │  })                                                             │
   └─────────────────────────────────────────────────────────────────┘
   │
   ▼
7. Response to client (使用 svc-a)


Request 2 at T=1800 (TTL 内, 有锁)
─────────────────────────────────────┘

1. SELECTOR.SELECT(ctx)
   │
   ▼
2. AffinityStage.Evaluate()
   │
   ├── store.Get(ruleUUID, sessionID) → entry (ExpiresAt: T=3600)
   │
   ├── ⭐ 检查: time.Now().After(entry.ExpiresAt)?
   │   └── T=1800 > T=3600? → NO (未过期)
   │
   ├── ⭐ 检查: IsAffinityEligible(services, entry.Service)?
   │   └── YES (假设 service 仍然可用)
   │
   ▼
3. → 返回 Result(svc-a, "affinity")
   │
   ▼
4. postProcess(result, ctx)
   │
   ├── ⭐ 检查: result.Source == "affinity"? → YES
   │
   ▼
   ┌─────────────────────────────────────────────────────────────────┐
   │  ⭐ 跳过锁定 (已锁定, 不刷新 TTL)                                │
   │  return (不执行 Set)                                            │
   └─────────────────────────────────────────────────────────────────┘
   │
   ▼
5. Response to client (使用 svc-a, 锁的 TTL 仍是 T=3600)


Request 3 at T=3601 (TTL 过期后, 有锁但过期)
─────────────────────────────────────┘

1. SELECTOR.SELECT(ctx)
   │
   ▼
2. AffinityStage.Evaluate()
   │
   ├── store.Get(ruleUUID, sessionID) → entry (ExpiresAt: T=3600)
   │
   ├── ⭐ 检查: time.Now().After(entry.ExpiresAt)?
   │   └── T=3601 > T=3600? → YES (已过期!)
   │
   ▼
   ┌─────────────────────────────────────────────────────────────────┐
   │  ⭐ 严格 TTL: 丢弃过期锁                                        │
   │  logrus.Info("affinity entry expired...; dropping pin")       │
   │  return nil, false (pass through to strategy)                  │
   └─────────────────────────────────────────────────────────────────┘
   │
   ▼
3. → 返回 nil, false (pass through)
   │
   ▼
4. LoadBalancerStage
   │
   ├── 重新选择 service:
   │   - 如果 svc-a 仍可用 → 可能选择 svc-a
   │   - 如果 svc-a 不可用 → 选择其他 service
   │   - 如果 tier 恢复 → 选择更高 tier 的 service
   │
   ▼
5. 假设选择: svc-b (可能和之前相同, 也可能不同)
   │
   ▼
6. result = Result(svc-b, "load_balancer")
   │
   ▼
7. postProcess(result, ctx)
   │
   ├── 检查: result.Source == "affinity"? → NO (是 "load_balancer")
   │
   ├── 检查: ttl := GetEffectiveAffinity(rule) → 3600 秒
   │
   ▼
   ┌─────────────────────────────────────────────────────────────────┐
   │  ⭐ 重新锁定 (新锁, 新 TTL)                                     │
   │  affinityStore.Set(..., AffinityEntry{                         │
   │      Service: svc-b,                                            │
   │      LockedAt: T=3601,                                         │
   │      ExpiresAt: T=3601 + 3600 = T=7201                         │
   │  })                                                             │
   └─────────────────────────────────────────────────────────────────┘
   │
   ▼
8. Response to client (使用 svc-b, 新的 TTL 到 T=7201)
```

## Key Design Decisions

### 1. Strict TTL (not sliding)

**Behavior**: Lock expires exactly at `LockedAt + SessionAffinity` seconds

**Why**:

- Honors the configured TTL value
- Allows operational control (draining, rebalancing)
- Predictable session lifecycle

**Alternative (Sliding TTL)**: Each request refreshes the TTL

- Pro: Active sessions never interrupted
- Con: Configuration becomes meaningless

### 2. No TTL Refresh in postProcess

**Code**:

```go
if result.Source == "affinity" || ctx.SessionID.IsEmpty() {
return // Don't re-lock, don't refresh TTL
}
```

**Why**: This is critical for strict TTL. If we refreshed TTL here, it would become sliding TTL.

### 3. Tier-Scoped Affinity

**Behavior**: Even within TTL, a pin can be dropped if:

- Pinned service's breaker is open (dead peer)
- A higher-tier service has recovered (cross-tier demotion)

**Why**: Ensures affinity respects tier priority and health, not just time.

## AffinityEntry Structure

```go
type AffinityEntry struct {
Service   *loadbalance.Service // The pinned service
MessageID string               // For tracing
LockedAt  time.Time // When the lock was created
ExpiresAt time.Time // When the lock expires (strict TTL)
}
```

## Pipeline Integration

```
Pipeline: Health → Affinity → Smart → LoadBalancer

AffinityStage.Evaluate():
    1. Check if affinity enabled (rule.Flags.SessionAffinity > 0)
    2. Check if session exists (!SessionID.IsEmpty())
    3. Get affinity entry (store.Get)
    4. ⭐ Check if expired (time.Now().After(entry.ExpiresAt))
    5. ⭐ Check if still eligible (typ.IsAffinityEligible)
    6. Return Result(service, "affinity") or pass through

postProcess():
    1. Check if result.Source == "affinity" → skip (don't refresh TTL)
    2. Get TTL (GetEffectiveAffinity)
    3. Set new lock (affinityStore.Set)
```

## Time-Based Scenarios

| Time   | Action        | State        | Result                                           |
|--------|---------------|--------------|--------------------------------------------------|
| T=0    | First request | No lock      | Create lock, ExpiresAt=T=3600                    |
| T=1800 | Request       | Lock valid   | Use lock, NO refresh                             |
| T=3601 | Request       | Lock expired | Drop lock, re-select, create new lock to T=7201  |
| T=5400 | Request       | Lock valid   | Use lock, NO refresh                             |
| T=7202 | Request       | Lock expired | Drop lock, re-select, create new lock to T=10802 |

## Edge Cases

### 1. Service Becomes Unavailable (Breaker Open)

Even if lock is valid (not expired), `IsAffinityEligible` drops the pin:

- Dead peer in same tier → drop, pick healthy peer
- Higher tier recovers → drop, promote to recovered tier

### 2. All Services Down

When every breaker is open, affinity degrades:

- Still honors pin to lowest tier (don't disappear)
- Request surfaces real upstream error instead of "no service"

### 3. Inactive Service

Inactive services never make tiers look "available":

- Inactive service's breaker state is ignored
- Prevents false tier availability

## Files

- `internal/server/routing/stage_affinity.go` - AffinityStage with strict TTL
- `internal/server/routing/selector.go` - postProcess with no-refresh logic
- `internal/typ/tactics.go` - IsAffinityEligible (tier-scoped affinity)
- `internal/server/routing/stage_affinity_test.go` - Tests including strict TTL

## Testing

See `internal/server/routing/stage_affinity_test.go`:

- `TestAffinity_StrictTTL_Expired` - Expired lock is dropped
- `TestAffinity_StrictTTL_NotExpired` - Valid lock is honored
- `TestAffinity_TierScope_*` - Tier-scoped affinity scenarios
