# Provider rewards (reset cards, daily check-in)

Providers hand out rewards that exist only in their own UI: GLM Coding Plan
reset cards, WorkBuddy's daily check-in credits, Codex banked resets. They are
valuable and easy to miss (some expire). Tingly-Box already holds these
accounts' credentials, so it can show them in the quota panel and, where it is
safe, act on them. Proposal: TEP-2240.

This file records the part that exists. WorkBuddy check-in and Codex use are
described under "Not done".

## Status of the protocol: community-derived, unverified here

None of these endpoints is documented by its vendor. The GLM contract below
comes from community clients (the console's own requests, cross-checked in
sub2api and `glm-auto-reset-card`). Tingly-Box has **not** been run against a
real account. So:

- every path, header spelling and host is in one place (`zai_glm_reset.go`);
- the card list is read tolerantly (several spellings of id / type / expiry /
  status; timestamps as unix s, unix ms, RFC 3339 or the console's
  `2006-01-02 15:04:05` in UTC+8; the array directly or under `list` /
  `records` / `rows` / `items`);
- an unrecognised list shape is an **error**, never "no cards" — showing
  nothing for a card the user holds is the failure this exists to prevent.

When a real capture disagrees, fix the reader and record it here.

## GLM reset cards

`GLMFetcher` (host `open.bigmodel.cn`, personal plans; the ZCode CN provider is
this fetcher) additionally calls

```
GET  /api/biz/customer-package-reset/list?targetType=PERSONAL
POST /api/biz/customer-package-reset/use
     {targetType:"PERSONAL", resetType, recordId, grantType:"DIRECT", requestId:<fresh uuid>}
```

with the plan key in `authorization` **without `Bearer`** (the monitor endpoint
the quota fetch uses is sent with it). `api.z.ai` and team plans are
unverified and not covered.

### Reading

Each available card becomes a `resource` breakdown (`Unit credits`, `Limit 1`,
like Codex's reset credits, so the cell shows "N/M" and the hover lists them)
with `UsageBreakdown.Redeem{ID, Scope}`. A card is spent, not
refilled: it carries no `ResetsAt` (its expiry is when it is *lost*, and is in
the description only). Spent and expired cards are not offered. A failed card
read is logged and **never drops the quota windows**.

### Spending

Irreversible, so the rules are fixed, not options:

1. **Only a listed, available card.** `Redeem` re-reads the list and refuses an
   id that is not in it (`ErrRewardNotFound`); the client's id is never trusted.
2. **Sent once, never retried.** A transport error, a 5xx or an unreadable
   answer is `ErrRedeemUnconfirmed` — the card may or may not be gone. The
   server still re-reads quota and returns it, so the caller sees the truth.
3. **HTTP 200 with `success:false` is a refusal** (`ErrRedeemRejected`, the
   upstream message is kept), not a transport success.
4. **One spend per provider at a time** (`Manager.redeeming`): a double click
   must not become two cards (`ErrRedeemBusy`).
5. **Confirm first.** The UI never sends on the first click; the dialog says
   what the card refills (a weekly card also refills the 5-hour window, without
   spending a 5-hour card, and the weekly cycle restarts from now).

No automatic spending: a card is worth most when the user picks the moment.
Automatic use needs its own opt-in design.

### API

`POST /api/v1/provider-quota/:uuid/rewards/:id/use` answers with a typed body,
because the client has to tell "not spent" from "maybe spent" from "spent":

| `status` | HTTP | meaning |
|---|---|---|
| `spent` | 200 | provider accepted; `usage` is the quota read back |
| `rejected` | 422 | provider said no; nothing spent; `message` is its own |
| `unconfirmed` | 202 | sent, outcome unknown; `usage` shows whether the card is gone |
| `not_found` | 404 | not in the list / no longer available |
| `busy` | 409 | another spend for this provider is running |
| `unsupported` | 404 | the provider has no spendable rewards |

`quota.Redeemer` is the optional interface a fetcher implements to take part;
`Manager.Redeem` finds the provider's fetcher and does the guarding.

## WorkBuddy daily check-in

`fetcher/workbuddy_checkin.go`. Paths and the account-region origin
(`ai.WorkBuddySiteFor`) follow the 薅点 desktop app, which is the reference for
this protocol: `POST /v2/billing/meter/checkin-activity-status` and
`POST /v2/billing/meter/daily-checkin` (note the `/v2`, the credit-meter paths
have none). Auth and client headers are the ones the credit meter already sends.
Request and response bodies are undocumented and **unverified against a real
account**, so the status parser is tolerant and anything it cannot read
clearly counts as "not claimable".

Rules, taken from how 薅点 stays safe:

1. **Only offered when claimable.** A `UsageBreakdown` with
   `Redeem{ID: "daily-checkin", Scope: "daily", Auto: true}` exists only when
   the status says today is open. Already claimed, unreadable or failed status
   shows nothing, and never drops the credit windows.
2. **Enterprise accounts are never touched** (no status call, no claim).
3. **Claim once, then confirm independently.** After the POST the status is read
   again; only a status that says "claimed" is `spent`. A repeat answer (HTTP
   400 / code 10001) is "already claimed", not an error.
4. **Uncertain is never retried** (`ErrRedeemUnconfirmed`).

`Auto` marks a reward that is safe to take without asking (idempotent,
harmless). The UI then shows "Check in" and skips the confirm dialog.

### Auto-claim

After each scheduled refresh, `Manager.AutoRedeem` takes every `Auto` reward
once per provider per local day (`autoClaimLog`, in memory, so a restart may try
once more, which the idempotent endpoint tolerates). A failed attempt is logged
and not retried until the next day's first refresh. `Config.AutoClaimRewards`
defaults to true; there is no UI toggle yet.

## Not done

- **Codex banked reset use.** Reading exists (`codex.go` lists
  `/wham/rate-limit-reset-credits`); `…/consume` is not implemented.
- A settings toggle for `AutoClaimRewards`.
- Doubao Work / Qoder CN daily rewards need the official client's session and
  do not fit the OAuth model.
