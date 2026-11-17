# Routing limits: account, person, team and model concurrency & usage operators

Status: **implemented** (backend, API, UI, smart-routing positions). Code: `ai/limits.go`, `internal/limits/`, `internal/routing/{limits,limits_actor,stage_limits}.go`, `internal/db/limit_policy_store.go`, `internal/protocolserver/limits.go`, `internal/server/{limits.go,module/limits/}`, frontend `src/components/limits/`. Person / team / model scopes: §10 (implemented). Research: [`.research/routing-concurrency-usage-operators.md`](../.research/routing-concurrency-usage-operators.md).
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

Non-goals: billing/credit enforcement, cross-instance coordination, fair-share scheduling between members of
a Team (caps only, no weighted queues), replacing the breaker/health mechanisms (they stay the *reactive* layer;
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
- **What a token is**: `max_tokens` counts `input + output` as normalized in `ai.TokenUsage` (the same figure
  `usage_records.total_tokens` stores, so live counting and the restart warm start agree). Cache **reads** are
  not counted (`CacheReadTokens` is kept apart; Anthropic's `cache_read_input_tokens` and OpenAI's
  `cached_tokens` are both excluded from input), cache **writes** are (normalization folds them into input),
  and reasoning tokens count only as far as the upstream includes them in output.
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

Aggregate caps for a Team (whole team, or per member) and caps per person are the actor scopes of §10; this
section only covers what an account/rule-level limit gives a Team on its own.

## 6. Config / API / UI surface

- `ai.Provider.Limits *Limits` → additive `providers.limits` JSON column (`serializer:json`).
- `loadbalance.Service.Limits *ai.Limits` → inside the existing `rules.services` JSON, no migration.
- Rule flags `limit_overflow`, `limit_queue_timeout` in `flag_registry.go` (registry-driven UI).
- `GET /api/v1/limits/status` → live state per limited provider/service (in-flight, window usage, pressure,
  state). Backed by `Registry.Stat`. Used for badges.
- UI: provider edit dialogs (API-key and OAuth) get an "Account limits" section with a live status line;
  each service node in the rule graph carries a small limits icon on its top-left, a sibling of the quota ring:
  its own control, hoverable for the numbers (both the rule's share and the account's) and clickable to open the
  editor dialog with live status. Limited nodes always show it (blue when the rule's own share is limited, grey
  when only the account is); unlimited nodes show a quiet icon only while the node is hovered or focused (always
  on touch devices), so a first limit is one click away without marking every node. There is deliberately no
  second entry in the hover action bar. The icon sits on the left because that bar is pinned top-right and would
  cover, and block hover on, anything placed there; it also stops
  `mouseover` so the node's identity tooltip does not open on top of it. Token budgets are typed as a plain
  count or with a K / M suffix (`500K`, `1.5M`) and shown back in that short form. The two overflow
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

Actor scopes (§10): `internal/limits` — staged lease (actor permit + candidate permit) released together,
failed second-stage acquire leaves the first stage's counters untouched, per-user window with fake clock,
template vs override resolution. `internal/routing` — actor gate queue/reject per policy, team + user
conjunction, usage block never queues, gate idempotent per request, failover keeps the actor permit.
`protocolserver` — sharing-key identity → subjects, streaming request holds user concurrency until the
stream ends / client cancels, 429 error codes. `db` — policy CRUD + cache, per-user warm-start query.

## 10. Limit scopes beyond account and rule (person, team, model)

Account and rule limits answer "how much may this *upstream* take". The next question — "how much may this
*person* / this *team* / this *model* take" — is a different axis: it does not depend on which account the
request lands on, so it must not live on `Provider` or on a rule (a rule is just a model name in a scope).
This section stacks it on §2–§4 without changing them.

### 10.1 What identity exists today (research)

| Credential | What the gateway knows | Notes |
|---|---|---|
| Sharing Key (`tb-share-…`) | `api_tokens` row: `user_id` (a fresh UUID **per key**), `team_id`, `display_name`. Auth middleware sets `CtxKeyUserID` + `CtxKeyTeamID`. | There is no member/person entity. Today one key == one `user_id` == one "person". If keys are ever grouped under a member, grouping by `user_id` already works. |
| Global model token | `CtxKeyUserID = DefaultAdminUserID`; `CtxKeyTeamID` empty (set to default only on `/tingly/team`). | The admin; a policy on it rate-limits the whole global token. |
| Enterprise context JWT | `CtxKeyEnterpriseUserID` (+ department, tier). | Only with the global token; an external identity. |
| Usage records | `usage_records.user_id`, `team_id`, `model` are indexed columns. | Source of truth for warm start. |

Teams have **no member list**: membership is "has a key bound to the team". The raw key string is also the
`token_id`, so it must never be used as a limits key or shown in status — `user_id` is the non-secret handle.

### 10.2 Subjects ("scopes")

`Subject` (§2) gains a scope; every subject has the same `ai.Limits` and key shape `<scope>:<ref>`:

| Scope | Key | Who it protects | Known at | Config home |
|---|---|---|---|---|
| `acct` | `acct:<providerUUID>` | the upstream credential | after pick (per candidate) | `Provider.Limits` (§2) |
| `svc` | `svc:<ruleUUID>/<prov>/<model>` | one rule's share of an account | after pick (per candidate) | `Service.Limits` (§2) |
| `model` | `model:<upstream model>` | a model across *all* accounts/rules (e.g. cap opus) | after pick (per candidate) | policy `model` |
| `user` | `user:<user_id>` / `euser:<enterprise id>` | one person (today: one sharing key) | request start | policy `user`, or inherited from `team_member` |
| `team` | `team:<team_id>` | everyone in a team together | request start | policy `team` |

**Per-member default.** Keys get a fresh `user_id`, so an admin cannot pre-create a per-person policy before
the key exists. Policy kind `team_member` (ref = team id) is a *template*: every distinct `user_id` seen in
that team gets its own counter `user:<uid>` governed by that template ("each member: 2 concurrent, 1M
tokens / 5 h"). An explicit `user` policy for that `user_id` **replaces** the inherited template for the same
key (override, not stacking) — otherwise a raised limit for one heavy user could never take effect.

### 10.3 Policy storage (one generic table)

`limit_policies(kind, ref, limits JSON, overflow, queue_timeout_seconds, updated_at)`, unique `(kind, ref)`,
`kind ∈ user | team | team_member | model`. Chosen over adding columns to `teams` / `api_tokens`: one store, one
API and one editor for all kinds; no schema change to auth tables; `model` has no owning row anyway.
`LimitPolicyStore` mirrors the table in memory (like Team/APIToken stores) because resolution is on the
request path; writes update the cache before returning, so an edit applies to the next request.
Account and rule limits stay where §2 put them (credential/rule properties, no migration).

### 10.4 Evaluation order and stacking

A request passes two gates, in this order:

```
1. ACTOR GATE   (request start, before routing)      user  +  team          all must pass
2. CANDIDATE    (LimitStage → pick → acquire)        acct  +  svc  +  model all must pass
```

- **Every applicable subject must pass (conjunction).** There is no "which limit wins" arbitration: the
  most restrictive one blocks. The verdict records the *scope that blocked* (`Reasons` + `Scope`), so the
  error and logs say whether the person, the team or the upstream is exhausted.
- **Order matters only for what we can do about a block.** A user/team block cannot be fixed by choosing
  another account, so it is checked first and never reaches routing (no wasted selection, no account
  permit taken then dropped). An `acct`/`svc`/`model` block can be routed around, so it filters candidates.
- **Pressure/soft steering applies to candidate subjects only.** `soft_percent` is ignored for `user`/`team`
  (there is nothing to steer to); the UI hides it for those scopes. `quota_percent` stays account-only.

### 10.5 Atomic acquire, staging and rollback

The registry already acquires a *list* of subjects all-or-nothing under one lock (nothing is incremented
unless every subject passes; `TestAllOrNothingAcrossSubjects`). The two gates are two such lists, and a
request owns two permits in the same per-request `Lease`:

1. `AdmitActor`: `TryAcquire([user…, team])` → one permit stored under lease key `actor` (idempotent per
   request via a gin flag, so a second `SelectService` call in one request does not double-count).
2. Selection + `TryAcquire([acct, svc, model])` → candidate permit under the service id (§3, unchanged).
3. Failover releases only the *candidate* permit of the failed attempt; the actor permit stays (the person
   made one request). Account windows count every upstream attempt; person/team windows count requests.

Rollback is structural, not manual: if step 2 fails for good (no candidate, 429, race exhaustion), nothing
is explicitly undone — `limitsMiddleware`'s `defer lease.ReleaseAll()` releases the actor permit (and the
lease closes, so a late `Hold` releases immediately). A failed `TryAcquire` has already incremented
nothing, so there is no half-acquired state anywhere. Tokens are recorded at completion to the actor
subjects *and* the final service's subjects.

### 10.6 Overflow per subject

§2's overflow policy is a property of the candidate *pool* (rule flag). Actor scopes get their own, stored on
the policy (`overflow`, `queue_timeout_seconds`) because a per-person cap is a fairness/quota decision, not a
routing one:

| Block | Default | Allowed | Behaviour |
|---|---|---|---|
| user/team **concurrency** | `queue`, 15 s | `queue`, `reject` | wait for one of that person's/team's requests to finish, else 429 |
| user/team **usage window** | `reject` (forced) | — | a window will not free up in seconds: 429 + `Retry-After` = time to oldest bucket expiry |
| acct / svc / model | rule flag (§2) | queue / reject / ignore | unchanged; `ignore` is **not** offered to actor scopes (it would defeat the cap) |

429 body distinguishes scope: `error.type = rate_limit_error`, `error.code = user_limit_exceeded |
team_limit_exceeded | upstream_limit_exceeded`, message names the dimension. The queue wait budget is shared
between the actor gate and the candidate stage so one request never waits longer than the larger timeout
plus one stage.

### 10.7 Configuration, status, observability

- API (control plane, `/api/v1`): `GET /limit-policies?kind=&ref=`, `PUT /limit-policies/{kind}/{ref}`
  (body: `limits`, `overflow`, `queue_timeout_seconds`; an empty `limits` object deletes), `DELETE …`.
  Validation: `kind` in the closed set; `user` ref must be an existing key's `user_id`, `team`/`team_member`
  ref an existing team, `model` ref non-empty; `soft_percent`/`quota_percent` rejected for actor kinds.
- UI: **Team page** — "Team limits" (whole team) and "Per-member limits" (template) dialogs; **Team Keys
  table** — a per-key "Limits" action (this person), labelled with the key's display name, with live
  in-flight / window readout; model limits are API-only for now (see §11).
- Status: `GET /limits/status` additionally lists scopes `user`, `team`, `model`; each entry carries
  `scope`, `ref`, a human `label` (team name / key display name), `inherited_from` (`team_member` for
  template-governed members) and the live counters. Per-user counters created from a template appear
  automatically as members make requests. Logs: `routing_limit_actor_*` (admitted/queued/rejected).
- **Per-instance caveat, now with a sharper edge.** Counters are in memory per process, so with N gateway
  instances a per-person cap is effectively N×. For accounts that is a capacity-planning nuisance; for a
  *person* cap it is a fairness/quota leak. Mitigations: (a) the registry is the single seam — an external
  counter store (Redis/SQLite-backed) could implement the same `Acquire/Release/Record/Assess` surface; (b)
  usage windows self-heal on restart from `usage_records` (shared DB if instances share it) — token/request
  budgets are therefore *approximately* global when the DB is shared and warm start is re-run; concurrency
  is not. Not built; documented as the deliberate boundary, matching the breaker (`tier-routing.md`).

### 10.8 Alternatives considered (scopes)

| Decision | Alternative | Why not |
|---|---|---|
| Generic `limit_policies` table | Columns on `teams`/`api_tokens`; limits on the key itself | Touches auth tables; no home for `model`; three editors. A key-level column is attractive for "one key = one person" but would hard-wire that mapping, which is the thing most likely to change. |
| `team_member` template + `user` override | Only explicit per-user policies | Keys are created with fresh UUIDs, so per-person policies would have to be created after each key — the common case ("every member gets X") would be manual. |
| Actor gate *before* routing | Treat user/team as extra candidate subjects | They don't vary per candidate: they'd be re-evaluated per candidate, mis-attribute a person's cap to "this account is full", and make failover/queue semantics ambiguous. |
| Conjunction (all must pass) | Priority/override chain between scopes | Caps protect different things; a team cap must hold even if a person's cap is generous. Override exists only where one subject key has two definitions (`user` vs `team_member`). |
| Person identity = `user_id` | Key display name / token id | Display names are editable and non-unique; token id is the secret. |
| `model` as candidate subject | Request-model (rule) scope | The rule *is* the request model; an upstream-model cap across rules/accounts is the new information. |
| No `ignore` for actor scopes | Same three policies everywhere | A per-person quota that can be ignored is not a quota. |

### 10.9 Implementation notes (as built)

- **Packages.** `ai.LimitPolicy` (+ `NormalizePolicy`) is the shared type; `db.LimitPolicyStore` (table
  `limit_policies`, in-memory mirror, wired into `StoreManager`) implements `routing.PolicyResolver`;
  `routing.LimitsGuard.AdmitActor` is gate 1, called from `SimpleSelector.SelectService` right after the
  selection context is built and before the pipeline; `Subjects()` adds the `model:` subject to gate 2.
- **Lease slots.** The actor permit lives under lease key `actor`; candidate permits under their service id.
  Failover's `lease.Release(failedServiceID)` therefore never drops it (test `TestFailoverKeepsActorPermit`).
- **Idempotence flag** `CtxKeyLimitActorDone`: set on first admission so a second selection in the same
  request does not double count. `count_tokens` pre-sets it: that helper call is exempt from person/team
  budgets (it still takes an account permit while it runs).
- **Overflow values.** Stored `overflow` ∈ `queue|reject`; anything else (including `ignore`) normalizes to
  `queue`. `queue_timeout_seconds` 0 = default 15 s. With two blocked actor scopes the stricter wins (reject
  beats queue, shorter timeout).
- **Identity refs.** `user` policy ref = sharing key `user_id`; enterprise-context callers use
  `euser:<id>`. `euser:` windows are not warm-started (usage records only carry the key's `user_id`).
  A team's members are derived from `api_tokens` (`ListTokensForTeam`) — no new membership table.
- **Warm start.** At boot every stored policy is warm-started; saving a policy via the API warm-starts it
  too, guarded by `Registry.WindowEmpty` so re-saving never double counts. Per-member templates seed each
  member that has no explicit `user` policy from `usage_records` grouped by `user_id`.
- **API.** `GET /limit-policies`, `PUT|DELETE /limit-policies/{kind}/{ref}`; `GET /limits/status` now also
  returns `user`/`team`/`model` entries (with `label`, `team_id`, `inherited_from`) plus one `user` entry per
  member key of a team with a template and a `team_member:<team>` entry for the template itself.
- **Errors.** 429 `rate_limit_error` with `error.code` = `user_limit_exceeded` | `team_limit_exceeded` |
  `upstream_limit_exceeded`, `Retry-After`.
- **UI.** Team page header "Limits" → one dialog with *Whole team* and *Each member* blocks (saved
  together; an emptied block clears its policy). Team Keys table → per-key "Limits" action (highlighted when
  a personal policy exists) with an inherited-from-template notice and live readout. Provider/service
  editors unchanged. Model limits: API only.

## 11. Not built (explicitly)

Model-limit editor UI (the `model` policy kind works through the API and the status list); scenario-wide caps; per-IP caps for anonymous traffic; weighted fair queuing between
members; external (multi-instance) counter store; grouping several keys under one member (identity is
`user_id`, one per key, until a member entity exists).
