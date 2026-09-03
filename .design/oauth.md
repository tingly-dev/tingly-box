# OAuth

How Tingly-Box signs users into upstream AI providers (Claude Code, Codex, Gemini,
Kimi, …), stores the resulting credentials as providers, keeps them fresh, and
recovers them when they break.

## Concepts

- **Issuer** — the upstream OAuth identity (`claude_code`, `codex`, `openai`,
  `gemini`, `antigravity`, `qwen_code`, `kimi_code`, `github`, `google`, `mock`, …).
  Validated by `ParseProviderType` (`ai/oauth/config.go`).
- **Provider** — a stored credential + endpoint (`ai/provider.go`, `Provider` struct).
  An OAuth provider has `AuthType = oauth` and an `OAuthDetail` (`ai/provider.go`)
  holding the access/refresh tokens, `ExpiresAt`, `Issuer`, a per-credential
  `UserID`, an optional `DeviceID` (Kimi), and an `ExtraFields` bag (e.g. Codex
  `id_token`, ChatGPT `account_id`). Persisted in `ProviderRecord`
  (`internal/data/db/provider_store.go`).
- **A provider's UUID is its identity.** Rules' load-balancing services, smart
  routing, the vision-proxy service, advisor config, and cached
  models all reference a provider **by UUID** (`internal/loadbalance`,
  `internal/typ/type.go`). Keeping the UUID stable across credential changes is what
  keeps a user's configuration intact — this drives the re-auth design below.

## Flow

```
AuthorizeOAuth  ──▶  SessionState (pending)  ──▶  user signs in upstream
  (POST /oauth/authorize)        │                        │
                                 │                        ▼
                                 │            auth-code: OAuthCallback (GET /oauth/callback)
                                 │            device-code: pollForDeviceCodeToken (background)
                                 ▼                        │
                       frontend polls /oauth/status       ▼
                                              createProviderFromToken
                                              ├─ create → AddProvider (new UUID)
                                              └─ re-auth → UpdateProvider (same UUID)
                                                         │
                                              SessionState (success, provider_uuid)
```

- **AuthorizeOAuth** (`internal/server/module/oauth/handler.go`) resolves the issuer,
  optionally auto-detects a proxy from existing providers, mints a `SessionState`
  (`ai/oauth/manager.go`), and returns either an auth URL (authorization-code flow)
  or device-code data (device-code flow). Kimi binds a fresh `X-Msh-Device-Id` to the
  whole flow and the persisted credential.
- **Terminal step** — both the auth-code callback and the device-code poller funnel
  through `createProviderFromToken`, which builds the `OAuthDetail`, picks the
  issuer's API base / style / endpoint mode, and saves the provider. The session is
  then marked `success` with the provider UUID; the frontend polls `/oauth/status`
  to learn the outcome.
- **Routes** (`internal/server/module/oauth/routes.go`): `authorize`, `callback`,
  `token`, `refresh`, `token` (DELETE = revoke), `tokens`, `status`, `cancel`, and
  the `oauth/providers[/:type]` client-config endpoints.

## Token lifecycle

- **Expiry** — `OAuthDetail.IsExpired` (`ai/provider.go`) treats a token as expired
  within a 5-minute buffer; `Provider.IsOAuthExpired` exposes it.
- **Background refresh** — `OAuthRefresher`
  (`internal/server/module/tokenrefresh/refresher.go`) ticks every ~2 min (+≤10%
  jitter) and refreshes any token due within 15 min, using the stored refresh
  token. It gives up on credentials expired more than ~72h ago.
- **Manual refresh** — `POST /oauth/refresh` (`RefreshOAuthToken`) overwrites the
  access/refresh tokens, expiry, and Codex `id_token` **in place** on the provider.
  It only works while the refresh token is still valid.

### Why the buffer must exceed the tick

Nothing on the request path refreshes on demand — the background loop is the only
thing keeping a token alive — so the refresh window is a hard client-visible
contract, not a tuning knob:

```
  interval > buffer  (the old 10 min / 5 min)      buffer > interval  (2 min / 15 min)

  ──tick──────────────tick──────────────tick──     ──t──t──t──t──t──t──t──t──t──t──
        │  buffer │                                     │◄──── buffer ────►│
        └─ due ───┴─ EXPIRED ─┐                         └─ due            expiry
                   ~5-6 min of│401 on every request       ~7 chances to refresh,
                              ▼                            0 min expired
```

With the old numbers a perfectly healthy credential produced a 5–6 minute 401
window **once per token lifetime**, because a token that came due just after a
tick waited a full interval for the next one. The buffer now spans ~7 ticks, so a
transient token-endpoint failure costs a retry instead of a client-visible 401.

### Failure modes the refresh path defends against

| Response | Old behavior | Now |
|---|---|---|
| No `expires_in` | `ExpiresAt` = `0001-01-01T00:00:00Z` → reads as "expired 2000 years ago" → trips the 72h cutoff → **silently never refreshed again** | stored as `""` (no known expiry), matching the create path; already-poisoned rows are re-refreshed instead of skipped (`minPlausibleExpiryYear`) |
| 200 with no `access_token` | blank credential persisted over a working one → guaranteed 401 | refresh rejected, existing credential kept |
| Non-200 | error carried only the body's **length** — undiagnosable | error carries the body (single-line, 512-byte cap), so it reaches the log and the "Token refresh failed" dialog |
| Codex `id_token` | updated by manual refresh only, stale after a background refresh | both paths keep it in lockstep with the access token |

