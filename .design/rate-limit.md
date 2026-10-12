# Per-Sharing-Key Rate Limiting (TEP-1851)

Status: implemented for requests-per-minute (RPM). Tracks
https://github.com/tingly-dev/tingly-box/issues/1851.

Related: `.design/team.md` (what a Sharing Key is and where it may call),
`.design/request-failover.pencil.md` (upstream 429 handling, which this is not).

## 1. Problem

A Team's Sharing Keys (`tb-share-…`) all draw on the same upstream quota,
which for subscription providers (Codex, Kimi, GLM coding plans) is scarce and
resets on fixed windows. Before this change the only per-key controls were
enable/disable and delete: one key could consume the Team's whole window and
every other member saw upstream 429s with no indication of who caused them.

## 2. Decision summary

- Limit is **per Sharing Key**, not per Team, model or provider. The problem
  is fairness between members of one Team; the key is the member's identity.
  Per-provider limits already exist implicitly (the upstream quota) and a
  per-model limit cannot say who consumed it. (Maintainer question on the
  TEP answered 2026-10-01.)
- MVP metric is **requests per minute**. The `Limiter` interface takes an
  opaque key and a limit so other metrics (tokens/day from `usage_daily`,
  max concurrency) can be added without changing the middleware.
- Budget lives on the key record: `api_tokens.rpm_limit` (integer,
  `0` = unlimited, at most `db.MaxSharingKeyRPMLimit` = 1,000,000 — larger
  values are indistinguishable from unlimited and would not round-trip
  through the UI's JavaScript number). **The default is `0` for new and
  existing keys alike**:
  pre-existing rows get the SQL default, new keys get
  `db.DefaultSharingKeyRPMLimit = 0`. Limiting is opt-in per key — shipping
  the feature changes nothing for any key until an operator chooses a number.
  (An earlier draft defaulted new keys to 60; dropped so that create-key has
  no hidden behaviour the admin did not type in.)
- Enforcement is **in-memory, per process**, token bucket per key: capacity =
  limit, refill = limit/60 per second. Counters reset on restart; for a
  one-minute window that is acceptable and avoids a storage round-trip on the
  hot path. The number is therefore a **sustained rate with burst allowance**:
  a key idle for a minute may make `limit` requests at once and then
  `limit` more spread over the following minute (up to ~2× the number inside
  one wall-clock minute). The UI helper text says so, so the admin sets the
  number knowing what it means.
- Exceed → `429` with `Retry-After` (whole seconds, rounded up) and the
  standard error envelope (`type: rate_limit_error`), so SDK clients back
  off the same way they do for upstream limits.

## 3. What is counted

| Request | Counted |
|---|---|
| `POST` model endpoints on `/tingly/team[/v1]` with a Sharing Key (`/chat/completions`, `/responses`, `/messages`, `/embeddings`, `/decisions`, `/images/*`) | yes |
| `POST /messages/count_tokens` (tokenizes the request without running the model; Claude Code calls it to size its context) | no |
| `GET /models`, `GET /quota`, `GET /responses/:id` (discovery; clients poll them) | no |
| Any request with the global model token or an enterprise identity | no |
| Sharing Key with `rpm_limit = 0` | no (never reaches the limiter) |

A denied request is not a model invocation: it is not recorded as usage, not
reported to the health monitor, and does not count against the bucket. The
converse does not hold: an **admitted** request consumes its token even if the
handler later rejects it (400 for an unknown model, upstream failure, …) —
the budget is on requests the key sends, which also keeps a misconfigured or
probing client from spending the Team's upstream quota for free.

## 4. Request flow

```
ModelAuthMiddleware            resolves tb-share-… → APITokenRecord
  c.Set(CtxKeyAuthKind, sharing_key)
  c.Set(CtxKeySharingKeyID, record.TokenID)
  c.Set(CtxKeySharingKeyRPMLimit, record.RPMLimit)
        │
debugHeaderGuard
        │
SharingKeyRateLimitMiddleware  POST + sharing key + limit>0 ?
  limiter.Allow(keyID, limit) → allowed: c.Next()
                              → denied : 429 + Retry-After, abort
        │
teamScopeMiddleware → handler
```

The middleware sits in `modelAuthChain`, so it covers every model route the
auth middleware covers; it decides from context only and never touches the
token store. The limit is passed on every call, so an edit in the UI applies
on the next request — the limiter keeps no copy of the budget to invalidate.
When a key's limit changes, its bucket is rebuilt full under the new limit
(a key must not keep a burst allowance from a larger old limit). The
operational corollary: **editing the number also resets that key's budget**,
including toggling between two values — an admin can use it deliberately to
unblock a key, and should not be surprised that a key at its cap gets a full
minute of burst right after an edit.

Two details that matter for correctness:

- The limiter reads its clock **under the bucket lock** and only ever moves
  `lastSeen` forward. Reading it before contending for the lock let two
  concurrent callers apply timestamps out of order and credit a refill
  twice (found in review).
- The context value for the budget is read with an explicit `int` type
  assertion, not gin's `GetInt`, which would silently yield 0 (= unlimited)
  for any other dynamic type. A mismatch is logged once and treated as
  unlimited (fail-open: the failure mode is "less limiting", never "whole
  team blocked"). `internal/protocolserver/sharing_key_ratelimit_chain_test.go`
  drives the real auth → guard → limiter chain so the contract between
  `auth.go` and the middleware is tested rather than restated.

## 5. API

| Endpoint | Change |
|---|---|
| `POST /api/v1/tokens` | optional `rpm_limit` (omit or `0` → unlimited; `0..1000000`) |
| `PUT /api/v1/tokens/{token_id}/rate-limit` | `{ "rpm_limit": n }` (`0..1000000`), returns the key info |
| `GET /api/v1/tokens*`, create/regenerate responses | include `rpm_limit` |

## 6. UX contract

Follows `.design/ux-principles.md`:

- One number, not a toggle plus a number (§6 smart defaults): the field is
  `rpm_limit` with `0` meaning unlimited; the create dialog prefills `0`, so
  a key is limited only when the admin types a number. An emptied field also
  reads as `0`, so clearing the prefilled value to type is not an error.
- The table column is headed "Requests/min" and shows the concrete value
  (`60 /min` or `Unlimited`), not a label such as "limited" (§5); header,
  dialog title and field label all say requests per minute (§3 one term).
- Editable in place from the row after creation (§10 done ≠ locked): the
  value is a real button (focusable, Enter/Space, tooltip on focus) with a
  dotted underline marking it as the editable caption in the row; Enter in
  the edit dialog saves.
- The helper text tells the admin what the key holder will see (HTTP 429 with
  a Retry-After wait), and the 429 body states the limit and the wait, so the
  key holder learns the budget from the error instead of asking the admin
  (§8).

## 7. Non-goals (this iteration)

- Per-Team aggregate caps.
- Token- or cost-based budgets.
- Persisting bucket state across restarts or sharing it between instances.
- A non-zero default or a team-level default budget inherited at key creation.

## 8. Key implementation files

| Layer | File |
|---|---|
| Limiter | `internal/ratelimit/limiter.go` (`Limiter`, `RPMLimiter`) |
| Middleware | `internal/middleware/sharing_key_ratelimit.go` |
| Auth → context | `internal/middleware/auth.go` (`CtxKeySharingKeyID`, `CtxKeySharingKeyRPMLimit`) |
| Chain | `internal/protocolserver/routes.go` (`modelAuthChain`) |
| Wiring | `internal/server/server.go` (`sharingKeyLimiter`, sweeper on `server.ctx`) |
| Persistence | `internal/db/api_token_store.go` (`RPMLimit`, `SetTokenRPMLimit`) |
| Control API | `internal/server/module/sharing/` |
| Frontend | `components/SharingKeysTable.tsx`, `hooks/useSharingKeyActions.tsx`, `services/tokenApi.ts` |