An upstream 401/403 is also a failover trigger (`internal/protocolserver/failover_dispatch.go`):
a broken credential on one service falls over to a sibling and is reported to the
health monitor, instead of surfacing to the client as "Please run /login".

- **Quota window nudge** — `quotawindow.Run`
  (`internal/server/module/quotawindow/quotawindow.go`) sends one tiny model
  request per hour to every enabled OAuth provider (direct SDK call via
  `E2EProber`, any cached model that goes through). Subscription windows only
  start counting on the first real request, so this keeps them moving while idle.

## Re-authentication (in place)

Recovery flow for a credential that has gone **permanently** invalid — refresh token
revoked, account re-consent required — where a refresh can no longer help. It re-runs
the interactive OAuth flow but **overwrites the credential on the existing provider**,
preserving its UUID and every config that references it.

### Why not delete + recreate

Deleting a provider runs `removeProviderServicesFromRules`
(`internal/server/config/provider.go`), which strips every rule service pointing at
that UUID, and recreation mints a fresh, unreferenced UUID. So the old recovery path
silently dismantled the user's routing. Re-auth keeps the UUID, so nothing downstream
moves.

### Mechanism

A target UUID rides the existing OAuth session lifecycle; the terminal step updates
instead of creating.

```
authorize(provider_uuid) ──▶ SessionState.TargetProviderUUID
        │                              │
        ▼                              ▼
  validate up front            createProviderFromToken
  (exists, OAuth, issuer        ├─ empty  → AddProvider (new UUID)
   matches)                     └─ set    → UpdateProvider (same UUID)
```

- **`OAuthAuthorizeRequest.provider_uuid`** (`types.go`) — optional. When set, re-auth.
- **AuthorizeOAuth** validates up front: the target must exist, be an OAuth provider,
  and its issuer must equal the requested provider type (can't re-auth a Claude
  provider with a Codex login). Stored on `SessionState.TargetProviderUUID`
  (`ai/oauth/manager.go`).
- **createProviderFromToken** — with a target set: re-validate the issuer, overwrite
  only the credential surface (`OAuthDetail`, re-enable), preserve UUID / name /
  endpoints / endpoint-mode, honor a proxy chosen during re-auth, then
  `UpdateProvider`. The model list is re-fetched. Returns the same UUID; the session
  completes exactly as the create path. Both flows are covered with no extra wiring
  because they already funnel through this function.

### Front-end

- The OAuth table (`frontend/src/components/OAuthTable.tsx`) exposes a **Reauthorize**
  overflow action, warning-colored when the token is expired. `CredentialPage` wires
  it to open `OAuthDialog` in direct mode for the provider's issuer, passing
  `reauthProviderUuid`. The dialog shows an info banner — *the provider keeps its name
  and UUID, so routing rules and model keys stay intact* — and labels the action
  "Reauthorize".
- **Refresh-failure guidance** — when `POST /oauth/refresh` fails (the refresh token
  is dead), `CredentialPage.handleRefreshToken` opens a "Token refresh failed" prompt
  showing the backend reason and offering **Reauthorize** as the way forward, rather
  than a dead-end toast. `api.oauthRefresh` normalizes non-2xx bodies so the real
  error message survives to the caller.

### Out of scope

- Auto-triggering re-auth from a request-time 401 (future: surface a CTA).
- Changing a provider's issuer/type via re-auth — explicitly rejected.

## Key files

| File | Role |
|---|---|
| `ai/provider.go` | `Provider` / `OAuthDetail`; expiry helpers |
| `ai/oauth/manager.go` | `SessionState` (incl. `TargetProviderUUID`), session storage |
| `ai/oauth/config.go` | `ParseProviderType` (known issuers) |
| `internal/server/module/oauth/handler.go` | authorize, callback, device-code poll, refresh, revoke; `createProviderFromToken` (create + re-auth) |
| `internal/server/module/oauth/types.go` | request/response models; `OAuthAuthorizeRequest.provider_uuid` |
| `internal/server/module/oauth/routes.go` | route registration |
| `internal/server/module/tokenrefresh/refresher.go` | periodic background refresh; expiry/credential guards |
| `internal/server/config/provider.go` | `AddProvider` / `UpdateProvider` / `DeleteProvider` + rule cleanup |
| `frontend/src/components/OAuthDialog.tsx` | provider picker + direct/re-auth mode (`reauthProviderUuid`) |
| `frontend/src/components/OAuthTable.tsx` | provider list, expiry display, Refresh / Reauthorize actions |
| `frontend/src/pages/CredentialPage.tsx` | wiring: reauthorize, refresh-failure prompt |
| `frontend/src/services/api.ts` | `oauthAuthorize` (`provider_uuid`), `oauthRefresh` (error normalization) |
